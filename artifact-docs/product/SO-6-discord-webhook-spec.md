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

### 6.2 Channel Adapter Pattern

Discord Webhook adapter implements a standard channel interface:

```
type NotificationChannel {
  deliver(notification: Notification) -> Result<void, Error>
  validate() -> Result<void, Error>
  supports(feature: string) -> bool
}
```

**Discord Webhook Implementation:**
- `deliver()`: POST to webhook URL with formatted message
- `validate()`: Send test message; confirm HTTP 204 response
- `supports()`: Returns true for "embeds", "mentions", "markdown"

### 6.3 Integration Points

| Point | Requirement |
|-------|-------------|
| Configuration storage | Encrypt webhook URL; store workspace_id, channel_name, created_at |
| Message formatting | Template engine for notification type → discord embed/content |
| Error handling | Treat HTTP 429 as retriable; 404/401 as webhook deleted/invalid |
| Audit logging | Log all webhook deliveries (success/failure, retry count) |
| Rate limiting | Implement per-workspace rate limit to respect Discord's 10 req/10s |

---

## 7. Configuration Model

### 7.1 Webhook Configuration Schema

```yaml
DiscordWebhookChannel:
  workspace_id: string          # SecondOrder workspace
  webhook_id: string            # Discord webhook UUID
  webhook_url: string           # Full URL (encrypted at rest)
  channel_name: string          # Human-readable: "general", "alerts"
  channel_id: string            # Discord channel UUID
  enabled: boolean              # Can pause without deleting
  created_at: timestamp         # Audit trail
  last_validated_at: timestamp  # When URL was last tested
  notification_settings:
    include_timestamp: boolean  # Add timestamp to embeds
    include_user_mention: boolean  # Mention creator of event
    include_thread_link: boolean   # Link to discussion thread
```

### 7.2 User-facing Configuration

**Setup flow:**
1. User: "Add Discord notifications"
2. System: Shows instructions to create webhook in Discord
3. User: Copies webhook URL from Discord
4. System: Paste URL → validate → confirm

**Management:**
- List connected Discord channels
- Test delivery (send sample message)
- Pause/resume channel
- Rotate webhook (delete and re-add)
- Delete channel

---

## 8. Message Templates

### 8.1 Template Structure

Templates define how each notification type maps to Discord format.

**Example: Task Assignment Notification**

```yaml
templates:
  task_assigned:
    title: "New Task Assigned"
    description_format: "{{assigner}} assigned {{task_title}} to you"
    embed_color: 3066993  # Blue
    fields:
      - name: "Task"
        value: "{{task_title}}"
        inline: false
      - name: "Priority"
        value: "{{priority}}"
        inline: true
      - name: "Due Date"
        value: "{{due_date}}"
        inline: true
    include_task_link: true
    mention_assignee: true
```

### 8.2 Standard Embed Fields

Every notification template should support:
- **Title**: Short description
- **Description**: Main message content
- **Color**: Status indicator (green=success, red=error, blue=info)
- **Timestamp**: When event occurred
- **Footer**: Source (e.g., "SecondOrder • workspace-name")

### 8.3 Content Constraints

| Content Type | Max Length | Handling |
|--------------|-----------|----------|
| Title | 256 chars | Truncate with … |
| Description | 2048 chars | Truncate with link to full details |
| Field value | 1024 chars per field | Split across multiple fields if needed |
| Embed count | 10 per message | Use max 1-2 for clarity |

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
