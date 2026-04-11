# SO-104: Verified capability and environment matrix backend support

## Overview

SO-104 closes backend exposure for the verified agent capability and environment matrix in the current run context by ensuring authenticated API routes are wired and by relying on the existing verified response contract and test suite.

## API surface

- `GET /api/v1/agents/capability-matrix`
- `GET /api/v1/agents/capability-matrix/contract`

Both endpoints are bearer-auth protected via `api.Auth(...)` in `cmd/secondorder/main.go`.

## Data and verification semantics

The matrix response uses snapshot semantics at request time:

- `run.generated_at_utc` and `run.verification.checked_at` are set to the current UTC timestamp when the response is generated.
- `run.running_runs_count` and `run.instance_name` are read from current DB state.
- Agent rows include:
  - identity (`agent_id`, `agent_slug`, `agent_name`, `archetype_slug`, `runner`)
  - `capabilities[]`
  - `environment_capabilities[]`
  - `credentials[]`

Each capability/credential includes verification provenance:

- `level` (`verified`, `unknown`, `unavailable`)
- `method`
- `source`
- `checked_at`
- `expires_at` (reserved for future freshness windows)

## Automated coverage

`internal/handlers/capability_matrix_test.go` covers:

- authenticated happy-path verified combinations
- unknown/unavailable edge cases for credentials and environment probes
- contract endpoint semantics
- auth requirements for both endpoints
- secret leak prevention assertions

## Limitations and operational considerations

- `merge_pull_request` remains `unknown` until a merge policy registry is implemented.
- Credential checks verify env-var presence, not external provider validity.
- Workspace capability depends on filesystem visibility from the API process.
- Freshness is request-time snapshot based; no cache TTL is currently applied.
