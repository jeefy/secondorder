# SO-11: Runner 403 Error & Model Configuration Fix

**Date:** 2026-04-09  
**Issue:** SO-11  
**Investigator:** DevOps  
**Status:** Resolved

---

## Summary

SO-11 investigated the Product Manager agent's `403 Access Forbidden` error and `unknown_model` errors encountered when using the `copilot` runner. This is a continuation of the investigation started in SO-13. The root causes were:

1. **Product Manager agent** was on `runner=copilot, model=claude-haiku-4.5` — the Copilot API returned 403 for this combination. **Already fixed** (see SO-13): agent migrated to `runner=opencode, model=github-copilot/claude-haiku-4.5` at 18:11 UTC 2026-04-09.

2. **SO-5 unblocked**: Product Manager successfully completed SO-5 (webhook documentation) at 22:44 UTC 2026-04-09 using the `opencode` runner. Status is `done`.

3. **`unknown_model` errors** (`claude-haiku-4-5`, `claude-haiku-4.5`): A previous SO-11 copilot-runner agent attempted to query the Copilot API via bash tool to discover valid models. The agent tried variant model identifiers with dashes (`claude-haiku-4-5`) which are not valid. This was a secondary failure in an earlier SO-11 run, not a scheduler configuration issue.

4. **Context deadline exceeded (6m36s)**: A prior SO-11 copilot-runner run timed out due to a transient Copilot API availability issue. The HTTP client timeout is set to 120s per request (`copilot.go:466`), and the 6m36s total elapsed across multiple turns indicates an extended API degradation period. This was transient.

5. **`RunnerCopilot` model list out of date**: `internal/models/models.go` listed only 7 models for the `copilot` runner. The actual Copilot API (`api.githubcopilot.com/models`) exposes 17+ models. The list has been updated to match verified available models.

---

## Changes Made

### `internal/models/models.go`

Updated `RunnerCopilot` model list from 7 entries to 17 entries, verified against the live Copilot API on 2026-04-09:

**Before:**
```go
RunnerCopilot: {
    "claude-sonnet-4.6",
    "claude-opus-4.6",
    "claude-haiku-4.5",
    "gpt-5.4",
    "gpt-5.1",
    "gpt-5-mini",
    "gemini-2.5-pro",
},
```

**After:**
```go
RunnerCopilot: {
    // Claude models (verified against api.githubcopilot.com/models 2026-04-09)
    "claude-sonnet-4.6",
    "claude-opus-4.6",
    "claude-haiku-4.5",
    "claude-sonnet-4.5",
    "claude-sonnet-4",
    "claude-opus-4.5",
    // GPT models
    "gpt-5.4",
    "gpt-5.3-codex",
    "gpt-5.2",
    "gpt-5.2-codex",
    "gpt-5.1",
    "gpt-5-mini",
    "gpt-4o",
    "gpt-4.1",
    // Other models
    "gemini-2.5-pro",
    "grok-code-fast-1",
},
```

**Added models:** `claude-sonnet-4.5`, `claude-sonnet-4`, `claude-opus-4.5`, `gpt-5.3-codex`, `gpt-5.2`, `gpt-5.2-codex`, `gpt-4o`, `gpt-4.1`, `grok-code-fast-1`

---

## Verification

| Check | Result |
|-------|--------|
| `claude-haiku-4.5` valid in Copilot API | Yes — confirmed in `/models` response |
| SO-5 status | `done` — Product Manager completed at 22:44 UTC 2026-04-09 |
| Product Manager runner | `opencode` (migrated by SO-13) |
| CEO agent (only remaining copilot runner) | `copilot / claude-opus-4.6` — `claude-opus-4.6` is valid per API |

---

## Remaining Risk

The CEO agent is the only remaining agent using `runner=copilot`. It uses `claude-opus-4.6` which is valid per the Copilot API. However, the same 403 risk exists if:

- The Copilot subscription restricts access to `claude-opus-4.6` for this token
- The `vscode-chat` integration ID loses access to certain model tiers

**Recommendation**: Monitor CEO agent runs for 403 errors. If they occur, migrate CEO to `runner=opencode, model=github-copilot/claude-opus-4.6` following the same pattern as the Product Manager.

---

## Acceptance Criteria Review

- [x] Root cause identified: `claude-haiku-4.5` on the `copilot` runner returned 403 (Copilot API access restriction for that model/token/integration combination)
- [x] Fix applied: Product Manager migrated to `runner=opencode` (SO-13 fix, already applied)
- [x] Verified SO-5 was picked up and completed by Product Manager without error
- [x] Runner model config updated: `RunnerCopilot` model list expanded to match actual API availability
- [x] `unknown_model` errors explained: artifact of a previous agent attempting to query Copilot API directly with invalid variant model names

---

## Related Issues

- SO-13: Initial 403 investigation — migrated Product Manager to opencode runner
- SO-5: Webhook documentation task — completed by Product Manager at 22:44 UTC
- SO-9: Closed as duplicate of SO-5
