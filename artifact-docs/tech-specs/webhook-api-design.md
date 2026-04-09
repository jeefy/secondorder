# Webhook API Design

**Issue:** SO-6
**Date:** 2026-04-09
**Status:** Proposed
**Author:** Architect Agent

## Overview

This document specifies the webhook ingestion API for secondorder. External systems (GitHub, n8n, Zapier) send HTTP POST requests to create issues and add comments. Authentication uses HMAC-SHA256 signature verification, separate from the existing agent API key system.

---

## 1. Endpoint Design

### 1.1 Create Issue from Webhook

```
POST /api/v1/webhooks/issues
```

Creates a new Issue. Accepts both a generic payload format and GitHub `issues.opened` event format.

### 1.2 Add Comment to Issue

```
POST /api/v1/webhooks/issues/{key}/comments
```

Adds a Comment to an existing Issue identified by its key (e.g., `SO-42`).

Both endpoints require HMAC-SHA256 signature verification (see Section 3).

---

## 2. Payload Schemas

### 2.1 Create Issue — Generic Format

**Request:**

```json
{
  "title": "string (required, max 500 chars)",
  "description": "string (optional, max 50000 chars)",
  "type": "string (optional, one of: task, bug, feature; default: task)",
  "priority": "integer (optional, 0-4; default: 0)",
  "source": "string (optional, e.g. 'github', 'n8n', 'zapier')",
  "external_id": "string (optional, for idempotency — e.g. GitHub issue URL)"
}
```

**Response (201 Created):**

```json
{
  "key": "SO-42",
  "title": "Fix login timeout",
  "status": "todo",
  "type": "bug",
  "external_id": "https://github.com/org/repo/issues/99"
}
```

**Idempotency:** If `external_id` is provided and an issue with that `external_id` already exists, return `200 OK` with the existing issue instead of creating a duplicate. This makes webhook retries safe.

### 2.2 Create Issue — GitHub `issues.opened` Format

The endpoint detects GitHub webhook payloads by the presence of `action` and `issue` top-level fields. When `action == "opened"` and `issue` is present, the handler maps:

| GitHub field | secondorder field |
|---|---|
| `issue.title` | `title` |
| `issue.body` | `description` |
| `issue.html_url` | `external_id` |
| (inferred) | `type: "task"` |
| (inferred) | `source: "github"` |

GitHub payloads with `action != "opened"` are accepted but ignored (return `200 OK` with `{"ignored": true, "reason": "unhandled action"}`).

### 2.3 Add Comment — Generic Format

**Request:**

```json
{
  "body": "string (required, max 50000 chars)"
}
```

**Response (201 Created):**

```json
{
  "id": "uuid",
  "issue_key": "SO-42",
  "author": "Webhook",
  "body": "Deployed to staging",
  "created_at": "2026-04-09T14:30:00Z"
}
```

### 2.4 Add Comment — GitHub `issue_comment.created` Format

Detected by `action == "created"` and `comment` top-level field. Maps:

| GitHub field | secondorder field |
|---|---|
| `comment.body` | `body` |
| `comment.user.login` | Prepended to body as `"[@login]: body"` |

The `{key}` path parameter identifies the target issue. For GitHub integration, the caller (n8n/Zapier) is responsible for mapping the GitHub issue to the secondorder issue key.

---

## 3. Authentication — HMAC-SHA256 Signature Verification

### 3.1 Mechanism

Webhook requests are authenticated via an HMAC-SHA256 signature passed in the `X-Webhook-Signature` header.

**Header format:**

```
X-Webhook-Signature: sha256=<hex-encoded HMAC-SHA256 digest>
```

**Verification algorithm:**

```
1. Read the raw request body bytes
2. Compute HMAC-SHA256(raw_body, webhook_secret)
3. Hex-encode the result
4. Compare with the value after "sha256=" prefix using constant-time comparison
5. If mismatch → 401 Unauthorized
```

This mirrors GitHub's webhook signature scheme (`X-Hub-Signature-256`), so GitHub-native integrations can use the same secret for both.

### 3.2 Secret Provisioning and Storage

**Scope:** Single global webhook secret (MVP). Per-source secrets can be added later.

**Configuration:** The webhook secret is stored in the `settings` table (existing key-value store used for `discord_webhook_url`, feature flags, etc.):

| Setting key | Value |
|---|---|
| `webhook_secret` | Raw secret string (min 32 characters) |

**Provisioning flow:**

1. Admin sets the secret via the Settings UI page (`/settings`) or directly in the database.
2. The secret is loaded at startup and cached in the `API` struct. A settings change triggers a reload.
3. The same secret is configured in the external system (GitHub webhook settings, n8n credential, Zapier connection).

**Why global, not per-webhook:** The MVP has two endpoints from a single logical integration surface. Per-source secrets add storage and rotation complexity without meaningful security benefit at this scale. The `source` field in the payload distinguishes origin when needed.

### 3.3 Secret Rotation

To rotate without downtime:

1. Set a new secret in settings.
2. Update the external system's secret.
3. During the transition window, the middleware can optionally accept either the old or new secret (dual-verification). This is an optional enhancement — for MVP, a brief downtime during rotation is acceptable.

---

## 4. Routing and Middleware Integration

### 4.1 New Middleware: `WebhookAuth`

The existing `api.Auth()` middleware extracts an agent from a Bearer token. Webhook endpoints need a different middleware that verifies HMAC signatures and does **not** inject an agent into the context (webhooks are not agent-scoped).

```go
// WebhookAuth middleware verifies HMAC-SHA256 signatures on webhook requests.
func (a *API) WebhookAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Read and buffer the request body (needed for both verification and handler)
        body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookPayloadSize+1))
        if err != nil || len(body) > maxWebhookPayloadSize {
            jsonError(w, "payload too large", http.StatusRequestEntityTooLarge)
            return
        }

        // 2. Extract signature from header
        sigHeader := r.Header.Get("X-Webhook-Signature")
        if sigHeader == "" {
            jsonError(w, "missing signature", http.StatusUnauthorized)
            return
        }

        // 3. Verify HMAC-SHA256
        if !verifyHMAC(body, sigHeader, a.webhookSecret) {
            jsonError(w, "invalid signature", http.StatusUnauthorized)
            return
        }

        // 4. Replace request body with buffered copy for downstream handler
        r.Body = io.NopCloser(bytes.NewReader(body))
        next(w, r)
    }
}
```

### 4.2 Route Registration in `main.go`

Add webhook routes after the existing API routes block:

```go
// Webhook routes (HMAC-auth, no agent context)
mux.HandleFunc("POST /api/v1/webhooks/issues", api.WebhookAuth(api.WebhookCreateIssue))
mux.HandleFunc("POST /api/v1/webhooks/issues/{key}/comments", api.WebhookAuth(api.WebhookCreateComment))
```

This follows the existing pattern of `middleware(handler)` wrapping used throughout the codebase.

### 4.3 Handler Implementation Notes

**`WebhookCreateIssue`:** Reuses the same `db.CreateIssue()`, `db.NextIssueKey()`, SSE broadcast, and activity logging as `api.CreateIssue`. Key differences:
- No agent in context — author is `"Webhook"` for activity logs.
- Supports GitHub payload detection and field mapping.
- Adds `external_id` based idempotency check (new column on `issues` table, nullable, with unique index).
- Auto-assigns to CEO agent (same as existing `CreateIssue` default behavior).

**`WebhookCreateComment`:** Reuses `db.CreateComment()` and SSE broadcast. Key differences:
- No agent in context — author is `"Webhook"`.
- No permission check (existing `CreateComment` checks agent assignment).
- Supports GitHub `issue_comment.created` payload detection.

### 4.4 API Struct Changes

Add the webhook secret to the `API` struct:

```go
type API struct {
    db            *db.DB
    sse           *SSEHub
    tmpl          *template.Template
    wake          func(agent *models.Agent, issue *models.Issue)
    telegram      TelegramNotifier
    discord       DiscordNotifier
    webhookSecret string  // HMAC-SHA256 shared secret for webhook verification
}
```

The secret is loaded from `database.GetSetting("webhook_secret")` during `NewAPI()` initialization.

---

## 5. Error Handling

### 5.1 Error Response Format

All error responses use the existing `jsonError` format for consistency:

```json
{
  "error": "human-readable error message"
}
```

### 5.2 Status Codes

| Status | Condition |
|---|---|
| `200 OK` | Idempotent replay (issue with `external_id` already exists), or ignored GitHub action |
| `201 Created` | Issue or comment successfully created |
| `400 Bad Request` | Malformed JSON, missing required fields, validation failure |
| `401 Unauthorized` | Missing or invalid `X-Webhook-Signature` |
| `404 Not Found` | Issue key in path does not exist (comment endpoint) |
| `409 Conflict` | Issue with same title already exists (and no `external_id` for idempotency) |
| `413 Payload Too Large` | Request body exceeds size limit |
| `429 Too Many Requests` | Rate limit exceeded |
| `500 Internal Server Error` | Unexpected server error |

### 5.3 Idempotency

- **Issue creation:** The `external_id` field provides idempotency. If a webhook fires twice for the same GitHub issue, the second request returns the existing issue rather than creating a duplicate.
- **Comment creation:** Comments are append-only and do not have natural idempotency keys. Duplicate comments from retries are acceptable — the caller should implement client-side deduplication if needed. An optional `idempotency_key` field can be added in a future iteration.

---

## 6. Security Considerations

### 6.1 Payload Size Limits

```go
const maxWebhookPayloadSize = 256 * 1024 // 256 KB
```

The `WebhookAuth` middleware enforces this limit by using `io.LimitReader` before reading the body. GitHub webhook payloads are typically under 25 KB; 256 KB provides headroom for large issue descriptions.

### 6.2 Rate Limiting

Apply a per-IP rate limit to webhook endpoints:

- **Limit:** 60 requests per minute per source IP.
- **Implementation:** A simple in-memory token bucket per IP, implemented as middleware wrapping the webhook routes. This can be upgraded to Redis-backed rate limiting if the deployment scales beyond a single instance.
- **Response:** `429 Too Many Requests` with `Retry-After` header.

```go
mux.HandleFunc("POST /api/v1/webhooks/issues",
    api.RateLimit(api.WebhookAuth(api.WebhookCreateIssue)))
mux.HandleFunc("POST /api/v1/webhooks/issues/{key}/comments",
    api.RateLimit(api.WebhookAuth(api.WebhookCreateComment)))
```

### 6.3 Replay Protection

HMAC-SHA256 does not inherently prevent replay attacks. Mitigations:

1. **Timestamp validation (recommended):** Require a `X-Webhook-Timestamp` header (Unix epoch seconds). Reject requests where `|now - timestamp| > 300` seconds (5-minute window). The timestamp is included in the HMAC computation to bind it to the signature.
   - Updated HMAC computation: `HMAC-SHA256(timestamp + "." + body, secret)`
   - This matches Stripe's and Slack's webhook patterns.
2. **Rate limiting** (Section 6.2) bounds the damage from replayed requests.
3. **Idempotency** (Section 5.3) ensures replayed issue-creation requests don't create duplicates.

### 6.4 Input Validation

- All string fields are length-bounded (title: 500, description/body: 50000).
- `type` must be one of the defined constants (`task`, `bug`, `feature`).
- `priority` must be an integer 0-4.
- `{key}` path parameter is validated against existing issues before processing.
- JSON decoding uses `json.NewDecoder` with `DisallowUnknownFields()` disabled (to tolerate extra fields from GitHub payloads).

### 6.5 No Agent Context

Webhook handlers do not operate in an agent context. This means:
- No agent permissions are checked (the HMAC signature is the sole authorization).
- Activity logs and comments use `"Webhook"` as the author.
- The `wake` callback is still invoked to notify the assigned agent of new issues.

---

## 7. Database Changes

### New Migration: `018_webhook_external_id.sql`

```sql
ALTER TABLE issues ADD COLUMN external_id TEXT;
CREATE UNIQUE INDEX idx_issues_external_id ON issues(external_id) WHERE external_id IS NOT NULL;
```

This adds the nullable `external_id` column used for webhook idempotency. The partial unique index ensures uniqueness only for non-null values.

The `webhook_secret` setting is stored in the existing `settings` table — no schema change needed.

---

## 8. Implementation Sequence

Recommended order for SO-7 (implementation):

1. Add `external_id` column migration.
2. Add `webhookSecret` field to `API` struct; load from settings.
3. Implement `verifyHMAC()` helper and `WebhookAuth` middleware.
4. Implement `WebhookCreateIssue` handler (generic format first, then GitHub mapping).
5. Implement `WebhookCreateComment` handler (generic format first, then GitHub mapping).
6. Register routes in `main.go`.
7. Add rate limiting middleware.
8. Add timestamp-based replay protection.

---

## 9. Example Requests

### Generic Issue Creation

```bash
# Compute signature
BODY='{"title":"Deploy failed on staging","type":"bug","source":"n8n"}'
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | cut -d' ' -f2)

curl -X POST http://localhost:3001/api/v1/webhooks/issues \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature: sha256=$SIG" \
  -d "$BODY"
```

### GitHub Issue Webhook

```bash
# GitHub sends this automatically when configured:
# POST /api/v1/webhooks/issues
# X-Hub-Signature-256: sha256=<computed>
# Content-Type: application/json
{
  "action": "opened",
  "issue": {
    "title": "Bug: login page 500 error",
    "body": "Steps to reproduce:\n1. Go to /login\n2. Click submit\n3. See 500",
    "html_url": "https://github.com/org/repo/issues/99"
  },
  "repository": {
    "full_name": "org/repo"
  }
}
```

### Add Comment

```bash
BODY='{"body":"Deployed fix to staging, please verify"}'
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | cut -d' ' -f2)

curl -X POST http://localhost:3001/api/v1/webhooks/issues/SO-42/comments \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature: sha256=$SIG" \
  -d "$BODY"
```
