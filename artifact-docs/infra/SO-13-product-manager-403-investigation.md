# SO-13: Product Manager Agent 403 Error Investigation

**Date:** 2026-04-09  
**Issue:** SO-13  
**Investigator:** DevOps  
**Status:** Resolved (runner migrated to `opencode`)

---

## Summary

The Product Manager agent encountered a `403 Access Forbidden` error from the GitHub Copilot API (`https://api.githubcopilot.com/chat/completions`) while executing issue SO-9. The error was **not** caused by the agent attempting to access the GitHub REST API to perform business logic — it was caused by the Copilot API endpoint itself returning a 403 to the secondorder scheduler's `copilot` runner.

The root cause was that the Product Manager agent was configured with `runner=copilot` using model `claude-haiku-4.5`, and the GitHub Copilot API rejected that specific model identifier with a 403 (Terms of Service restriction). The workaround — migrating the agent to `runner=opencode` — was already applied at 18:11 UTC on 2026-04-09, and subsequent runs succeeded.

---

## Root Cause

### What happened

At 17:17:28 UTC, the secondorder scheduler spawned the Product Manager agent to handle issue SO-9 (webhook documentation task). The scheduler used the `copilot` runner, which calls:

```
POST https://api.githubcopilot.com/chat/completions
Authorization: Bearer <gh auth token>
```

The Copilot API returned HTTP 403 with body:
```
Access to this endpoint is forbidden. Please review our Terms of Service.
```

The scheduler logged this as:
```
ERR scheduler: agent failed agent="Product Manager" run_id=76fccad7... runner=copilot model=claude-haiku-4.5 issue_key=SO-9 elapsed=3s error="API error 403: ..."
```

### Why the 403 occurred

The Copilot runner (`internal/scheduler/copilot.go`) calls the GitHub Copilot Chat Completions API with model `claude-haiku-4.5`. At the time of the failure, the Copilot API rejected this model with a 403. This is a **Copilot API access control restriction**, not a GitHub REST API call by the agent.

Evidence that this was not a GitHub REST API call:
- The agent's task was purely documentation (read local files, write markdown)
- The 403 came from `https://api.githubcopilot.com/chat/completions` — the LLM inference endpoint, not the GitHub REST API
- The agent had completed 2 bash tool calls (secondorder API calls) before the 403 hit, confirming it was the LLM API returning 403, not an unauthorized tool call

Also observed in activity log at 17:44 UTC, a similar error for SO-11 (different agent):
```
runner=copilot model=claude-sonnet-4.6 issue_key=SO-11 elapsed=6m36s error="API request: ...context deadline exceeded"
[tool:bash] {"error":{"code":"unknown_model","message":"Unknown model: claude-haiku-4.5",...}}
```
This confirms the Copilot API was rejecting or timing out on certain model identifiers.

---

## Timeline

| Time (UTC) | Event |
|-----------|-------|
| 17:17:25 | Product Manager (runner=copilot, model=claude-haiku-4.5) spawned for SO-9 |
| 17:17:28 | Run fails: `API error 403: Access to this endpoint is forbidden` |
| 17:28:40 | Error captured in CEO backlog |
| 17:30:03 | SO-11 created: "Investigate and fix runner 403 error for Product Manager agent" |
| 17:30:18 | SO-13 created: "Investigate Product Manager agent 403 errors when accessing GitHub API" |
| 18:11:02 | Last failed run with old config |
| 18:11:47 | Product Manager agent config updated: `runner=opencode`, `model=github-copilot/claude-haiku-4.5` |
| 22:42:28 | Product Manager successfully completes SO-5 using `runner=opencode` |

---

## Affected Agents

| Agent | Runner at incident | Impact |
|-------|-------------------|--------|
| Product Manager | copilot / claude-haiku-4.5 | Direct — 5 failed runs across SO-5 and SO-9 |
| CEO | copilot / claude-opus-4.6 | Indirect — CEO run on SO-9 completed (different model, not blocked) |
| Other agents | opencode, gemini, codex | Not affected — different runner paths |

Only the `copilot` runner is susceptible. Agents using `opencode`, `gemini`, or `codex` runners are unaffected.

---

## How the copilot Runner Works

`internal/scheduler/copilot.go` implements a custom LLM agentic loop that:

1. Retrieves a GitHub token via `gh auth token`
2. Posts to `https://api.githubcopilot.com/chat/completions` directly
3. Executes tool calls (bash, read_file, write_file, list_dir, supermemory)
4. Loops until `finish_reason != tool_calls` or `maxTurns` reached

The request includes headers:
```
Authorization: Bearer <gh-token>
Copilot-Integration-Id: vscode-chat
Editor-Version: vscode/1.99.0
```

The 403 response from the Copilot API indicates the specific `(token, model, integration-id)` combination was forbidden by GitHub. This can occur when:
- The GitHub token's Copilot subscription does not include access to the requested model tier
- GitHub's Copilot API rate-limits or restricts certain `Integration-Id` + model combinations
- The model name used (`claude-haiku-4.5`) was not in the Copilot API's accepted model list at that time

---

## Resolution

The Product Manager agent's runner was migrated from `copilot` to `opencode` with model `github-copilot/claude-haiku-4.5`. The `opencode` runner invokes the `opencode` CLI in pure mode:

```
opencode --pure run "<prompt>" --format json --dangerously-skip-permissions -m github-copilot/claude-haiku-4.5
```

The `opencode` CLI manages its own Copilot API authentication and model resolution, bypassing the direct `api.githubcopilot.com` call in the scheduler. This resolved the 403 issue — the 22:42 run of SO-5 completed successfully.

---

## Impact on Active Tasks

- **SO-5** (webhook documentation): **Not blocked.** Product Manager successfully completed SO-5 at 22:44 UTC using the `opencode` runner.
- **SO-9**: Closed as duplicate of SO-5 before the runner change. No action needed.

---

## Recommendations

1. **Monitor copilot runner for 403s.** The `copilot` runner remains configured for the CEO agent (`runner=copilot, model=claude-opus-4.6`). If the CEO hits the same 403, consider migrating it to `runner=opencode` as well.

2. **Log runner type in run records.** The `runs` table does not record `runner`/`model` at execution time — it only stores `agent_id`. The current join gives the *current* agent config, not the config at run time. This makes post-hoc diagnosis harder. Consider adding `runner` and `model` columns to `runs`.

3. **Improve copilot runner error handling.** A 403 from the Copilot API should trigger a retry with exponential backoff or an agent-level alert, rather than a hard failure. Currently the run just fails and the agent is not retried automatically.

4. **Token/model validation at spawn time.** Consider validating that the `(gh-token, model)` combination is accessible before spawning a full agent run. A lightweight `/models` or test call could detect 403s early.

---

## Files Referenced

- `internal/scheduler/copilot.go` — Copilot runner implementation (line 23: `copilotAPIURL`, line 479: 403 error path)
- `internal/scheduler/opencode.go` — OpenCode runner implementation
- `internal/scheduler/scheduler.go` — Runner dispatch (line 247: switch statement)
- `internal/models/models.go` — Runner constants and valid model lists
