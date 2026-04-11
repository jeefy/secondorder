# SO-91 UI Spec: Verified capability matrix on dashboard and agent detail

## Scope delivered

- Dashboard now renders a **Verified Capability Matrix** panel backed by backend status payloads only.
- Agent detail now renders the same matrix panel filtered to the selected agent.
- UI surfaces representative capability categories required by product spec:
  - Credentials / integrations
  - Allowed external actions
  - Environment capabilities

## Data source and contract alignment

- Rendering data is sourced from backend capability matrix structures produced by:
  - `GET /api/v1/agents/capability-matrix`
  - `GET /api/v1/agents/capability-matrix/contract`
- Frontend does not infer status transitions; it displays backend-provided status values and metadata.

## UX behavior and states

- Status legend and row badges explicitly show:
  - `verified` (green)
  - `unknown` (amber)
  - `unavailable` (gray)
- Loading/refresh state is explicit with HTMX indicator (`Refreshing`) during partial updates.
- Empty state is explicit: `No agents available for this run context.`
- Error state is explicit: red error card when capability data cannot be loaded.

## Safety and secrecy controls

- No secret values or raw tokens are rendered.
- Credential references are shown as sanitized stable refs (for example `cred:<agent-slug>:primary_api_key`).
- Unsafe identifiers are not exposed in the matrix UI.

## Routing and refresh

- Added partial-render routes:
  - `GET /dashboard/capability-matrix`
  - `GET /agents/{slug}/capability-matrix`
- Matrix panel refreshes on run/agent events:
  - `run-started`
  - `run-complete`
  - `agent-changed`

## Files

- `internal/handlers/ui.go`
- `internal/templates/dashboard.html`
- `internal/templates/agent_detail.html`
- `internal/templates/dashboard_capability_matrix.html`
- `internal/templates/templates.go`
- `cmd/secondorder/main.go`
