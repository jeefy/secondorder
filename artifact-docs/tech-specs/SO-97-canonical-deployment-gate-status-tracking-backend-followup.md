# SO-97: Canonical deployment gate status tracking backend follow-up

## Scope

This increment hardens canonical deployment gate behavior around compatibility and deterministic reads so every deployment/release issue can be treated as a single canonical gate source, including legacy issue type values.

## What changed

- Extended deployment gate issue-type normalization in `internal/models/models.go`:
  - added compatibility constant `TypeDeployLegacy = "deploy"`
  - added `IsDeploymentGateIssueType(issueType string) bool`
  - normalized checks through lowercasing and trimming
- Unified gate applicability checks:
  - `internal/db/queries.go` now calls `models.IsDeploymentGateIssueType(...)`
  - `internal/handlers/api.go` gate projection now uses the shared helper
- Made canonical gate projections deterministic when legacy duplicate rows exist:
  - issue query subselects in `internal/db/queries.go` now select the most recently updated gate row using `ORDER BY updated_at DESC, created_at DESC, id DESC LIMIT 1`
  - `GetDeploymentGateByIssueKey` now applies the same ordering
  - this preserves current-state correctness for compatibility-sensitive databases while still honoring the canonical model going forward
- Added test coverage:
  - `internal/db/db_test.go`
    - `TestDeploymentGateCanonicalCreationForLegacyDeployIssueType`
  - `internal/handlers/handlers_test.go`
    - `TestGetIssue_IncludesCanonicalDeploymentGateForLegacyDeployType`

## Acceptance criteria mapping

- One canonical gate per release/deployment target:
  - persistence keeps `deployment_gates.issue_key` unique (existing migration contract), and compatibility reads now deterministically select the latest state if legacy duplicates are present
- Rechecks append history on canonical record:
  - existing `AppendDeploymentGateStatus(...)` behavior remains append-only for `deployment_gate_events`
- API exposes current gate/unblock state:
  - `GET /api/v1/issues/{key}` continues returning issue gate fields and canonical `deployment_gate` payload for release/deployment classes, now including legacy `deploy` values
- Automated tests for create/recheck/query/compatibility:
  - existing tests cover create/recheck/query
  - this change adds explicit legacy compatibility tests for DB creation and API projection paths

## Validation

- `go test ./internal/db -run 'TestDeploymentGate(CanonicalCreationForDeploymentIssue|CanonicalCreationForLegacyDeployIssueType|RecheckAppendsEventsOnSingleGate|LegacyCompatibilityBackfillsHistory|GetIssueIncludesCurrentGateFields)$' -count=1`
- `go test ./internal/handlers -run 'TestGetIssue_(ExposesDeploymentGateFields|IncludesCanonicalDeploymentGateAndHistory|IncludesCanonicalDeploymentGateForLegacyDeployType|DoesNotIncludeDeploymentGateForNonDeploymentIssueType)$' -count=1`
- `go test ./internal/db ./internal/handlers ./internal/models`

## Migration and operational follow-up

- No new SQL migration is required for this increment.
- Recommended operational follow-up in environments predating canonical enforcement:
  - run a one-time data audit for duplicate `deployment_gates.issue_key` rows (should be impossible in current schema but may exist from pre-constraint snapshots)
  - if duplicates are found in any historical clone, keep the newest row as canonical and archive or remove older duplicates after backup
- Future schema hardening option (not required now):
  - add a data-integrity startup check that logs or alerts if duplicate gate rows are detected in non-standard restored datasets
