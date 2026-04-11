# SO-93: Run metadata migration lineage reconciliation

## Problem

QA for SO-77 reported clean-environment canonical deployment gate validation failures caused by missing `runs` snapshot columns in the schema lineage used by the branch under validation.

## Reconciliation outcome

- The required schema migration is `internal/db/migrations/026_run_execution_metadata.sql`.
- This lineage already includes migration `026` (dependency present via prior lineage integration/cherry-pick work), so no additional schema migration file is required for SO-93.
- SO-93 adds regression tests to lock in clean-install and upgrade-path guarantees.

## Test coverage added

In `internal/db/db_test.go`:

- `TestOpenCreatesRunExecutionMetadataSnapshotColumns`
  - Verifies a clean database bootstrap includes all six run snapshot columns:
    - `runner_snapshot`
    - `model_snapshot`
    - `git_worktree_snapshot`
    - `git_branch_snapshot`
    - `git_commit_sha_snapshot`
    - `gate_target_snapshot`
- `TestRunMigrationsAddsRunExecutionMetadataSnapshotColumnsFromV25`
  - Simulates a pre-026 database at migration version 25 and verifies `RunMigrations()` applies migration 026 and adds all six snapshot columns.

## Validation run

- Passed: `go test ./internal/db ./internal/handlers ./internal/scheduler`
- Passed: `go test ./internal/...`

Repository-wide `go test ./...` is currently noisy in this workspace due to unrelated backup/auxiliary directories with missing external module dependencies; SO-93 validation focuses on canonical backend packages used by clean-environment gate coverage.

## Compatibility notes

- No destructive schema changes.
- Existing databases upgrade in place via migration `026`.
- Legacy rows remain compatible because snapshot columns are nullable.
