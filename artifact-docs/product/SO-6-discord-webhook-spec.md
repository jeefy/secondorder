# Discord Webhook Integration Specification

**Issue:** SO-6  
**Author:** Product Lead  
**Date:** 2026-04-09  
**Status:** Specification for Implementation

---

## 1. Executive Summary

This document specifies the integration of Discord Webhooks as a notification delivery channel for the SecondOrder notification pipeline. Discord Webhooks provide a simple, low-overhead mechanism for delivering rich formatted notifications to Discord channels without requiring bot account setup or OAuth flows.

**Key Finding:** Discord Webhooks are well-suited for MVP scope, with clear constraints and straightforward lifecycle management.

---

## 2. Discord Webhook API Overview

### 2.1 What is a Discord Webhook?

A Discord Webhook is a lightweight integration point that allows external services to post messages to Discord channels. Unlike bot accounts, webhooks:
- Don't require persistent connections or socket handling
- Have minimal authentication overhead (single URL with embedded token)
- Support message content, embeds, mentions, and file attachments
- Can be managed directly via Discord's API or the UI

### 2.2 Core API Endpoint

**POST** `https://discordapp.com/api/webhooks/{webhook_id}/{webhook_token}`

This is the only endpoint needed for message delivery in the MVP scope.

---

## 3. API Capabilities and Constraints

### 3.1 Message Content Capabilities

| Feature | Supported | Constraint |
|---------|-----------|-----------|
| Text content | ✅ Yes | Max 2000 characters |
| Embeds | ✅ Yes | Max 10 embeds per message |
| Markdown formatting | ✅ Yes | Discord's markdown subset |
| Mentions (@user, @role, @everyone) | ✅ Yes | Must use proper mention format: `<@USER_ID>` |
| URLs and links | ✅ Yes | No special handling required |
| Images/attachments | ✅ Yes | File upload or image embed |
| Reactions | ❌ No | Cannot add reactions via webhook |
| Threading/replies | ✅ Yes | Can target specific thread |

### 3.2 Rate Limiting

- **Per-webhook:** 10 requests per 10 seconds (Discord's global rate limit)
- **Behavior:** Discord uses HTTP 429 responses with `Retry-After` header
- **Implication:** For MVP, implement basic retry logic with exponential backoff

### 3.3 Message Delivery

- **Acknowledgment:** HTTP 204 No Content response = message posted successfully
- **No guaranteed delivery:** If webhook is deleted mid-request, message may not appear
- **Idempotency:** No built-in request deduplication; client must implement if needed
- **Async delivery:** Response indicates acceptance, not that message is visible

### 3.4 Limitations

| Constraint | Impact | Mitigation |
|-----------|--------|-----------|
| No message editing via webhook | Cannot update sent notifications | Design for immutable notifications |
| No message deletion via webhook | Cannot remove sent notifications | Document cleanup procedures |
| No reactions via webhook | Cannot gauge response | Use Discord threads for discussion |
| Webhook-generated messages not pingable | Cannot mention webhook messages | Design notification format accordingly |
| Max 2000 chars per message | Long notifications won't fit | Implement truncation or multi-message strategy |
| No authentication beyond token in URL | Token exposure = full channel access | Treat webhook URLs as secrets |

---

## 4. Webhook Lifecycle Management

### 4.1 Webhook Creation

**Scenario:** User enables Discord notifications for a workspace/project.

**Process:**
1. User connects their Discord account via OAuth or manual webhook URL paste
2. System stores webhook URL (encrypted at rest)
3. System validates webhook by sending a test message
4. On success, notification channel is activated

**API:** Discord provides webhook creation via UI or API (`POST /channels/{channel_id}/webhooks`)

**MVP Scope Decision:** For MVP, support manual webhook URL paste only. Users create webhooks in Discord UI and paste the URL into SecondOrder.

### 4.2 Webhook Rotation

**Scenario:** Webhook URL compromised or needs rotation.

**Process:**
1. User deletes old webhook in Discord UI
2. User creates new webhook and updates URL in SecondOrder
3. SecondOrder validates new URL
4. Old messages continue to exist; new messages use new webhook

**Note:** No automatic rotation mechanism in Discord. This is a user-initiated process.

### 4.3 Webhook Deletion

**Scenario:** User disables Discord notifications.

**Process:**
1. SecondOrder deletes webhook URL from database
2. User should delete webhook in Discord UI (cleanup)
3. If webhook URL is accidentally leaked, delete from Discord UI to revoke access

---

## 5. Message Formatting Options

### 5.1 Content-based Messages (Simple)

**Use case:** Brief notifications (status changes, simple alerts)

```json
{
  "content": "Your build has completed successfully",
  "username": "SecondOrder Bot"
}
```

### 5.2 Embed-based Messages (Rich)

**Use case:** Complex notifications with structured data

```json
{
  "username": "SecondOrder Bot",
  "embeds": [
    {
      "title": "Deployment Notification",
      "description": "Version 1.2.3 deployed to production",
      "color": 3066993,
      "fields": [
        {
          "name": "Status",
          "value": "✅ Success",
          "inline": true
        },
        {
          "name": "Duration",
          "value": "2m 34s",
          "inline": true
        },
        {
          "name": "Changes",
          "value": "5 commits, 42 files changed",
          "inline": false
        }
      ],
      "timestamp": "2026-04-09T14:30:00.000Z"
    }
  ]
}
```

### 5.3 Markdown Formatting

Discord supports a subset of markdown:
- **Bold:** `**text**`
- **Italic:** `*text*`
- **Code:** `` `code` `` or ` ```code block``` `
- **Links:** `[text](url)`
- **Mentions:** `<@USER_ID>` or `<@&ROLE_ID>`

---

## 6. Notification Pipeline Integration

### 6.1 Architecture Position

```
┌─────────────────────────────┐
│ Notification Trigger        │
│ (event, filter rules)       │
└──────────┬──────────────────┘
           │
┌──────────▼──────────────────┐
│ Notification Service        │
│ (route to channels)         │
└──────────┬──────────────────┘
           │
      ┌────┴────┬──────────┬──────────┐
      │          │          │          │
   ┌──▼──┐   ┌──▼──┐   ┌──▼──┐   ┌──▼──┐
   │Email│   │Slack│   │Push │ │Discord
   │     │   │     │   │Notif│ │Webhook
   └─────┘   └─────┘   └─────┘ └──────┘
```

### 6.2 Prior Art: Telegram Integration Reference

SecondOrder already has a working Telegram notification integration (`internal/telegram/bot.go`) that serves as a reference implementation for how bots interact with external services.

**Telegram Bot Pattern:**
- **Bot Struct:** Encapsulates API token, target chat ID, and HTTP client
- **Send Methods:** Separate methods per message type (`SendWorkBlockApproval`, `SendMessage`)
- **Event Handling:** `OnApproval` callback for interactive approvals (buttons in Telegram)
- **Polling Pattern:** Long-polling for updates using `getUpdates` API with offset tracking
- **Error Handling:** Basic HTTP status checking; retries at polling level

**Discord Webhook Pattern (Different approach):**
- **No Bot Struct:** Webhooks don't maintain state or poll for updates
- **Single Send Method:** Generic POST to webhook URL with rich embeds
- **No Interactive Responses:** One-way communication (no inline approval buttons)
- **Async Delivery:** Fire-and-forget POST; no polling/callbacks needed
- **Error Handling:** Retry logic for rate limits (429) and transient failures

**Key Differences:**
| Aspect | Telegram | Discord Webhook |
|--------|----------|-----------------|
| **Architecture** | Bot with polling | Webhook receiver |
| **State** | Maintains chat_id, token | Stateless (URL contains token) |
| **Communication** | Bidirectional (send + poll) | Unidirectional (send only) |
| **Interactivity** | Inline buttons, callbacks | No interactive elements |
| **Multiple Destinations** | Single chat_id | Multiple webhooks per workspace |
| **Config** | 2 values (token, chatID) | Webhook URL + event filters |

**Discord Adapter Will Follow:**
- Separate Discord adapter struct (similar pattern to Telegram)
- Template-based message formatting (similar to `SendWorkBlockApproval` approach)
- Event filtering/selection (Telegram has implicit filtering via buttons; Discord needs explicit config)
- Retry logic for resilience (Telegram retries at polling level; Discord retries at send level)

**Discord Adapter Will NOT Use:**
- Polling/long-connections (webhooks are push-based)
- Interactive buttons (Discord buttons would require a separate bot account)
- Callbacks (one-way delivery model)

### 6.3 Channel Adapter Pattern

Discord Webhook adapter implements a standard channel interface:

```
type NotificationChannel {
  deliver(notification: Notification) -> Result<void, Error>
  validate() -> Result<void, Error>
  supports(feature: string) -> bool
}
```

**Discord Webhook Implementation:**
- `deliver()`: POST to webhook URL with formatted message, applying event filters
- `validate()`: Send test message; confirm HTTP 204 response
- `supports()`: Returns true for "embeds", "mentions", "markdown"; false for "interactive_buttons"

### 6.4 Integration Points

| Point | Requirement |
|-------|-------------|
| Configuration storage | Store global + per-agent webhook configs; encrypt URLs |
| Message formatting | Template engine for notification type → discord embed (Section 8) |
| Event filtering | Check global + per-agent event_filters before sending |
| Error handling | Treat HTTP 429 as retriable; 404/401 as webhook deleted/invalid |
| Audit logging | Log all webhook deliveries (success/failure, retry count, event type) |
| Rate limiting | Implement per-workspace rate limit to respect Discord's 10 req/10s |

---

## 7. Configuration Model

### 7.1 Global vs Per-Agent Configuration

Discord webhook configuration operates at two levels:

#### **Global Configuration (Workspace-level)**
- Applies to all agents in a workspace
- Single webhook URL receives notifications from all agents
- Users can select which event types to receive
- Managed by workspace admins in Settings > Discord Webhooks
- Use case: Centralized team notifications channel

```yaml
GlobalDiscordWebhookConfig:
  workspace_id: string              # Workspace identifier
  webhook_url: string               # Encrypted at rest
  enabled: boolean
  event_filters:
    run_started: boolean            # Include run_started events
    run_completed: boolean          # Include run_completed events
    comment_added: boolean          # Include comment_added events
    status_changed: boolean         # Include status_changed events
    work_block_approved: boolean    # Include work_block_approved events
    work_block_rejected: boolean    # Include work_block_rejected events
  created_at: timestamp
  updated_at: timestamp
  created_by_user_id: string        # Audit trail
```

#### **Per-Agent Configuration (Agent-level)**
- Applies to a specific agent
- Agent-specific webhook URL (same or different from global)
- Users can override which events to receive per agent
- Managed per-agent in Agent Settings > Notifications
- Use case: Agent-specific channels (e.g., debugging channel for experimental agent)

```yaml
AgentDiscordWebhookConfig:
  agent_id: string                  # Agent identifier
  workspace_id: string              # Parent workspace
  webhook_url: string               # Optional; if null, inherits global
  enabled: boolean                  # Agent can override global setting
  use_global: boolean               # If true, ignore webhook_url and event_filters
  event_filters:                    # If use_global=false, these override global
    run_started: boolean
    run_completed: boolean
    comment_added: boolean
    status_changed: boolean
    work_block_approved: boolean
    work_block_rejected: boolean
  created_at: timestamp
  updated_at: timestamp
```

**Priority/Inheritance:**
1. If agent has per-agent config with `use_global=false` → use agent config
2. If agent has per-agent config with `use_global=true` → use global config
3. If agent has no per-agent config → use global config
4. If no global config exists → no notifications sent

### 7.2 Webhook Configuration Schema (Database)

```yaml
DiscordWebhookChannel:
  id: string                        # Primary key (UUID)
  workspace_id: string              # SecondOrder workspace
  agent_id: string                  # NULL for global, set for per-agent
  webhook_id: string                # Discord webhook UUID
  webhook_url: string               # Full URL (encrypted at rest)
  channel_name: string              # Human-readable: "general", "alerts"
  channel_id: string                # Discord channel UUID
  enabled: boolean                  # Can pause without deleting
  use_global: boolean               # For per-agent configs
  event_filters:                    # JSON object
    run_started: boolean
    run_completed: boolean
    comment_added: boolean
    status_changed: boolean
    work_block_approved: boolean
    work_block_rejected: boolean
  created_at: timestamp             # Audit trail
  updated_at: timestamp
  last_validated_at: timestamp      # When URL was last tested
  last_validated_status: string     # "success" | "failed" | "unknown"
  created_by_user_id: string        # Who configured this
  notification_settings:
    include_timestamp: boolean      # Add timestamp to embeds
    include_user_mention: boolean   # Mention creator of event
    include_thread_link: boolean    # Link to discussion thread
```

### 7.3 User-facing Configuration Flows

**Setup Global Webhook (Workspace Admin):**
1. Navigate to Settings > Notifications > Discord
2. Click "Add Discord Webhook"
3. Paste webhook URL from Discord
4. System validates webhook with test message
5. Select events to receive:
   - ☐ Run Started
   - ☐ Run Completed
   - ☐ Comment Added
   - ☐ Status Changed
   - ☐ Work Block Approved
   - ☐ Work Block Rejected
6. Click "Save and Test"
7. Success confirmation or error handling

**Setup Per-Agent Override (Agent Owner):**
1. Navigate to Agent Settings > Notifications
2. Toggle "Use custom Discord webhook" (default: use global)
3. Optionally paste different webhook URL
4. Optionally override event filters
5. Click "Save"

**Management (both levels):**
- List configured webhooks (global + per-agent)
- Test delivery (send sample message)
- Enable/disable without deleting
- Rotate webhook (delete and re-add)
- Edit event filters
- Delete webhook configuration

---

## 8. SecondOrder Event Definitions and Triggers

### 8.1 Event Triggering Table

The following SecondOrder events trigger Discord webhook notifications. Users can select which events they want to receive:

| Event | Description | Trigger Condition | User Benefit |
|-------|-------------|-------------------|--------------|
| **run_started** | Agent run begins execution | Run transitions to RUNNING state | Track when agents start working |
| **run_completed** | Agent run finishes (success/failure) | Run transitions to DONE, FAILED, or CANCELLED state | Monitor run outcomes and status |
| **comment_added** | Comment posted on issue/work block | New comment created by any user | Stay informed of team discussion |
| **status_changed** | Issue or work block status changes | Status field modified | Track progress and blockers |
| **work_block_approved** | Work block marked ready for approval | Work block transitions to APPROVED state | Confirm go-ahead decisions |
| **work_block_rejected** | Work block marked for revision | Work block transitions to REJECTED state | Alert to rework needs |

### 8.2 Message Format Per Event Type

Each event type has a dedicated embed template for Discord:

#### **run_started**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "🚀 Agent Run Started",
    "description": "Run #{{run_id}} started by {{initiator_name}}",
    "color": 3447003,
    "fields": [
      {
        "name": "Agent",
        "value": "{{agent_name}}",
        "inline": true
      },
      {
        "name": "Workspace",
        "value": "{{workspace_name}}",
        "inline": true
      },
      {
        "name": "Goal",
        "value": "{{goal}}",
        "inline": false
      }
    ],
    "timestamp": "{{created_at}}"
  }]
}
```

**Color:** 3447003 (Blue)

---

#### **run_completed**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "✅ Agent Run Completed",
    "description": "Run #{{run_id}} finished with status {{status}}",
    "color": "{{status == 'DONE' ? 3066993 : 15158332}}",
    "fields": [
      {
        "name": "Agent",
        "value": "{{agent_name}}",
        "inline": true
      },
      {
        "name": "Status",
        "value": "{{status == 'DONE' ? '✅ Success' : '❌ Failed'}}",
        "inline": true
      },
      {
        "name": "Duration",
        "value": "{{duration_seconds}}s",
        "inline": true
      },
      {
        "name": "Summary",
        "value": "{{summary}}",
        "inline": false
      }
    ],
    "timestamp": "{{completed_at}}"
  }]
}
```

**Color:** 3066993 (Green) for success, 15158332 (Red) for failure

---

#### **comment_added**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "💬 New Comment",
    "description": "{{author_name}} commented on {{target_title}}",
    "color": 9807270,
    "fields": [
      {
        "name": "Author",
        "value": "{{author_name}}",
        "inline": true
      },
      {
        "name": "Target",
        "value": "{{target_type}}: {{target_title}}",
        "inline": true
      },
      {
        "name": "Comment",
        "value": "{{comment_preview}}",
        "inline": false
      }
    ],
    "timestamp": "{{created_at}}"
  }]
}
```

**Color:** 9807270 (Purple)

---

#### **status_changed**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "📋 Status Changed",
    "description": "{{item_title}} status updated",
    "color": 16776960,
    "fields": [
      {
        "name": "Item",
        "value": "{{item_type}}: {{item_title}}",
        "inline": true
      },
      {
        "name": "Changed By",
        "value": "{{changed_by_name}}",
        "inline": true
      },
      {
        "name": "Old Status",
        "value": "{{old_status}}",
        "inline": true
      },
      {
        "name": "New Status",
        "value": "{{new_status}}",
        "inline": true
      }
    ],
    "timestamp": "{{changed_at}}"
  }]
}
```

**Color:** 16776960 (Yellow)

---

#### **work_block_approved**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "✅ Work Block Approved",
    "description": "{{block_title}} has been approved",
    "color": 3066993,
    "fields": [
      {
        "name": "Work Block",
        "value": "{{block_title}}",
        "inline": true
      },
      {
        "name": "Approved By",
        "value": "{{approver_name}}",
        "inline": true
      },
      {
        "name": "Goal",
        "value": "{{goal}}",
        "inline": false
      },
      {
        "name": "Next Step",
        "value": "Ready for {{transition_target}}",
        "inline": false
      }
    ],
    "timestamp": "{{approved_at}}"
  }]
}
```

**Color:** 3066993 (Green)

---

#### **work_block_rejected**
```json
{
  "username": "SecondOrder Bot",
  "embeds": [{
    "title": "❌ Work Block Rejected",
    "description": "{{block_title}} needs revision",
    "color": 15158332,
    "fields": [
      {
        "name": "Work Block",
        "value": "{{block_title}}",
        "inline": true
      },
      {
        "name": "Rejected By",
        "value": "{{rejector_name}}",
        "inline": true
      },
      {
        "name": "Goal",
        "value": "{{goal}}",
        "inline": false
      },
      {
        "name": "Feedback",
        "value": "{{feedback}}",
        "inline": false
      }
    ],
    "timestamp": "{{rejected_at}}"
  }]
}
```

**Color:** 15158332 (Red)

---

### 8.3 Standard Embed Fields

Every notification template supports:
- **Title**: Short description with emoji indicator
- **Description**: Main message content
- **Color**: Event-specific status indicator (green=success, red=failure, blue=info, yellow=change, purple=comment)
- **Timestamp**: When event occurred (ISO 8601)
- **Footer**: Source (e.g., "SecondOrder • workspace-name")
- **Fields**: Structured key-value pairs for additional context

### 8.4 Content Constraints

| Content Type | Max Length | Handling |
|--------------|-----------|----------|
| Title | 256 chars | Truncate with … |
| Description | 2048 chars | Truncate with link to full details |
| Field value | 1024 chars per field | Truncate with … if needed |
| Embed count | 10 per message | Use exactly 1 per event in MVP |

---

## 9. Delivery Guarantees and Error Handling

### 9.1 Delivery Model

**At-most-once delivery:** 
- If POST succeeds (HTTP 204), message is posted
- If POST fails, message is not retried automatically
- Network errors are logged for user visibility

### 9.2 Error Handling Strategy

| HTTP Status | Cause | Action |
|------------|-------|--------|
| 204 | Success | Log delivery |
| 400 | Bad request (malformed JSON) | Log error; fix template; alert user |
| 401/403 | Unauthorized (token invalid) | Mark channel invalid; prompt re-auth |
| 404 | Webhook deleted | Mark channel deleted; prompt user |
| 429 | Rate limited | Retry with exponential backoff (max 3 retries) |
| 5xx | Discord server error | Retry with backoff (max 3 retries) |
| Timeout | Network issue | Log; don't retry to avoid queue buildup |

### 9.3 Retry Logic

For HTTP 429 and 5xx errors:
```
attempt 1: immediate
attempt 2: wait 1 second + random jitter (0-1s)
attempt 3: wait 3 seconds + random jitter (0-3s)
failure:   log error, inform user via dashboard
```

### 9.4 What's Not Guaranteed

- **No persistence**: If webhook URL deleted, message is lost
- **No deduplication**: Same notification sent twice = two messages
- **No ordering**: Multiple webhooks may deliver out of order
- **No editing**: Once sent, message content can't be changed

---

## 10. MVP Scope and Recommendations

### 10.1 MVP Features

✅ **Include in MVP:**
1. Manual webhook URL configuration (paste URL from Discord)
2. Test message validation on setup
3. Single-embed notification templates
4. Basic message formatting (title, description, color, fields)
5. Error logging and retry logic
6. Per-workspace rate limiting
7. Enable/disable notifications without deletion
8. Audit log of deliveries

❌ **Exclude from MVP:**
- Automatic webhook management (Discord OAuth)
- Multi-message support for long notifications
- Advanced template engine
- Mention routing based on notification type
- Webhook rotation UI
- Delivery analytics dashboard

### 10.2 Rationale

**Why this scope:**
- Webhook URL paste is simpler than OAuth integration
- Single embed per notification is cleaner than complex multi-embed logic
- Rate limiting is essential; retry logic handles Discord's limits
- Audit logging helps with debugging and support

**Why not included:**
- OAuth adds complexity with token management (future phase)
- Multi-message strategy is premature until we see actual notification lengths
- Advanced templating can evolve based on user feedback

### 10.3 Post-MVP Enhancements

**Phase 2 candidates:**
1. Discord OAuth for one-click setup
2. Thread-based conversation threading for notifications
3. Reaction-based event triggering (experimental)
4. Per-notification-type template customization
5. Webhook analytics (delivery success rates, response times)

---

## 11. Implementation Roadmap

### Phase 1: Core Delivery (MVP)
- [ ] Design Discord channel adapter
- [ ] Implement webhook URL validation
- [ ] Build notification template system
- [ ] Add Discord channel to notification service
- [ ] Implement retry logic and error handling
- [ ] Add configuration UI
- [ ] End-to-end testing

### Phase 2: OAuth & Management
- [ ] Add Discord OAuth flow
- [ ] Implement automatic webhook creation
- [ ] Build webhook rotation UI
- [ ] Add channel management dashboard

### Phase 3: Advanced Features
- [ ] Notification scheduling
- [ ] Rich analytics
- [ ] Custom webhook naming
- [ ] Conditional routing rules

---

## 12. Security Considerations

### 12.1 Webhook URL Protection

- **At rest:** Encrypt in database (AES-256 or equivalent)
- **In transit:** Use HTTPS only; never expose in logs
- **In code:** Never commit webhook URLs to version control
- **User perspective:** Treat like passwords—don't share

### 12.2 Access Control

- Only workspace members can add/remove Discord webhooks
- Webhook access is scoped to the Discord channel (not the account)
- Deleted webhooks immediately revoke channel access
- Audit log tracks who added/removed webhooks

### 12.3 Discord Best Practices

- Set meaningful webhook names (e.g., "SecondOrder Notifications")
- Use a dedicated Discord server/channel for notifications
- Regularly rotate webhooks if shared access suspected
- Monitor Discord audit log for unauthorized message delivery

---

## 13. Testing Strategy

### 13.1 Unit Tests
- Template rendering correctness
- Webhook URL parsing and validation
- Retry logic with mock failures
- Rate limit detection

### 13.2 Integration Tests
- End-to-end webhook delivery
- Error handling for invalid/deleted webhooks
- Rate limit backoff behavior
- Configuration CRUD operations

### 13.3 Manual Verification
- Create test Discord server
- Verify message formatting in Discord client
- Test webhook URL rotation
- Verify error messages are user-friendly

---

## 14. Open Questions and Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Support threads? | Post to main channel first; threads in phase 2 | Keep MVP simple; channels are primary |
| Custom usernames per message? | Use single "SecondOrder Bot" username | Simpler; consistent branding |
| Support attachments? | No; use embeds only in MVP | Webhook file uploads less common |
| Store message ID? | Yes; for future edit/delete support | Low overhead; enables phase 2 |
| Multiple webhooks per workspace? | Yes; separate channels supported | Natural Discord UX |

---

## 15. Acceptance Criteria Verification

✅ **All criteria met:**

1. **Written spec in artifact-docs/** — This document covers:
   - API capabilities and constraints (section 3)
   - Message formatting options (section 5)
   - Webhook lifecycle (section 4)
   - Integration with pipeline (section 6)
   - Configuration model (section 7)
   - Message templates (section 8)
   - Delivery guarantees (section 9)

2. **Clear MVP scope recommendation** — Section 10 outlines:
   - Specific features included in MVP
   - Features deferred to phase 2+
   - Clear rationale for each decision
   - Implementation roadmap with phases

---

## 16. References

- **Discord Webhook API:** https://discord.com/developers/docs/resources/webhook
- **Message Formatting:** https://discord.com/developers/docs/resources/channel#message-object
- **Embed Specification:** https://discord.com/developers/docs/resources/channel#embed-object
- **Rate Limiting:** https://discord.com/developers/docs/topics/rate-limits

---

## 17. Sign-off

This specification is ready for engineering implementation review. The scope is achievable within a typical sprint and provides sufficient functionality for production use.

**Next steps:**
1. Engineering review and feedback
2. Refine implementation plan if needed
3. Begin Phase 1 development
4. User testing with beta group
