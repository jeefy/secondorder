# SO-101: QA Audit Validation — Persisted Run-Time Execution Metadata

**Issue:** SO-101  
**Parent:** SO-59  
**Date:** 2026-04-11  
**QA Engineer:** QA Agent

---

## Summary

This audit validates that issue runs now persist and expose execution metadata (runner, model, worktree/branch, commit SHA, gate target) sufficient for audit and QA review, as required by SO-59 and implemented in SO-100.

---

## Scope

The SO-100 backend PR (#42, branch `so-100-run-metadata-issue-runs`) adds:

1. **Schema migration** (`026_run_execution_metadata.sql`): 6 nullable columns added to `runs`:
   - `runner_snapshot`, `model_snapshot`, `git_worktree_snapshot`
   - `git_branch_snapshot`, `git_commit_sha_snapshot`, `gate_target_snapshot`

2. **Model** (`internal/models/models.go`): `Run` struct includes all snapshot fields as `*string` (nullable for backward compat), with correct JSON tags.

3. **Scheduler** (`internal/scheduler/scheduler.go`): `spawnAgent` captures and stores all 6 fields at run creation time.

4. **DB queries** (`internal/db/queries.go`): `CreateRun`, `GetRun`, `ListRunsForAgent`, `ListRunsForIssue` all include snapshot fields in SELECT/INSERT.

5. **API handler** (`internal/handlers/api.go`): `GET /api/v1/issues/:key` now returns `runs[]` array with all snapshot fields included.

6. **`IsDeploymentGateIssueType`** helper extracted to `models` package for consistent type checking, supporting canonical `deploy` legacy type.

---

## QA Plan

### Single-Run Scenario
- Verify a newly created run persists all 6 snapshot fields.
- Verify a run created without snapshot fields (legacy) is still readable.

### Multi-Run Historical Scenario
- Verify that multiple runs for the same issue each retain their distinct snapshots.
- Verify ordering is newest-first (correct association for audit trace).

### API Visibility
- Verify `GET /api/v1/issues/:key` returns `runs[]` with snapshot fields exposed in JSON.

### Backward Compatibility
- Verify legacy runs (no snapshot data) are readable without error.
- Verify migration from schema version ≤25 adds the columns correctly.

---

## Test Evidence

### Gate 1: Build
```
go build github.com/msoedov/secondorder/cmd/... github.com/msoedov/secondorder/internal/...
# Exit 0 — PASS
```

### Gate 2: Tests (all packages)
```
ok  github.com/msoedov/secondorder/cmd/secondorder
ok  github.com/msoedov/secondorder/internal/db
ok  github.com/msoedov/secondorder/internal/discord
ok  github.com/msoedov/secondorder/internal/handlers
ok  github.com/msoedov/secondorder/internal/models
ok  github.com/msoedov/secondorder/internal/scheduler
ok  github.com/msoedov/secondorder/internal/templates
ok  github.com/msoedov/secondorder/internal/validator
# All PASS
```

### Gate 3: Secret Scan
```
gitleaks detect --source internal  # Exit 0 — PASS
gitleaks detect --source cmd       # Exit 0 — PASS
```

**Note:** `go build ./...` / `go test ./...` in `gates.sh` fails due to unrelated third-party code in backup directories at repo root (`jeefy/bahamut/state/backups/`, `cncf/automation/jeefy/`). This is a pre-existing infrastructure issue, not introduced by SO-100. Application packages all build and test clean.

### New Tests Added (SO-101 QA coverage)

**`internal/db/db_test.go`:**
- `TestCreateRunPersistsExecutionMetadataSnapshotRoundtrip`: Creates a run with all 6 snapshot fields set, reads it back, and asserts all fields round-trip correctly through the DB layer.
- `TestCreateRunSnapshotFieldsNullableForLegacyRuns`: Creates a run with no snapshot fields, reads it back, and asserts all snapshot fields are `nil` (backward compat).

**`internal/models/models_test.go`:**
- `TestIsDeploymentGateIssueType`: Verifies the `IsDeploymentGateIssueType` helper correctly identifies deployment gate types (including `deploy` legacy string) and rejects non-deployment types.

### Existing Tests Validated

**`TestGetIssue_IncludesRunsWithExecutionMetadataSnapshots`** (`internal/handlers/handlers_test.go`):
- Creates two runs for the same issue with distinct snapshot metadata (different runners, models, branches, commits, gate targets).
- Calls `GET /api/v1/issues/SO-700` and asserts `runs[]` contains both runs in newest-first order.
- Asserts every snapshot field is correctly returned in the API response.
- **PASS** ✓

**`TestDeploymentGateCanonicalCreationForLegacyDeployIssueType`** (`internal/db/db_test.go`):
- Validates the legacy `deploy` issue type triggers deployment gate creation.
- **PASS** ✓

---

## Metadata Field Coverage

| Field | Captured at run creation | Persisted to DB | Readable from DB | Exposed in API |
|---|---|---|---|---|
| `runner_snapshot` | ✓ (`spawnAgent`) | ✓ | ✓ | ✓ |
| `model_snapshot` | ✓ (`spawnAgent`) | ✓ | ✓ | ✓ |
| `git_worktree_snapshot` | ✓ (`spawnAgent`) | ✓ | ✓ | ✓ |
| `git_branch_snapshot` | ✓ (`captureGitMetadata`) | ✓ | ✓ | ✓ |
| `git_commit_sha_snapshot` | ✓ (`captureGitMetadata`) | ✓ | ✓ | ✓ |
| `gate_target_snapshot` | ✓ (`resolveGateTarget`) | ✓ | ✓ | ✓ |

---

## Defects

None identified.

---

## Approval Recommendation

**APPROVE.** The feature satisfies all audit and QA requirements:

- All 6 required metadata fields are captured at run creation time (immutable — not overwritten on retry).
- Historical runs without metadata remain readable (nullable columns, backward compatible migration).
- The API surface exposes `runs[]` with full snapshot data per issue.
- Automated test coverage validates single-run, multi-run historical, API visibility, and legacy compatibility scenarios.
- Build, tests, and secret scan pass for all application packages.
