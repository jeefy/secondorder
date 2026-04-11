# SO-80 Audit: Run-time Execution Metadata Persistence

Date: 2026-04-11
Auditor: Auditor
Scope: Validate whether persisted run-time execution metadata is sufficient and trustworthy for investigations, QA reviews, and retry analysis.

---

## Verdict

Current run-time execution metadata persistence is **not yet trustworthy enough** for audit and investigation use.

The highest-risk problem is a schema/code mismatch: the application code attempts to read and write execution snapshot fields on `runs`, but the migrated SQLite schema in this branch does not contain those columns. Focused test runs fail on that mismatch, which means the system cannot reliably persist the execution context it appears to promise.

---

## What Is Persisted Today

Evidence from `internal/db/migrations/001_init.sql`, `internal/db/queries.go`, and `internal/models/models.go` shows that the intended run record includes:

- run identity: `id`, `agent_id`, `issue_key`, `mode`, `status`
- run output artifacts: `stdout`, `diff`
- token and cost totals: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_create_tokens`, `total_cost_usd`
- timing: `started_at`, `completed_at`, `created_at`

The `models.Run` struct and DB query layer also expect snapshot fields:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`

Related supporting metadata is also persisted elsewhere:

- `cost_events` rows linked by `run_id`
- session-scoped API keys linked by `run_id`
- `audit_runs.run_id` linkage for audit mode
- issue-level retry count inferred from count of `runs` per issue

---

## Findings

### 1. Critical: code expects snapshot columns that do not exist in the migrated schema

Code path:

- `internal/db/queries.go:558-566` inserts snapshot columns into `runs`
- `internal/db/queries.go:575-585` reads the same columns back out
- `internal/models/models.go:217-239` exposes those fields on `models.Run`

Schema reality:

- `internal/db/migrations/001_init.sql:41-58` creates `runs` without any snapshot columns
- `sqlite3 so.db ".schema runs"` confirms the live table also lacks those columns
- there is no migration in `internal/db/migrations/` adding those fields

Verification:

- `go test ./internal/db ./internal/scheduler ./internal/handlers` fails with `table runs has no column named runner_snapshot`

Auditability impact:

- persisted execution context is incomplete and unreliable
- run creation can fail entirely in environments using the current schema
- QA and investigators cannot trust that the DB reflects the runner/model/git state used for a run

Owner:

- Founding Engineer to add the missing migration and restore schema/code alignment

### 2. High: scheduler does not populate the snapshot fields before `CreateRun`

Observed behavior:

- `internal/scheduler/scheduler.go:178-190` creates the `models.Run`
- the run is inserted immediately via `s.db.CreateRun(run)`
- no assignment to `RunnerSnapshot`, `ModelSnapshot`, `GitWorktree`, `GitBranch`, `GitCommitSHA`, or `GateTarget` was found before insert

Auditability impact:

- even after schema repair, the intended execution context would still persist as null unless the scheduler populates it
- retry analysis cannot answer basic questions like “which runner/model/commit/worktree produced this failure?”

Owner:

- Founding Engineer to populate immutable run-start snapshots before persisting the run

### 3. High: architecture docs overstate the current implementation

`docs/architecture.md:166` states that agents can self-report usage via `POST /api/v1/runs/{id}/tokens`.

Search across the codebase found no implementation of that route; the only match is the documentation line itself.

Auditability impact:

- operators and QA may rely on a recovery path for token accounting that does not actually exist
- documentation currently overstates the trustworthiness and redundancy of cost metadata capture

Owner:

- Founding Engineer to either implement the endpoint or remove the claim from docs
- QA to verify the selected behavior in a clean environment once fixed

### 4. Medium: retry analysis is only count-based, not causally reconstructable

Current retry analysis support is limited to counting `runs` per issue:

- `internal/db/queries.go:1331-1335` counts runs for an issue
- `internal/handlers/api.go:330-345` enforces the retry limit from that count

What is missing for investigation-quality retry analysis:

- explicit retry attempt number persisted on the run
- retry parent/previous run linkage
- reason for retry or requeue
- actor that triggered the retry path

Auditability impact:

- investigators can detect that retries happened, but not reliably reconstruct why a later run existed or which prior run it superseded
- QA reviews must infer retry context indirectly from issue comments and timestamps

Owner:

- Product Lead to decide whether richer retry lineage is required
- Founding Engineer to implement if approved

### 5. Medium: run event history is declared in architecture but not used for execution tracing

Evidence:

- `internal/db/migrations/001_init.sql:169-177` creates `run_events`
- `docs/architecture.md:268` describes `run_events` as a detailed run event log
- no `CreateRunEvent`-style usage was found for scheduler lifecycle milestones

Auditability impact:

- there is no append-only lifecycle trail for run spawn, cancellation, timeout, stdout flush milestones, token ingestion, or completion transitions
- investigators must rely on mutable `runs.stdout` and final status instead of immutable event history

Owner:

- Founding Engineer to decide whether `run_events` should become the authoritative execution timeline

---

## Sufficiency Assessment

### Investigations

Not sufficient yet.

What works:

- final status, timestamps, stdout, diff, and cost totals are conceptually the right foundation
- `run_id` links related records such as cost events and audit runs

What blocks trust:

- schema/code drift means intended metadata is not reliably persisted
- no immutable event timeline exists for lifecycle reconstruction
- no durable capture of runner/model/git execution context at run start

### QA reviews

Partially sufficient.

What QA can use today:

- per-run stdout and diff
- started/completed timestamps
- issue-level run history count

What QA still lacks:

- authoritative runner/model snapshot for reproducibility
- reliable retry lineage
- documented and implemented token self-report fallback

### Retry analysis

Only minimally sufficient.

The system can answer “how many runs happened for this issue?” but not “which run was a retry of which prior run, under what changed conditions, and why?”.

---

## Trustworthiness Assessment

Recorded fields are **not fully trustworthy** today because:

1. the code and database schema disagree on what is persisted
2. the most important context fields are not being populated by the scheduler before insert
3. architecture documentation claims at least one API capability that is not implemented

This is a correctness problem, not just a documentation polish issue.

---

## Recommended Updates

### Founding Engineer

- Add a migration for all run snapshot columns already referenced by code
- Populate run snapshot fields before calling `CreateRun`
- Decide and implement whether `run_events` should record spawn, timeout, cancel, completion, and token-ingest milestones
- Either implement `POST /api/v1/runs/{id}/tokens` or remove the architecture claim
- Add regression tests that cover run creation on a migrated DB and verify snapshot persistence

### QA Engineer

- After the fix lands, verify on a clean database that a new run persists snapshot columns and remains viewable in run detail/history pages
- Validate that retry-limit behavior still works and that run history remains queryable after migration
- Verify whichever token reporting behavior is declared in docs is actually present

### Product Lead

- Decide whether retry lineage needs first-class persistence beyond simple run counting
- If yes, define the minimum fields required for investigations: attempt number, previous run id, retry trigger, and retry reason

---

## Cross-reference Check

### Matches codebase state

- `runs` stores stdout, diff, timing, status, and token/cost totals
- issue retry limit is based on counting historical runs
- audit runs link back to `runs` via `run_id`

### Does not match codebase state

- `models.Run` and DB query code expect run snapshot columns that are absent from the actual schema in this branch
- `docs/architecture.md` documents a token self-report endpoint that is not implemented
- `docs/architecture.md` describes `run_events` as a detailed run log, but execution tracing is not using that table

---

## Evidence

- `internal/models/models.go:217-239`
- `internal/db/queries.go:551-645`
- `internal/db/migrations/001_init.sql:41-58`
- `internal/db/migrations/001_init.sql:169-177`
- `internal/scheduler/scheduler.go:178-190`
- `internal/scheduler/scheduler.go:219-330`
- `internal/handlers/api.go:330-345`
- `docs/architecture.md:166`
- `docs/architecture.md:257-268`
- `sqlite3 so.db ".schema runs"`
- `go test ./internal/db ./internal/scheduler ./internal/handlers`

---

## Final Assessment

SO-80 should be considered an **audit failure with actionable remediation**, not a pass.

The design intent is close to what investigations and QA need, but the current implementation is not yet dependable because execution metadata persistence is internally inconsistent. Until schema, scheduler population, tests, and docs are aligned, recorded run metadata should be treated as incomplete evidence.
