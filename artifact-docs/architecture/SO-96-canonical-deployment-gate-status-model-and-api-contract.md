# SO-96 Architecture: Canonical deployment gate status model and API contract

Date: 2026-04-11
Issue: SO-96
Owner: Architect
Status: Proposed contract alignment for backend and QA

## Summary

Secondorder should use **one canonical deployment gate record per `release` or `deployment` issue** and an **append-only evaluation/event history** for every initial check and recheck.

The canonical gate row is the source for the **current unblock answer**. History rows preserve auditability and recheck chronology.

This note consolidates the contract expected by backend and QA based on the current rollout (`SO-62`, `SO-63`, `SO-75`, `SO-76`) and resolves the key semantics needed for SO-96:

- one gate aggregate per gated issue,
- one current state plus append-only history,
- stable read-path semantics for current unblock condition,
- additive migration/backfill,
- legacy comments/files remain narrative artifacts, not machine source of truth.

---

## 1. Canonical entity model

### 1.1 Aggregate

For issue types `release` and `deployment`:

- an issue has **zero or one** canonical gate record,
- a canonical gate has **zero or more** history rows,
- the gate row stores the **current projection**,
- the history table stores **immutable prior evaluations/rechecks**.

### 1.2 Canonical persistence shape

#### `deployment_gates`

One row per gated issue.

Recommended/current contract fields:

| Field | Meaning |
|---|---|
| `id` | Gate identifier |
| `issue_key` | Parent issue key; unique |
| `status` | Current gate status projection |
| `unblock_state` | Current machine-readable unblock classification |
| `unblock_condition` | Current human-readable unblock requirement or readiness note |
| `current_reason` | Current evaluation reason/classifier if stored |
| `created_at` | Creation timestamp |
| `updated_at` | Last projection update |

#### `deployment_gate_events` / `deployment_gate_evaluations`

Append-only child history table.

| Field | Meaning |
|---|---|
| `id` | Event/evaluation identifier |
| `gate_id` | Parent canonical gate |
| `seq` | Monotonic per-gate sequence if available |
| `status` | Status recorded by that evaluation |
| `unblock_state` | Machine-readable unblock classification for that evaluation |
| `unblock_condition` | Human-readable unblock condition for that evaluation |
| `reason` | Why this event exists (`created`, `recheck`, `legacy_backfill`, etc.) |
| `created_at` | Evaluation timestamp |

### 1.3 Naming compatibility rule

Current implementation/docs use `deployment_gate_events` and some earlier notes use `deployment_gate_evaluations`.

**Decision:** treat these as the same conceptual history stream. Backend may keep current table naming, but the contract requirement is append-only immutable history with one current projection on `deployment_gates`.

---

## 2. Lifecycle and invariants

### 2.1 Required invariants

1. **At most one canonical gate per gated issue.**
2. **History is append-only.** Rechecks insert new rows; they do not mutate or delete prior history.
3. **Current projection must match latest history entry.**
4. **Gate update + history append must happen transactionally.**
5. **Legacy narrative artifacts are not authoritative state.**

### 2.2 Creation rules

Create or ensure a canonical gate when:

- a `release` issue is created,
- a `deployment` issue is created,
- an existing issue is converted into one of those types,
- a legacy gated issue is read or updated and has not yet been backfilled.

### 2.3 Recheck rules

A recheck must:

1. resolve the existing canonical gate by `issue_key`,
2. append a new immutable history row,
3. refresh the current projection on the canonical gate row.

This applies even when the result is semantically unchanged.

#### Important example: blocked remains blocked

If a deployment is still blocked after a new check, backend should still append a history row when the operation represents a new evaluation. This preserves chronology for QA and audit review.

### 2.4 Resolution rules

A gate may move from blocked to unblocked/approved/resolved according to workflow, but the canonical model does not require creating a new record. The same gate row persists for the lifetime of the issue unless an explicit future archival/reopen design is introduced.

---

## 3. Status model

## 3.1 Current state fields

The contract should expose both:

- `status`: workflow-oriented current gate status,
- `unblock_state`: machine-readable answer to “is this currently blocked from proceeding?”,
- `unblock_condition`: the current textual requirement or explanation.

### 3.2 Recommended values

#### `status`

Supported values may follow current issue/gate workflow conventions, including:

- `pending`
- `blocked`
- `unblocked`
- `approved`
- `resolved`

Implementation may continue to project compatible issue-style statuses where needed, but **API semantics for current unblock behavior must not require consumers to infer from issue status alone**.

#### `unblock_state`

Use a compact canonical enum:

- `blocked` — current state is blocking deploy/release progress
- `unblocked` — current state does not block progress
- `unknown` — state cannot be confidently derived yet, usually during migration/backfill ambiguity

### 3.3 Field semantics

- `status` is the broader current gate lifecycle state.
- `unblock_state` is the normalized query field for current gating behavior.
- `unblock_condition` is the textual explanation or requirement attached to the current state.

### 3.4 Normalization guidance

Where older values already exist, backend should normalize them into `unblock_state` rather than asking consumers to map ad hoc strings.

Current known compatibility mapping from prior rollout:

- `blocked -> blocked`
- `open|passed -> unblocked`
- `closed -> unknown`

If historical data is ambiguous, prefer `unknown` over guessing.

---

## 4. API/read-path contract

### 4.1 Current unblock query semantics

The canonical read answer for current unblock behavior is:

- first choice: `deployment_gate.unblock_state`
- compatible flat issue projection: `issue.unblock_state`
- human context: `deployment_gate.unblock_condition` / `issue.unblock_condition`

Consumers should **not** infer current gate state from:

- issue comments,
- markdown decision files,
- count/order of gate artifacts,
- raw issue workflow status alone.

### 4.2 Required issue detail behavior

`GET /api/v1/issues/{key}` for `release`/`deployment` issues should expose:

```json
{
  "issue": {
    "key": "SO-601",
    "type": "deployment",
    "gate_status": "blocked",
    "unblock_state": "blocked",
    "unblock_condition": "wait for canary metrics"
  },
  "deployment_gate": {
    "issue_key": "SO-601",
    "status": "blocked",
    "unblock_state": "blocked",
    "unblock_condition": "wait for canary metrics"
  },
  "deployment_gate_history": [
    {
      "reason": "created",
      "status": "blocked",
      "unblock_state": "blocked",
      "unblock_condition": "wait for QA signoff"
    },
    {
      "reason": "recheck",
      "status": "blocked",
      "unblock_state": "blocked",
      "unblock_condition": "wait for canary metrics"
    }
  ]
}
```

### 4.3 Read-path guarantees

For gated issue types:

- the API must project the **latest canonical gate row**,
- history must be returned in append order (or clearly sortable order),
- legacy gates missing history should be repaired/seeded before or during projection if that is the chosen compatibility strategy,
- non-gated issue types should not emit misleading gate payloads.

### 4.4 Query/filter semantics

If list/filter APIs are added or already exist, the canonical filter for “what is blocked right now?” should operate on `deployment_gates.unblock_state = 'blocked'` rather than comment scanning or history scans.

If only issue-level fields are currently filterable, those issue fields must be projections of the canonical gate row.

---

## 5. Migration and backfill expectations

### 5.1 Migration goals

Migration must:

- create canonical gate rows for existing gated issues,
- seed at least one history record for preexisting state,
- preserve current gate meaning,
- preserve old artifacts/comments for audit context,
- avoid breaking clients that ignore new fields.

### 5.2 Backfill behavior

For existing `release` and `deployment` issues:

1. create one canonical gate row if absent,
2. set current projection from best available existing state,
3. append a synthetic initial history row such as `legacy_backfill` if no history exists,
4. derive `unblock_state` conservatively,
5. preserve legacy comments/files as non-authoritative evidence.

### 5.3 Ambiguity handling

When historical source material does not clearly distinguish blocked vs unblocked, backend should backfill:

- `status = pending` or equivalent non-terminal status,
- `unblock_state = unknown`,
- best-effort `unblock_condition` if textual evidence exists.

### 5.4 Compatibility with existing artifacts

Existing gate/decision artifacts remain useful for:

- audit trail,
- human narrative,
- historical context.

They should **not** continue to act as the machine-readable current-state source after migration. New automation should write to canonical gate persistence first, with comments/artifacts treated as mirrors if still needed.

---

## 6. Impact on existing gate/decision artifacts

### 6.1 Canonical source-of-truth change

After rollout, the canonical source of truth is the gate aggregate (`deployment_gates` + history), not standalone decision markdown/comments.

### 6.2 Allowed continued use of artifacts

Issue comments and artifact docs may still be created for:

- reviewer context,
- deployment narratives,
- signoff notes,
- operational breadcrumbs.

But they must be treated as **secondary representations**.

### 6.3 Practical rule for teams

When a recheck occurs:

- backend updates canonical gate state,
- backend appends history,
- optional issue comment/artifact may summarize the recheck,
- consumers querying current unblock state must read the API, not the summary artifact.

---

## 7. Risks and open questions for backend and QA

### 7.1 Backend risks

1. **Duplicate gate creation race**
   - Risk: concurrent updates create parallel gate rows.
   - Mitigation: unique constraint on `issue_key` plus transactional ensure/upsert behavior.

2. **Projection/history drift**
   - Risk: gate row updates but history append fails, or vice versa.
   - Mitigation: one transaction for append + projection refresh.

3. **Semantically no-op rechecks being dropped**
   - Risk: backend suppresses history rows if status text did not change.
   - Mitigation: define explicit “new evaluation” writes as history-appending even when resulting state matches prior state.

4. **Legacy ambiguity causing overconfident backfill**
   - Risk: migration guesses wrong current state from comments.
   - Mitigation: prefer `unknown` when evidence is weak.

5. **Split naming in implementation/docs**
   - Risk: confusion between `events` and `evaluations` naming.
   - Mitigation: keep one conceptual contract and document aliases clearly.

### 7.2 QA focus areas

1. verify exactly one canonical gate row per gated issue,
2. verify rechecks append history rather than creating new gates,
3. verify blocked-to-blocked rechecks still create auditable history entries when intended,
4. verify issue detail fields match nested `deployment_gate` projection,
5. verify backfilled legacy rows gain initial history without losing current status/condition,
6. verify non-deployment/non-release issues do not expose false gate data,
7. verify current blocked/unblocked answer is taken from canonical projection, not comments.

### 7.3 Open question to track

The remaining implementation alignment question is whether `unblock_state` is already fully persisted in the canonical table everywhere or partially derived in API projection for compatibility. Either approach is acceptable short-term, but the long-term target should be explicit persistence or deterministic projection from canonical gate state so list/filter behavior stays consistent.

---

## 8. Recommended contract decision

Adopt the following as the canonical deployment gate contract:

- **one** `deployment_gates` row per `release`/`deployment` issue,
- **append-only** `deployment_gate_history`/event rows for all evaluations and rechecks,
- **current unblock query answer** comes from `unblock_state` on the canonical gate projection,
- `unblock_condition` carries the human-readable current requirement,
- migration/backfill is additive and conservative,
- legacy comments/docs remain secondary artifacts only.

This is the model backend should implement consistently and QA should validate against.
