# SO-92: Clean-environment run metadata migration lineage

## Context

`SO-90` introduced run execution metadata snapshot reads/writes in `internal/db/queries.go` and `internal/models/models.go`, but its branch lineage did not include migration `026_run_execution_metadata.sql`. In a clean database, queries against `runs.runner_snapshot`, `runs.model_snapshot`, and related snapshot fields failed during gates.

## Implementation choice

Chosen approach: add migration `026_run_execution_metadata.sql` to the affected lineage instead of reverting query/model usage.

Rationale:
- Snapshot fields are already part of the intended canonical deployment gate provenance behavior and are validated by existing tests.
- Reverting query/model usage would discard intended metadata capture and weaken deployment gate traceability.
- Adding the missing migration restores schema/code alignment with minimal behavioral risk.

## Change

- Added `internal/db/migrations/026_run_execution_metadata.sql` with six `ALTER TABLE runs ADD COLUMN ...` statements:
  - `runner_snapshot`
  - `model_snapshot`
  - `git_worktree_snapshot`
  - `git_branch_snapshot`
  - `git_commit_sha_snapshot`
  - `gate_target_snapshot`

## QA and compatibility notes

- Existing databases that already applied migration 026 are unaffected.
- Clean-environment databases now receive schema required by current run queries before runtime access.
- QA rerun should execute full `artifact-docs/gates.sh` in a clean environment to validate canonical gate flow remains intact.
