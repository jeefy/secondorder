# Discord Webhook Notifications Specification

**Date:** April 9, 2026  
**Status:** Draft  
**Author:** Product Team  
**Related Issue:** SO-9

---

## 1. Overview

Discord webhook notifications provide real-time updates about system events directly to Discord channels. This spec defines which events trigger notifications, message formatting per event type, configuration options, and error handling behavior.

The implementation follows the existing Telegram integration pattern (`internal/telegram/bot.go`) as a reference for consistency, while leveraging the Discord webhook capabilities already partially implemented in `internal/discord/webhook.go`.

---

## 2. Triggerable Events

The following events generate Discord notifications:

### 2.1 Run Lifecycle Events

- **Run Started:** When an agent begins executing a task
  - Triggered: Immediately when `Run.Status` transitions to `running`
  - Data available: Agent name, issue key, run mode, start time
  
- **Run Completed:** When an agent successfully finishes executing a task
  - Triggered: When `Run.Status` transitions to `completed`
  - Data available: Agent name, issue key, total tokens used, cost, completion time, exit status
  
- **Run Failed:** When an agent execution fails
  - Triggered: When `Run.Status` transitions to `failed`
  - Data available: Agent name, issue key, error reason, partial output (if any)

### 2.2 Issue Status Changes

- **Status Change:** When an issue's status is updated
  - Triggered: When `Issue.Status` changes (e.g., todo → in_progress, in_progress → done)
  - Data available: Issue key, title, previous status, new status, updated timestamp
  - **Note:** Initial ticket creation does not trigger a notification (too noisy)

### 2.3 Comments

- **New Comment:** When a comment is added to an issue
  - Triggered: When a new `Comment` is created
  - Data available: Issue key, commenter (agent or user), comment body (first 1000 chars)
  - Filter: Comments from agents can be configured per-agent

### 2.4 Work Block Approvals

- **Approval Requested:** When a work block approval is requested
  - Triggered: When an `Approval` record is created
  - Data available: Issue key, work block title, requester, approval type
  - Buttons: "Approve" and "Reject" interactive buttons (future phase)

- **Approval Resolved:** When an approval is approved or rejected
  - Triggered: When `Approval.Status` transitions to approved/rejected
  - Data available: Issue key, approval status, reviewer, resolution time

---

## 3. Message Format Specification

### 3.1 Base Message Structure

All Discord notifications use Discord's embed format for rich formatting:

```json
{
  "content": "",  // Optional plain text if embed is empty
  "embeds": [
    {
      "title": "Event Title",
      "description": "Brief description of what happened",
      "color": 5865394,  // Hex color in decimal (e.g., 0x5865F2 = 5865394)
      "timestamp": "2026-04-09T14:30:00Z",  // ISO 8601
      "fields": [
        {
          "name": "Field Name",
          "value": "Field Value",
          "inline": true
        }
      ]
    }
  ],
  "username": "SecondOrder Bot",
  "avatar_url": "https://example.com/avatar.png"  // Optional
}
```

### 3.2 Color Coding by Event Type

| Event Type | Color | Hex Code | Decimal |
|---|---|---|---|
| Run Started | Blue | `#5865F2` | 5865394 |
| Run Completed | Green | `#57F287` | 5763719 |
| Run Failed | Red | `#ED4245` | 15548997 |
| Status Changed (Todo → In Progress) | Yellow | `#FEE75C` | 16776540 |
| Status Changed (→ Done) | Green | `#57F287` | 5763719 |
| Status Changed (→ Blocked) | Red | `#ED4245` | 15548997 |
| Comment Added | Blue | `#5865F2` | 5865394 |
| Approval Requested | Purple | `#9C27B0` | 10204336 |
| Approval Approved | Green | `#57F287` | 5763719 |
| Approval Rejected | Red | `#ED4245` | 15548997 |

### 3.3 Event-Specific Message Formats

#### 3.3.1 Run Started

**Title:** `🚀 Run Started`  
**Embed fields:**
- Agent: `Agent Name` (inline)
- Mode: `task | heartbeat | manual` (inline)
- Issue: `SO-123` (link to issue if available)
- Start Time: `2026-04-09 14:30 UTC` (inline)

**Example:**
```
Title: 🚀 Run Started
Description: Agent has begun executing a task
Fields:
  - Agent: claude-code (inline)
  - Mode: task (inline)
  - Issue: SO-123
  - Start Time: 2026-04-09 14:30 UTC (inline)
Color: #5865F2 (Blue)
```

#### 3.3.2 Run Completed

**Title:** `✅ Run Completed`  
**Embed fields:**
- Agent: `Agent Name` (inline)
- Issue: `SO-123`
- Status: `success | partial | timeout`
- Tokens Used: `Input: XXX | Output: YYY | Cache Hit: ZZZ` (inline)
- Cost: `$0.123` (inline)
- Duration: `5m 30s` (inline)

**Example:**
```
Title: ✅ Run Completed
Description: Task execution finished successfully
Fields:
  - Agent: claude-code (inline)
  - Issue: SO-123
  - Status: success (inline)
  - Tokens: Input: 5,000 | Output: 2,500 (inline)
  - Cost: $0.045 (inline)
  - Duration: 5m 30s (inline)
Color: #57F287 (Green)
```

#### 3.3.3 Run Failed

**Title:** `❌ Run Failed`  
**Embed fields:**
- Agent: `Agent Name` (inline)
- Issue: `SO-123`
- Error: `Brief error description` (multi-line, truncate to 500 chars)
- Duration: `Time before failure` (inline)
- Partial Output: `Last 200 chars of stdout if available` (optional)

**Example:**
```
Title: ❌ Run Failed
Description: Task execution encountered an error
Fields:
  - Agent: claude-code (inline)
  - Issue: SO-123
  - Error: Connection timeout to external API after 30s
  - Duration: 30s (inline)
Color: #ED4245 (Red)
```

#### 3.3.4 Status Change

**Title:** `🔄 Status Changed: SO-123`  
**Embed fields:**
- Issue Title: `"Fix login bug"` (inline)
- From: `todo` (inline)
- To: `in_progress` (inline)
- Updated: `2026-04-09 14:30 UTC` (inline)
- Assignee: `claude-code` (inline, if changed)

**Color selection logic:**
- If "done" or "board_review": Green (#57F287)
- If "blocked": Red (#ED4245)
- If "in_progress": Yellow (#FEE75C)
- Default: Blue (#5865F2)

**Example:**
```
Title: 🔄 Status Changed: SO-123
Description: Fix login bug
Fields:
  - From: todo (inline)
  - To: in_progress (inline)
  - Assignee: claude-code (inline)
  - Updated: 2026-04-09 14:30 UTC (inline)
Color: #FEE75C (Yellow)
```

#### 3.3.5 Comment Added

**Title:** `💬 Comment on SO-123`  
**Embed fields:**
- Issue: `SO-123: Issue Title` (clickable link)
- Author: `Agent Name | User Name`
- Comment: `First 1000 characters of comment body`
- Posted: `2026-04-09 14:30 UTC`

**Truncation:** If comment > 1000 chars, append `... (truncated, view in app for full comment)`

**Example:**
```
Title: 💬 Comment on SO-123
Description: Here's the first part of the comment text that was added to the issue...
Fields:
  - Issue: SO-123: Fix login bug
  - Author: claude-code (inline)
  - Posted: 2026-04-09 14:30 UTC (inline)
Color: #5865F2 (Blue)
```

#### 3.3.6 Approval Requested

**Title:** `⏳ Approval Needed: SO-123`  
**Embed fields:**
- Issue: `SO-123: Work Block Title`
- Requested By: `Agent Name`
- Type: `activate | ready_to_ship` (inline)
- Expires: `Time window for approval if applicable` (inline)
- Details: `Work block goal or description` (optional, up to 500 chars)

**Example:**
```
Title: ⏳ Approval Needed: SO-123
Description: Review and approve this work block to proceed
Fields:
  - Issue: SO-123: Implement caching layer
  - Requested By: claude-code (inline)
  - Type: activate (inline)
  - Details: Add Redis caching for API responses (optional)
Color: #9C27B0 (Purple)
```

#### 3.3.7 Approval Resolved

**Title:** `✔️ Approval Resolved: SO-123`  
**Embed fields:**
- Issue: `SO-123: Work Block Title`
- Status: `approved | rejected` (inline)
- Decision By: `Agent/User Name` (inline)
- Resolved: `2026-04-09 14:30 UTC` (inline)
- Comment: `Reviewer comment if provided` (optional, up to 300 chars)

**Color:** Green if approved, Red if rejected

**Example:**
```
Title: ✔️ Approval Resolved: SO-123
Description: Work block has been approved for activation
Fields:
  - Issue: SO-123: Implement caching layer
  - Status: approved (inline)
  - Approved By: product-lead (inline)
  - Resolved: 2026-04-09 14:30 UTC (inline)
Color: #57F287 (Green)
```

---

## 4. Configuration Model

### 4.1 Configuration Hierarchy

Discord notifications support two levels of configuration:

1. **Global Configuration** (System-wide defaults)
   - Enable/disable notifications
   - Default webhook URL (fallback)
   - Default event subscriptions (which events send to default webhook)
   
2. **Per-Agent Configuration** (Agent-specific overrides)
   - Custom webhook URL (per agent)
   - Event filter (which events this agent subscribes to)
   - Can override global defaults

### 4.2 Configuration Storage

Discord webhook settings are stored in the `notifications` table (new table):

```
Table: notifications
  - id (UUID)
  - scope (enum: 'global' | 'agent') -- 'global' has no agent_id
  - agent_id (UUID, nullable)
  - type (enum: 'discord') -- for future extensibility
  - webhook_url (string, encrypted)
  - enabled (boolean)
  - events (JSON array of event types to subscribe to)
  - created_at (timestamp)
  - updated_at (timestamp)
  - created_by (string)
```

### 4.3 Event Subscription Model

Each configuration specifies which events it subscribes to:

```json
{
  "events": [
    "run.started",
    "run.completed",
    "run.failed",
    "issue.status_changed",
    "issue.comment_added",
    "approval.requested",
    "approval.resolved"
  ]
}
```

### 4.4 Configuration Examples

**Example 1: Global Default (All Events)**
```json
{
  "scope": "global",
  "type": "discord",
  "webhook_url": "https://discordapp.com/api/webhooks/xxx/yyy",
  "enabled": true,
  "events": [
    "run.started",
    "run.completed",
    "run.failed",
    "issue.status_changed",
    "issue.comment_added",
    "approval.requested",
    "approval.resolved"
  ]
}
```

**Example 2: Agent Override (Only Critical Events)**
```json
{
  "scope": "agent",
  "agent_id": "6b65a5b7-8196-4487-b39b-d979b4b86a1c",
  "type": "discord",
  "webhook_url": "https://discordapp.com/api/webhooks/aaa/bbb",
  "enabled": true,
  "events": [
    "run.failed",
    "issue.status_changed",
    "approval.requested"
  ]
}
```

**Example 3: Agent Override (Disabled)**
```json
{
  "scope": "agent",
  "agent_id": "6b65a5b7-8196-4487-b39b-d979b4b86a1c",
  "type": "discord",
  "webhook_url": "https://discordapp.com/api/webhooks/xxx/yyy",
  "enabled": false,
  "events": []
}
```

### 4.5 Webhook URL Storage Security

- Webhook URLs are **encrypted at rest** using the system's secrets manager
- URLs are never logged in plain text
- Only decrypted when actively sending to Discord
- Rotation is supported (create new config, disable old one)

### 4.6 Configuration API

The existing notification configuration can be accessed via:

- `POST /api/v1/notifications/discord` - Create/update webhook config
- `GET /api/v1/notifications/discord` - List active Discord webhooks
- `DELETE /api/v1/notifications/discord/{config_id}` - Disable/delete config
- `PUT /api/v1/notifications/discord/{config_id}` - Update events or URL

---

## 5. Error Handling & Retry Behavior

### 5.1 Retry Strategy

When Discord webhook delivery fails, implement exponential backoff with jitter:

**Retry Policy:**
- **Max Retries:** 3 attempts
- **Initial Backoff:** 100ms
- **Backoff Multiplier:** 2x (100ms → 200ms → 400ms)
- **Max Backoff:** 10 seconds (capped)
- **Jitter:** Add ±10% random jitter to prevent thundering herd

**Example retry schedule:**
- Attempt 1: Immediate
- Attempt 2: 100ms + jitter
- Attempt 3: 200ms + jitter
- Attempt 4: 400ms + jitter
- Failure: Log and continue (non-blocking)

### 5.2 Retryable vs Non-Retryable Errors

**Retryable (retry with backoff):**
- Network timeout (connection or read timeout)
- 5xx server errors (500, 502, 503, 504)
- Rate limiting (429)
- Temporary connection failures

**Non-Retryable (fail immediately, log warning):**
- 400 Bad Request (invalid message format)
- 401 Unauthorized (invalid webhook token)
- 404 Not Found (webhook deleted)
- 413 Payload Too Large (message exceeds 8MB)

### 5.3 Discord API Response Handling

| Status Code | Action | Log Level |
|---|---|---|
| 204 No Content | Success | Debug |
| 400 Bad Request | Log error, don't retry | Warning |
| 401 Unauthorized | Disable config, alert | Error |
| 404 Not Found | Disable config, alert | Warning |
| 429 Too Many Requests | Retry with backoff | Warning |
| 5xx | Retry with backoff | Warning |
| Timeout | Retry with backoff | Info |

### 5.4 Context Cancellation

- All webhook sends respect context deadlines
- If context is cancelled mid-send, stop immediately and log
- Failed sends are NOT queued for retry; they are dropped with a log entry
- This prevents memory bloat from queued messages on long-running processes

### 5.5 Degraded Mode

When Discord is unavailable:
- Log all failed send attempts with details (which event, agent, issue)
- Continue normal operation (notifications are best-effort)
- Alerts can monitor logs for repeated webhook failures (e.g., "has failed 10 times in 5m")
- No buffering or queue persistence (transient failure tolerance only)

### 5.6 Circuit Breaker (Future Phase)

Future implementation could add:
- Track failures per webhook URL
- Disable webhook temporarily after N consecutive failures
- Re-enable after cooldown period
- Alert when circuit opens/closes

---

## 6. Implementation Reference: Telegram Pattern

The existing Telegram integration (`internal/telegram/bot.go`) provides a reference pattern:

**Key patterns to follow:**
- Simple `struct` with HTTP client and configuration (see `Bot` struct)
- Public methods for specific event types (`SendWorkBlockApproval`, `SendMessage`)
- Markdown escaping for user-provided content
- Inline keyboard buttons for interactive responses (future phase for Discord)
- Long polling for inbound updates (not applicable for webhook-based Discord)

**Differences from Telegram:**
- Discord uses webhooks (one-way push) vs. Telegram's long polling (two-way)
- Discord embeds are richer than Telegram's Markdown formatting
- Discord webhook messages don't require auth tokens per send (webhook URL includes auth)
- No need for callback polling (Discord doesn't support incoming updates via webhook)

---

## 7. Acceptance Criteria Checklist

- [x] Document which events trigger Discord notifications (run start/complete, comments, status changes, work block approvals)
- [x] Define message format per event type (embeds, fields, colors)
- [x] Specify configuration model: global vs per-agent, webhook URL storage, event selection
- [x] Define error/retry behavior (what happens when Discord is down)
- [x] Reference existing Telegram integration pattern in internal/telegram/bot.go as prior art

---

## 8. Future Enhancements (Out of Scope)

These features are intentionally excluded from this spec but documented for future consideration:

1. **Interactive Buttons** - Add "Approve" and "Reject" buttons to approval messages (requires webhook receiver)
2. **Message Threading** - Group related events (e.g., all updates for one run) in Discord threads
3. **Slash Commands** - Allow users to trigger actions via Discord slash commands
4. **Rich Link Previews** - Generate preview cards for linked issues/runs
5. **Custom Message Templates** - Allow users to customize message format per event type
6. **Webhook Signature Verification** - Validate Discord webhook authenticity (if adding inbound receiver)
7. **Circuit Breaker Pattern** - Disable webhooks after repeated failures
8. **Message Deduplication** - Prevent duplicate notifications for the same event
9. **Batch Notifications** - Aggregate multiple events into single message during high activity

---

## 9. Testing Strategy (TBD)

Tests should cover:
- Message format validation (ensure embeds match spec)
- Color coding correctness
- Webhook URL validation
- Retry logic with timeouts
- Configuration persistence
- Permission-based filtering (agents only see their own configs)
- Event filtering (only configured events trigger sends)
- Truncation of long text fields
- Markdown/special character escaping in user content

---

## Appendix A: Event Timeline Example

This sequence shows notifications for a typical work scenario:

```
14:00 - Run Started
  Agent: claude-code
  Issue: SO-123
  [Blue embed with start time]

14:05 - Issue Status Changed  
  Issue: SO-123
  From: todo → in_progress
  [Yellow embed]

14:08 - Comment Added
  Issue: SO-123
  Author: claude-code
  Comment: "Found the issue, implementing fix..."
  [Blue embed]

14:25 - Run Completed
  Issue: SO-123
  Status: success
  Tokens: 8,000 input, 3,500 output
  Cost: $0.067
  [Green embed with completion time]

14:26 - Issue Status Changed
  Issue: SO-123
  From: in_progress → in_review
  [Yellow embed]

14:27 - Approval Requested
  Issue: SO-123
  Work Block: "Deploy to staging"
  Requested By: claude-code
  Type: ready_to_ship
  [Purple embed]

14:35 - Approval Resolved
  Issue: SO-123
  Status: approved
  Approved By: product-lead
  [Green embed]

14:36 - Issue Status Changed
  Issue: SO-123
  From: in_review → done
  [Green embed]
```

---

## Document History

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-04-09 | Product Team | Initial spec definition |
