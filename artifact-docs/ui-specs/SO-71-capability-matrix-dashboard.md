# SO-71 UI Spec: Verified Capability Matrix Dashboard Panel

## What shipped

- Added a dashboard panel that renders the verified agent capability/environment matrix for the current run context.
- Panel groups each agent into three categories:
  - Credentials / integrations
  - Allowed external actions
  - Environment capabilities
- Each item exposes status (`verified`, `unknown`, `unavailable`) plus source metadata and reason codes.

## UX behavior

- `Verified` status is shown in green.
- `Unknown` status is shown in amber.
- `Unavailable` status is shown in gray.
- Empty state: "No agents available for this run context."
- Error state: red error card if capability data cannot be loaded.
- Loading/refresh state: panel refreshes via HTMX on run/agent events and supports request indicators.

## Placement and routes

- Dashboard embeds the panel below existing dashboard sections.
- Added partial refresh endpoint:
  - `GET /dashboard/capability-matrix`

## Data contract alignment

- Dashboard rendering is mapped from the same capability matrix backend structures used by:
  - `GET /api/v1/agents/capability-matrix`
  - `GET /api/v1/agents/capability-matrix/contract`

## CEO walkthrough (no screenshot required)

1. Open `/dashboard`.
2. Scroll to **Verified Capability Matrix**.
3. Confirm each agent row displays credentials, external actions, and environment capabilities.
4. Verify status badges and source lines are visible for each capability.
5. Trigger a run or update an agent; confirm the panel auto-refreshes without page reload.
6. For state checks:
   - Set an agent `api_key_env` to an env var not present to see `unknown`.
   - Set an invalid `working_dir` to see `unavailable`.
   - Enable `chrome_enabled` and ensure `Chrome MCP access` shows `verified`.
