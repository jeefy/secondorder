# SO-70 Capability Matrix API

## Endpoint

- `GET /api/v1/agents/capability-matrix`
- `GET /api/v1/agents/capability-matrix/contract`

## Purpose

Expose a backend-attested capability/environment matrix for the current run context so planning and audit flows do not rely on inferred permissions.

## Response shape

- `run`: current snapshot context (`generated_at_utc`, `instance_name`, running run count, verification metadata)
- `agents[]`: one row per agent
  - `capabilities[]`: external actions (`archetype_patch_submission`, `merge_pull_request`)
  - `environment_capabilities[]`: runtime access (`workspace_access`, `chrome_mcp_access`)
  - `credentials[]`: sanitized credential inventory with stable refs (`cred:<agent-slug>:primary_api_key`)

## Status semantics

- `verified`: backend-attested from current runtime/database state
- `unknown`: verification source exists but cannot attest value now
- `unavailable`: capability/credential is configured absent or inaccessible

## Verification/source metadata

Each capability/credential includes:

- `verification.level`
- `verification.method`
- `verification.source`
- `verification.checked_at`
- `verification.expires_at` (currently null)

## Operational assumptions

- Credential checks use env var presence only (`os.LookupEnv`); values are never returned.
- Working directory capability uses `os.Stat(agents.working_dir)` as a runtime filesystem probe.
- Merge rights are currently `unknown` because there is no provider-side merge permission registry in backend yet.
- API is auth-protected with existing bearer key middleware.

## QA notes

- Happy-path test validates verified states for credential/env/action fields.
- Missing/unknown test validates unknown credential and unavailable workspace/chrome states.
