# Verified Agent Capability and Environment Matrix

**Status**: Product specification for issue SO-60 (Feature: Expose verified agent capability and environment matrix)  
**Last Updated**: 2026-04-11  
**Assigned to**: Product Manager (SO-68)

---

## Executive Summary

The verified agent capability and environment matrix addresses a critical gap in planning and audit workflows: teams currently make assumptions about agent permissions and capabilities based on guesswork rather than verified data.

This feature provides a **source-of-truth view of each agent's capabilities in the current run context**, including:
- Verified credentials and integrations the agent can access
- Allowed external actions (e.g., PR merges, archetype patches, CI/CD operations)
- Key environment capabilities and constraints
- Clear status indicators (verified, unavailable, unknown) for each capability

The capability matrix will be exposed through both API and dashboard surfaces, supporting planning, audit documentation, and operational governance workflows.

---

## Problem Statement

### The Challenge
When executing autonomous agents in complex environments, planners, auditors, and operations teams need to understand:
- What credentials is each agent configured with?
- Can this agent merge pull requests?
- Does this agent have production access?
- What integrations can it reach?

**Current state**: These questions are answered by reading documentation, checking Slack threads, or making educated guesses. Documentation frequently becomes stale or incomplete.

**Risk**: Planning and audit documentation relies on implicit assumptions that may be incorrect, leading to:
- Misunderstandings about actual agent permissions
- Inconsistent documentation across runbooks
- Audit gaps when permissions change without documentation updates
- Time spent reconciling agent capabilities during incident response

---

## Solution

### Core Approach
Implement a **verified agent capability and environment matrix** that:

1. **Exposes verified data** — Display each agent's actual credentials, permissions, and capabilities for the current execution context
2. **Distinguishes data quality** — Clearly indicate which capabilities are verified (confirmed via configuration/runtime), unavailable (agent not configured), or unknown (not exposed by this version)
3. **Provides multiple surfaces** — Make the data available via both API (for programmatic use) and dashboard (for human inspection)
4. **Includes source metadata** — Specify how each capability was verified (e.g., from IAM policy, runtime check, explicit config)
5. **Reflects current context** — Show capabilities as they exist for the current run, not stored static data

### What Gets Exposed

#### Credentials & Integrations
- Cloud provider credentials (AWS, Azure, GCP account/permissions)
- Version control system integrations (GitHub token scope, GitLab permissions)
- Third-party API keys (Slack, PagerDuty, monitoring platforms)
- Deployment platform access (Kubernetes contexts, container registries)

#### External Actions
- PR merge/review authority (what repository scopes can this agent affect?)
- Archetype patch submission rights
- CI/CD pipeline manipulation (can trigger builds, modify workflows?)
- Production environment access (deploy, scale, modify configuration?)

#### Environment Capabilities
- Compute resources available (CPU, memory constraints)
- Network access (internal services, external APIs, firewalls)
- Persistent storage access (databases, object storage, caches)
- Audit and logging access (can this agent view/search logs?)

---

## Intended Users and Workflows

### 1. Planning and Architecture Reviews
**Who**: Engineering leads, architects, product managers planning feature rollouts  
**When**: Before assigning a new task to an agent or delegation  
**Workflow**:
1. Reviewer checks the capability matrix for the assigned agent
2. Confirms agent has necessary credentials (e.g., staging AWS account access)
3. Documents actual permissions in the plan/RFC (not guessed)
4. Mentions any unavailable capabilities and their impact

**Example**: "Agent can access staging AWS, but not production (unavailable). Deployment validation limited to staging metrics only."

### 2. Audit and Compliance Documentation
**Who**: Security, compliance, audit teams  
**When**: During quarterly reviews or incident investigations  
**Workflow**:
1. Pull capability matrix for all active agents
2. Compare against expected/approved permission set
3. Flag discrepancies (extra capabilities, missing expected perms)
4. Document actual vs. intended access for audit trail

**Example**: "Agent X has GitHub token scope but we approved read-only access; investigate if scope was expanded."

### 3. Incident Response and Root Cause Analysis
**Who**: On-call engineers, SREs, incident commanders  
**When**: During or after a security or operational incident  
**Workflow**:
1. Quickly check if the implicated agent had capability to perform the observed action
2. Verify whether agent could have accessed affected systems
3. Simplify "could this agent do X?" discussions

**Example**: "Agent Y didn't have production Kubernetes access (unavailable) — it could not have modified the cluster. Focus investigation on other vectors."

### 4. Permission Change Management
**Who**: Platform engineers, security operations  
**When**: After adding/revoking agent credentials  
**Workflow**:
1. Make the permission change
2. Check the capability matrix to confirm change is reflected
3. Share updated matrix with relevant teams
4. Close the change ticket once verified

**Example**: "Revoked Agent Z's production AWS access. Capability matrix now shows 'unavailable' for prod AWS. All internal docs updated."

---

## Data Interpretation Guide

### Capability States

Each capability in the matrix will have a **verification status**:

| Status | Meaning | Example |
|--------|---------|---------|
| **Verified** | Configuration or runtime check confirms the agent has this capability. Credentials are valid and scoped appropriately. | "GitHub token verified; scopes: repo:write, workflow:write (org repo X only)" |
| **Unavailable** | Agent has no configuration for this capability. It was explicitly not set up or has been revoked. | "Production AWS: unavailable" |
| **Unknown** | This version of the system does not expose this capability type or the verification check is not implemented. Do not assume the agent lacks the capability. | "Slack integration: unknown (check configuration manually)" |

### Trust Boundaries and Verification Metadata

Each capability entry includes **source metadata** to explain how the verification was performed:

- **Configuration-based**: Verified by reading the agent's stored configuration (e.g., "AWS role: `agent-staging-deployer`")
- **Runtime-check**: Verified by executing a capability test at runtime (e.g., "GitHub API call succeeded with token scopes: X, Y, Z")
- **Inferred**: Derived from other verified data (e.g., "Can submit PRs because: GitHub repo:write scope verified AND target repo exists")
- **Policy-based**: Derived from organizational policy or platform rules (e.g., "Cannot access production because account is tagged as staging-only")

### What "Verified" Does NOT Mean

⚠️ **Important**: "Verified" means the capability is configured and current. It does **not** mean:
- The agent should use that capability (policy questions are separate)
- The credential is still valid (external services revoke access unpredictably)
- There are no network/firewall restrictions in place
- The agent has approval to perform actions with that capability

### Stale or Missing Credentials
If a credential was valid at matrix generation time but the external service revoked it in the meantime (e.g., GitHub token disabled), the matrix will not automatically reflect the revocation. Treat verified credentials as "last known good as of run start time."

---

## User Workflows and Access Patterns

### API Usage (Programmatic)

**Endpoint**: `GET /api/v1/agents/{agent_id}/capabilities` (or similar, per architecture)

**Expected usage**:
- Pre-execution checks: "Can this agent perform the requested operation?"
- Compliance automation: Script periodic matrix exports for audit trails
- Dashboards and monitoring: Embed capability status in agent execution dashboards

**Response format** (example):
```json
{
  "agent_id": "agent-xyz",
  "agent_name": "Deploy Agent",
  "run_context": "production-us-east",
  "generated_at": "2026-04-11T14:30:00Z",
  "capabilities": {
    "credentials": [
      {
        "type": "aws",
        "name": "production-deployer",
        "status": "verified",
        "scope": "us-east regions, EC2, RDS, S3 buckets tagged prod:*",
        "source": "configuration",
        "expires_at": "2026-12-31T23:59:59Z"
      }
    ],
    "external_actions": [
      {
        "action": "merge_pull_request",
        "status": "verified",
        "scope": "org:acme, repos: backend/*, frontend/*",
        "source": "runtime_check",
        "note": "GitHub token verified with repo:write scope"
      }
    ],
    "environment": [
      {
        "name": "production_compute",
        "status": "verified",
        "scope": "Kubernetes cluster: prod-us-east (5 namespaces)",
        "source": "policy_based",
        "note": "Agent role has cluster-admin for specified namespaces only"
      }
    ]
  },
  "limitations": [
    "staging_aws: unavailable — agent not configured for staging",
    "production_approval: unknown — approval chain not exposed in this version"
  ]
}
```

### Dashboard Usage (Human Inspection)

**Location**: Agent detail page in operation dashboard  
**View**: Capability matrix card or expandable section

**Expected UX**:
- Color-coded status badges (green = verified, gray = unavailable, yellow = unknown)
- Capability grouped by category (credentials, actions, environment)
- Scope/constraints displayed inline
- Source metadata available in tooltips or expandable details
- Links to documentation or audit logs where relevant

**Typical use**: Planner opens agent detail, scans the capability matrix in 10 seconds, confirms "yes, this agent can access staging AWS" before writing the plan.

---

## Scope and Limitations

### What This Feature Addresses
✅ Reduces guesswork about agent permissions and credentials  
✅ Creates a durable, auditable source of truth for the current run  
✅ Supports planning, audit, and incident response workflows  
✅ Provides clear status indicators (verified/unavailable/unknown)  

### What This Feature Does NOT Address
❌ Approval workflows or policy enforcement ("Should the agent use this capability?" is a separate governance question)  
❌ External service outages (e.g., if GitHub is down, the matrix can't verify GitHub scope)  
❌ Real-time credential revocations by external services (matrix reflects state at run start)  
❌ Granular action-level constraints (e.g., "can merge PRs but not delete repos") — captured at capability level  
❌ Cost or quota management (e.g., "API rate limits exceeded")  

### Future Enhancements (Not in Scope)

1. **Capability requirement templates**: Pre-defined capability sets for common agent roles (e.g., "Deployer", "Code Review", "Data Export")
2. **Capability change tracking**: Audit log of when capabilities were added/removed and by whom
3. **Approval integration**: Require explicit approval before agents exercise high-risk capabilities
4. **Credential rotation workflows**: Automated rotation with capability matrix updates
5. **Predictive warnings**: Flag when an agent is approaching credential expiration or quota limits
6. **Cross-environment capability comparison**: Compare agent capabilities across dev/staging/prod
7. **Capability-to-workflow mapping**: Link capabilities to specific agent tasks and workflows

---

## Security and Privacy Considerations

### Data Sensitivity
The capability matrix contains sensitive information:
- **Credential scope and type** (indicates what systems the agent can access)
- **Audit log access** (indicates observability into agent actions)
- **External service integrations** (indicates external dependencies)

### Access Control
- The API endpoint **MUST** be protected by the same access controls that govern the agents themselves
- Users who can see the agent's execution context should see its capability matrix
- Matrix should not expose secrets (credential values, API key strings) — only scope and verification status
- Dashboard view should respect the same RBAC as the agent detail page

### Audit Trail
- Log all matrix queries (who requested, when, for which agent)
- Tie matrix changes to permission change tickets
- Include matrix generation timestamp in every export for compliance

---

## Implementation Notes for Architecture and Engineering Teams

### Backend Considerations
- Matrix generation should reflect the **exact state at run start**, not drift over time
- Include source metadata so downstream teams can understand where each capability comes from
- Cache wisely: refresh on permission changes, not on every request
- Return graceful "unavailable/unknown" states rather than failing the entire matrix if one capability check fails

### Frontend Considerations
- Status badges should use consistent color/icon vocabulary across the app
- Scope information may be long; use abbreviations with expandable tooltips
- Link capability entries to configuration pages or documentation where relevant
- Provide export functionality (CSV, JSON) for audit documentation

### QA and Validation
- Test matrix accuracy across representative agent types (deployer, reviewer, data export)
- Validate matrix reflects permission additions and revocations within expected latency
- Test edge cases: missing credentials, expired tokens, network failures during capability checks
- Confirm API and dashboard show identical data

---

## Documentation and Runbook Updates

This feature requires updates to the following internal documentation:

1. **Agent Configuration Guide**: Add section on how to set up and verify agent credentials
2. **Planning and Architecture Template**: Include capability matrix checklist for architectural reviews
3. **Audit Runbook**: Document how to extract and review agent capability matrices
4. **Incident Response Playbook**: Add "verify agent capability" as a quick diagnostic step
5. **Permission Change Runbook**: Include matrix verification as part of change validation

---

## Success Criteria

This feature will be considered successful when:

1. **Planning workflows** adopt capability matrix checks as standard practice (audit through RFC comments)
2. **Audit documentation** references actual capability matrices instead of assumptions
3. **Incident response** resolves "could this agent have done X?" questions in seconds instead of minutes
4. **Permission changes** are validated against the matrix within 1 business day
5. **No planning surprises**: Zero incidents attributable to incorrect agent capability assumptions in the first 30 days post-launch

---

## Related Documentation

- **Strategic Decision**: See wiki page "Decision: verified agent capability and environment matrix"
- **Architecture & Design**: Output from issue SO-69 (Design verified agent capability/environment matrix experience and data contract)
- **Implementation Details**: Issues SO-70 (Backend), SO-71 (Frontend), SO-67 (QA)

---

## Document Revision History

| Date | Author | Change |
|------|--------|--------|
| 2026-04-11 | Product Manager | Initial product specification; ready for architecture review and implementation |

