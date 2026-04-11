# SO-80 Audit: Run-time Execution Metadata Persistence

Date: 2026-04-11
Auditor: Auditor
Scope: Validate whether persisted run-time execution metadata is sufficient and trustworthy for investigations, QA reviews, and retry analysis.

---

## Verdict

Current run-time execution metadata persistence is **still not trustworthy enough** for audit and investigation use.

The implementation on this branch is internally consistent and focused tests pass, but the persisted `runs` record is still too sparse for investigation-quality provenance. The system records lifecycle, output, and token/cost totals, yet it does not persist immutable runner, model, repository, commit, or gate-target snapshots at run start.

---

## What Is Persisted Today

Evidence from `internal/db/migrations/001_init.sql`, `internal/db/queries.go`, and `internal/models/models.go` shows that the current run record includes:

- run identity: `id`, `agent_id`, `issue_key`, `mode`, `status`
- run output artifacts: `stdout`, `diff`
- token and cost totals: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_create_tokens`, `total_cost_usd`
- timing: `started_at`, `completed_at`, `created_at`

Related supporting metadata is also persisted elsewhere:

- `cost_events` rows linked by `run_id`
- session-scoped API keys linked by `run_id`
- `audit_runs.run_id` linkage for audit mode
- issue-level retry count inferred from count of `runs` per issue

---

## Findings

### 1. High: no immutable execution snapshot is persisted on `runs`

Current behavior:

- `internal/db/migrations/001_init.sql:41-58` defines `runs` with lifecycle, stdout/diff, token totals, and timestamps only
- `internal/db/queries.go:393-478` inserts and updates only those same fields
- `internal/models/models.go` no longer exposes runner/model/git/gate snapshot fields on `models.Run`
- `internal/scheduler/scheduler.go:175-187` creates the run row before execution without capturing runner/model/worktree/commit metadata

Auditability impact:

- investigators cannot answer which runner, model, branch, commit, worktree, or gate target produced a given run
- QA cannot correlate a historical result to an exact execution context
- retries cannot be compared across changes in runner, model, or code state using structured data

Owner:

- Backend Engineer to persist immutable run-start execution snapshots before insert

### 2. High: architecture docs overstate the current implementation

`docs/architecture.md:166` states that agents can self-report usage via `POST /api/v1/runs/{id}/tokens`.

Search across the codebase found no implementation of that route; the only match is the documentation line itself.

Auditability impact:

- operators and QA may rely on a recovery path for token accounting that does not actually exist
- documentation currently overstates the trustworthiness and redundancy of cost metadata capture

Owner:

- Founding Engineer to either implement the endpoint or remove the claim from docs
- QA to verify the selected behavior in a clean environment once fixed

### 3. Medium: retry analysis is only count-based, not causally reconstructable

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

### 4. Medium: run event history is declared in architecture but not used for execution tracing

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

- no durable capture of runner/model/git/gate execution context at run start
- no immutable event timeline exists for lifecycle reconstruction
- docs still describe capabilities that are not implemented

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

1. the persisted schema omits the key execution provenance fields needed for historical reconstruction
2. the scheduler does not capture immutable run-start snapshots before insert
3. architecture documentation claims at least one API capability that is not implemented

This is a correctness problem, not just a documentation polish issue.

---

## Recommended Updates

### Founding Engineer

- Extend `runs` with immutable execution snapshot fields for runner, model, worktree, branch, commit, and gate target
- Populate run snapshot fields before calling `CreateRun`
- Decide and implement whether `run_events` should record spawn, timeout, cancel, completion, and token-ingest milestones
- Either implement `POST /api/v1/runs/{id}/tokens` or remove the architecture claim
- Add regression tests that cover run creation on a migrated DB and verify snapshot persistence

### QA Engineer

- After the fix lands, verify on a clean database that a new run persists execution snapshot fields and remains viewable in run detail/history pages
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

- architecture/design docs for runtime metadata describe snapshots that the current implementation does not persist
- `docs/architecture.md` documents a token self-report endpoint that is not implemented
- `docs/architecture.md` describes `run_events` as a detailed run log, but execution tracing is not using that table

---

## Evidence

- `internal/models/models.go:217-239`
- `internal/db/queries.go:393-478`
- `internal/db/migrations/001_init.sql:41-58`
- `internal/db/migrations/001_init.sql:169-177`
- `internal/scheduler/scheduler.go:175-187`
- `internal/scheduler/scheduler.go:219-330`
- `internal/handlers/api.go:330-345`
- `docs/architecture.md:166`
- `docs/architecture.md:257-268`
- `go test ./internal/db ./internal/scheduler ./internal/handlers`

---

## Final Assessment

SO-80 should be considered an **audit failure with actionable remediation**, not a pass.

The design intent is close to what investigations and QA need, but the current implementation is not yet dependable because the persisted run record does not capture the execution context required for historical reconstruction. Until immutable run-start snapshots and matching docs/tests are in place, recorded run metadata should be treated as incomplete evidence.
