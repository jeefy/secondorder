# SO-87 Runtime execution metadata reconciliation

Date: 2026-04-11
Issue: SO-87
Owner: Architect
Status: Implemented in documentation

## Purpose

This note reconciles the runtime execution metadata architecture with the current implementation in the repository. It addresses the audit finding that some docs reference an unimplemented runtime metadata endpoint and clarifies the current source of truth for execution provenance, token accounting, and retry analysis.

## Executive summary

The implementation already persists a useful subset of run execution metadata on the `runs` table, but the architecture doc overstated one write path and some future-state semantics:

- `POST /api/v1/runs/{id}/tokens` is **not implemented** and should not be documented as an active API flow.
- The **current source of truth** for persisted runtime execution metadata is the `runs` table, populated primarily by the scheduler at run creation and run completion time.
- The **current source of truth** for token/cost accounting is the scheduler’s parsing of runner stdout plus the derived `cost_events` rows.
- The **current retry model** is run-count based per issue (`CountRunsForIssue`) with a hard threshold used in reassignment/retry wake logic; it is not yet a lineage-aware retry model.
- The recommended future direction remains a richer lineage-based model that distinguishes reruns, reviewer-requested rework, reassignment, and branch/head changes.

---

## Current implementation: authoritative persisted fields

## Primary persisted record

The authoritative runtime execution record is `runs`.

Current persisted fields visible in the implementation include:

- lifecycle
  - `id`
  - `agent_id`
  - `issue_key`
  - `mode`
  - `status`
  - `started_at`
  - `completed_at`
  - `created_at`
- captured execution metadata snapshot fields
  - `runner_snapshot`
  - `model_snapshot`
  - `git_worktree_snapshot`
  - `git_branch_snapshot`
  - `git_commit_sha_snapshot`
  - `gate_target_snapshot`
- execution outputs and accounting
  - `stdout`
  - `diff`
  - `input_tokens`
  - `output_tokens`
  - `cache_read_tokens`
  - `cache_create_tokens`
  - `total_cost_usd`

These fields are exposed through the `models.Run` struct and persisted by `internal/db/queries.go`.

## Provenance of each field

### Fields captured at run creation by the scheduler

When a run is spawned, `internal/scheduler/scheduler.go` constructs the run record and persists it via `CreateRun(...)`.

Captured at start:

- `runner_snapshot`
  - provenance: resolved from the agent runner after session override handling
  - current shape: simple string, e.g. `opencode`, `claude_code`, `gemini`
- `model_snapshot`
  - provenance: current agent model after override handling
  - current shape: simple string, not yet a structured provider/family snapshot
- `git_worktree_snapshot`
  - provenance: `agent.WorkingDir`
- `git_branch_snapshot`
  - provenance: `git rev-parse --abbrev-ref HEAD` in the working directory
- `git_commit_sha_snapshot`
  - provenance: `git rev-parse HEAD` in the working directory
- `gate_target_snapshot`
  - provenance: `resolveGateTarget(issueKey, mode)`
  - current shape:
    - `issue:SO-87` when tied to an issue run
    - otherwise the run mode such as `heartbeat` or `audit`

### Fields finalized at run completion by the scheduler

When the runner process exits, the scheduler:

- parses token usage from stdout
- captures `git diff HEAD`
- updates the run row with final status and usage data via `CompleteRun(...)`

Captured at completion:

- `status`
- `stdout`
- `diff`
- `input_tokens`
- `output_tokens`
- `cache_read_tokens`
- `cache_create_tokens`
- `total_cost_usd`
- `completed_at`

### Derived accounting record

The scheduler also writes a `cost_events` row when parsed cost is greater than zero.

That means:

- `runs` is the authoritative per-run execution record
- `cost_events` is the authoritative per-agent/per-day rollup source for usage queries and budgets

---

## What is implemented vs. proposed

## Implemented today

Implemented and documented as current behavior:

- scheduler-created run rows
- persisted snapshots for runner/model/worktree/branch/commit/gate target
- persisted token/cost totals on the run row
- cost-event emission for usage/budget aggregation
- issue-level and UI-level access to run history through existing DB queries and run detail pages

## Not implemented today

The following should be treated as future-state or proposal-only, not current API behavior:

- `POST /api/v1/runs/{id}/tokens`
- structured nested JSON snapshots for runner/model/code/gate target
- dedicated `GET /api/v1/issues/{key}/runs` run-history API endpoint for agents
- dedicated `GET /api/v1/runs/{id}` REST detail endpoint for agents
- a cross-issue `GET /api/v1/runs` audit query endpoint
- lineage-aware retry classification and first-class rerun lineage fields

---

## Unimplemented token endpoint reconciliation

## Incorrect prior statement

The prior architecture documentation stated:

> Agents can also self-report via `POST /api/v1/runs/{id}/tokens`.

That statement is inaccurate for the current codebase.

## Actual implementation

Token usage is currently captured by scheduler-side parsing of runner stdout:

- Claude/OpenCode/Gemini/Codex output is parsed from stream-json or equivalent runner events
- the parsed values populate the `runs` row
- cost rollups are written to `cost_events`

There is no handler or route implementing a run-token self-report endpoint.

## Documentation rule going forward

Any mention of `POST /api/v1/runs/{id}/tokens` must be either:

- removed from current-state docs, or
- explicitly labeled **proposed / not implemented**

---

## Retry analysis semantics: current state

## What the system currently means by “retry”

Today, retry analysis is approximated by **number of runs associated with an issue key**.

Current implementation signals:

- audit tooling uses `CountRunsForIssue(issueKey)` as a proxy for retry-heavy work
- update logic in `internal/handlers/api.go` uses `runCount > 6` as a hard threshold when deciding whether to auto-wake an assignee after being sent back to `in_progress`
- reassignment can bypass that threshold
- CEO updates can bypass that threshold

So the current semantics are:

- a “retry” is effectively any additional run on the same issue
- the model does **not** distinguish why a new run happened
- the model does **not** distinguish whether the new run was on the same branch/head, after reassignment, after review feedback, or after unrelated issue mutation

## Current gaps

This run-count proxy is useful operationally, but incomplete analytically.

It cannot answer:

- whether run N+1 is a true retry of run N versus a fresh attempt after reassignment
- whether the rerun happened on the same commit SHA or after a code change
- whether the additional run was caused by reviewer-requested changes, crash recovery, timeout, or branch drift
- whether repeated runs represent one long lineage or several separate execution branches

## Current recommendation for interpreting metrics

Until lineage is implemented:

- treat `runs per issue` as a **coarse operational indicator**, not a precise retry taxonomy
- use it for alerting and hotspot discovery
- do not use it as a definitive measure of agent quality without looking at commit, assignee, and comment history

---

## Recommended lineage-based direction

## Why lineage is needed

A better retry model should describe relationships between runs rather than only counting rows.

The recommended future model is lineage-based so the system can distinguish:

- initial attempt
- rerun of the same execution target
- rework after reviewer feedback
- reassignment to another agent
- rerun after environment/tooling failure
- rerun against a changed branch or commit

## Suggested lineage fields

Recommended future additions on `runs` or a companion relation:

- `lineage_id`
  - stable id for a logical chain of work on the same issue/goal
- `parent_run_id`
  - direct predecessor when a run is a rerun or follow-up
- `retry_reason`
  - enum such as `review_feedback`, `timeout`, `runner_failure`, `manual_requeue`, `reassignment`, `qa_recheck`, `unknown`
- `supersedes_run_id`
  - optional explicit replacement link
- `requested_by_agent_id`
  - who caused the rerun when known
- `same_head_as_parent`
  - derived convenience boolean
- `start_commit_sha` and optionally `end_commit_sha`

## Suggested semantic rules

A future lineage-aware classifier should treat a run as:

- **same-head retry** when parent and child share issue, assignee intent, and start commit SHA
- **rework iteration** when the rerun follows reviewer or QA feedback and starts from a different commit than the parent
- **handoff/reassignment** when assignee changes
- **revalidation** when the run target is a gate/check rather than implementation work
- **recovery rerun** when the predecessor failed due to timeout/cancellation/crash

## Benefits

This would allow much better answers to questions like:

- Which agents need better instructions versus which issues simply needed multiple code iterations?
- Are retries caused by unstable tooling, weak acceptance criteria, or genuine product churn?
- How many reruns were same-head waste versus legitimate new-commit validations?

---

## Architecture guidance after reconciliation

## Current source-of-truth statement

Use this wording in architecture and wiki material:

> Runtime execution metadata is currently persisted on the `runs` table. The scheduler is the source that captures run-start provenance (runner, model, worktree, branch, commit, gate target) and run-completion accounting/output (stdout, diff, token totals, cost, completion status). `cost_events` provides the derived accounting ledger used for usage and budget rollups.

## Current limitation statement

> The current retry model is issue-level run counting, not a lineage-aware retry graph. Retry counts should be treated as coarse indicators until explicit parent/lineage semantics are implemented.

## Future-state statement

> Lineage-aware run relationships are recommended for future implementation so retry analysis can distinguish reruns, review-driven rework, reassignment, recovery, and revalidation flows.

---

## Repo docs updated by SO-87

This reconciliation should be reflected in:

- `docs/architecture.md`
- this artifact note
- the shared wiki page for runtime execution metadata

## Decision

For the current system, the authoritative persisted runtime execution metadata lives on `runs`, not on comments or a token self-report endpoint. Token accounting is scheduler-derived, and retry analysis remains coarse until lineage metadata is implemented.
