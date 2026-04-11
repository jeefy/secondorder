# SO-99: Run-time execution metadata schema and audit semantics

Date: 2026-04-11
Parent: SO-59
Owner: Architect
Status: Proposed implementation guidance

## Purpose

This note defines the canonical schema semantics for run-time execution metadata persisted on issue runs, with emphasis on auditability and historical interpretation.

It builds on the existing snapshot fields already defined for `runs` and clarifies:

- which fields are required for new run records,
- what each field means,
- which values are immutable historical facts versus mutable/latest agent configuration,
- how API and storage layers should preserve compatibility,
- and how QA, auditors, and API clients should interpret historical rows.

## Scope

This guidance applies to persisted run metadata associated with a single execution attempt.

In scope:

- runner
- model
- worktree / branch
- commit SHA
- gate target metadata
- versioning / immutability expectations
- historical interpretation rules

Out of scope:

- lineage graph fields such as `lineage_id` or `parent_run_id`
- token accounting schema changes
- comment or approval semantics

## Canonical storage model

The authoritative execution-provenance record for a run is the `runs` row created at dispatch time.

Current additive snapshot fields on `runs`:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`

These fields are snapshots, not foreign-key lookups to current agent configuration.

## Required fields and semantics

For all newly created runs, the scheduler/API layer should attempt to populate every snapshot field below at run creation time.

### 1. `runner_snapshot` (required for new runs)

Meaning:
- the actual runner selected for this execution attempt at dispatch time
- example values: `claude_code`, `codex`, `gemini`, `copilot`, `opencode`

Semantics:
- must reflect the resolved runtime value after defaults and session overrides are applied
- must not be rewritten if the agent's configured runner changes later
- should be treated as the execution engine identity for this run

### 2. `model_snapshot` (required for new runs)

Meaning:
- the actual model selected for this execution attempt at dispatch time

Semantics:
- must reflect the resolved model used to start the run
- must be captured after any scheduler/session override logic
- must not be rewritten if the agent's configured default model changes later

Interpretation note:
- this is a historical string snapshot, not a guarantee that the model identifier remains valid in the provider catalog later

### 3. `git_worktree_snapshot` (required for new runs when a working directory exists)

Meaning:
- the filesystem worktree / working directory used to execute the run

Semantics:
- should store the exact path passed to the runner at start time
- should not be normalized later in a way that changes historical meaning
- must remain unchanged even if the agent's configured working directory changes later

Interpretation note:
- this field identifies the execution context path, not necessarily a clean git repository

### 4. `git_branch_snapshot` (best-effort required for git-backed runs)

Meaning:
- the branch name observed in the worktree at run start

Semantics:
- should be captured from the actual worktree immediately before or during run creation
- may be null if the directory is not a git repository or branch resolution fails
- must not be backfilled later from current branch state

Interpretation note:
- null means unknown/unavailable at capture time, not "main" or "unchanged"

### 5. `git_commit_sha_snapshot` (best-effort required for git-backed runs)

Meaning:
- the repository HEAD commit SHA observed at run start

Semantics:
- should be captured from the actual worktree immediately before or during run creation
- may be null if the directory is not a git repository or SHA resolution fails
- must not be recomputed later from the current repository head

Interpretation note:
- this is the strongest available run-start code provenance field today
- if absent, consumers should treat code-state reconstruction as incomplete

### 6. `gate_target_snapshot` (required when a run has an explicit execution target)

Meaning:
- the target object or mode the run is primarily executing against

Current canonical format:
- `issue:<ISSUE_KEY>` for issue-linked runs
- a run mode label for non-issue runs, e.g. `heartbeat`, `audit`

Semantics:
- should represent the dispatch target known at run creation time
- must be immutable once the run is created
- should not be rewritten if issue relationships or gate policies evolve later

Interpretation note:
- despite the name, this field is currently a generic target snapshot, not exclusively a deployment-gate identifier
- if richer gate metadata is needed later, it should be added additively rather than changing the meaning of existing values

## Historical accuracy and immutability rules

## Principle

Run snapshot fields are historical facts about how a run started. They are not live references to the agent's latest settings.

### Immutable historical fields

The following fields should be treated as immutable after `CreateRun` succeeds:

- `runner_snapshot`
- `model_snapshot`
- `git_worktree_snapshot`
- `git_branch_snapshot`
- `git_commit_sha_snapshot`
- `gate_target_snapshot`
- `started_at`
- `agent_id`
- `issue_key`
- `mode`

Rule:
- `CompleteRun` and any later reconciliation code may append completion/outcome data, but must not overwrite run-start provenance snapshots.

### Mutable lifecycle/outcome fields

These may legitimately change after run creation:

- `status`
- `stdout`
- `diff`
- token/cost fields
- `completed_at`

Rule:
- updating outcome fields must not alter the interpretation of the run-start snapshot fields.

## Latest agent config vs historical run snapshot

Consumers must distinguish:

- **historical run snapshot**: what this run actually used when it started
- **current agent config**: what the agent is configured to use now

Required interpretation rule:
- when historical and current values differ, clients must prefer the snapshot field for any audit, QA, billing explanation, or forensic timeline concerning that run

Examples:

- If an agent now uses `codex/gpt-5.4-pro` but an older run has `runner_snapshot=claude_code` and `model_snapshot=sonnet`, the older run must be reported as a Claude/Sonnet run.
- If an agent moved from one worktree path to another, historical run review must use `git_worktree_snapshot`, not the agent's current `working_dir`.

## Historical run interpretation for consumers

### Auditors

Auditors should interpret the snapshot fields as the authoritative evidence of:

- who/what executed the run,
- which code location was targeted,
- and which branch/commit was visible at start.

Audit rule:
- if a snapshot field is null, the correct conclusion is "not captured / unavailable," not an inferred replacement from current agent state.

### QA

QA should use snapshots to validate:

- a run executed against the expected branch/worktree,
- the expected runner/model was actually used,
- repeated runs on the same issue may differ in execution context even if the issue assignment stayed constant.

QA rule:
- run history should be read row-by-row as separate attempts; later runs do not retroactively redefine earlier runs.

### API clients

API clients should treat run snapshot fields as append-only provenance.

Client rule:
- fields ending in `_snapshot` should be rendered and cached as historical data for that run
- clients must not silently replace null or stale-looking snapshot values with current agent config values

## Versioning and schema evolution expectations

### Backward compatibility

The snapshot columns are additive and nullable.

Compatibility requirements:

- legacy rows created before the metadata rollout remain valid with null snapshots
- APIs must continue to serialize null for missing historical data
- migrations must not rewrite historical rows with guessed values

### Forward compatibility

If richer execution metadata is needed later, evolve additively.

Preferred evolution pattern:

- keep existing string snapshot fields stable
- add new columns or nested API fields for richer structure
- do not reinterpret existing values in place

Examples of additive future fields:

- `git_dirty_snapshot`
- `end_commit_sha_snapshot`
- `gate_target_type`
- `gate_target_id`
- `model_provider_snapshot`
- `model_family_snapshot`

### API contract guidance

For API responses exposing runs:

- keep the current snapshot field names stable
- preserve nullability semantics
- document that `_snapshot` means "captured at run creation time"
- avoid deriving these fields from live agent joins on read

## Storage and API implementation guidance

### Write-time capture

The scheduler or API path that creates the run should:

1. resolve final runner/model values,
2. capture worktree/branch/commit from the actual execution directory,
3. derive gate target from the dispatch context,
4. persist the run row before subprocess execution begins.

Rationale:
- provenance must reflect the run as launched, not as reconstructed after it finishes.

### Read-time behavior

Run-read APIs should expose persisted snapshot values directly from `runs`.

Avoid at read time:

- substituting current agent runner/model when snapshot is null,
- recalculating branch or commit from the current repository state,
- mutating response meaning based on present-day issue or gate configuration.

### Naming consistency note

The current storage name `gate_target_snapshot` should be interpreted as a generalized execution-target snapshot.

Implementation guidance:
- keep the current column/API field for compatibility
- if product semantics later require deployment-gate-specific targeting, add explicit fields rather than renaming or changing the meaning of existing data

## Recommended consumer-facing summary semantics

Use the following wording in docs and API references:

> Run metadata snapshots represent the execution context observed when the run started. They are immutable historical provenance fields and may differ from the agent's current configuration.

And:

> Null snapshot values indicate metadata was unavailable or not yet supported for that run; consumers must not infer replacements from current state unless explicitly labeled as derived/current.

## Risks and open questions

### 1. Null provenance on non-git or failing-git environments

Risk:
- branch and commit may be absent for valid runs, reducing forensic precision.

QA implication:
- tests should verify null is preserved and interpreted correctly.

### 2. `gate_target_snapshot` naming may overpromise gate specificity

Risk:
- consumers may assume the field always references a deployment gate.

Mitigation:
- document current semantics clearly and add explicit structured gate fields later if needed.

### 3. Resolved model identity may still be underspecified

Risk:
- a plain string such as `default` or a provider alias may be insufficient for long-term audit precision.

Follow-up option:
- add richer model/provider snapshot fields while preserving `model_snapshot`.

### 4. Worktree path portability

Risk:
- stored filesystem paths may not be portable across hosts or after environment changes.

Interpretation rule:
- the path is historical execution evidence, not a guarantee that the path still exists.

### 5. Start-time-only commit capture may miss in-run code mutation

Risk:
- a run may modify the repository during execution, and the start SHA alone cannot describe the final state.

Follow-up option:
- add `end_commit_sha_snapshot` and/or dirty-worktree metadata in a future change.

### 6. Legacy rows remain partially opaque

Risk:
- older runs will continue to have null metadata, limiting trend and audit completeness.

Compatibility decision:
- prefer honest nulls over inferred backfills.

## QA recommendations

QA should verify at least:

1. new runs persist all available snapshot values at creation time
2. completion updates do not mutate snapshot fields
3. agent runner/model changes after a run do not alter historical run responses
4. non-git or git-failure cases return null branch/commit snapshots without read-time substitution
5. legacy rows with null snapshots remain readable through API/UI
6. issue-linked runs render `gate_target_snapshot` as `issue:<KEY>` and non-issue runs preserve mode-based targets

## Decision summary

- The `runs` table is the source of truth for per-run execution provenance.
- Snapshot fields on `runs` are immutable historical facts captured at run creation time.
- Historical run interpretation must prefer snapshot values over current agent configuration.
- Null historical values must remain null rather than being inferred from present-day state.
- Future schema enrichment should be additive and backward compatible.
