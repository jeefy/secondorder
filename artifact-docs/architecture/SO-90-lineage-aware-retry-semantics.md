# SO-90 Architecture: Lineage-aware retry analysis semantics and gap closure plan

Date: 2026-04-11
Issue: SO-90
Parent: SO-80
Owner: Architect
Status: Proposed documentation and follow-up plan

## Summary

The current system records **runs**, but retry analysis is still implemented as a simple **count of runs per issue**. That is operationally useful for hotspot detection and coarse retry limits, but it is **not sufficient for investigation-quality causal analysis**.

This document defines:

- the **current semantics** of retry analysis,
- the **desired lineage-aware model** for investigations and QA review,
- the **minimum metadata** required for trustworthy retry reconstruction,
- the **provenance expectations** for those fields,
- and the **engineering follow-up** required to close the remaining SO-80 audit gap.

## Current state: count-based retry semantics

### What the implementation does today

Current retry behavior is issue-centric rather than lineage-centric:

- every run is stored as an independent row in `runs`
- `CountRunsForIssue(issueKey)` returns `COUNT(*) FROM runs WHERE issue_key=?`
- reassignment / stuck detection uses that count as a coarse retry proxy
- audit prompt generation also uses this issue-level run count as a signal

### What current semantics can answer reliably

Today, the system can answer:

- how many runs occurred for an issue,
- when each run started and completed,
- which runner/model/worktree/branch/commit snapshot each run started with,
- what stdout, diff, tokens, and cost each run produced.

### What current semantics cannot answer reliably

Today, the system cannot reliably distinguish whether run N is:

- a retry of run N-1,
- a fresh run after reviewer-requested changes,
- a rerun on the same commit,
- a rerun on a new commit,
- a reassignment to a different agent,
- a recovery from timeout or crash,
- a gate revalidation run,
- or an unrelated follow-on execution on the same issue.

### Current audit conclusion

The current implementation is acceptable for:

- hotspot detection,
- simple retry-limit enforcement,
- rough operational heuristics.

It is **not acceptable as lineage-aware evidence** for a trustworthy investigation into why a run happened or how retries relate to one another.

## Desired model: lineage-aware retry semantics

### Core concept

A **run** is a single execution event.
A **lineage** is the chain of causally related runs that belong to the same investigation/retry thread.

The lineage model should allow an investigator to reconstruct:

1. where a chain started,
2. which run directly preceded another run,
3. why the next run happened,
4. whether code changed between runs,
5. whether the executor changed between runs,
6. and whether the run is actually a retry versus a new validation thread.

### Required semantic distinction

The model must distinguish at least these cases:

1. **first_attempt**  
   First run in a lineage for the issue context.
2. **same_head_rerun**  
   New run on materially the same code snapshot, usually due to infra/transient failure or operator rerun.
3. **reviewer_rework**  
   New run caused by requested changes after review or QA feedback.
4. **new_commit_rework**  
   New run after implementation changed the code snapshot.
5. **reassignment**  
   New run where ownership/executor changed and the run is part of handing work to a different agent.
6. **timeout_recovery**  
   New run caused by crash, timeout, restart recovery, or explicit recovery workflow.
7. **gate_revalidation**  
   New run intended to re-check a gate or validate a prior outcome, not to continue implementation.
8. **manual_rerun**  
   Human or operator-triggered rerun where the precise cause is not one of the structured reasons above.

A future implementation may collapse or rename some values, but these distinctions are the minimum needed for investigation-quality semantics.

## Intended lineage model

### Required identifiers

Each run should support the following lineage fields:

- `lineage_id` — stable identifier shared by all runs in the same causal chain
- `parent_run_id` — direct predecessor run in the chain; NULL for first run in the lineage
- `retry_reason` — controlled vocabulary explaining why this run exists relative to its parent
- `retry_sequence` — monotonic integer within a lineage, starting at 1

### Recommended commit boundary fields

To support trustworthy code-change analysis, the lineage model should also persist:

- `start_commit_sha_snapshot` — commit observed at run start
- `end_commit_sha_snapshot` — commit observed when the run finished, if available
- `code_change_class` — derived classification such as `same_commit`, `different_commit`, `unknown`, `dirty_workspace`

The current system already has a run-start commit snapshot equivalent (`git_commit_sha_snapshot`), but not lineage semantics that connect runs or classify their relationship.

### Invariants

The lineage model should satisfy these invariants:

1. A run belongs to exactly one lineage.
2. A first-attempt run has `parent_run_id = NULL` and `retry_reason = first_attempt`.
3. A non-first run has a non-null `parent_run_id` referencing an earlier run.
4. `parent_run_id` must reference a run on the same issue unless explicitly expanded later for cross-issue lineage.
5. `retry_sequence` must strictly increase within a lineage.
6. `lineage_id` must be immutable after run creation.
7. `parent_run_id` and `retry_reason` must be set at run creation, not inferred later from comments.

## Minimum metadata required for trustworthy investigations

The following is the minimum metadata set needed to make retry investigations trustworthy rather than best-effort.

### 1) Causal linkage fields

#### `lineage_id` (required)
Purpose:
- groups all causally related runs together
- separates independent run threads on the same issue

Trust expectation:
- assigned by scheduler/service logic at dispatch time
- immutable once the run is created

#### `parent_run_id` (required except first attempt)
Purpose:
- identifies the immediate predecessor run this run follows from
- allows reconstruction of the direct retry chain rather than only chronological guesses

Trust expectation:
- chosen by scheduler or API layer from the triggering context
- must not be backfilled from comment heuristics in normal operation

#### `retry_sequence` (required)
Purpose:
- provides deterministic order within a lineage
- avoids ambiguity when timestamps are equal or close together

Trust expectation:
- derived transactionally from lineage state at run creation

### 2) Reason/provenance fields

#### `retry_reason` (required)
Purpose:
- states why this run exists relative to its parent

Recommended vocabulary:
- `first_attempt`
- `same_head_rerun`
- `reviewer_rework`
- `new_commit_rework`
- `reassignment`
- `timeout_recovery`
- `gate_revalidation`
- `manual_rerun`
- `unknown`

Trust expectation:
- set from structured trigger context, issue transition context, or recovery workflow
- not inferred solely from natural-language comments

#### `trigger_type` (recommended)
Purpose:
- separates the mechanical trigger from the business reason

Examples:
- `issue_checkout`
- `review_request_changes`
- `scheduler_recovery`
- `manual_dispatch`
- `heartbeat`
- `gate_recheck`

Trust expectation:
- scheduler/API assigned at dispatch time

#### `trigger_actor_agent_id` / `trigger_actor_type` (recommended)
Purpose:
- records who or what caused the retry

Examples:
- reviewer agent
- CEO agent
- system recovery loop
- board/human

Trust expectation:
- populated from authenticated caller or internal scheduler identity

### 3) Code state fields

#### `git_commit_sha_snapshot` or `start_commit_sha_snapshot` (required)
Purpose:
- identifies the code revision at run start

Trust expectation:
- captured from the actual working directory at run creation
- immutable after capture

#### `git_dirty_snapshot` (required for trustworthy same-code claims)
Purpose:
- determines whether commit SHA alone is sufficient to represent the workspace

Trust expectation:
- captured from the actual working tree at run creation
- if missing, investigators must treat same-commit comparisons as potentially incomplete

#### `end_commit_sha_snapshot` (recommended)
Purpose:
- tells whether the run changed repo HEAD during execution
- supports stronger rework classification between parent and child runs

Trust expectation:
- captured at completion if available

### 4) Executor and target continuity fields

#### `agent_id` / `runner_snapshot` / `model_snapshot` (already required and largely present)
Purpose:
- shows whether a retry is actually a reassignment or executor change

Trust expectation:
- already captured at run start and should remain immutable

#### `gate_target_snapshot` (required when applicable)
Purpose:
- distinguishes implementation retry chains from gate revalidation chains

Trust expectation:
- captured from dispatch context at run creation

## Provenance expectations

For audit-quality lineage, the system should follow these provenance rules.

### Strong provenance

These fields must come from structured system state at dispatch time, not from later interpretation:

- `lineage_id`
- `parent_run_id`
- `retry_sequence`
- `retry_reason`
- `trigger_type`
- `trigger_actor_*`
- start-time git snapshot fields
- run-start runner/model snapshots

### Weak provenance (acceptable only as fallback)

These sources are useful for narrative context but should not be the source of truth for lineage fields:

- issue comments
- markdown docs
- raw stdout text
- later review summaries

### Provenance rule for audits

If any lineage field is reconstructed after the fact from comments or timestamps rather than captured at dispatch time, the result should be labeled **best-effort** rather than **trustworthy structured provenance**.

## Investigation semantics: how reviewers should interpret the model

### Current interpretation guidance

Until lineage fields exist, reviewers should treat additional runs on the same issue as only a **coarse signal** that multiple executions occurred.

They should **not** conclude from count alone that:

- the issue retried six times for the same failure,
- the latest run superseded the previous run,
- no code changed between runs,
- the same agent continued the same work thread,
- or the system crossed a true retry threshold for the same causal problem.

### Desired future interpretation guidance

With lineage fields implemented, QA and investigators should be able to answer:

- Show me the lineage for this run.
- Show the direct parent run and why this run was created.
- Tell me whether code changed since the parent run.
- Tell me whether this was rework, reassignment, recovery, or gate revalidation.
- Tell me which actor initiated the next run.

## Whether the current implementation satisfies audit needs

### Short answer

**No, not fully.**

The current implementation satisfies operational and partial QA needs, but it does **not** satisfy the specific SO-80 audit gap for lineage-aware retry analysis.

### What is already sufficient

The current implementation is already sufficient for:

- viewing run history per issue,
- seeing start-time runner/model/worktree/branch/commit snapshots,
- examining stdout/diff/cost by run,
- enforcing a simple issue-level retry threshold.

### What remains insufficient

The current implementation remains insufficient for trustworthy retry investigations because it lacks:

- stable lineage grouping,
- direct parent-child retry linkage,
- structured retry reason,
- structured trigger provenance,
- explicit code-change classification between runs.

### Audit disposition

Therefore the remaining SO-80 audit gap should be considered **documentation-defined but implementation-open**.

It should not be described as resolved merely because the limitation is documented.
Documentation now makes the gap explicit, but **engineering follow-up is still required** for true lineage-aware closure.

## Recommended engineering follow-up

## Follow-up recommendation 1: Add lineage fields to `runs`

### Recommendation

Add the following columns to `runs`:

- `lineage_id TEXT NOT NULL`
- `parent_run_id TEXT NULL`
- `retry_sequence INTEGER NOT NULL`
- `retry_reason TEXT NOT NULL`
- `trigger_type TEXT NULL`
- `trigger_actor_agent_id TEXT NULL`
- `trigger_actor_type TEXT NULL`
- `end_commit_sha_snapshot TEXT NULL`
- `git_dirty_snapshot BOOLEAN NULL`
- `git_dirty_reason_snapshot TEXT NULL`

### Acceptance criteria

1. A migration adds the columns above.
2. New runs always persist `lineage_id`, `retry_sequence`, and `retry_reason`.
3. Non-first lineage runs persist `parent_run_id`.
4. Tests verify referential and ordering behavior for at least a 3-run chain.
5. `CompleteRun` can append completion-only fields without mutating start-time lineage fields.

## Follow-up recommendation 2: Define dispatch-time lineage assignment rules

### Recommendation

Scheduler and API-triggered run creation should explicitly choose lineage behavior based on trigger context.

### Minimum dispatch rules

- fresh issue execution -> new lineage, `retry_reason=first_attempt`
- rerun after reviewer/QA request changes -> same lineage, parent = prior run, `retry_reason=reviewer_rework`
- reassignment-triggered rerun -> same lineage, parent = prior run, `retry_reason=reassignment`
- recovery after stale/failed running process -> same lineage if resuming same issue thread, `retry_reason=timeout_recovery`
- gate recheck workflow -> same or separate lineage by policy, but it must be explicit and documented

### Acceptance criteria

1. Scheduler has a single function responsible for deriving lineage metadata.
2. Unit tests cover first attempt, reassignment, recovery, and review-driven rework.
3. API/UI pathways that create reruns pass enough trigger context for deterministic lineage assignment.

## Follow-up recommendation 3: Expose lineage in read APIs

### Recommendation

Expose lineage information in run history endpoints and issue detail summaries so QA can inspect retry chains without log interpretation.

### Acceptance criteria

1. `ListRunsForIssue` returns lineage fields.
2. A run detail response includes parent linkage and retry reason.
3. Optional API filter allows querying by `lineage_id`.
4. UI/API documentation explains the meaning of lineage fields and controlled vocab values.

## Follow-up recommendation 4: Add investigation-oriented tests

### Recommendation

Add tests that verify investigation semantics, not only schema persistence.

### Acceptance criteria

1. Tests prove two runs on the same issue can be distinguished as same-lineage versus different-lineage if policy allows.
2. Tests prove same-commit rerun and new-commit rework produce different classifications when inputs differ.
3. Tests prove reassignment changes executor snapshot while preserving lineage continuity.
4. Tests prove count-based thresholds are not the sole source for lineage decisions.

## Gap closure recommendation for SO-80

SO-80 can be considered closed in one of two ways, depending on the agreed review bar:

### Option A: documentation-accepted partial closure

If SO-80 only requires that the residual lineage gap be precisely defined and tracked, then this documentation is sufficient to close the architecture/documentation portion while explicitly creating engineering follow-up.

### Option B: implementation-complete closure

If SO-80 requires retry investigations to be genuinely lineage-aware in production behavior, then SO-80 should remain open until the follow-up engineering above is implemented and validated.

### Architect recommendation

Prefer **Option B** for strict audit closure:
- current state is well documented,
- but the audit gap itself is still real,
- so true closure should require lineage field implementation and tests.

## Decision

The intended retry-analysis model is **lineage-aware**, not merely count-based. A trustworthy retry investigation requires explicit run lineage metadata captured at dispatch time: at minimum `lineage_id`, `parent_run_id`, `retry_sequence`, `retry_reason`, and trustworthy code/executor provenance. The current implementation does not yet satisfy that bar. Follow-up engineering is required to convert retry analysis from an operational heuristic into audit-quality lineage reconstruction.
