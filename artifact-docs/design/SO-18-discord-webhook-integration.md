# SO-18 Discord Webhook Notification Integration

## Overview

This design document specifies the Discord webhook integration for SecondOrder notifications. The implementation follows the existing Telegram notification pattern while adapting to Discord's webhook capabilities and constraints.

## Event Mapping

Discord notifications are triggered for the following events:

### 1. Work Block Approval Required
**When:** 
- New work block created with "proposed" transition
- Work block completed and ready to ship

**Type:** Interactive notification requiring user response

**Trigger Points:** 
- `internal/handlers/api.go` - `CreateWorkBlock()` line ~1007
- `internal/handlers/api.go` - Issue resolution with ready_to_ship transition line ~356

### 2. Issue Stuck Detection
**When:** 
- Issue reaches max run count without resolution
- Requires human intervention

**Type:** Alert notification (informational only)

**Trigger Point:** 
- `internal/handlers/api.go` - `ResolveIssue()` line ~338

### 3. General Messages
**When:** 
- Custom business logic notifications

**Type:** Flexible message format

## Configuration Model

### Settings Storage
Configuration stored in SQLite `settings` table (existing pattern):

- `discord_webhook_url` (string, empty string = disabled)
- `feature_discord` (boolean, default: false)

### Configuration Flow
1. User provides Discord webhook URL via admin settings UI
2. Feature flag `feature_discord` gates the functionality
3. At startup, `cmd/secondorder/main.go` checks both conditions before initializing Discord notifier
4. If webhook URL exists and feature flag is enabled, notifier is initialized

### Webhook URL Validation
- Webhook URL must be HTTPS and point to Discord API (`discord.com` domain)
- Invalid URLs should log a warning but not crash startup
- Empty webhook URL is treated as "disabled"

## Message Format Design

Discord messages use the Webhook API with embeds for rich formatting. The design follows Discord best practices for readability and consistency.

### Message Type 1: Work Block Approval Request

```json
{
  "content": null,
  "embeds": [
    {
      "title": "Work Block Ready for Review",
      "description": "A work block requires your approval to proceed.",
      "color": 3447003,
      "fields": [
        {
          "name": "Block Title",
          "value": "[Block Title Here]",
          "inline": false
        },
        {
          "name": "Goal",
          "value": "[Goal Description Here]",
          "inline": false
        },
        {
          "name": "Status",
          "value": "proposed",
          "inline": true
        }
      ],
      "footer": {
        "text": "SecondOrder Work Block Management"
      },
      "timestamp": "2026-04-10T10:00:00Z"
    }
  ],
  "components": [
    {
      "type": 1,
      "components": [
        {
          "type": 2,
          "label": "Activate",
          "style": 3,
          "custom_id": "approve:BLOCK_ID"
        },
        {
          "type": 2,
          "label": "Reject",
          "style": 4,
          "custom_id": "reject:BLOCK_ID"
        }
      ]
    }
  ]
}
```

**Color Codes:**
- Proposed block (review needed): 3447003 (Blue)
- Ready to ship (final approval): 2197520 (Green)

**Embed Fields:**
- `title`: Action required headline
- `description`: Brief context
- `fields`: Structured data (Block Title, Goal, Status)
- `footer`: Branding
- `timestamp`: When notification was sent
- `components`: Interactive buttons (see Interactivity section below)

### Message Type 2: Issue Stuck Alert

```json
{
  "content": null,
  "embeds": [
    {
      "title": "⚠️ Issue Stuck - Human Intervention Needed",
      "description": "An issue has exceeded its retry limit.",
      "color": 15158332,
      "fields": [
        {
          "name": "Issue",
          "value": "SO-XXX",
          "inline": true
        },
        {
          "name": "Run Count",
          "value": "5",
          "inline": true
        },
        {
          "name": "Action Required",
          "value": "Review the issue and determine if goals need adjustment or manual intervention.",
          "inline": false
        }
      ],
      "footer": {
        "text": "SecondOrder Issue Tracking"
      },
      "timestamp": "2026-04-10T10:00:00Z"
    }
  ]
}
```

**Color Code:**
- Alert/Warning: 15158332 (Orange/Red)

### Message Type 3: Generic Messages

For simple text messages, use basic embed format:

```json
{
  "content": null,
  "embeds": [
    {
      "title": "SecondOrder Notification",
      "description": "[Message text]",
      "color": 9807270,
      "footer": {
        "text": "SecondOrder"
      },
      "timestamp": "2026-04-10T10:00:00Z"
    }
  ]
}
```

**Color Code:**
- Neutral/Info: 9807270 (Gray)

## Interactivity & Button Handling

### Discord Interaction Model
Unlike Telegram's polling model, Discord sends HTTP POST requests when users interact with buttons. However, **the current design does NOT implement Discord's interaction webhook** due to complexity and the need for additional infrastructure.

### Current Implementation Approach
Work block approvals send notifications with visual buttons, but button clicks are not processed. This is acceptable because:

1. The buttons provide context and visual affordance to users
2. Users can approve/reject via the web UI or system commands
3. Implementing Discord interactions would require:
   - Additional webhook endpoint for Discord to POST interactions to
   - Interaction token validation
   - Complex async response handling
   - Discord interaction tokens expire in 3 seconds

### Future Enhancement
If interaction support becomes necessary, implement:
- `/api/webhooks/discord-interactions` endpoint
- Validate Discord interactions via `X-Signature-Ed25519` header
- Process `type: 3` (MESSAGE_COMPONENT) interactions
- Call existing approval handlers with decoded block ID

## Rate Limiting & Error Handling

### Rate Limiting Strategy
Discord rate limits vary by webhook endpoint:
- Default: ~10 requests per second per webhook
- After hitting limit: HTTP 429 response with Retry-After header

**Implementation:**
- Use exponential backoff for HTTP 429 responses
- Max retries: 3 attempts with increasing delays (1s, 2s, 4s)
- Log rate limit hits for monitoring
- Queue messages with channel buffering if needed

### Error Handling
```go
// Scenarios handled:
- Invalid webhook URL -> Log warning, continue (graceful degradation)
- Network timeout -> Retry with backoff, eventually fail silently
- Malformed response -> Log error, continue
- Webhook disabled (404/410) -> Log warning, disable notifier
- Discord API error (5xx) -> Retry with backoff

// Pattern: Never block application flow
// Notifications are fire-and-forget async operations
```

### Retry Policy
- HTTP timeout: 30 seconds (match Telegram timeout)
- Max retries: 3 attempts
- Backoff: 1s, 2s, 4s
- Final failure: Log and continue (don't crash application)

## DiscordNotifier Interface Definition

```go
package handlers

type DiscordNotifier interface {
    // SendWorkBlockApproval sends an interactive work block approval request
    // Parameters:
    //   blockID: unique identifier for the work block
    //   title: display title of the work block
    //   goal: description of the work block goal
    //   transition: state transition ("proposed" or "ready_to_ship")
    SendWorkBlockApproval(blockID, title, goal, transition string) error
    
    // SendMessage sends a simple text notification
    // Parameters:
    //   text: message content (plain text)
    SendMessage(text string) error
}
```

## Implementation Package Structure

```
internal/discord/
├── notifier.go        # Main notifier implementation
├── message.go         # Message formatting & embedding
└── notifier_test.go   # Unit tests
```

### File Descriptions

**notifier.go:**
- `Notifier` struct with `webhookURL` and `*http.Client`
- `New()` constructor with URL validation
- `SendWorkBlockApproval()` implementation
- `SendMessage()` implementation
- Error handling and retry logic

**message.go:**
- Helper functions to build Discord embeds
- Color constants
- Timestamp formatting
- Message validation

## Configuration UI Integration

The existing admin settings UI (`internal/handlers/ui.go`) already handles Discord configuration:

- Discord webhook URL input field (text)
- Feature flag toggle (checkbox)
- Settings appear in "feature_flags" section

No changes needed to UI handler code; configuration model is pre-integrated.

## Testing Strategy

### Unit Tests (`internal/discord/notifier_test.go`)

1. **Valid Message Sending**
   - Mock http.Client
   - Verify POST to correct webhook URL
   - Verify correct JSON payload structure

2. **Error Scenarios**
   - Network timeout
   - HTTP 429 (rate limited)
   - HTTP 4xx/5xx errors
   - Invalid webhook URL

3. **Message Formatting**
   - Work block approval embed structure
   - Stuck issue alert formatting
   - Generic message formatting

4. **Retry Logic**
   - Verify backoff timing
   - Verify max retry count

### Integration Tests
- Handler tests already exist in `internal/handlers/handlers_test.go`
- Use `stubDiscord` pattern (already exists) for integration testing
- Test notification triggers from issue/work block operations

## Implementation Checklist

- [ ] Create `internal/discord/notifier.go` with Notifier struct and methods
- [ ] Create `internal/discord/message.go` with embed builders
- [ ] Create `internal/discord/notifier_test.go` with unit tests
- [ ] Update `cmd/secondorder/main.go` to initialize Discord notifier
- [ ] Verify handler integration (already references `a.discord`)
- [ ] Test with real Discord webhook
- [ ] Document webhook creation in admin guide

## Migration Notes

- No database schema changes required (uses existing settings table)
- Feature flag defaults to `false` (opt-in)
- Webhook URL stored as plain text (consider encryption in future)
- No user data changes needed

## Future Enhancements

1. **Discord Interactions Support**
   - Implement interaction webhook endpoint
   - Process button clicks for direct approval/rejection
   - Add slash commands for manual triggering

2. **Message Threading**
   - Group related notifications in Discord threads
   - Link work block updates to original notification

3. **Rich Formatting**
   - Expand embeds with more contextual data
   - Add work block history summaries

4. **Rate Limit Handling**
   - Implement token bucket queue
   - Persistent queue for failed messages

5. **Webhook Management**
   - Test webhook connectivity from UI
   - Automatic re-creation if webhook becomes invalid

6. **Encryption**
   - Encrypt webhook URL at rest (with key rotation)
   - Add webhook secrets/signatures for security

## Comparison with Telegram Implementation

| Aspect | Telegram | Discord |
|--------|----------|---------|
| **Authentication** | API token + chat ID | Webhook URL |
| **Polling** | Long polling in background | N/A (webhook push) |
| **Interactivity** | Callback buttons + polling | Interaction webhook (future) |
| **Message Format** | Markdown text | Rich embeds with components |
| **Startup** | Spawns goroutine for polling | Simple HTTP initialization |
| **Error Recovery** | Built-in retry in polling loop | Explicit retry with backoff |
| **Configuration** | Two separate settings | Single webhook URL |

## Discord Webhook Creation Guide

Users must create a Discord webhook manually:

1. Go to Discord server settings → Integrations → Webhooks
2. Click "New Webhook"
3. Select target channel
4. Copy webhook URL
5. Paste into SecondOrder admin settings
6. Enable feature flag
7. Test with a work block notification

Documentation to be added to admin guide.
