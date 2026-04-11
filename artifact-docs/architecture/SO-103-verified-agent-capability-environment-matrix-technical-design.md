# SO-103 — Technical design for verified agent capability/environment matrix

## Purpose

Define the backend, API, and presentation approach for exposing a **verified agent capability and environment matrix** for the **current run context**, with clear source-of-truth rules, verification semantics, freshness behavior, privacy/security controls, implementation dependencies, and test guidance.

This design builds on prior recommendations in:
- `artifact-docs/architecture/so-69-verified-agent-capability-matrix.md`
- `artifact-docs/tech-specs/SO-83-verified-agent-capability-and-environment-matrix-api.md`
- wiki page: `architecture-decision-verified-agent-capability-snapshot-and-verification-model`

## Goals

- Give agents and humans a trustworthy, machine-readable view of what an agent can do **in this run**.
- Prevent unsafe inference from prompt text, archetype defaults, or raw credential presence alone.
- Separate three concerns in the contract:
  1. **Capability status** — what the platform believes is possible.
  2. **Verification level** — how strongly that belief is supported.
  3. **Freshness** — whether the assertion is recent enough to trust operationally.
- Provide one canonical backend snapshot that can power both API and dashboard surfaces.

## Non-goals

- Replacing authoritative policy enforcement at action execution time.
- Exposing raw secrets, secret store identifiers, or full provider internals.
- Letting the frontend derive permissions independently.

---

## 1. Canonical model

### 1.1 Source of truth

The canonical source of truth is a **run-scoped capability snapshot** keyed by:

- `run_id`
- `agent_id`

The snapshot is assembled by the backend from trusted sources only. It may be generated on demand and cached briefly, or materialized and refreshed asynchronously, but the logical contract is the same: **the backend owns the matrix**.

### 1.2 Snapshot sections

Each snapshot contains:

- `run` — run/environment metadata affecting capability truth
- `agent` — the evaluated agent identity
- `capabilities[]` — action- or access-oriented assertions
- `environment_capabilities[]` — tool, filesystem, network, and runtime assertions
- `credentials[]` — sanitized credential references linked to capabilities
- `summary` — aggregate counts for UI convenience
- `freshness` — overall snapshot generation metadata

### 1.3 Assertion shape

Every capability, environment capability, and credential assertion includes:

- `status`
- `verification`
- `freshness`
- `provenance`
- optional `reason_codes`
- optional sanitized `details`

This keeps presentation simple and avoids special-case inference.

---

## 2. Trusted data sources

The backend should join information from the following trusted sources.

### 2.1 Run and runner metadata

Used for:
- current `run_id`
- agent identity in the run
- workspace path / repository presence
- runner type
- basic environment configuration
- current branch or detached state if available

Trust level:
- **verified** when emitted by the runner control plane or persisted run metadata.

### 2.2 Credential broker / secret injection records

Used for:
- whether a credential was injected into the current run
- provider identity (`github`, `slack`, `secondorder`, etc.)
- credential kind (`api_key`, `oauth_token`, `app_installation`, etc.)
- injection timestamp / expiry if known
- sanitized scope metadata if broker tracks it

Trust level:
- **verified** for credential presence in the run when the broker or backend injected it.

Important limitation:
- credential presence alone does **not** prove a higher-level action is allowed.

### 2.3 Policy/action registry

Used for:
- mapping action keys to required policies, scopes, constraints, and enforcement paths
- determining whether the platform intends to allow an operation category
- associating capability keys with enforcement references

Trust level:
- **verified** for platform policy intent.

Important limitation:
- policy allowance alone does not prove runtime/tool/provider prerequisites are currently satisfied.

### 2.4 Runtime probes

Used for:
- tool existence and versions (`git`, `gh`, browser tooling)
- workspace accessibility
- filesystem write permissions
- network reachability / restricted egress characteristics
- service availability checks within approved boundaries

Trust level:
- **verified** at the time of probe.

Important limitation:
- runtime probes can become stale quickly and need TTL-based freshness semantics.

### 2.5 Provider verifiers / integration adapters

Used for:
- token validity
- provider scopes / installation permissions
- repository/org access verification
- API operation-specific checks where available

Examples:
- GitHub installation permission check
- GitHub repo visibility/access check
- Secondorder session permission check

Trust level:
- **verified** when the provider/integration check succeeds and the result is within freshness bounds.

Important limitation:
- providers vary in precision and cost; not every capability will have a direct verifier.

### 2.6 Derived configuration metadata

Used for:
- archetype defaults
- agent role hints
- static integration declarations
- configured but unverified environment features

Trust level:
- **derived**, not fully verified for the current run.

---

## 3. Verification semantics

### 3.1 Status vs verification vs freshness

These dimensions must remain separate.

#### Status
Meaning: operational result of the assertion.

Recommended capability status enum:
- `allowed`
- `denied`
- `restricted`
- `unavailable`
- `unknown`

Recommended environment status enum:
- `available`
- `unavailable`
- `restricted`
- `unknown`

Recommended credential status enum:
- `present`
- `absent`
- `expired`
- `restricted`
- `unknown`

#### Verification level
Meaning: confidence based on trusted evidence.

Enum:
- `verified` — backend has direct trusted evidence for this run or current resource scope
- `derived` — inferred from trusted configuration or registry data, but not directly proven for the current run/action/resource
- `unverified` — no trusted evidence available, or evidence is too weak to rely on

#### Freshness state
Meaning: time validity of the evidence.

Enum:
- `fresh`
- `static`
- `stale`
- `expired`

Rules:
- `static` is for evidence that does not meaningfully age within the run, e.g. runner type, archetype assignment, immutable run metadata.
- `fresh` means within TTL or generated synchronously on the current request.
- `stale` means past preferred TTL but still retained for debugging/display.
- `expired` means clients should not rely on the assertion for decisioning without refresh.

### 3.2 Verification rules by evidence type

| Evidence type | Verification level | Freshness default | Notes |
|---|---|---:|---|
| Run metadata from control plane | `verified` | `static` | Immutable per run once created |
| Secret injection record | `verified` | `fresh` or `static` depending on token expiry availability | Proves presence, not action authorization |
| Policy registry mapping | `verified` | `static` | Proves configured policy intent |
| Runtime tool probe | `verified` | `fresh` | Tool/network checks should expire |
| Provider scope check | `verified` | `fresh` | Use short TTL |
| Archetype default / static config inference | `derived` | `static` | Never sufficient for sensitive action authorization |
| Missing verifier | `unverified` | `expired` or `static` with null timestamps | Must not be shown as allowed |

### 3.3 Combining evidence into a final assertion

A capability often depends on multiple sub-assertions. Final status should be computed conservatively.

Examples:

- `github.pull_request.create`
  - requires policy allowance
  - requires valid GitHub credential or installation
  - requires repo scope access
  - may require network/API reachability

Resulting status rules:
- `allowed` only when all required prerequisites are satisfied and sufficiently fresh
- `restricted` when action is generally allowed but limited by scope or runtime conditions
- `denied` when authoritative evidence shows the action is not permitted
- `unavailable` when a non-policy prerequisite is absent, such as no network or missing tooling
- `unknown` when the system cannot verify enough to conclude safely

### 3.4 Conservative interpretation rules

- If verification is not `verified`, sensitive actions should not be surfaced as confidently executable.
- `derived` may support explanatory UI, but not authorization claims for risky operations.
- `stale` and especially `expired` assertions must degrade to `unknown` or require explicit refresh for agent preflight use.
- Frontend must never upgrade an assertion based on local reasoning.

---

## 4. Freshness and refresh behavior

### 4.1 Freshness model

Every assertion includes:

- `checked_at`
- `expires_at`
- `freshness.state`
- optional `freshness.ttl_seconds`

The snapshot also includes:

- `freshness.generated_at`
- `freshness.snapshot_expires_at`
- `freshness.partial_staleness` boolean

### 4.2 Suggested TTL classes

Use TTL by evidence class rather than one global TTL.

| Assertion class | Suggested TTL |
|---|---:|
| Run identity / agent identity / immutable run metadata | none (`static`) |
| Secret injection presence | until run end or provider token expiry if known |
| Provider permission/scope validation | 5–15 minutes |
| Network reachability probes | 1–5 minutes |
| Tool availability/version probes | 5–30 minutes |
| Filesystem/workspace checks | 5–15 minutes |
| Derived config assertions | none (`static`) |

### 4.3 Refresh triggers

The snapshot should be refreshed when any of the following occur:

- run starts
- agent assignment changes within run context
- credential injection / revocation / expiry event occurs
- policy registry version changes
- workspace changes materially (repo attached, branch changes, remount)
- runtime/network probe invalidation event occurs
- explicit client refresh request
- action preflight requests a capability whose assertion is stale/expired

### 4.4 Read-time behavior

Recommended behavior for `GET` requests:

1. Read latest snapshot for `(run_id, agent_id)`.
2. If all required assertions are fresh/static, return immediately.
3. If some assertions are stale but not critical, return current snapshot with staleness markers.
4. If critical assertions are expired and cheap to refresh, refresh synchronously.
5. If refresh is expensive or failing, return existing snapshot with explicit `expired`/`unknown` semantics instead of guessing.

This avoids blocking the UI unnecessarily while preserving safety for automation.

### 4.5 Action-preflight behavior

If the matrix is used later by planners or action preflight:
- stale/expired sensitive assertions should trigger targeted refresh
- authoritative enforcement still happens in the execution path
- the matrix is explainability and planning input, not the final enforcement decision

---

## 5. API contract

## 5.1 Recommended endpoints

### Current actor / current run
`GET /api/v1/me/current-run/capability-matrix`

Primary consumer-friendly endpoint. Resolves the authenticated caller and current run context server-side.

### Explicit run/agent lookup
`GET /api/v1/runs/{run_id}/agents/{agent_id}/capability-matrix`

Used by admin, QA, support, and orchestrator flows with stronger authorization.

### Optional metadata endpoint
`GET /api/v1/capability-matrix/contract`

Returns enums, reason codes, and capability key catalog for clients.

## 5.2 Recommended response shape

```json
{
  "run": {
    "id": "run_123",
    "environment_id": "env_prodlike_01",
    "started_at": "2026-04-11T13:00:00Z",
    "workspace": {
      "repo_owner": "acme",
      "repo_name": "platform",
      "branch": "so-103-capability-matrix",
      "has_git": true,
      "has_github_cli": true,
      "network_egress": "restricted"
    }
  },
  "agent": {
    "id": "e21fd6d2-926d-4c8e-b570-6ded83fb6c17",
    "slug": "architect",
    "display_name": "Architect",
    "archetype": "software-architect"
  },
  "summary": {
    "allowed": 4,
    "denied": 1,
    "restricted": 2,
    "unavailable": 1,
    "unknown": 3
  },
  "freshness": {
    "generated_at": "2026-04-11T13:00:08Z",
    "snapshot_expires_at": "2026-04-11T13:05:08Z",
    "partial_staleness": false
  },
  "capabilities": [
    {
      "key": "github.pull_request.create",
      "category": "external_action",
      "display_name": "Create pull request",
      "status": "allowed",
      "verification": {
        "level": "verified",
        "method": "server_policy_plus_provider_check",
        "source": "capability-aggregator"
      },
      "freshness": {
        "state": "fresh",
        "checked_at": "2026-04-11T13:00:05Z",
        "expires_at": "2026-04-11T13:10:05Z",
        "ttl_seconds": 600
      },
      "provenance": {
        "sources": [
          "policy_registry",
          "credential_broker",
          "github_integration"
        ],
        "evidence_refs": [
          "policy:github.pr.create:v3",
          "credref_github_primary"
        ]
      },
      "scope": {
        "type": "repository",
        "value": "acme/platform"
      },
      "constraints": ["repo_access_required"],
      "credential_refs": ["credref_github_primary"],
      "reason_codes": []
    }
  ],
  "environment_capabilities": [
    {
      "key": "tool.git",
      "display_name": "Git CLI available",
      "status": "available",
      "verification": {
        "level": "verified",
        "method": "runtime_probe",
        "source": "runner"
      },
      "freshness": {
        "state": "fresh",
        "checked_at": "2026-04-11T13:00:01Z",
        "expires_at": "2026-04-11T13:15:01Z",
        "ttl_seconds": 900
      },
      "provenance": {
        "sources": ["runner_probe"],
        "evidence_refs": []
      },
      "details": {
        "version": "2.47.0"
      }
    }
  ],
  "credentials": [
    {
      "id": "credref_github_primary",
      "provider": "github",
      "kind": "app_installation",
      "display_name": "GitHub app installation",
      "status": "present",
      "verification": {
        "level": "verified",
        "method": "secret_injection_record_plus_scope_check",
        "source": "credential_broker"
      },
      "freshness": {
        "state": "fresh",
        "checked_at": "2026-04-11T13:00:03Z",
        "expires_at": "2026-04-11T13:30:03Z",
        "ttl_seconds": 1800
      },
      "provenance": {
        "sources": ["credential_broker", "github_integration"],
        "evidence_refs": []
      },
      "scopes": ["pull_requests:write", "contents:write"],
      "secret_material_exposed": false,
      "reason_codes": []
    }
  ]
}
```

## 5.3 Contract rules

- `status` is always required.
- `verification.level` is always required.
- `freshness.state` is always required even when timestamps are null.
- `credential_refs` must reference `credentials[].id`, not raw secret names.
- `reason_codes` are machine-readable and stable enough for UI mapping.
- `details` must remain sanitized and optional.

## 5.4 Reason code guidance

Examples:
- `missing_required_credential`
- `provider_scope_insufficient`
- `network_egress_blocked`
- `tool_not_installed`
- `workspace_not_accessible`
- `policy_denied`
- `resource_scope_mismatch`
- `verifier_unavailable`
- `assertion_expired`

---

## 6. Presentation/UI contract

The UI should be a direct projection of the API, not an independent interpreter.

### 6.1 Dashboard panel

Add a **Current Run Capability Matrix** panel containing:
- run ID and agent identity
- summary chips/counts by status
- filters for capability category and status
- searchable table of capabilities
- separate sections/tabs for `Capabilities`, `Environment`, and `Credentials`
- verification badge: `Verified`, `Derived`, `Unverified`
- freshness badge: `Fresh`, `Static`, `Stale`, `Expired`
- mapped explanations from `reason_codes`
- last checked timestamps and refresh affordance if supported

### 6.2 UI behavior rules

- `unknown`, `stale`, and `expired` must never use success styling that implies safe execution.
- UI must not hide denied/unavailable reasons when available.
- UI must not infer capability from a present credential row.
- UI may show a warning banner when `partial_staleness=true`.
- UI must tolerate partial data and per-assertion freshness differences.

### 6.3 Minimal rendering model

For each row, render:
- name/key
- status badge
- verification badge
- freshness badge
- scope
- constraints
- linked credentials
- reason text
- last checked

---

## 7. Security and privacy considerations

### 7.1 Sensitive data handling

Must never expose:
- secret values
- full tokens
- secret storage paths or IDs if they reveal internals
- raw env var names when they leak implementation detail
- hidden backend-only policy internals unless explicitly approved

### 7.2 Access control

- Responses must be scoped to the authenticated actor and authorized run context.
- Explicit run/agent endpoints require stricter auth than self/current-run endpoint.
- Multi-tenant boundaries must be enforced at org/workspace/repo scope.

### 7.3 Capability visibility sensitivity

The matrix itself may reveal privileged integration presence or organizational topology.

Mitigations:
- sanitize credential display names
- avoid over-detailed scope disclosure unless needed
- only return resource scopes relevant to the authorized viewer
- redact unsupported provider metadata by default

### 7.4 Anti-inference controls

- Do not expose enough metadata for clients to reconstruct secret names.
- Do not let credential presence imply action permission.
- Do not mark provider-specific capabilities `allowed` without either direct verification or an approved derived rule for low-risk cases.

### 7.5 Auditability

Action execution paths should log:
- the capability key evaluated
- enforcement result
- snapshot generation time / assertion checked time if referenced
- policy version or evidence refs used

### 7.6 Abuse and cost controls

- Provider verification and runtime probes may be rate-limited or cached.
- Endpoint should support bounded refresh behavior to avoid amplification attacks.
- Expensive provider checks should be selectively refreshed, not globally recomputed on every request.

---

## 8. Implementation approach

### 8.1 Backend components

#### Capability registry
Defines:
- stable capability keys
- display names and categories
- required evidence inputs
- reason code vocabulary
- optional provider-specific verifier hooks

#### Snapshot aggregator service
Responsibilities:
- load current run/agent context
- collect evidence from trusted sources
- compute per-assertion status/verification/freshness
- persist or cache snapshot
- expose a read model to handlers

#### Verifier adapters
Examples:
- credential broker adapter
- runner probe adapter
- GitHub verifier adapter
- policy registry adapter
- network probe adapter

#### API handlers
Expose:
- current-run matrix
- explicit run/agent matrix
- optional contract metadata

#### Audit/logging integration
Record snapshot generation, refresh reasons, and verifier failures.

### 8.2 Suggested persistence/cache model

A pragmatic design is:
- persistent snapshot row per `(run_id, agent_id)`
- embedded assertion JSON or normalized child tables
- per-assertion timestamps and evidence refs
- short-lived cache for read amplification reduction

Either persistence or cache is acceptable if the contract remains run-scoped and freshness-aware.

### 8.3 Recommended initial capability catalog

Action capabilities:
- `github.pull_request.create`
- `github.pull_request.comment`
- `github.pull_request.merge`
- `github.branch.push`
- `github.actions.trigger`
- `integration.archetype_patch.submit`
- `secondorder.issue.comment`
- `secondorder.issue.status.update`

Environment capabilities:
- `tool.git`
- `tool.gh_cli`
- `tool.chrome_mcp`
- `workspace.read`
- `workspace.write`
- `network.github.com`
- `network.public_internet`

Credential providers:
- `github`
- `secondorder`
- others as integrations mature

---

## 9. Dependencies

### Backend dependencies
- run metadata service/store
- credential broker or secret injection record source
- policy/action registry
- runner probe framework
- provider integration clients for scoped verification
- authz middleware and audit logging

### Frontend dependencies
- capability matrix endpoint(s)
- reason-code-to-copy mapping
- status/verification/freshness badge components

### Operational dependencies
- TTL configuration by verifier class
- observability for verifier errors and stale assertion rates
- rate limiting / caching strategy for provider checks

---

## 10. Testing considerations

### 10.1 Contract tests

Validate:
- required fields are always present
- enum values remain within documented set
- `credential_refs` resolve to known sanitized credential IDs
- no secret material appears in payloads

### 10.2 Aggregation logic tests

Scenarios:
- policy allows + credential present + provider scope valid => `allowed`
- policy allows + credential absent => `unavailable` or `unknown` per rule
- policy denies despite credential present => `denied`
- runtime tool missing => environment `unavailable`
- provider verifier timeout => downgrade to `unknown` with reason code
- stale/expired assertion changes planner-safe semantics

### 10.3 Freshness tests

Validate:
- TTL transitions from `fresh` to `stale` to `expired`
- refresh triggers regenerate only affected assertions when possible
- snapshot-level `partial_staleness` is correct

### 10.4 Security tests

Validate:
- no token value leakage
- no secret-name leakage through IDs or details
- unauthorized run/agent access is denied
- multi-tenant scope isolation holds

### 10.5 UI tests

Validate:
- unknown/unverified/expired do not render as allowed
- reason codes display correct text
- partial data does not break rendering
- filters and search work across sections

### 10.6 End-to-end tests

Cover at least:
- current-run self lookup
- explicit run/agent lookup with authorization
- capability state changes after credential injection/revocation
- provider scope downgrade reflected after refresh

---

## 11. Rollout plan

### Phase 1
Backend snapshot model + current-run API endpoint behind feature flag.

### Phase 2
Add explicit run/agent endpoint and contract metadata endpoint.

### Phase 3
Dashboard capability matrix using the same API contract.

### Phase 4
Use matrix for agent planning/preflight explainability while keeping action enforcement authoritative elsewhere.

---

## 12. Key decisions

- Use a **run-scoped backend-owned capability snapshot** as the canonical source of truth.
- Keep **status**, **verification**, and **freshness** as separate dimensions.
- Only backend-trusted sources can produce `verified` assertions.
- Expired or insufficiently verified assertions must degrade conservatively.
- API is the primary contract; dashboard is a direct presentation of the same model.
- Privacy controls require sanitized credential references and zero secret exposure.
