# SO-65: Persist execution metadata on issue runs

## What changed

- Added migration `internal/db/migrations/026_run_execution_metadata.sql` with six nullable snapshot columns on `runs`:
  - `runner_snapshot`
  - `model_snapshot`
  - `git_worktree_snapshot`
  - `git_branch_snapshot`
  - `git_commit_sha_snapshot`
  - `gate_target_snapshot`
- Extended `models.Run` to expose those snapshot fields in API/UI serialization.
- Updated run persistence and scan paths in `internal/db/queries.go`:
  - `CreateRun`
  - `GetRun`
  - `ListRunsForAgent`
  - `ListRunsForIssue`
  - `scanRuns`
- Updated scheduler dispatch to snapshot metadata at run start (before execution), not after:
  - runner/model at dispatch time
  - worktree path from execution working dir
  - git branch + commit SHA from current repo context (best effort)
  - gate target from dispatch context (`issue:<key>` for issue runs, fallback to mode)
- Updated API issue detail payload (`GET /api/v1/issues/{key}`) to include `runs`, so consumers can read per-run snapshot metadata directly.
- Updated run and issue UI templates to render the new execution metadata.

## Why

This preserves immutable provenance per run so audits and QA do not depend on mutable agent config or moving git refs after execution.

## Backward compatibility

- All new DB columns are nullable; existing rows remain readable.
- Legacy runs naturally serialize with omitted metadata fields.

## Validation

- Added DB coverage for persistence/round-trip of new fields in `internal/db/db_test.go`.
- Added scheduler coverage to assert metadata is captured at dispatch in `internal/scheduler/scheduler_test.go`.
- Added handler coverage to assert API response includes run metadata in `internal/handlers/handlers_test.go`.
