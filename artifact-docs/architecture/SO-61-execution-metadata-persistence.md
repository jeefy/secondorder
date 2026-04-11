# SO-61: Execution metadata persistence design for issue runs

## Summary

This design adds immutable execution provenance to each `runs` record so audits, QA reviews, and incident analysis can answer: **what agent configuration, code snapshot, and gate target produced this run?**

The proposal extends the run model with a dedicated execution metadata payload captured at run start and preserved unchanged for the lifetime of the run. It records:

- runner
- model
- working directory reference
- git branch and/or worktree path
- commit SHA
- gate target
- selected provenance and capture status fields

The design is backward-compatible for existing runs, preserves auditability by treating captured metadata as append-only snapshot data, and avoids overloading mutable agent config as historical truth.

---

## Goals

- Persist the exact execution context for every run tied to an issue.
- Make run provenance available in DB, REST responses, and UI serialization.
- Distinguish immutable run snapshot data from mutable current agent configuration.
- Support existing runs without metadata.
- Preserve audit usefulness even when the working tree is dirty, detached, or not a git repo.

## Non-goals

- Reconstructing a full software bill of materials.
- Cryptographic attestation/signing in this phase.
- Blocking execution on provenance capture failures.
- Capturing full environment variable state.

---

## Why snapshot on `runs` instead of deriving later

Current run records store execution output (`stdout`, `diff`, tokens, cost, status), but provenance fields like agent runner/model and git context are only available indirectly from mutable agent config and the live repo state.

That is insufficient for auditability because:

- agents can change runner/model after the run
- branches can move
- worktree paths can be reused
- `HEAD` changes immediately after the run
- non-git or detached-head contexts become impossible to interpret later

Therefore execution metadata must be **captured at dispatch time and stored on the run itself**.

---

## Proposed data model

## Option chosen: dedicated nullable columns on `runs`

Add the following columns to `runs`:

| Column | Type | Null | Meaning |
|---|---|---:|---|
| `runner_snapshot` | TEXT | yes | Runner used for this run at dispatch time |
| `model_snapshot` | TEXT | yes | Model used for this run at dispatch time |
| `working_dir_snapshot` | TEXT | yes | Agent working dir used to launch the run |
| `git_branch_snapshot` | TEXT | yes | Resolved branch name when available |
| `git_worktree_snapshot` | TEXT | yes | Absolute or normalized worktree path actually executed |
| `git_commit_sha_snapshot` | TEXT | yes | Full 40-char commit SHA at start |
| `git_tree_dirty_snapshot` | INTEGER | yes | 1 if uncommitted changes were present before execution, else 0 |
| `gate_target_snapshot` | TEXT | yes | Gate target requested for this run |
| `provenance_capture_status` | TEXT | yes | `captured`, `partial`, `unavailable` |
| `provenance_capture_error` | TEXT | yes | Short diagnostic when capture is partial/unavailable |

### Rationale

Dedicated columns are preferable to a JSON blob for this phase because they:

- match the current repo style of explicit columns and typed structs
- simplify SQLite queries, filtering, and future dashboards
- keep API serialization obvious
- avoid per-field JSON parsing in Go templates and handlers

A JSON extension column could be added later if new provenance fields become highly variable.

---

## Run model changes

Extend `internal/models.Run` with nullable snapshot fields:

```go
RunnerSnapshot            *string `json:"runner_snapshot,omitempty"`
ModelSnapshot             *string `json:"model_snapshot,omitempty"`
WorkingDirSnapshot        *string `json:"working_dir_snapshot,omitempty"`
GitBranchSnapshot         *string `json:"git_branch_snapshot,omitempty"`
GitWorktreeSnapshot       *string `json:"git_worktree_snapshot,omitempty"`
GitCommitSHASnapshot      *string `json:"git_commit_sha_snapshot,omitempty"`
GitTreeDirtySnapshot      *bool   `json:"git_tree_dirty_snapshot,omitempty"`
GateTargetSnapshot        *string `json:"gate_target_snapshot,omitempty"`
ProvenanceCaptureStatus   *string `json:"provenance_capture_status,omitempty"`
ProvenanceCaptureError    *string `json:"provenance_capture_error,omitempty"`
```

Pointers are preferred so existing runs serialize cleanly as omitted/null and backward compatibility is explicit.

---

## Capture semantics

## Capture time

Capture execution metadata **once, immediately before subprocess launch and after final execution location is resolved**.

That ensures the snapshot reflects the exact code context the runner receives.

## Sources of truth

| Field | Source |
|---|---|
| `runner_snapshot` | agent config at dispatch |
| `model_snapshot` | agent config at dispatch |
| `working_dir_snapshot` | configured `agent.WorkingDir` |
| `git_worktree_snapshot` | actual launch directory after worktree/branch resolution |
| `git_branch_snapshot` | `git branch --show-current` or equivalent |
| `git_commit_sha_snapshot` | `git rev-parse HEAD` |
| `git_tree_dirty_snapshot` | `git status --porcelain` non-empty |
| `gate_target_snapshot` | scheduler-dispatch parameter or configured gate request for the run |

## Provenance states

### `captured`
All required fields were captured successfully, or non-git fields were captured and git fields are intentionally empty because the run is not repo-backed.

### `partial`
Run started, but one or more expected fields could not be captured.
Examples:
- `git rev-parse HEAD` failed in a directory expected to be a repo
- branch unavailable because of detached HEAD
- gate target resolution failed while runner/model were captured

### `unavailable`
No meaningful provenance could be captured beyond base run creation. This should be rare and accompanied by `provenance_capture_error`.

---

## Required vs optional fields

## Required for all runs

These should always be populated when dispatch begins:

- `runner_snapshot`
- `model_snapshot`
- `working_dir_snapshot`
- `provenance_capture_status`

## Required when run executes inside a git repo

- `git_worktree_snapshot`
- `git_commit_sha_snapshot`

## Optional depending on repo state

- `git_branch_snapshot` — may be empty in detached HEAD
- `git_tree_dirty_snapshot` — may be unavailable if git status fails
- `gate_target_snapshot` — optional until gate targeting is consistently modeled upstream

This split avoids treating detached-head builds as failures while still insisting on commit SHA when a git repo exists.

---

## Gate target modeling

`gate_target_snapshot` should record the execution target that QA/audit would care about, not the entire gate result.

Examples:

- `pr`
- `branch:main`
- `worktree:so-61`
- `issue:SO-61`
- `review`
- `ship-readiness`

If parent issue SO-59 introduces a canonical gate target enum elsewhere, this run field should store the canonical serialized form rather than inventing a separate taxonomy.

### Recommendation

Treat this field as a **string snapshot of an already-resolved target**, not a foreign key. Gate configurations evolve; runs need immutable historical values.

---

## DB migration

Create a new migration, e.g.:

`025_run_execution_metadata.sql`

```sql
ALTER TABLE runs ADD COLUMN runner_snapshot TEXT;
ALTER TABLE runs ADD COLUMN model_snapshot TEXT;
ALTER TABLE runs ADD COLUMN working_dir_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_branch_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_worktree_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_commit_sha_snapshot TEXT;
ALTER TABLE runs ADD COLUMN git_tree_dirty_snapshot INTEGER;
ALTER TABLE runs ADD COLUMN gate_target_snapshot TEXT;
ALTER TABLE runs ADD COLUMN provenance_capture_status TEXT;
ALTER TABLE runs ADD COLUMN provenance_capture_error TEXT;
```

### Migration notes

- SQLite `ALTER TABLE ... ADD COLUMN` is safe for this additive change.
- No backfill is required for correctness.
- Existing rows remain null and must be handled at read/render time.

Optional index if filtering by commit/branch becomes common later:

```sql
CREATE INDEX IF NOT EXISTS idx_runs_commit_sha_snapshot ON runs(git_commit_sha_snapshot);
CREATE INDEX IF NOT EXISTS idx_runs_issue_created_at ON runs(issue_key, created_at DESC);
```

Indexing branch/commit is not required initially.

---

## Query and persistence changes

Update all run-related queries in `internal/db/queries.go`:

- `CreateRun`
- `GetRun`
- `ListRunsForAgent`
- `ListRunsForIssue`
- `scanRuns`

### Write path

`CreateRun` should insert snapshot values at run creation.

### Completion path

`CompleteRun` must **not overwrite snapshot metadata**. Provenance is a start-of-run snapshot, not an end-of-run recalculation.

This immutability is important for audits.

---

## Scheduler / runner capture flow

Add a provenance capture step in the scheduler before subprocess execution:

```text
resolve execution directory
  -> capture base agent snapshot (runner/model/working_dir)
  -> inspect git context in actual launch dir
  -> resolve gate target for this run
  -> create run record with snapshot fields
  -> launch subprocess
```

## Suggested helper shape

Introduce an internal struct, e.g.:

```go
type RunProvenance struct {
    Runner string
    Model string
    WorkingDir string
    GitBranch *string
    GitWorktree *string
    GitCommitSHA *string
    GitTreeDirty *bool
    GateTarget *string
    CaptureStatus string
    CaptureError *string
}
```

And a helper like:

```go
CaptureRunProvenance(agent *models.Agent, execDir string, gateTarget *string) RunProvenance
```

This keeps provenance logic centralized and testable.

---

## Git capture rules

### Determine repo presence

Check whether `execDir` is inside a git work tree:

```bash
git rev-parse --is-inside-work-tree
```

If false:
- persist non-git fields
- set git fields null
- `provenance_capture_status = captured` if non-git execution is valid in product semantics, otherwise `partial`

### Commit SHA

Use:

```bash
git rev-parse HEAD
```

Store full SHA, not abbreviated SHA.

### Branch

Use:

```bash
git branch --show-current
```

If empty, store null. Do not invent pseudo-values like `detached` unless the UI wants to derive them for display.

### Worktree path

Prefer the actual execution directory path. That is what matters operationally, whether it came from a branch checkout or a git worktree.

### Dirty tree

Use:

```bash
git status --porcelain
```

If output is non-empty, store `true`.

### Provenance caveat

The existing run `diff` captures post-run changes. `git_tree_dirty_snapshot=true` clarifies whether the repo was already dirty before the agent started, which matters for audit integrity.

---

## API and serialization implications

## Existing endpoints affected

Any endpoint returning a `Run` should include the new fields automatically once the struct is extended.

This likely includes:

- run detail UI backing handlers
- issue detail payloads that embed runs
- any future REST run endpoints

## JSON behavior

Recommended behavior:

- new fields appear as `null`/omitted for legacy rows
- new runs populate fields when available
- no breaking change to existing consumers because fields are additive

### Example serialized run

```json
{
  "id": "run_123",
  "agent_id": "agent_456",
  "issue_key": "SO-61",
  "mode": "issue",
  "status": "completed",
  "runner_snapshot": "codex",
  "model_snapshot": "gpt-5.4-pro",
  "working_dir_snapshot": "/workspace/msoedov/secondorder",
  "git_branch_snapshot": "so-61-exec-metadata",
  "git_worktree_snapshot": "/workspace/msoedov/secondorder",
  "git_commit_sha_snapshot": "0123456789abcdef0123456789abcdef01234567",
  "git_tree_dirty_snapshot": false,
  "gate_target_snapshot": "issue:SO-61",
  "provenance_capture_status": "captured",
  "provenance_capture_error": null,
  "started_at": "2026-04-11T13:00:00Z",
  "completed_at": "2026-04-11T13:03:00Z"
}
```

---

## UI implications

The run detail page should display an **Execution Metadata** section near status/tokens.

Recommended fields to render:

- runner
- model
- gate target
- working dir
- branch
- commit SHA
- worktree path
- dirty tree flag
- provenance capture status
- capture error, if any

### Display rules

- Missing values render as `—`
- Dirty tree should be visually emphasized when true
- Commit SHA should be monospace and copyable
- Branch and worktree should not be conflated

This is particularly useful on issue detail pages where multiple runs need comparison.

---

## Backward compatibility

## Existing runs

Existing runs will have null metadata fields.

Handling guidance:

- do not backfill from current agent config or current git state
- show `metadata unavailable (legacy run)` in UI when all snapshot fields are null
- preserve current API behavior with additive fields only

### Why no backfill

Backfilling from present-day state would create false provenance and damage trust in audit data.

---

## Auditability and provenance constraints

## Immutability

Snapshot fields must be write-once from normal application flows.

Practical rule:
- set on `CreateRun`
- never mutate on `CompleteRun`
- only allow admin/manual DB edits outside normal runtime

## Truthfulness over completeness

If provenance is partial, store partial truth with a capture error rather than fabricated values.

## Full SHA only

Use full 40-character SHA to avoid ambiguity across repos and long-lived history.

## Path sensitivity

Absolute paths may expose infrastructure layout. For local self-hosted Secondorder this is generally acceptable, but if path disclosure is a concern later, introduce a display-safe path policy while keeping raw paths in DB for audit.

## Mutable git refs

Branch names are informational only. Commit SHA is the authoritative code identity.

## Dirty workspace semantics

A run in a dirty tree is still auditable, but should be considered lower-confidence provenance than a clean tree at a known commit.

---

## Failure handling

Provenance capture should be **best effort, non-fatal**.

If metadata capture fails:
- still create and run the job
- mark `provenance_capture_status = partial` or `unavailable`
- persist concise `provenance_capture_error`
- log structured warning server-side

This avoids taking down execution because `git` is unavailable while still exposing reduced audit confidence.

---

## Testing strategy

Add tests in three layers.

## DB tests

- migration applies cleanly
- `CreateRun` persists snapshot fields
- `GetRun`/`ListRunsForIssue` scan nullable fields correctly
- legacy rows with nulls still deserialize safely

## Scheduler tests

- git repo, clean branch
- detached HEAD
- dirty tree
- non-git working dir
- git command failure returns partial/unavailable status

## Handler/UI tests

- run JSON includes metadata fields
- run detail page renders values
- legacy run renders fallback text without panic

---

## Implementation plan

1. Add migration `025_run_execution_metadata.sql`.
2. Extend `models.Run` with nullable snapshot fields.
3. Update run queries/scanners in `internal/db/queries.go`.
4. Add scheduler provenance capture helper.
5. Populate snapshot fields during run creation only.
6. Render execution metadata in run detail and issue detail views.
7. Add tests for DB, scheduler, and handler rendering.
8. Update architecture documentation/wiki.

---

## Open alignment note with SO-59

This design assumes `gate target` becomes available to the scheduler as a resolved string value. If SO-59 defines a more structured gate object, the run record should still store its immutable serialized target string plus, optionally later, a foreign key to a gate execution entity.

The run snapshot should remain independent of mutable gate definitions.

---

## Recommendation

Adopt **explicit run snapshot columns** and capture provenance at dispatch time. This is the lowest-risk, most queryable, and most audit-friendly design for the current Go + SQLite architecture.

The critical design principles are:

- snapshot on run creation
- keep snapshots immutable
- prefer null/partial truth over inferred backfill
- treat commit SHA as the primary code identity
- preserve both branch and actual worktree path when available
