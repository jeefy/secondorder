# Webhook Ingestion Architecture

**Issue:** SO-2
**Status:** Proposed
**Author:** Architect Agent
**Date:** 2026-04-09

---

## 1. Overview

This document defines the architecture for receiving incoming webhooks from external services (starting with GitHub) and translating them into secondorder Issues and Comments. The design adds new routes to the existing HTTP server in `cmd/secondorder/main.go` rather than introducing a separate service, keeping the single-binary deployment model intact.

---

## 2. Decision: New Routes in Existing API

**Option A — New microservice:** Rejected. secondorder is a single Go binary with an embedded SQLite database. Adding a separate service would require inter-process communication, a shared database, or a message queue — none of which exist today.

**Option B — New routes in existing mux (chosen):** Webhook endpoints are registered alongside existing `/api/v1/*` routes. They share the same `*db.DB`, `*SSEHub`, and wake function, which are required to create issues, post comments, and trigger agent runs.

The webhook routes use a **different auth middleware** than the existing `api.Auth` (Bearer token). Webhooks authenticate via HMAC-SHA256 signature verification on each request.

---

## 3. Request Flow

```
External Service (e.g. GitHub)
    │
    │  POST /api/v1/webhooks/{source}/issues
    │  Headers: X-Webhook-Signature-256: sha256=<hmac>
    │  Body: source-specific JSON payload
    │
    ▼
┌─────────────────────────────────────────────────┐
│  HTTP Server (cmd/secondorder/main.go)          │
│                                                 │
│  1. Route match: /api/v1/webhooks/{source}/*    │
│  2. WebhookAuth middleware                      │
│     a. Read raw body (limit: 1 MB)              │
│     b. Compute HMAC-SHA256(body, shared_secret) │
│     c. Compare with X-Webhook-Signature-256     │
│     d. Reject with 401 if mismatch              │
│  3. Source adapter resolves {source} → adapter   │
│     (e.g. "github" → GitHubAdapter)             │
│  4. Adapter normalizes payload → NormalizedEvent │
│  5. Handler creates Issue or Comment via db.*    │
│  6. SSE broadcast + optional agent wake          │
│  7. Return 200 with created resource key         │
└─────────────────────────────────────────────────┘
```

---

## 4. Authentication: HMAC-SHA256 Signature Verification

Webhook endpoints do **not** use Bearer token auth. Instead, the sending service signs the payload body with a shared secret, and secondorder verifies the signature.

### 4.1 Shared Secret Storage

The shared secret is stored in the existing `secrets` table:

```sql
-- Already exists in 001_init.sql
CREATE TABLE IF NOT EXISTS secrets (
    id         TEXT PRIMARY KEY,
    key        TEXT NOT NULL UNIQUE,
    value      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Secret key convention: `webhook_secret_{source}` (e.g. `webhook_secret_github`).

The secret can also be provided via environment variable `WEBHOOK_SECRET_{SOURCE}` (e.g. `WEBHOOK_SECRET_GITHUB`) as a fallback, matching the existing pattern where `GITHUB_PAT` / `GITHUB_TOKEN` are read from env.

### 4.2 Verification Algorithm

```
WebhookAuth(source string) middleware:
    1. rawBody = io.ReadAll(r.Body)  // capped at 1 MB via http.MaxBytesReader
    2. sigHeader = r.Header.Get("X-Webhook-Signature-256")
       - If empty, also check X-Hub-Signature-256 (GitHub's header name)
    3. secret = db.GetSecret("webhook_secret_" + source)
       - Fallback: os.Getenv("WEBHOOK_SECRET_" + strings.ToUpper(source))
       - If no secret configured: return 500 "webhook not configured for source"
    4. expected = "sha256=" + hex(hmac.New(sha256.New, []byte(secret)).Sum(rawBody))
    5. if !hmac.Equal([]byte(sigHeader), []byte(expected)): return 401
    6. Stash rawBody in request context for handler to parse
```

This matches GitHub's webhook signature format (`sha256=<hex>`). Other sources that use the same HMAC-SHA256 scheme work out of the box.

### 4.3 Rate Limiting

Apply a per-source rate limit using a simple token bucket or sliding window counter stored in memory:

- Default: **120 requests/minute per source**
- Configurable via setting `webhook_rate_limit_{source}` in the `settings` table (migration 005)
- Returns `429 Too Many Requests` when exceeded

Since secondorder is a single process, an in-memory `sync.Map` with atomic counters is sufficient — no need for Redis or external state.

---

## 5. API Contract

### 5.1 Create Issue from Webhook

```
POST /api/v1/webhooks/{source}/issues
```

**Headers:**
| Header | Required | Description |
|--------|----------|-------------|
| `Content-Type` | Yes | `application/json` |
| `X-Webhook-Signature-256` | Yes | `sha256=<hex HMAC of raw body>` |
| `X-Webhook-Delivery` | No | Idempotency key (e.g. GitHub delivery UUID) |

**Request Body (source-specific):** The raw payload from the external service. The source adapter extracts the normalized fields.

**Normalized internal representation** (what the adapter produces):

```json
{
  "external_id": "github:issues:12345",
  "title": "Bug: login page crashes on Safari",
  "description": "Full markdown body from the external issue",
  "type": "bug",
  "priority": 0,
  "source": "github",
  "source_url": "https://github.com/owner/repo/issues/42",
  "metadata": {
    "repo": "owner/repo",
    "author": "octocat",
    "labels": ["bug", "frontend"]
  }
}
```

**Response (201 Created):**

```json
{
  "key": "SO-156",
  "external_id": "github:issues:12345",
  "status": "created"
}
```

**Response (200 OK — duplicate, idempotent):**

```json
{
  "key": "SO-156",
  "external_id": "github:issues:12345",
  "status": "already_exists"
}
```

**Error Responses:**
| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{"error":"invalid payload"}` | Adapter cannot parse body |
| 401 | `{"error":"signature verification failed"}` | HMAC mismatch |
| 404 | `{"error":"unknown source"}` | No adapter for `{source}` |
| 409 | `{"error":"issue with this title already exists","existing_key":"SO-X"}` | Title collision (matches existing `CreateIssue` behavior) |
| 429 | `{"error":"rate limit exceeded"}` | Too many requests |
| 500 | `{"error":"webhook not configured for source"}` | No secret found |

### 5.2 Add Comment from Webhook

```
POST /api/v1/webhooks/{source}/comments
```

**Headers:** Same as above.

**Normalized internal representation:**

```json
{
  "external_id": "github:issue_comment:67890",
  "external_issue_id": "github:issues:12345",
  "issue_key": "SO-156",
  "body": "Comment markdown body",
  "author": "octocat",
  "source": "github",
  "metadata": {
    "repo": "owner/repo",
    "comment_url": "https://github.com/owner/repo/issues/42#issuecomment-67890"
  }
}
```

**Issue resolution order:**
1. If `issue_key` is present in the payload — use directly
2. Else, look up the issue by `external_id` of the parent issue in the `webhook_events` table
3. If not found — return 404

**Response (201 Created):**

```json
{
  "comment_id": "uuid",
  "issue_key": "SO-156",
  "status": "created"
}
```

**Error Responses:**
| Status | Body | Condition |
|--------|------|-----------|
| 400 | `{"error":"invalid payload"}` | Cannot parse |
| 401 | `{"error":"signature verification failed"}` | HMAC mismatch |
| 404 | `{"error":"issue not found"}` | Cannot resolve target issue |
| 429 | `{"error":"rate limit exceeded"}` | Too many requests |

### 5.3 Generic Webhook Event Endpoint (Optional)

For sources that send multiple event types on a single URL (like GitHub's webhook configuration):

```
POST /api/v1/webhooks/{source}
```

This endpoint inspects the event type header (e.g. GitHub's `X-GitHub-Event`) and routes internally:
- `issues` (action: opened) → issue creation
- `issue_comment` (action: created) → comment creation
- Other events → 200 OK with `{"status":"ignored","event":"<type>"}`

This is the **recommended endpoint for GitHub** since GitHub sends all events to a single URL.

---

## 6. Source Adapter Pattern

### 6.1 Interface

```go
// internal/handlers/webhook.go

type NormalizedIssue struct {
    ExternalID  string            // "github:issues:12345"
    Title       string
    Description string
    Type        string            // "task", "bug", "feature"
    Priority    int
    Source      string            // "github", "gitlab", etc.
    SourceURL   string
    Metadata    map[string]string // source-specific key/value pairs
}

type NormalizedComment struct {
    ExternalID       string
    ExternalIssueID  string            // maps to parent issue's external_id
    IssueKey         string            // direct SO key if known
    Body             string
    Author           string
    Source           string
    Metadata         map[string]string
}

type WebhookAdapter interface {
    // Source returns the adapter name (e.g. "github")
    Source() string

    // ParseEvent determines the event type from headers/body and
    // returns the parsed event. Returns ("ignored", nil, nil) for
    // events we don't care about.
    ParseEvent(headers http.Header, body []byte) (eventType string, event any, err error)
}
```

`ParseEvent` returns one of:
- `("issue", *NormalizedIssue, nil)`
- `("comment", *NormalizedComment, nil)`
- `("ignored", nil, nil)` — acknowledge but don't act

### 6.2 GitHub Adapter

```go
// internal/handlers/webhook_github.go

type GitHubAdapter struct{}

func (g *GitHubAdapter) Source() string { return "github" }

func (g *GitHubAdapter) ParseEvent(headers http.Header, body []byte) (string, any, error) {
    eventType := headers.Get("X-GitHub-Event")
    switch eventType {
    case "issues":
        return g.parseIssueEvent(body)
    case "issue_comment":
        return g.parseCommentEvent(body)
    default:
        return "ignored", nil, nil
    }
}
```

### 6.3 GitHub Payload Mapping

**GitHub `issues` event (action: opened) → NormalizedIssue:**

| GitHub Field | secondorder Field | Mapping |
|---|---|---|
| `issue.id` | `ExternalID` | `"github:issues:" + strconv.Itoa(id)` |
| `issue.title` | `Title` | Direct |
| `issue.body` | `Description` | Direct (markdown) |
| `issue.labels[].name` | `Type` | `"bug"` if labels contain "bug"; `"feature"` if "feature" or "enhancement"; else `"task"` |
| `issue.html_url` | `SourceURL` | Direct |
| `repository.full_name` | `Metadata["repo"]` | Direct |
| `issue.user.login` | `Metadata["author"]` | Direct |
| `issue.labels[].name` | `Metadata["labels"]` | Comma-joined |

**GitHub `issue_comment` event (action: created) → NormalizedComment:**

| GitHub Field | secondorder Field | Mapping |
|---|---|---|
| `comment.id` | `ExternalID` | `"github:issue_comment:" + strconv.Itoa(id)` |
| `issue.id` | `ExternalIssueID` | `"github:issues:" + strconv.Itoa(id)` |
| `comment.body` | `Body` | Direct (markdown) |
| `comment.user.login` | `Author` | Direct |
| `comment.html_url` | `Metadata["comment_url"]` | Direct |

**Filtered actions:** Only `action: "opened"` for issues and `action: "created"` for comments are processed. All other actions (edited, closed, deleted, etc.) return `"ignored"`.

---

## 7. Data Model Changes

### 7.1 New Table: `webhook_events`

This table serves two purposes:
1. **Idempotency:** Deduplicating repeated deliveries of the same event
2. **Mapping:** Linking external IDs to secondorder issue keys for comment routing

```sql
-- New migration: 022_webhook_events.sql

CREATE TABLE IF NOT EXISTS webhook_events (
    id              TEXT PRIMARY KEY,
    external_id     TEXT NOT NULL,           -- "github:issues:12345"
    source          TEXT NOT NULL,           -- "github"
    event_type      TEXT NOT NULL,           -- "issue", "comment"
    issue_key       TEXT,                    -- SO-156 (set after issue created)
    delivery_id     TEXT,                    -- X-Webhook-Delivery header value
    status          TEXT NOT NULL DEFAULT 'processed',  -- "processed", "ignored", "failed"
    raw_payload     TEXT,                    -- original JSON (optional, for debugging)
    error_message   TEXT,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webhook_events_external_id
    ON webhook_events(external_id);

CREATE INDEX IF NOT EXISTS idx_webhook_events_source
    ON webhook_events(source);

CREATE INDEX IF NOT EXISTS idx_webhook_events_delivery
    ON webhook_events(delivery_id)
    WHERE delivery_id IS NOT NULL;
```

### 7.2 No Changes to Existing Tables

The `issues` and `comments` tables are unchanged. Webhook-created issues use the same `db.CreateIssue` and `db.CreateComment` paths as agent-created issues. The `webhook_events` table is the only addition.

---

## 8. Error Handling and Idempotency

### 8.1 Idempotency Strategy

Webhooks can be delivered multiple times (network retries, GitHub redelivery button). The system handles this at two levels:

1. **Delivery-level dedup:** If `X-Webhook-Delivery` (or equivalent) is present, check `webhook_events.delivery_id`. If already processed, return the original response with `"status":"already_exists"`.

2. **Entity-level dedup:** Before creating an issue, check `webhook_events.external_id`. If a record exists with `event_type="issue"`, return the existing `issue_key`. This catches redeliveries even without the delivery header.

3. **Title-level dedup:** Falls through to the existing `GetIssueByTitle` check in the `CreateIssue` flow, returning 409 if a title collision occurs.

### 8.2 Error Handling

| Failure Mode | Behavior |
|---|---|
| Invalid HMAC signature | 401 — do not process, do not store |
| Malformed JSON payload | 400 — store in `webhook_events` with `status=failed` for debugging |
| Unknown event type | 200 — acknowledge to prevent retries, store as `status=ignored` |
| DB write failure on issue creation | 500 — do not store webhook_event (so retry will re-attempt) |
| DB write failure on webhook_event only | Log error, still return 201 (the issue was created) |
| Rate limit exceeded | 429 — GitHub will retry with exponential backoff |

### 8.3 Payload Size Limit

Enforce a **1 MB** request body limit using `http.MaxBytesReader`. GitHub payloads are typically 5-20 KB; this limit prevents abuse while allowing large issue bodies.

---

## 9. File Inventory (Implementation Plan)

| File | Action | Description |
|------|--------|-------------|
| `internal/handlers/webhook.go` | **Create** | WebhookAuth middleware, handler functions, adapter registry, NormalizedIssue/Comment types |
| `internal/handlers/webhook_github.go` | **Create** | GitHubAdapter: ParseEvent, payload parsing, field mapping |
| `internal/handlers/webhook_test.go` | **Create** | Tests for auth, routing, idempotency, payload normalization |
| `internal/db/migrations/022_webhook_events.sql` | **Create** | webhook_events table DDL |
| `internal/db/queries.go` | **Modify** | Add: CreateWebhookEvent, GetWebhookEventByExternalID, GetWebhookEventByDeliveryID |
| `internal/models/models.go` | **Modify** | Add: WebhookEvent struct |
| `cmd/secondorder/main.go` | **Modify** | Register webhook routes on mux (no auth wrapper — uses WebhookAuth internally) |

### 9.1 Route Registration

```go
// cmd/secondorder/main.go — after existing API routes

// Webhook routes (HMAC auth, not Bearer token)
webhook := handlers.NewWebhookHandler(database, sse, wake)
mux.HandleFunc("POST /api/v1/webhooks/{source}", webhook.HandleEvent)
mux.HandleFunc("POST /api/v1/webhooks/{source}/issues", webhook.HandleIssueEvent)
mux.HandleFunc("POST /api/v1/webhooks/{source}/comments", webhook.HandleCommentEvent)
```

---

## 10. Extensibility

### 10.1 Adding a New Source

To add support for a new webhook source (e.g. GitLab, Linear, Jira):

1. Create `internal/handlers/webhook_{source}.go` implementing `WebhookAdapter`
2. Register the adapter in the adapter registry map in `webhook.go`
3. Configure the shared secret: insert into `secrets` table with key `webhook_secret_{source}` or set env var `WEBHOOK_SECRET_{SOURCE}`
4. If the source uses a different signature header or algorithm, override in the adapter or add a `VerifySignature` method to the interface

### 10.2 Future: Webhook Registration UI

A UI page at `/settings/webhooks` could be added to:
- Generate and display shared secrets per source
- Show the webhook URL to copy into GitHub/GitLab settings
- Display recent webhook deliveries from `webhook_events` table
- Toggle sources on/off

This is out of scope for the initial implementation but the `webhook_events` table and `secrets` table already support it.

---

## 11. Security Considerations

| Concern | Mitigation |
|---|---|
| **Replay attacks** | Idempotency via `external_id` and `delivery_id` dedup. Optional: reject events older than 5 minutes by checking timestamp header. |
| **Payload tampering** | HMAC-SHA256 signature verification on every request |
| **Secret exposure** | Secrets stored in DB (not in code). Env var fallback follows existing pattern. `secrets.value` is never exposed in API responses. |
| **Denial of service** | 1 MB body limit + per-source rate limiting (120 req/min default) |
| **Injection via payload** | All fields pass through Go's `database/sql` parameterized queries (no raw SQL interpolation). Issue titles/descriptions are treated as plain text by the DB layer. |
| **Unauthorized issue creation** | Only webhook-authenticated requests can create issues through this path. Created issues are assigned to the CEO agent for triage, matching the existing `CreateIssue` flow. |

---

## 12. Open Questions

1. **Raw payload storage:** Should `webhook_events.raw_payload` store the full JSON body? This aids debugging but increases DB size. Recommendation: store it, add a setting to disable, and add a cleanup cron to purge payloads older than 30 days.

2. **Comment author display:** Webhook-created comments use the external author name (e.g. "octocat") in the `author` field with `agent_id = NULL`. This is consistent with how "Board" comments work today. The UI already handles `agent_id = NULL` gracefully.

3. **Issue assignment:** Webhook-created issues should be assigned to the CEO agent for triage (matching existing `CreateIssue` fallback behavior). The CEO then delegates as normal.
