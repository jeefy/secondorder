# SO-78 Architecture: Run-time execution metadata persistence for issue runs

Date: 2026-04-11
Issue: SO-78
Owner: Architect
Status: Proposed for backend implementation

## Summary

This document defines how the system should persist **run-time execution metadata** for each issue run so that QA, audit, and operators can answer:

- who or what executed a run,
- which runner/model configuration was used,
- which repository execution context was used,
- which exact code revision was evaluated,
- whether the run targeted a specific gate subject,
- what fields are immutable historical facts versus mutable convenience summaries,
- and how historical run records should be exposed via API/UI.

The recommendation is to treat each issue run as an immutable execution record with a stable identity and a structured metadata snapshot captured at run start and finalized at run completion. Current issue-level summary fields may be denormalized for convenience, but the authoritative audit trail must live on the run record itself.

---

## Goals

- Persist a durable, queryable execution record for every issue run.
- Capture runner identity and execution environment as a snapshot at run time.
- Capture model and provider details used for the run.
- Capture worktree, branch, and commit SHA used by the run.
- Capture optional gate-target metadata for runs that evaluate or act on a deployment/QA gate.
- Preserve historical facts immutably for audit and QA review.
- Expose run history without requiring log scraping or comment parsing.
- Support future analytics such as success rate by model, branch, or runner.

## Non-goals

- Replacing raw execution logs or transcripts.
- Defining product policy for which metadata must be shown in the UI by default.
- Designing a generalized provenance graph across all entities beyond issue runs.

---

## Design principles

### 1) Runs are historical facts

An issue run is a historical event. Once a run starts, its captured execution metadata should be treated as a point-in-time fact, not a live reference that changes when agent or repository configuration changes later.

### 2) Snapshot, don’t infer later

If a run used `runner=opencode`, `model=github-copilot/claude-haiku-4.5`, branch `feat/so-78`, and commit `abc1234`, those exact values should be stored on the run. Audit consumers should not have to reconstruct them from current agent settings, git state, or logs.

### 3) Immutable core, mutable derived summaries

Core run metadata should be append-only / immutable after creation or finalization. Issue-level convenience fields such as “last_run_model” may be updated freely because they are derived views, not the audit record.

### 4) Structured first, markdown/comments second

Comments and artifacts remain useful for narrative context, but QA/audit queries should rely on structured API fields and relational records.

---

## Recommended domain model

## Primary entity: `issue_runs`

Assume the system already has or will have an issue-run concept. This proposal extends that concept so that a single run contains:

- lifecycle state,
- actor snapshot,
- execution configuration snapshot,
- repo/worktree snapshot,
- optional gate-target snapshot,
- timing,
- optional outcome summaries.

### Canonical shape

```json
{
  "id": "run_01J...",
  "issue_id": "e7854db7-f07c-43fd-b249-1a4077e8a74a",
  "issue_key": "SO-78",
  "attempt": 3,
  "status": "completed",
  "started_at": "2026-04-11T20:33:56Z",
  "completed_at": "2026-04-11T20:48:14Z",
  "trigger": "issue_checkout",
  "runner_snapshot": {
    "agent_id": "e21fd6d2-926d-4c8e-b570-6ded83fb6c17",
    "agent_slug": "architect",
    "agent_name": "Architect",
    "runner_key": "opencode",
    "runner_label": "OpenCode",
    "executor_type": "agent",
    "host": "worker-01",
    "workspace_root": "."
  },
  "model_snapshot": {
    "model_id": "github-copilot/claude-haiku-4.5",
    "model_family": "claude",
    "model_variant": "haiku-4.5",
    "provider": "github-copilot",
    "temperature": null
  },
  "code_snapshot": {
    "repository": {
      "vcs": "git",
      "repo_root": ".",
      "remote_url": "git@github.com:org/repo.git",
      "remote_name": "origin"
    },
    "worktree": {
      "path": ".",
      "kind": "primary",
      "is_detached_head": false
    },
    "git_ref": {
      "branch": "main",
      "commit_sha": "1dd032c4...",
      "commit_sha_short": "1dd032c",
      "dirty": true,
      "dirty_reason": "untracked_files"
    }
  },
  "gate_target_snapshot": {
    "gate_type": "deployment_gate",
    "target_scope": "production",
    "target_issue_key": "SO-43",
    "target_entity_type": "issue",
    "target_entity_id": "...",
    "evaluation_mode": "recheck"
  },
  "outcome": {
    "result": "success",
    "exit_reason": "completed_normally",
    "qa_disposition": null
  },
  "metadata_version": 1,
  "created_at": "2026-04-11T20:33:56Z",
  "updated_at": "2026-04-11T20:48:14Z"
}
```

---

## Required persisted metadata

## 1) Runner identity snapshot

Persist the identity of the actor and executor used for the run.

### Required fields

- `agent_id`
- `agent_slug`
- `agent_name` or display label
- `runner_key` — system enum such as `opencode`, `copilot`, `manual`, `system`
- `executor_type` — `agent`, `human`, `system`

### Recommended fields

- `host` / worker identifier
- `workspace_root`
- `scheduler_run_id` or orchestration correlation id

### Rationale

Agent assignments and runner configuration can change over time. Historical runs must still show who executed them and through which execution channel.

### Immutability rule

Runner snapshot fields are immutable after run creation except for an administrative repair path.

---

## 2) Model snapshot

Persist the exact model identifier the run used.

### Required fields

- `model_id` — full canonical identifier, e.g. `github-copilot/claude-haiku-4.5`

### Recommended fields

- `provider` — e.g. `github-copilot`, `openai`, `anthropic`
- `model_family` — e.g. `claude`, `gpt`, `gemini`
- `model_variant` — e.g. `haiku-4.5`
- `api_base` if multiple backends/providers may serve the same label
- parameter snapshot if operationally relevant, such as `temperature`, `reasoning_effort`, or policy profile

### Rationale

A model alias may later resolve differently, or an agent’s configured model may change. QA and audit need the exact model used by the historical run.

### Immutability rule

Model snapshot is immutable after run creation.

---

## 3) Worktree / branch / commit snapshot

Persist the repository execution context seen by the run.

### Required fields

- `worktree_path`
- `branch`
- `commit_sha`

### Recommended fields

- `repo_root`
- `remote_name`
- `remote_url` or normalized repo identifier
- `is_detached_head`
- `dirty`
- `dirty_reason` or dirty-file count
- `merge_base_sha` when a run compares against a target branch
- `head_commit_timestamp` when convenient for reporting

### Commit SHA requirement

`commit_sha` is the most important immutable code identity field. It should be captured once when the run starts execution against the repo context. If the run itself changes the workspace later, the original starting SHA should still remain persisted as the run’s starting code snapshot.

If the system wants richer provenance, it may additionally store:

- `start_commit_sha`
- `end_commit_sha`

But for first release, one required `commit_sha` field plus optional `end_commit_sha` is sufficient.

### Dirty workspace handling

A run may start with uncommitted or untracked changes. Audit users need to know this because `commit_sha` alone is then insufficient to fully reproduce the workspace state.

Recommendation:

- store `dirty` boolean,
- store a coarse `dirty_reason` enum such as `modified_files`, `untracked_files`, `staged_changes`, `mixed`,
- do not try to persist full diffs in the run record.

### Immutability rule

Starting code snapshot fields are immutable after capture. Optional completion-time fields such as `end_commit_sha` may be added only when the run finalizes.

---

## 4) Gate target metadata snapshot

Some issue runs are ordinary work runs. Others are explicitly evaluating or acting on a gate, such as QA validation or deployment gate recheck. For those runs, persist the gate target as a structured snapshot.

### Required when applicable

- `gate_type` — e.g. `deployment_gate`, `qa_gate`, `policy_gate`
- `target_scope` — e.g. `production`, `staging`, `release`, `issue`
- `target_entity_type` — typically `issue`
- `target_entity_id` and/or `target_issue_key`

### Recommended fields

- `evaluation_mode` — `initial`, `recheck`, `approval`, `audit`, `verification`
- `gate_record_id` if a canonical gate record already exists
- `gate_seq_target` or expected gate version if the run was triggered to assess a specific gate state

### Rationale

Without this snapshot, historical run history cannot answer “what gate was this run actually validating?” especially if the issue executing the run differs from the issue being gated.

### Immutability rule

Gate target snapshot is immutable after run creation.

---

## Immutability model

## Immutable fields

The following should be treated as historical facts and not mutated during ordinary operation:

- run identity (`id`, `issue_id`, `attempt`)
- runner snapshot
- model snapshot
- start-time code snapshot (`worktree_path`, `branch`, `commit_sha`, `dirty`, etc.)
- gate target snapshot
- `started_at`
- trigger/correlation identifiers

## Mutable but constrained fields

These may change as the run progresses:

- `status`
- `completed_at`
- `heartbeat_at` if the system tracks liveness
- `outcome.result`
- `outcome.exit_reason`
- optional completion-time fields such as `end_commit_sha`

## Administrative repair path

If bad data is captured due to a bug, do not silently overwrite the historical row in normal code paths. Prefer one of:

1. append a repair event in a companion run-events table, or
2. allow privileged repair updates while writing `repaired_at`, `repair_reason`, and `repaired_by` fields.

Recommendation: keep v1 simple and disallow ordinary mutation of snapshot fields at the application layer.

---

## Recommended relational schema

This proposal assumes an existing `issue_runs` table may already exist. If so, extend it. If not, create it with the fields below.

## Table: `issue_runs`

Core lifecycle plus denormalized summary fields.

### Columns

- `id` text/uuid PK
- `issue_id` uuid/text not null FK
- `attempt` integer not null
- `status` text not null
- `trigger` text null
- `runner_agent_id` uuid/text null
- `runner_agent_slug` text null
- `runner_agent_name` text null
- `runner_key` text null
- `executor_type` text null
- `host_name` text null
- `workspace_root` text null
- `model_id` text null
- `model_provider` text null
- `model_family` text null
- `model_variant` text null
- `repo_vcs` text null default `git`
- `repo_root` text null
- `repo_remote_name` text null
- `repo_remote_url` text null
- `worktree_path` text null
- `worktree_kind` text null
- `git_branch` text null
- `git_commit_sha` text null
- `git_commit_sha_short` text null
- `git_is_detached_head` boolean not null default false
- `git_dirty` boolean not null default false
- `git_dirty_reason` text null
- `git_end_commit_sha` text null
- `gate_type` text null
- `gate_target_scope` text null
- `gate_target_entity_type` text null
- `gate_target_entity_id` text null
- `gate_target_issue_key` text null
- `gate_record_id` text null
- `gate_evaluation_mode` text null
- `result` text null
- `exit_reason` text null
- `metadata_version` integer not null default 1
- `snapshot_json` jsonb/text null
- `started_at` timestamptz not null
- `heartbeat_at` timestamptz null
- `completed_at` timestamptz null
- `created_at` timestamptz not null
- `updated_at` timestamptz not null

### Constraints

- unique `(issue_id, attempt)`
- check enum-like constraints in application or DB for `status`, `executor_type`, `result`

### Indexes

- index `(issue_id, started_at desc)`
- index `(runner_agent_id, started_at desc)`
- index `(runner_key, started_at desc)`
- index `(model_id, started_at desc)`
- index `(git_commit_sha)`
- index `(gate_target_issue_key, started_at desc)`
- index `(status, started_at desc)`

### Why both columns and `snapshot_json`

Recommendation:

- store commonly queried fields as first-class columns,
- optionally store the complete structured snapshot in `snapshot_json` for forward-compatible evolution.

This gives efficient filters for QA/audit while still allowing future metadata growth without a migration for every nested field.

---

## Optional companion table: `issue_run_events`

If the system already models lifecycle events separately, use a child events table for heartbeat, repair, or phase transitions.

```json
{
  "id": "runevt_01J...",
  "run_id": "run_01J...",
  "seq": 4,
  "event_type": "heartbeat",
  "payload": {"status": "in_progress"},
  "created_at": "2026-04-11T20:40:00Z"
}
```

Recommended use cases:

- heartbeats
- phase transitions
- administrative repairs
- linkage to produced artifacts or comments

This is optional for SO-78, because the core requirement is metadata persistence on the run record itself.

---

## API contract recommendations

## 1) Extend issue detail to include recent run summary

`GET /api/v1/issues/{key}` should expose a concise run summary so consumers can see the latest execution context without an extra query.

### Example

```json
{
  "key": "SO-78",
  "status": "in_progress",
  "latest_run": {
    "id": "run_01J...",
    "attempt": 3,
    "status": "completed",
    "started_at": "2026-04-11T20:33:56Z",
    "completed_at": "2026-04-11T20:48:14Z",
    "runner": {
      "agent_slug": "architect",
      "runner_key": "opencode"
    },
    "model": {
      "model_id": "github-copilot/claude-haiku-4.5"
    },
    "git": {
      "branch": "main",
      "commit_sha": "1dd032c4...",
      "dirty": true
    },
    "gate_target": null,
    "result": "success"
  }
}
```

This is a convenience summary only. The authoritative audit trail is the run-history endpoint.

---

## 2) New issue run history endpoint

Recommended endpoint:

`GET /api/v1/issues/{key}/runs`

### Response shape

```json
{
  "issue": {
    "key": "SO-78"
  },
  "runs": [
    {
      "id": "run_01J3",
      "attempt": 3,
      "status": "completed",
      "started_at": "2026-04-11T20:33:56Z",
      "completed_at": "2026-04-11T20:48:14Z",
      "runner_snapshot": {
        "agent_id": "e21fd6d2-926d-4c8e-b570-6ded83fb6c17",
        "agent_slug": "architect",
        "agent_name": "Architect",
        "runner_key": "opencode",
        "executor_type": "agent"
      },
      "model_snapshot": {
        "model_id": "github-copilot/claude-haiku-4.5",
        "provider": "github-copilot",
        "model_family": "claude",
        "model_variant": "haiku-4.5"
      },
      "code_snapshot": {
        "worktree_path": ".",
        "branch": "main",
        "commit_sha": "1dd032c4...",
        "dirty": true,
        "dirty_reason": "untracked_files"
      },
      "gate_target_snapshot": null,
      "outcome": {
        "result": "success",
        "exit_reason": "completed_normally"
      }
    }
  ]
}
```

### Query parameters

Recommended filters:

- `limit`
- `before`
- `status`
- `runner_key`
- `model_id`
- `result`

This endpoint is the primary audit and QA surface.

---

## 3) Run detail endpoint

Recommended endpoint:

`GET /api/v1/runs/{id}`

Use this for complete per-run inspection, including any optional raw snapshot JSON and linked artifacts/events.

### Example additions

- scheduler correlation ids
- artifact links
- emitted comments
- associated gate evaluation id

---

## 4) Cross-issue audit query endpoint

QA and platform operators will eventually need cross-issue lookups.

Recommended endpoint:

`GET /api/v1/runs`

### Supported filters

- `issue_key`
- `agent_id` / `agent_slug`
- `runner_key`
- `model_id`
- `branch`
- `commit_sha`
- `gate_target_issue_key`
- `gate_type`
- `started_after`
- `started_before`
- `result`

### Example uses

- “Show all runs that used a given model.”
- “Show all QA gate rechecks against SO-43.”
- “Show all runs executed from a dirty workspace.”
- “Show all runs on commit `abc1234`.”

---

## Write-path recommendations

## Run creation timing

Capture and persist the execution snapshot as early as possible when a run is created or checked out.

Recommended flow:

1. Create run row with `status=starting` or `in_progress`.
2. Capture runner/model snapshot from scheduler/agent configuration.
3. Capture repo/worktree/git snapshot from the actual workspace.
4. Capture optional gate-target snapshot from the triggering context.
5. Persist all snapshot fields in the initial transaction if possible.
6. Update only mutable lifecycle/outcome fields when the run completes.

### Why early capture matters

If capture is delayed until the end of the run, branch and commit may already have changed and the persisted record will no longer represent the actual starting execution context.

---

## Exposure rules for audit and QA

## Audit requirements

Audit users should be able to answer, from structured data alone:

- Which agent executed this run?
- Which runner and model were used?
- Which branch and commit were involved?
- Was the workspace dirty?
- What gate target, if any, was being evaluated?
- When did the run happen and what was the result?

## QA requirements

QA should be able to:

- correlate a validation result to an exact commit SHA,
- verify whether a run evaluated the intended gate target,
- review previous runs on the same issue in chronological order,
- compare reruns across branches/models/runners.

## UI guidance

For issue pages, show:

- latest run summary,
- a “Run history” table,
- columns for attempt, start time, agent, runner, model, branch, commit, dirty flag, gate target, result.

For detail drawer/page, show:

- full immutable snapshot,
- completion details,
- linked comments/artifacts/events.

---

## Backward compatibility and migration

## Existing state

Today, some execution context may exist implicitly in:

- issue comments,
- agent configuration tables,
- git workspace at inspection time,
- artifact docs,
- logs.

Those sources are not sufficient for reliable historical audit because they are mutable, incomplete, or hard to query.

## Migration strategy

### Phase 1: additive schema introduction

- extend `issue_runs` or create it if absent,
- populate new snapshot fields for all future runs,
- expose run history APIs.

### Phase 2: best-effort historical backfill

For older runs, backfill only when reliable data exists.

Backfill priority order:

1. explicit run records/logs with model/runner/commit,
2. issue comments that name branch/commit/model,
3. scheduler/agent configuration at known timestamps if confidently reconstructable.

### Backfill rule

Do not fabricate precision. If old data is unknown, leave fields null and mark:

```json
{
  "metadata_version": 1,
  "snapshot_json": {
    "migration_warning": "Historical run metadata incomplete; branch and commit were not reconstructable"
  }
}
```

---

## Field-level recommendations

## Runner identity

Recommendation:

- snapshot both `agent_id` and `agent_slug`.

Why:

- IDs are stable for joins,
- slugs are human-readable and useful even if the agent row is later removed or renamed.

## Model

Recommendation:

- store full canonical `model_id` string,
- also split provider/family/variant into separate optional columns for analytics.

## Worktree/branch

Recommendation:

- persist both `worktree_path` and `git_branch`.

Why:

Multiple worktrees may use the same branch name or detached state. The path helps operators understand the concrete execution context.

## Commit SHA

Recommendation:

- store full 40-char SHA when available,
- optionally also store short SHA for display.

## Gate target metadata

Recommendation:

- make gate-target fields nullable on all runs,
- require them only for gate-oriented run types.

---

## Invariants

1. Every issue run has a stable run id.
2. Every issue run should capture runner identity snapshot.
3. Every issue run should capture model snapshot when an LLM-backed executor is used.
4. Every repo-backed issue run should capture branch and commit SHA at start.
5. Snapshot fields represent point-in-time facts and are not overwritten by later config changes.
6. Historical run records are returned newest-first but remain individually immutable.
7. Latest issue-level run summaries are denormalized views, not source of truth.
8. QA/audit queries must not require parsing markdown comments or logs to answer basic provenance questions.

---

## Minimal viable implementation

If scope must stay small, the minimum implementation that still satisfies SO-78 is:

- extend `issue_runs` with:
  - `runner_agent_id`
  - `runner_agent_slug`
  - `runner_key`
  - `model_id`
  - `worktree_path`
  - `git_branch`
  - `git_commit_sha`
  - `git_dirty`
  - `gate_type`
  - `gate_target_issue_key`
  - `started_at`
  - `completed_at`
  - `result`
- add `GET /api/v1/issues/{key}/runs`
- embed `latest_run` summary in `GET /api/v1/issues/{key}`
- treat those snapshot fields as immutable after run start

This delivers audit usefulness quickly while leaving room for richer nested snapshots and event modeling later.

---

## Recommended implementation plan

1. Extend or create `issue_runs` persistence with snapshot columns.
2. Capture runner/model/git/gate context at run creation time.
3. Enforce application-level immutability on snapshot fields.
4. Add run history read API on issues.
5. Add run detail endpoint.
6. Add cross-issue run filtering for QA/audit.
7. Backfill historical data only where confidence is high.
8. Optionally add `issue_run_events` later for heartbeat and repair tracking.

---

## Decision

The system should persist **run-time execution metadata as an immutable snapshot on each issue run**. The persisted run record must include runner identity, model, worktree/branch, commit SHA, and optional gate-target metadata. Historical run records should be exposed through structured run-history APIs and issue-level summaries so QA and audit users can inspect exact execution provenance without reconstructing it from mutable configuration or logs.
