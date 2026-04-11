# SO-73 Frontend Delivery Status: Verified Capability and Environment Matrix

## Delivered UI behavior

- Dashboard renders a **Verified Capability Matrix** panel sourced from backend capability semantics for the current run context.
- Matrix rows are grouped per agent with representative categories:
  - Credentials / integrations
  - Allowed external actions
  - Environment capabilities
- Status states are explicitly distinguished and labeled in-panel:
  - `verified`
  - `unknown`
  - `unavailable`
- Status labels now include the contract semantics text so users can understand each state without referring to API docs.

## Safe handling requirements

- No secret values are rendered; credentials are represented with sanitized refs (`cred:<agent-slug>:primary_api_key`).
- Unknown/unavailable states are rendered with explicit reason codes and muted metadata for safe interpretation.

## Empty/loading/error handling

- Empty state: "No agents available for this run context."
- Error state: clear inline error card when matrix data cannot be loaded.
- Loading/refresh state: HTMX indicator shows `Refreshing` while matrix is reloaded on run/agent events.

## Implementation references

- `internal/templates/dashboard_capability_matrix.html`
- `internal/handlers/ui.go`
- `internal/handlers/capability_matrix.go`

## QA evidence checklist

1. Open `/dashboard` and confirm the **Verified Capability Matrix** section is present.
2. Verify each legend card shows both state name and explanatory contract text.
3. Confirm at least one row displays all three categories.
4. Trigger run/agent updates and confirm `Refreshing` indicator appears during HTMX reload.
5. Validate empty/error behavior by testing with no agents and with a forced data-load failure.
