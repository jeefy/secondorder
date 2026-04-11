# SO-80 Audit: Runtime execution metadata persistence

Date: 2026-04-11
Issue: SO-80
Owner: Auditor
Status: Follow-up audit after SO-86/SO-87 implementation review

## Scope

Review the implemented run-time execution metadata persistence from an auditability perspective and validate whether the recorded fields are sufficient and trustworthy for:

- investigations,
- QA reviews,
- and retry analysis.

This follow-up audit re-checks the original SO-80 findings against the current repository state and related documentation updates.

## Evidence reviewed

- `internal/db/migrations/026_run_execution_metadata.sql`
- `internal/models/models.go`
- `internal/db/queries.go`
- `internal/scheduler/scheduler.go`
- `internal/db/db_test.go`
- `docs/architecture.md`
- `artifact-docs/architecture/SO-87-runtime-execution-metadata-reconciliation.md`
- wiki page `run-time-execution-metadata-persistence-for-issue-runs`

## Executive summary

The original SO-80 audit findings around schema/code drift, missing start-time snapshot population, and incorrect architecture claims about `POST /api/v1/runs/{id}/tokens` are now resolved in the current checkout.

The system now persistently records a consistent start-time execution snapshot on `runs` with matching migration, model, query, scheduler, and regression-test coverage for:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`

The current metadata is sufficient for basic provenance checks and QA review of which runner, model, worktree, branch, commit, and issue-target context a run started with.

However, two auditability gaps remain:

1. worktree cleanliness is not persisted, so a commit SHA cannot always be treated as a fully trustworthy reproduction key for investigations;
2. retry analysis remains issue-level run counting rather than lineage-aware provenance.

## Current findings

### Resolved: schema/code drift on run snapshot persistence

Resolved.

Evidence:

- migration `026_run_execution_metadata.sql` adds the six snapshot columns on `runs`
- `models.Run` includes matching fields
- `CreateRun`, `GetRun`, and list queries in `internal/db/queries.go` all read/write the same fields
- `TestCreateRunPersistsSnapshots` in `internal/db/db_test.go` verifies round-trip persistence

Audit impact:

- persisted fields are now structurally trustworthy as stored data rather than doc-only intent

### Resolved: scheduler now populates execution snapshot before insert

Resolved.

Evidence:

- `internal/scheduler/scheduler.go` sets `RunnerSnapshot`, `ModelSnapshot`, `GitWorktree`, `GitBranch`, `GitCommitSHA`, and `GateTarget` before calling `CreateRun(...)`
- branch and commit are captured via `captureGitMetadata(...)` before insert
- gate target is derived before insert

Audit impact:

- the run row now captures start-time provenance early enough to be meaningfully historical for branch/commit/runner/model context

### Resolved: architecture/docs no longer claim an implemented run-token endpoint

Resolved.

Evidence:

- `docs/architecture.md` explicitly states there is no implemented `POST /api/v1/runs/{id}/tokens` endpoint
- `artifact-docs/architecture/SO-87-runtime-execution-metadata-reconciliation.md` marks the endpoint as unimplemented future-state only
- the wiki page carries the same reconciliation

Audit impact:

- current-state docs now match actual write-path behavior for token accounting

### Open: commit SHA is not sufficient as a full reproduction key when the worktree is dirty

Open.

Evidence:

- the scheduler captures branch and commit via `captureGitMetadata(...)`
- the scheduler captures `git diff HEAD` at completion only
- there is no persisted start-time dirty flag, dirty reason, or equivalent run-start workspace cleanliness signal
- no schema/model fields exist for dirty-state provenance

Audit impact:

- investigators and QA can identify the starting branch and commit, but cannot determine from structured start-time metadata alone whether the run began from a clean checkout or from uncommitted/untracked local changes
- this weakens trust in `git_commit_sha_snapshot` as a standalone reproduction key
- completion-time `diff` is not a substitute because it reflects post-run workspace state, not necessarily the exact state at dispatch

Severity: medium

### Open: retry analysis is still count-based, not lineage-based

Open.

Evidence:

- `CountRunsForIssue(...)` remains the implemented retry proxy
- `internal/handlers/api.go` and scheduler logic still use issue-level run count operationally
- docs and wiki now correctly describe this limitation rather than overstating capability
- there are no `lineage_id`, `parent_run_id`, `retry_reason`, or same-head retry fields in the current schema/model

Audit impact:

- the system can identify that an issue has multiple runs
- the system cannot reliably distinguish:
  - same-head reruns,
  - reviewer-requested rework,
  - reassignment,
  - timeout/crash recovery,
  - or gate revalidation
- this is insufficient for trustworthy retry taxonomy or higher-confidence root-cause analysis

Severity: medium

## Sufficiency assessment

### Investigations

Partially sufficient.

What is now trustworthy:

- which agent/run row executed
- which runner and model were used
- which worktree path was used
- which branch and commit were captured at run start
- which issue-oriented target context was associated with the run
- what stdout, token totals, cost, and completion status were recorded

What is still missing for stronger incident reconstruction:

- whether the workspace was dirty at run start
- why a follow-up run occurred in relation to a prior run

### QA review

Mostly sufficient for current implementation-level QA review.

QA can now correlate a run to:

- agent,
- runner,
- model,
- branch,
- commit,
- and issue-target context.

This is enough for basic review of whether a result was produced against the expected repository context, assuming a clean checkout or external confidence in workspace hygiene.

Remaining limitation:

- QA still cannot prove from structured metadata alone that the evaluated workspace exactly matched the persisted commit SHA

### Retry analysis

Not sufficient for high-confidence audit analysis.

Current metadata supports only coarse signals such as:

- how many runs an issue accumulated,
- and which branch/commit each run started from.

It does not support trustworthy lineage questions like:

- Was this a same-head retry?
- Was this rerun caused by review feedback?
- Did a new assignee continue a previous lineage or start a new one?

## Trustworthiness conclusion

The execution metadata persistence is materially improved and is now trustworthy for basic run provenance.

The original high-risk trust failure caused by schema/code/write-path drift is resolved.

Full audit sign-off for investigation and retry-analysis trustworthiness should remain pending until at least the following are addressed:

- persist a run-start workspace cleanliness signal (`dirty` and ideally `dirty_reason`)
- add lineage-aware retry metadata, or explicitly limit audit consumers to coarse run-count interpretation

## Recommendations

### Backend engineer

- Add run-start workspace cleanliness metadata to `runs`, at minimum a boolean dirty flag and preferably a coarse dirty-reason enum.
- Capture that cleanliness signal before run insert in the same path that captures branch and commit.
- Add regression tests proving dirty-state persistence at run creation.

### Architect

- Keep current docs explicit that commit SHA identifies the starting HEAD commit but not the full workspace state when local modifications exist.
- Keep retry semantics documented as coarse operational heuristics until lineage fields are implemented.

### QA engineer

- Validate review scenarios against both clean and dirty worktrees once dirty-state capture exists.
- Define acceptance coverage for lineage-aware retry analysis if a future issue introduces `parent_run_id` or equivalent fields.

## Recommended follow-up issues

- New backend follow-up for start-time dirty-worktree persistence
- Future backend/architecture follow-up for lineage-aware retry provenance

## Final audit status

Status: partially resolved

Resolved since the original audit:

- schema/code drift on persisted run snapshot fields
- missing scheduler population before insert
- incorrect documentation of the unimplemented tokens endpoint

Still open:

- start-time dirty-worktree provenance
- lineage-aware retry metadata
