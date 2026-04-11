# Product Requirements Document: Verified Agent Capability and Environment Matrix

**Issue:** SO-102  
**Parent:** SO-60  
**Owner:** Product Manager  
**Status:** Delivered (API + Dashboard)  
**Last Updated:** 2026-04-11

---

## Executive Summary

The verified agent capability and environment matrix provides a source-of-truth view of what each agent can do in the current run context: verified credentials, allowed external actions, and environment capabilities. This eliminates guesswork during planning, audit, and incident response by exposing capability facts alongside verification provenance.

**Scope delivered:**
- Authenticated REST API endpoint: `GET /api/v1/agents/capability-matrix`
- Dashboard panel rendering the same verified semantics
- Explicit status semantics (verified / unknown / unavailable)
- Sanitization and trust-boundary controls preventing secret exposure

---

## Target Users and Primary Use Cases

### 1. **Planning & Architecture Teams**
*When:* Allocating work to agents, reviewing who has permissions for a task  
*Need:* Quick verification of actual agent credentials and integrations before assigning work  
*Success:* Team references matrix instead of making assumptions about agent permissions

### 2. **Audit & Compliance Teams**
*When:* Reviewing audit trails, validating permission scope against policy  
*Need:* Reviewable matrices for all agents with source metadata explaining how verification occurred  
*Success:* Audit reports cite actual capability matrices instead of documented assumptions

### 3. **On-Call / Incident Response Engineers**
*When:* Root cause analysis: "Could this agent have performed that action?"  
*Need:* Fast dashboard answers showing verified capabilities and limitations  
*Success:* Resolves "could this agent do X?" questions in seconds instead of minutes

### 4. **Platform / DevOps Teams**
*When:* Rotating credentials, updating agent permissions, or provisioning new integrations  
*Need:* Verify that changes are reflected in the capability matrix  
*Success:* Team shares updated matrices with relevant teams after credential rotation

---

## Required Matrix Fields

All matrix data must include **verification metadata** distinguishing verified facts from unknown/unavailable states.

### **Credentials & Integrations**
Each agent's configured credentials and their scope:
- Credential provider (GitHub, AWS, Azure, GCP, etc.)
- Credential type (OAuth token, API key, SSH key, session)
- Scope/permissions (e.g., pull_requests:write, contents:write)
- Verification level (verified, unknown, unavailable)
- Verification method (secret injection record, provider scope check, etc.)
- **Never expose actual secret values or raw env var names**

**Example verified credential:**
```json
{
  "id": "credref_github_primary",
  "provider": "github",
  "kind": "oauth_token",
  "display_name": "GitHub app installation",
  "status": "present",
  "verification": {
    "level": "verified",
    "method": "secret_injection_record_plus_scope_check",
    "source": "credential-broker",
    "checked_at": "2026-04-11T13:00:03Z",
    "expires_at": "2026-04-11T13:30:03Z"
  },
  "scopes": ["pull_requests:write", "contents:write"],
  "secret_material_exposed": false
}
```

### **External Actions**
Specific capabilities the agent may perform (not just "has GitHub token"):
- Pull request creation / merge / commenting
- Branch push and CI/CD trigger
- Archetype patch submission
- Issue status updates
- Deployment/integration actions
- Status: allowed, denied, restricted, or unavailable
- Reason codes explaining denied/unavailable states (e.g., "missing_merge_permission")

**Example denied capability:**
```json
{
  "key": "github.pull_request.merge",
  "category": "external_action",
  "display_name": "Merge pull request",
  "status": "denied",
  "verification": {
    "level": "verified",
    "method": "provider_scope_check",
    "source": "github-integration",
    "checked_at": "2026-04-11T13:00:07Z",
    "expires_at": "2026-04-11T13:30:07Z"
  },
  "reason_codes": ["missing_merge_permission"]
}
```

### **Environment Capabilities**
Runtime environment and tool availability:
- Tool availability: Git CLI, GitHub CLI, Docker, etc.
- Network access: GitHub API, public internet, etc.
- Compute resources: storage, memory, execution environment
- Verification method and checked timestamps

**Example environment capability:**
```json
{
  "key": "tool.git",
  "display_name": "Git CLI available",
  "status": "available",
  "verification": {
    "level": "verified",
    "method": "runtime_probe",
    "source": "runner",
    "checked_at": "2026-04-11T13:00:01Z"
  },
  "details": {
    "version": "2.47.0"
  }
}
```

### **Run Context**
Metadata about the run and when verification occurred:
- Run ID and start time
- Instance name and workspace identity
- Generated timestamp (when matrix was computed)
- Verification metadata: overall level, method, checked_at, expires_at

---

## Freshness & Verification Expectations

### **Verification Semantics**

Each capability is classified using one of three levels:

| Level | Meaning | When to Use | Examples |
|-------|---------|-------------|----------|
| **Verified** | Backend has confirmed this capability exists in current run context through trusted sources | Runtime probes, server-side credential broker, provider scope checks, policy registry | Git CLI present; GitHub OAuth token injected; merge permission granted by GitHub |
| **Unknown** | Capability not checked, not exposed by this system, or requires runtime context we lack | Pending verification; system has no verifier; stale data | Merge permission not yet verified in this run |
| **Unavailable** | Agent has no configuration for this capability; it was not set up or was revoked | Configuration absence; revoked permissions | Agent has no GitHub token; no AWS credentials |

### **Trust Boundaries: What Counts as "Verified"**

✅ **Trusted verification sources:**
- Runtime container / host capability probe (filesystem, network, process environment)
- Server-side credential broker injection record (we injected the token into the runtime)
- Provider token introspection or successful scope verification (GitHub API confirms token has merge permission)
- Server policy registry (backend policy explicitly grants the action)
- Known integration installation state (backend knows this integration is configured)

❌ **Not trusted as verified:**
- Agent self-report ("I think I have this capability")
- Prompt text or agent-supplied metadata
- Frontend-only inference ("has GITHUB_TOKEN env var therefore has all GitHub capabilities")
- Naming conventions alone (env var name `AWS_ACCESS_KEY` ≠ verified AWS access)

**The backend owns verification status.** Frontend must render it, not compute it.

### **Freshness Expectations**

- **Snapshot generation:** Matrix is generated at run start time; captured as `generated_at_utc` in response
- **TTL / Expiration:** Each capability includes `expires_at` timestamp indicating when re-verification is recommended
  - Credential scope checks: ~30 minutes (provider state may change)
  - Runtime probes: null (stable for run lifetime unless explicitly revoked)
  - Policy grants: null (static for run lifetime)
- **Stale handling:** Clients should avoid acting on expired assertions without refresh
- **No real-time revocation:** Matrix reflects state at run start, not live credential revocation; real-time revocation handling is out of scope

---

## Delivery Surfaces

### **Primary Surface: Authenticated REST API**

**Endpoints:**
- `GET /api/v1/agents/capability-matrix` — Fetch capability matrix for current run
- `GET /api/v1/agents/capability-matrix/contract` — Fetch explicit status semantics / enum documentation

**Authorization:** Bearer token (existing auth middleware)  
**Consumers:**
- Backend services performing action execution (preflight checks)
- Agents querying their own capabilities for decision-making
- QA automation validating run context
- External tooling / orchestration requiring deterministic capability lookup

**Use:** When you need machine-readable, deterministic capability data for automation, policy enforcement, or integration

### **Secondary Surface: Dashboard Panel**

**Component:** "Current Run Capability Matrix" panel in the run details view  
**Rendering:** Projection of API response with human-friendly formatting
- Agent identity and run ID
- Summary badges: # allowed / denied / unknown
- Searchable table of capabilities
- Separate sections for credentials and environment capabilities
- Verification status badges: `Verified`, `Unknown`, `Unavailable`
- Reason text mapped from reason_codes
- Last checked timestamp for each capability

**Consumers:**
- Humans debugging run context
- On-call engineers performing RCA
- Audit reviewers validating agent permissions
- Platform teams verifying credential rotation

**Use:** When you need a human-friendly inspection surface to understand what an agent can/cannot do in the current run

### **Prioritization**

**Both surfaces are required** for full success:
1. API is the **source of truth** — deterministic, machine-readable, auditable
2. Dashboard is the **operational visibility surface** — fast human answer during incidents

Neither alone is sufficient:
- API alone: planning/automation teams have answers, but incident response and audit require log data
- Dashboard alone: operational visibility works, but lacks deterministic contract for automation

---

## Success Criteria

### **Functional Success**
- ✅ Planning workflows adopt capability matrix checks as standard practice before assigning work
- ✅ Audit documentation references actual matrices instead of assumptions
- ✅ Incident response resolves "could this agent do X?" questions in seconds (sub-30s dashboard load + discovery)
- ✅ Platform teams verify credential changes via matrix within 2 minutes of updates

### **Quality Success**
- ✅ Zero incidents from incorrect capability assumptions in first 30 days post-launch
- ✅ All capabilities are either explicitly verified or marked unknown (no false positives)
- ✅ API contract remains stable across runs with no unexpected field additions/removals
- ✅ Dashboard loading time <2s for typical agents (≤50 capabilities)

### **Security Success**
- ✅ No secret values (tokens, keys, passwords) exposed in API or UI
- ✅ No raw sensitive env var names leaked; only sanitized references
- ✅ All matrix queries logged for compliance
- ✅ Access restricted to authenticated users authorized for the run

---

## Non-Goals & Out of Scope

### **Explicitly NOT Included**

1. **Policy Approval Workflows**  
   The matrix shows what an agent *can* do, not what it's *approved* to do. Policy enforcement and approval gatekeeping are separate systems.

2. **Credential Rotation Orchestration**  
   The matrix reflects state at run start. Automatic credential rotation, re-injection, or live revocation handling is out of scope.

3. **Granular Action-Level Constraints**  
   Matrix shows "can merge PRs: yes/no" not "can merge PRs only in this repo, during office hours, with CODEOWNERS approval." Constraint synthesis is a future phase.

4. **Cost, Quota, and Rate Limit Management**  
   Matrix exposes capability presence, not quota exhaustion, rate limits, or usage costs.

5. **Real-Time Credential Revocation**  
   If a credential is revoked at an external provider (GitHub revokes an OAuth token) after run start, the matrix will not reflect that revocation in real-time. Detect-and-refresh is manual for now.

6. **Capability Requirement Templates**  
   "Here's a template of capabilities needed for a common agent role" — future enhancement, not in MVP.

7. **Capability Change Audit Logs**  
   "This capability changed from X to Y between runs" — out of scope; we store snapshots, not change history.

---

## Implementation Dependencies

### **Backend Implementation**
- ✅ **Completed (SO-83):**
  - Authenticated endpoints for capability matrix data
  - Verification metadata on run, agent, credential, and capability level
  - Sanitization rules preventing secret value exposure
  - Integration with credential broker, policy registry, and runtime probers
  - Contract semantics endpoint documenting status enums

- **Remaining:** None (backend delivery complete and QA-validated)

### **Frontend Implementation**
- ✅ **Completed (SO-91):**
  - Dashboard panel rendering capability matrix
  - Verification badges and status indicators
  - Searchable capability table
  - Separate credential and environment sections
  - Loading and error state handling

- **Remaining:** None (frontend delivery complete and CEO-reviewed)

### **QA & Validation**
- ✅ **Completed (SO-83 / SO-91 review gates):**
  - Contract tests for enums, required fields, and absent-secret guarantees
  - Scenario tests for allowed/denied/unknown transitions
  - Security tests confirming no secret material leaks
  - UI tests ensuring unknown/unavailable states do not render as allowed

- **Remaining:** None (delivery is QA-validated)

### **Documentation**
- 📋 **Recommended:** Publish capability key catalog and meanings
- 📋 **Recommended:** Document verification levels and staleness interpretation
- 📋 **Recommended:** Add operator guidance for adding new capability verifiers

These are post-launch recommendations, not blockers for SO-60 closure.

---

## Risks & Mitigation

### **Risk: Users Misinterpret "Verified" as "Policy-Approved"**
**Exposure:** High  
**Impact:** Agent overreaches permissions, policy violation  
**Mitigation:**
- Dashboard includes explanation text: "Verified means the agent can do this; it does not mean they are approved to by policy"
- API documentation is explicit
- Audit training clarifies the distinction

### **Risk: Stale Verification Used for Sensitive Actions**
**Exposure:** Medium  
**Impact:** Agent attempts action with revoked credential at provider  
**Mitigation:**
- Each capability includes `expires_at`; clients must check staleness
- API response includes checked_at + expires_at; clients implement refresh
- Action execution logs which matrix version was used for auditability

### **Risk: Secret Leakage via Reason Codes or Details**
**Exposure:** Low (mitigated by code review)  
**Impact:** Credentials exposed in plaintext in API/UI  
**Mitigation:**
- Reason codes are machine-readable enums (cannot leak raw data)
- Details field sanitized; no raw secret names or values
- Security tests in QA gate verify this contract

### **Risk: Capability List is Incomplete**
**Exposure:** Medium  
**Impact:** Agent assumes "unknown" means "denied" and fails to attempt action it could perform  
**Mitigation:**
- Explicit distinction between unknown/unavailable/denied in UI and API
- Documentation clarifies "unknown" = "system does not expose this capability type"
- Capability key catalog published upfront with scope

---

## Metrics & Monitoring

### **Adoption Metrics**
- % of teams referencing matrix in planning docs (target: 80% by week 4)
- % of RCA tickets mentioning matrix discovery (target: 60% by week 2)
- API call volume / unique agents per day (baseline for growth)

### **Quality Metrics**
- Incidents caused by capability assumption errors (target: 0)
- API response time (p95 <500ms)
- Dashboard load time (p95 <2s)
- False positive / false negative capability assertions (target: 0)

### **Security Metrics**
- Secret exposure incidents (target: 0)
- Unauthorized API access attempts (monitor and log)
- Audit coverage: % of capability-based actions with logged matrix assertion

---

## Future Enhancements (Not in Scope for SO-60)

1. **Capability requirement templates** for common agent archetypes
2. **Capability change tracking** across run boundaries
3. **Approval integration** for high-risk capabilities
4. **Credential rotation workflows** with automated matrix refresh
5. **Cross-environment comparison** ("does agent have same capabilities in staging vs prod?")
6. **Expiration warnings** for credentials approaching expiry
7. **Integration with action-level policy enforcement** (not just capability presence)

---

## Acceptance Criteria — VERIFIED ✅

- ✅ **User-facing requirements and primary use cases documented**  
  — Four primary user personas with specific use cases defined above (planning teams, audit, incident response, platform)

- ✅ **Required matrix fields and freshness/verification expectations defined**  
  — Credentials, external actions, environment capabilities with verification semantics (verified/unknown/unavailable)
  — Freshness expectations: snapshot at run start, expires_at per capability, TTL semantics

- ✅ **Recommended delivery surface(s) and prioritization specified**  
  — API (source of truth) + Dashboard (human visibility)
  — Both required; neither alone sufficient
  — API for deterministic automation; Dashboard for operational inspection

- ✅ **Non-goals, risks, and dependencies documented**  
  — Non-goals: policy approval, credential rotation, granular constraints, cost/quota, real-time revocation, templates, audit logs
  — Risks: misinterpretation of "verified," stale verification, secret leakage, incomplete capability list
  — Dependencies: all implemented (SO-83 backend, SO-91 frontend)

- ✅ **Success criteria defined**  
  — Functional: adoption in planning, audit, incident response workflows
  — Quality: no incidents, all capabilities verified or unknown, stable API contract, fast dashboard
  — Security: no secret exposure, access restricted, queries logged

---

## Related Documentation

- **Architecture:** [SO-69 — Verified agent capability/environment matrix](../architecture/so-69-verified-agent-capability-matrix.md)
- **Backend Tech Spec:** [SO-83 — Verified agent capability and environment matrix API](../tech-specs/SO-83-verified-agent-capability-and-environment-matrix-api.md)
- **Wiki Product Spec:** [Product Specification: Verified Agent Capability and Environment Matrix](https://wiki/product-specification-verified-agent-capability-and-environment-matrix)
- **Backend Implementation:** Completed under SO-83 (GitHub PR #31)
- **Frontend Implementation:** Completed under SO-91 (GitHub PR #34)
- **Closure Decision:** [SO-95 dashboard scope closure recommendation for SO-60](https://wiki/decision-so-95-dashboard-scope-closure-recommendation-for-so-60)

