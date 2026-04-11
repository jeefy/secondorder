# SO-88 QA Assessment: Persisted Execution Metadata Coverage

Date: 2026-04-11
Issue: SO-88
QA Engineer: QA Engineer
Parent Issue: SO-80
Audit Report Reference: `artifact-docs/audits/SO-80-runtime-execution-metadata-audit.md`
Related PRs: #22 (audit), #24 (docs reconcile — SO-87)
Related Backend Fix: commit `5a75b376` (feat(runs): persist immutable execution metadata snapshots — SO-86)

---

## Executive Summary

The audit findings from SO-80 identified four critical/high problems in run-time execution metadata persistence. As of the backend fix commit (`5a75b376`), **three of the four audit findings have been addressed sufficiently to support QA review and incident investigation workflows**. One finding (retry lineage) remains a documented gap with agreed-upon future direction.

All tests pass as of this review (`go test ./... -count=1`).

---

## Verification Checklist

### Finding 1 (Critical): Schema/code drift on snapshot columns

**Status: RESOLVED**

- Migration `026_run_execution_metadata.sql` adds the six snapshot columns to `runs`.
- `TestCreateAndGetRun` in `internal/db/db_test.go` asserts all six fields are persisted and retrievable by value.
- The test previously would have failed with `table runs has no column named runner_snapshot`; it now passes cleanly.

**QA verdict:** Sufficient for regression prevention. Drift between code and schema is now caught by the test.

---

### Finding 2 (High): Scheduler does not populate snapshot fields before `CreateRun`

**Status: RESOLVED**

- `internal/scheduler/scheduler.go:184-197` now sets `RunnerSnapshot`, `ModelSnapshot`, `GitWorktree`, `GitBranch`, `GitCommitSHA`, and `GateTarget` before calling `db.CreateRun`.
- `TestCaptureGitMetadataInRepo` verifies the `git rev-parse` calls return real branch and SHA values.
- `TestResolveGateTarget` covers all three gate target cases (issue run, heartbeat, audit).
- `TestCaptureGitMetadataInvalidDir` verifies graceful degradation when `workingDir` is missing.

**QA verdict:** Sufficient for run-start provenance. An investigator looking at a historical run row will see the runner, model, worktree, branch, and commit used. The `ptrString` null-guarding means empty values degrade to NULL rather than blank strings, which is safe for query filters.

**Residual gap (low):** `GitBranch` and `GitCommitSHA` are conditionally set only when `captureGitMetadata` returns non-empty values. If the agent's `WorkingDir` is not a git repo, branch and commit will be NULL. This is acceptable for the current system (all agents run in git worktrees) but is worth asserting explicitly if non-git modes are added.

---

### Finding 3 (High): Architecture docs claim unimplemented `POST /api/v1/runs/{id}/tokens`

**Status: RESOLVED (via SO-87, PR #24)**

- `docs/architecture.md` now correctly states the endpoint does not exist and labels it future-state only.
- `artifact-docs/architecture/SO-87-runtime-execution-metadata-reconciliation.md` documents authoritative current state vs. proposed future API surface.
- The wiki page `run-time-execution-metadata-persistence-for-issue-runs` has been updated with the reconciled current state.
- No handler for `POST /api/v1/runs/{id}/tokens` exists in any route registration in `cmd/secondorder/main.go`.

**QA verdict:** Documentation now matches implementation. No QA test is needed for a non-existent endpoint; the absence of a route registration is the verification.

---

### Finding 4 (Medium): Retry analysis is count-based, not lineage-aware

**Status: DOCUMENTED GAP — not yet implemented, accepted for future work**

- `internal/db/queries.go:1331` still implements `CountRunsForIssue` as a simple run count.
- `internal/handlers/api.go:330-345` still uses `runCount > 6` as the reassignment threshold.
- No `parent_run_id`, `lineage_id`, or `retry_reason` field exists on `runs`.
- The architecture doc and wiki now explicitly document this as coarse and non-lineage-aware.

**QA verdict:** Acceptable for current operational needs (hotspot detection, retry limit enforcement). Insufficient for causal reconstruction of why a specific run existed. See enumerated validation scenarios below.

---

## Validation Scenarios Enumerated

### Scenarios now testable (post-fix)

| # | Scenario | How to Validate | Current Coverage |
|---|----------|----------------|-----------------|
| 1 | Run snapshot fields are persisted at creation | `TestCreateAndGetRun` | Covered |
| 2 | `runner_snapshot` matches configured runner | Scheduler unit test population | Covered via `TestResolveGateTarget` + `TestCaptureGitMetadataInRepo` |
| 3 | `git_branch_snapshot` is non-null when working dir is a git repo | `TestCaptureGitMetadataInRepo` | Covered |
| 4 | `git_commit_sha_snapshot` is 40-char SHA when in a git repo | `TestCaptureGitMetadataInRepo` | Covered |
| 5 | `gate_target_snapshot` is `issue:<key>` for task runs | `TestResolveGateTarget/issue_run` | Covered |
| 6 | `gate_target_snapshot` is mode string for non-issue runs | `TestResolveGateTarget/heartbeat_run`, `audit_run` | Covered |
| 7 | Graceful null degradation when not in a git repo | `TestCaptureGitMetadataInvalidDir` | Covered |
| 8 | `CompleteRun` persists token totals and `completed_at` | `TestCompleteRun` | Covered |
| 9 | `ListRunsForIssue` returns runs linked to an issue | `TestListRunsForIssue` | Covered |
| 10 | `CountRunsForIssue` returns accurate retry count | `TestCountRunsForIssue` | Covered |

### Scenarios not yet testable / gaps

| # | Scenario | Gap Description | Acceptance Criteria for Follow-up |
|---|----------|----------------|----------------------------------|
| A | Distinguish same-head rerun vs. new-commit rework | No `parent_run_id` or `start_commit_sha` field | Add lineage fields; test that two runs on same issue with different commit SHAs can be categorized |
| B | Identify retry reason (reviewer feedback vs. timeout vs. reassignment) | No `retry_reason` enum field | Add `retry_reason` field; test that scheduler populates it from context (wakeReason) |
| C | Reconstruct full retry chain for an issue | No run-to-run parent linkage | Add `parent_run_id`; test transitively linked chain from `ListRunsForIssue` |
| D | Validate that `runner_snapshot` matches actual runner used (not just configured) | No post-execution verification | End-to-end integration test: dispatch real run, verify row matches dispatch args |
| E | Assert snapshot fields are immutable after run creation | No DB constraint or application-level guard | Add constraint or test that `UpdateRun` does not overwrite snapshot fields |
| F | Cross-issue run audit query (e.g., all runs by model X) | No `GET /api/v1/runs` endpoint | API endpoint + test when implemented |

---

## Investigation Workflow Assessment

### Sufficient for current use

An investigator can currently answer:

- What runner and model were used for a given run?
- What git branch and commit SHA was the agent on when the run started?
- What was the gate target (issue key or run mode)?
- What tokens/cost were consumed?
- What stdout and diff did the run produce?
- How many times has an issue been run?

This is adequate for **incident triage** and **single-run debugging**.

### Not yet sufficient for full investigation

An investigator cannot yet reliably answer:

- Was this run a retry of a previous failure, or a fresh start?
- Which run did this supersede?
- Did the code change between this run and the previous one?
- What caused the retry (reviewer rejection vs. timeout vs. manual reassignment)?

These gaps are **documented but accepted** pending lineage field implementation (see gap A-C above).

---

## Retry Analysis Expectations

For future retry analysis to be investigation-quality, the following data must be present on each `runs` row:

- `parent_run_id TEXT` — references the run this is a retry of (NULL for first-attempt runs)
- `retry_reason TEXT` — controlled vocabulary: `reviewer_rework`, `timeout_recovery`, `reassignment`, `revalidation`, `first_attempt`
- `lineage_id TEXT` — shared across all runs in a retry chain for a single issue attempt

Until these fields are added, retry analysis remains a manual operation based on issue comments, timestamps, and run counts.

**Acceptance criteria for closing gap A/B/C:**
1. Migration adds `parent_run_id`, `retry_reason`, `lineage_id` to `runs`.
2. Scheduler populates them correctly based on dispatch context.
3. Tests verify that two consecutive runs for the same issue have correct `parent_run_id` and `retry_reason`.
4. `ListRunsForIssue` returns runs in retry chain order.

---

## Test Coverage Summary

**Existing coverage (sufficient):**
- `internal/db/db_test.go:TestCreateAndGetRun` — all six snapshot fields round-trip
- `internal/scheduler/scheduler_test.go:TestResolveGateTarget` — gate target logic
- `internal/scheduler/scheduler_test.go:TestCaptureGitMetadataInRepo` — git metadata capture
- `internal/scheduler/scheduler_test.go:TestCaptureGitMetadataInvalidDir` — graceful degradation

**Missing tests (follow-up required for full investigation-quality coverage):**
- Immutability guard: assert snapshot fields are not overwritten by `CompleteRun` or `UpdateRunStdout`
- Retry lineage: scheduler sets `parent_run_id` on consecutive issue runs
- Integration: end-to-end dispatch populates all snapshot fields correctly on real run

---

## Decision

The persisted execution metadata is **sufficient for QA review and incident investigation workflows at the current maturity level**, given that:

1. The critical schema/code drift is fixed and regression-tested.
2. The scheduler populates all six snapshot fields before run creation.
3. Documentation accurately reflects what is and isn't implemented.
4. Residual gaps (retry lineage) are documented with concrete acceptance criteria.

**Remaining gaps are follow-up items, not blockers for SO-80 closure**, provided the backend and docs fixes (SO-86, SO-87) land merged.

SO-80 should be reviewed for closure once SO-86 and SO-87 PRs are merged.
