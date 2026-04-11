# SO-79: Run-time execution metadata persistence

## Scope

Persist immutable run-time execution metadata for issue runs so audits and QA can identify the exact runner/model and git context used at dispatch time.

## Data model

Migration: `internal/db/migrations/026_run_execution_metadata.sql`

Added nullable snapshot columns to `runs`:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`

All fields are additive and nullable for backward compatibility. Existing rows remain unchanged.

## Runtime capture behavior

Implemented in `internal/scheduler/scheduler.go`.

At run creation (before subprocess launch), scheduler now snapshots:

- **Runner**: resolved runner used for execution (defaults to `claude_code` when agent runner is unset)
- **Model**: current agent model
- **Worktree**: agent working directory
- **Git branch**: `git rev-parse --abbrev-ref HEAD` (best effort)
- **Git commit SHA**: `git rev-parse HEAD` (best effort)
- **Gate target**:
  - `issue:<ISSUE_KEY>` for issue-linked runs
  - run mode for non-issue runs (`heartbeat`, `audit`, etc.)

If git metadata cannot be resolved, branch/commit snapshots remain null.

## Immutability and compatibility

- Snapshots are persisted through `CreateRun` and not modified by `CompleteRun`.
- Legacy run rows are still readable with null metadata.

## Verification

Automated coverage added:

- `internal/db/db_test.go`
  - `TestCreateAndGetRun` validates metadata snapshot persistence/retrieval
- `internal/scheduler/scheduler_test.go`
  - `TestResolveGateTarget`
  - `TestCaptureGitMetadataInvalidDir`
  - `TestCaptureGitMetadataInRepo`

Validation command:

`go test ./internal/db ./internal/scheduler ./internal/handlers`
