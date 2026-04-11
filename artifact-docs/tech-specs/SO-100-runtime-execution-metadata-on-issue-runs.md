# SO-100: Runtime execution metadata persistence on issue runs

## Scope

This change ensures issue run history can be audited with immutable execution snapshots captured at run creation time.

## Implemented behavior

- `runs` records persist execution snapshot fields:
  - `runner_snapshot`
  - `model_snapshot`
  - `git_worktree_snapshot`
  - `git_branch_snapshot`
  - `git_commit_sha_snapshot`
  - `gate_target_snapshot`
- Snapshot values are written when the run is created and remain unchanged across later run updates.
- `GET /api/v1/issues/{key}` now includes a `runs` array containing all runs for that issue ordered by `created_at DESC`.
- Each run object in `runs` includes the snapshot metadata fields required for audit and QA review.

## Test coverage

- DB migration coverage validates snapshot columns exist on clean database creation and migration from v25.
- DB create/read coverage validates persisted snapshot values survive round-trip through `CreateRun` and `GetRun`.
- API coverage validates `GET /api/v1/issues/{key}` returns issue runs with per-run snapshot metadata and preserves historical differences between older and newer runs.

## Limitations and follow-up

- API inclusion of runs is currently on the issue detail endpoint only (`GET /api/v1/issues/{key}`).
- If payload size becomes a concern for issues with very large run history, a follow-up can add pagination or filtering for the embedded `runs` list while keeping immutable snapshots unchanged.
