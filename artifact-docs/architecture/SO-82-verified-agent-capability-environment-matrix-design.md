# SO-82 — Verified agent capability and environment matrix technical design

## Purpose

Define the backend-centered technical design for a verified agent capability and environment matrix that safely exposes current run-context capabilities for agents and humans. This design builds on the product specification and the earlier API/dashboard recommendation, and adds a concrete verification model, data source architecture, refresh strategy, API contract, and backend/dashboard implications.

## Goals

- Provide a trusted, machine-readable source of truth for what an agent can do in a specific run context.
- Distinguish verified facts from unavailable, unknown, stale, and partially derived assertions.
- Use only backend-trusted data sources for verification status.
- Support both API and dashboard consumers from a single canonical contract.
- Prevent secret leakage while still exposing enough detail for planning, audit, and incident response.
- Make capability checks refreshable without forcing every request to perform live provider calls.

## Non-goals

- Implement policy approval workflows.
- Guarantee real-time revocation awareness for third-party credentials.
- Replace enforcement gates; the matrix is a source of truth and preflight surface, not the only enforcement path.
- Expose raw secret names, secret values, or internal storage identifiers.

## Core design decision

Adopt a **run-scoped capability snapshot with verifier-backed assertions**.

At run creation, the platform creates a capability snapshot for each active agent in that run by aggregating trusted signals from:

1. **Run context metadata** — workspace, repo, branch, network policy, mounted tools.
2. **Credential broker records** — what sanitized credentials/integrations were injected into the run.
3. **Policy/action registry** — what actions the platform would allow the agent to request.
4. **Runtime probes** — what tools, network paths, and environment properties are actually reachable.
5. **Optional provider verifiers** — GitHub/GitLab/cloud/API scope checks where a safe verifier exists.

The snapshot is then refreshed according to verifier TTLs and explicit refresh triggers. API and dashboard surfaces read from the same snapshot model.

## High-level architecture

```text
+--------------------+      +----------------------+      +----------------------+
| Run Orchestrator   |----->| Capability Snapshot  |<-----| Runtime Probe Agent  |
| / Runner Lifecycle |      | Service              |      | (runner-side)        |
+--------------------+      +----------------------+      +----------------------+
           |                           ^         ^                    ^
           |                           |         |                    |
           v                           |         |                    |
+--------------------+                 |         |         +----------------------+
| Credential Broker  |-----------------+         +---------| Provider Verifiers   |
| / Secret Injection |                                     | GitHub, cloud, etc. |
+--------------------+                                     +----------------------+
           |
           v
+--------------------+
| Policy / Action    |
| Registry           |
+--------------------+
           |
           v
+--------------------+       +----------------------+       +----------------------+
| Capability Matrix  |<----->| REST API             |<----->| Dashboard            |
| Store (snapshots)  |       | /api/v1/...          |       | Current run view     |
+--------------------+       +----------------------+       +----------------------+
```

## Canonical data model

The canonical model is a **CapabilitySnapshot** keyed by `(run_id, agent_id)`.

### Snapshot entity

```ts
CapabilitySnapshot {
  snapshot_id: string
  run_id: string
  agent_id: string
  schema_version: string
  generated_at: timestamp
  refreshed_at: timestamp
  policy_version: string | null
  overall_status: 'fresh' | 'partially_stale' | 'stale'
  next_refresh_at: timestamp | null
  summary: {
    capabilities_allowed: number
    capabilities_denied: number
    capabilities_restricted: number
    capabilities_unavailable: number
    capabilities_unknown: number
    credentials_present: number
    environment_available: number
  }
  capabilities: CapabilityAssertion[]
  credentials: CredentialAssertion[]
  environment_capabilities: EnvironmentAssertion[]
  warnings: SnapshotWarning[]
}
```

### Assertion shape

All assertions share common provenance fields:

```ts
BaseAssertion {
  key: string
  display_name: string
  status: string
  verification: {
    level: 'verified' | 'derived' | 'unverified'
    freshness: 'fresh' | 'stale' | 'expired' | 'static'
    method: string
    source: string
    checked_at: timestamp | null
    expires_at: timestamp | null
    evidence_ref: string | null
  }
  reason_codes: string[]
  reason_detail: string | null
}
```

## Verification model

The matrix must separate **capability outcome** from **verification confidence**.

### 1. Outcome status

#### Action/integration capability status
- `allowed` — action is permitted in the current run scope.
- `denied` — explicitly not permitted by policy, provider scope, or missing prerequisites.
- `restricted` — possible only under listed constraints or narrower scope.
- `unavailable` — no credential/integration/configuration exists for this capability.
- `unknown` — platform cannot safely assert availability.

#### Credential status
- `present`
- `absent`
- `restricted`
- `expired`
- `unknown`

#### Environment capability status
- `available`
- `unavailable`
- `restricted`
- `unknown`

### 2. Verification level

- `verified` — backend-trusted source directly attests the assertion for this run.
- `derived` — calculated from verified facts plus deterministic rules, without direct provider proof.
- `unverified` — no trusted verifier currently exists or verification failed inconclusively.

### 3. Freshness state

- `fresh` — assertion checked and still within TTL.
- `static` — assertion comes from immutable run configuration and does not need periodic refresh.
- `stale` — last check exceeded preferred TTL but still may be useful for inspection.
- `expired` — assertion should not be used for automation without refresh.

### Verification decision table

| Source condition | Outcome example | Verification level | Freshness |
|---|---|---:|---|
| Secret injected + provider scope check succeeds | GitHub PR create allowed | verified | fresh |
| Secret absent by broker record | GitHub merge unavailable | verified | static |
| Policy allows action but provider scope not checked yet | PR merge allowed | derived | fresh/static |
| Probe failed due transient network issue | GitHub API reachability unknown | unverified | stale/expired |
| Capability type unsupported | Slack capability unknown | unverified | static |

### Important interpretation rule

A capability is considered **automation-safe** only when:
- outcome is `allowed` or `restricted`, and
- verification level is `verified` or approved `derived`, and
- freshness is not `expired`.

Dashboard can display stale/expired data for debugging, but execution gates should not rely on expired assertions.

## Trusted data sources

### A. Run orchestrator / runner metadata
**Examples**:
- run id, agent id, repo, branch, workspace
- mounted binaries (`git`, `gh`, `kubectl`)
- network policy profile
- available filesystem/storage mounts

**Why trusted**: generated by platform control plane and runner.

**Best used for**:
- environment capabilities
- scope fields
- static run metadata

### B. Credential broker / secret injection records
**Examples**:
- credential reference assigned to run
- provider type
- sanitized display label
- injection success/failure
- issue/lease expiration

**Why trusted**: server-side secret broker is authoritative for what was injected.

**Best used for**:
- `present` vs `absent`
- credential inventory
- initial capability derivation
- refresh triggers when credentials rotate/revoke

### C. Policy and action registry
**Examples**:
- which actions agent archetype may invoke
- repo/org/environment restrictions
- action preconditions
- internal platform permissions

**Why trusted**: authoritative for platform-enforced actions.

**Best used for**:
- `secondorder.issue.comment`
- `secondorder.issue.status.update`
- `integration.archetype_patch.submit`
- constraints and reason codes

### D. Runtime probes
**Examples**:
- command availability/version
- DNS/TCP/HTTP reachability to approved endpoints
- storage mount presence
- environment guardrails

**Why trusted**: executed by runner-side probe component; verifies actual run environment.

**Best used for**:
- `tool.git`, `tool.gh_cli`
- `network.github.com`
- internet egress / restricted egress markers
- compute and filesystem assertions

### E. Provider verifiers
**Examples**:
- GitHub token/app installation scope check
- cloud STS identity lookup
- kube auth can-i style checks
- API whoami/me endpoints where safe

**Why trusted**: direct provider confirmation, when available.

**Best used for**:
- repository scope and merge/write permissions
- cloud account identity and coarse service rights
- deployment target reachability

### Explicitly untrusted sources for verification
- agent prompt text
- agent self-report
- frontend-inferred state
- hard-coded credential env var naming conventions
- documentation-only assumptions

## Capability taxonomy

Use a registry of stable machine keys. Initial categories:

### Internal platform actions
- `secondorder.issue.comment`
- `secondorder.issue.status.update`
- `secondorder.issue.reassign`
- `integration.archetype_patch.submit`

### Repository/VCS actions
- `github.branch.push`
- `github.pull_request.create`
- `github.pull_request.comment`
- `github.pull_request.merge`
- `github.actions.trigger`

### Environment/tool capabilities
- `tool.git`
- `tool.gh_cli`
- `tool.kubectl`
- `network.github.com`
- `network.public_internet`
- `filesystem.workspace.write`
- `storage.persistent.available`

### Cloud/platform capabilities
- `aws.sts.identity`
- `kubernetes.cluster.access`
- `container_registry.push`

The registry should define:
- key
- category
- display name
- expected scope type
- supported verifiers
- default TTL
- sensitivity class

## Snapshot generation lifecycle

### On run start
1. Runner registers run metadata.
2. Credential broker records injected credential refs.
3. Snapshot service creates a pending snapshot for each active agent.
4. Static assertions are computed immediately from run metadata and broker records.
5. Runtime probes execute baseline checks.
6. Provider verifiers execute for supported credentials/capabilities.
7. Snapshot is marked `fresh` or `partially_stale` depending on verifier success.

### On read
- API serves latest snapshot.
- If requested with `refresh=if_stale`, backend schedules or performs selective refresh for stale assertions.
- Dashboard should prefer cached snapshot and surface freshness badges rather than forcing live provider checks on every load.

### On change events
Regenerate or incrementally refresh snapshot when any of the following occur:
- credential added/removed/rotated/revoked
- policy version changes for action gating
- run network profile changes
- workspace/tooling profile changes
- explicit operator refresh action

## Refresh and update strategy

Use **hybrid event-driven + TTL refresh**.

### TTL recommendations by source

| Source type | Example | TTL |
|---|---|---:|
| Run static metadata | workspace write access | none/static |
| Runtime tools | `git --version` | 15-60 min |
| Network reachability | github.com egress | 5-15 min |
| Broker-issued credential presence | token injected | until credential lease expiry or event |
| Provider scope check | GitHub write scope | 15-30 min |
| Internal policy grants | SO action permissions | until policy version change |

### Refresh modes

- **Full snapshot refresh**: used at run start and major environment changes.
- **Selective assertion refresh**: refresh only stale assertions in categories requested by API/UI.
- **Event-triggered invalidation**: mark affected assertions stale/expired immediately after credential or policy change.

### Recommended API behavior

Default reads should return immediately from stored snapshot with freshness metadata.
Optional query params:
- `refresh=none` (default)
- `refresh=if_stale`
- `refresh=force` for privileged users/operators only

### Failure handling

If one verifier fails:
- do not fail the whole snapshot.
- preserve the last known assertion if present.
- downgrade freshness to `stale` or `expired`.
- add reason codes such as `verifier_timeout`, `provider_unreachable`, `probe_failed`.

## Storage implications

Store snapshots in backend persistence as JSON or normalized tables, but treat the **API schema** as canonical.

Recommended minimum persistence fields:
- run_id
- agent_id
- snapshot payload
- schema_version
- generated_at
- refreshed_at
- next_refresh_at
- overall_status
- policy_version

If normalized tables are preferred later for audit/search, the system can still materialize the API contract from normalized records.

## API contract

### Primary endpoint

`GET /api/v1/runs/{run_id}/agents/{agent_id}/capability-matrix`

### Current-context convenience endpoint

`GET /api/v1/me/current-run/capability-matrix`

### Optional summary endpoint for dashboard lists

`GET /api/v1/runs/{run_id}/capability-matrices`

Returns summary rows only for all agents in a run.

### Query params

- `refresh=none|if_stale|force`
- `include=capabilities,credentials,environment,warnings` (default all)
- `category=internal_action,repo_action,environment,...` (repeatable)

### Response contract

```json
{
  "run": {
    "id": "run_123",
    "environment_id": "env_prodlike_01",
    "started_at": "2026-04-11T13:00:00Z",
    "workspace": {
      "repo_owner": "acme",
      "repo_name": "platform",
      "branch": "so-82-capability-matrix",
      "network_profile": "restricted-egress"
    }
  },
  "agent": {
    "id": "e21fd6d2-926d-4c8e-b570-6ded83fb6c17",
    "slug": "architect",
    "display_name": "Architect",
    "archetype": "software-architect"
  },
  "snapshot": {
    "schema_version": "2026-04-11",
    "generated_at": "2026-04-11T13:00:08Z",
    "refreshed_at": "2026-04-11T13:10:02Z",
    "overall_status": "fresh",
    "next_refresh_at": "2026-04-11T13:20:02Z",
    "policy_version": "policy_v44"
  },
  "summary": {
    "allowed": 6,
    "denied": 1,
    "restricted": 2,
    "unavailable": 3,
    "unknown": 1,
    "stale": 0,
    "expired": 0
  },
  "capabilities": [
    {
      "key": "github.pull_request.create",
      "category": "repo_action",
      "display_name": "Create pull request",
      "status": "allowed",
      "scope": {
        "type": "repository",
        "value": "acme/platform"
      },
      "constraints": ["github.repo_write_required"],
      "credential_refs": ["credref_github_primary"],
      "verification": {
        "level": "verified",
        "freshness": "fresh",
        "method": "provider_scope_check",
        "source": "github-verifier",
        "checked_at": "2026-04-11T13:09:55Z",
        "expires_at": "2026-04-11T13:39:55Z",
        "evidence_ref": "ev_github_scope_123"
      },
      "reason_codes": []
    },
    {
      "key": "github.pull_request.merge",
      "category": "repo_action",
      "display_name": "Merge pull request",
      "status": "denied",
      "scope": {
        "type": "repository",
        "value": "acme/platform"
      },
      "constraints": [],
      "credential_refs": ["credref_github_primary"],
      "verification": {
        "level": "verified",
        "freshness": "fresh",
        "method": "provider_scope_check",
        "source": "github-verifier",
        "checked_at": "2026-04-11T13:09:55Z",
        "expires_at": "2026-04-11T13:39:55Z",
        "evidence_ref": "ev_github_scope_123"
      },
      "reason_codes": ["missing_merge_permission"]
    }
  ],
  "environment_capabilities": [
    {
      "key": "tool.git",
      "display_name": "Git CLI available",
      "status": "available",
      "details": {
        "version": "2.47.0"
      },
      "verification": {
        "level": "verified",
        "freshness": "fresh",
        "method": "runtime_probe",
        "source": "runner-probe",
        "checked_at": "2026-04-11T13:09:40Z",
        "expires_at": null,
        "evidence_ref": "ev_probe_git_456"
      },
      "reason_codes": []
    }
  ],
  "credentials": [
    {
      "id": "credref_github_primary",
      "provider": "github",
      "kind": "app_installation",
      "display_name": "GitHub app installation",
      "status": "present",
      "resource_scope": {
        "type": "repository",
        "value": "acme/platform"
      },
      "scopes": ["pull_requests:write", "contents:write"],
      "secret_material_exposed": false,
      "verification": {
        "level": "verified",
        "freshness": "fresh",
        "method": "secret_injection_record_plus_scope_check",
        "source": "credential-broker",
        "checked_at": "2026-04-11T13:09:55Z",
        "expires_at": "2026-04-11T13:39:55Z",
        "evidence_ref": "ev_cred_789"
      },
      "reason_codes": []
    }
  ],
  "warnings": [
    {
      "code": "no_verifier_for_slack",
      "severity": "info",
      "message": "Slack capability types are not yet verifier-backed in this run."
    }
  ]
}
```

## Authorization and safety model

### Access rules
- Requester must be authenticated and authorized for the run and agent context.
- Agent-facing endpoint returns only the calling agent's own current-run matrix unless elevated privileges exist.
- Human dashboard access should inherit agent/run detail RBAC.

### Data minimization rules
Never expose:
- secret values
- raw bearer tokens
- internal secret store paths/IDs
- unsafe env var names if they reveal implementation detail
- full provider org/account topology if not necessary for the stated scope

May expose when approved:
- sanitized display labels
- coarse scopes (`repo:write`, `org:read`)
- resource scope limited to relevant repo/org/environment
- last-four hint only where policy explicitly allows it

### Audit requirements
Log:
- who queried the matrix
- run_id / agent_id
- refresh mode used
- whether force refresh triggered provider calls
- response schema version

## Backend components and responsibilities

### 1. Capability registry
Owns stable keys, categories, default TTLs, sensitivity, and verifier mappings.

### 2. Snapshot aggregation service
Collects data from broker, policy registry, runner probes, and verifiers; computes canonical snapshot.

### 3. Verifier adapters
Provider-specific modules with standard interface:

```ts
interface CapabilityVerifier {
  canVerify(assertionKey: string, context: VerificationContext): boolean
  verify(context: VerificationContext): Promise<VerificationResult[]>
  ttlSeconds: number
}
```

### 4. Snapshot store
Persists latest run-scoped snapshot and refresh metadata.

### 5. Matrix API layer
Authorizes requests, optionally triggers refresh, returns canonical response.

### 6. Action gate integration
Action execution layer should optionally read the same snapshot/evidence metadata for explainability, but still perform enforcement from authoritative policy at execution time.

## Dashboard implications

### Current run capability matrix view
Add a dedicated panel/tab on run or agent detail pages:
- summary counters by status
- table grouped by category
- separate tabs/sections for capabilities, credentials, environment
- badge pair for outcome status + verification freshness
- searchable/filterable keys and providers
- explicit legend explaining `unknown`, `unavailable`, `stale`, `expired`

### UX requirements
- dashboard renders API data only; no client-side permission inference.
- stale assertions should display warning styling, not green success.
- unknown should be visually distinct from unavailable.
- provide "last checked" and "next refresh" details.
- export should use same sanitized contract.

### Dashboard list pages
Use summary endpoint to display per-agent capability health for a run without loading full matrices.

## Reason codes and explainability

Use normalized machine-readable reason codes, for example:
- `credential_absent`
- `credential_expired`
- `provider_scope_missing`
- `policy_denied`
- `network_restricted`
- `tool_not_installed`
- `verifier_unavailable`
- `verifier_timeout`
- `provider_unreachable`
- `no_verifier_available`

API should also support optional human-readable `reason_detail` where safe.

## Recommended rollout plan

### Phase 1 — Backend foundation
- Implement capability registry.
- Add snapshot storage and canonical API contract.
- Support static metadata, broker records, policy grants, and runtime probes.
- Ship behind feature flag.

### Phase 2 — Provider-backed verification
- Add GitHub verifier first.
- Add coarse cloud identity verifiers where already supported by platform credentials.
- Introduce freshness TTLs and selective refresh.

### Phase 3 — Dashboard exposure
- Add current run capability matrix UI.
- Add summary list views and export.
- Add operator refresh controls.

### Phase 4 — Enforcement/automation integration
- Use matrix as preflight/planning input.
- Thread evidence refs into audit and action-explanation logs.

## Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Matrix becomes stale and misleading | bad planning/execution assumptions | freshness metadata, TTLs, stale/expired states, optional refresh |
| Provider verification is slow | poor API latency | snapshot reads by default, async refresh, selective verifier execution |
| Sensitive scope leakage | security exposure | data minimization policy, sanitized contract, RBAC |
| Frontend interprets presence as permission | unsafe user decisions | backend-computed statuses only, explicit status legend |
| Partial verifier failures cause false negatives | trust erosion | preserve last-known + mark stale, structured warnings |
| Capability taxonomy drifts across teams | inconsistent integrations | central capability registry and schema versioning |

## Open implementation notes

- Prefer additive schema versioning; new capability keys and reason codes should not break existing clients.
- Keep provider verifier outputs normalized before entering API schema.
- Treat `unknown` as a first-class state, not an error fallback.
- Avoid coupling the dashboard too tightly to any one provider-specific detail set.

## Recommendations to implementation teams

### Backend
- Build the snapshot service first; do not let the dashboard define the data model.
- Make broker/policy/probe integrations explicit and testable.
- Add schema contract tests and secret-leak regression tests.

### Frontend
- Render verification and freshness distinctly.
- Build with large `reason_detail` and long scope strings in mind.
- Do not collapse `unknown` into `unavailable`.

### QA
- Test transitions: present→revoked, verified→stale, stale→refreshed, unknown→verified.
- Verify no secret material or raw env var names leak.
- Validate API/dashboard parity from identical fixture payloads.

## Deliverables from this design

This issue should enable downstream work to implement:
- canonical snapshot schema
- verifier adapter interface
- run-scoped matrix endpoint(s)
- refresh and invalidation behavior
- dashboard rendering model
- audit-safe exposure rules
