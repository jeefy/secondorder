# SO-86: Run snapshot persistence drift remediation

## Context

Audit follow-up SO-86 found schema/application drift for run execution snapshot fields in the `runs` table. The application expected to persist execution provenance fields (`runner_snapshot`, `model_snapshot`, `git_*`, `gate_target_snapshot`) but schema migrations in this branch did not include those columns.

This caused run insert failures (`table runs has no column named runner_snapshot`) and prevented trusted metadata capture before run creation.

## Persisted fields (authoritative)

The run record now persists the following immutable snapshot fields at creation time:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`

`gate_target_snapshot` semantics:

- issue run: `issue:{ISSUE_KEY}` (example: `issue:SO-86`)
- non-issue run: scheduler mode string (`heartbeat`, `audit`, etc.)

## Trust guarantees

- Snapshot is captured once in scheduler `spawnAgent` before `CreateRun` insert.
- Values are sourced from scheduler-controlled runtime context and local git introspection (`git rev-parse`), not from agent output.
- Snapshot columns are never updated in `CompleteRun`; completion only updates mutable execution results (`status`, `stdout`, `diff`, token and cost fields).
- This makes persisted metadata suitable for QA investigations, audit trails, and retry analysis inputs.

## Schema and migration

Added migration:

- `internal/db/migrations/026_run_execution_metadata.sql`

Migration adds all six snapshot columns to `runs`.

### Backfill stance

No data backfill is performed for existing rows. Historical runs created before migration remain with `NULL` snapshot values. This is acceptable for legacy records; trust guarantees apply to runs created after migration is applied.

## Test coverage

Added/updated automated coverage for drift prevention and trusted capture:

- DB persistence assertions in `TestCreateAndGetRun` for all six snapshot fields.
- Scheduler tests for gate-target derivation (`TestResolveGateTarget`).
- Scheduler tests for git metadata capture behavior:
  - invalid path returns empty branch/SHA
  - initialized repository returns non-empty branch and 40-char commit SHA

These tests guard against schema drift and runtime regressions where required fields stop being populated.
