# Webhook Integration Guide

This guide explains how to integrate your tools and workflows with secondorder using webhooks. Webhooks allow external systems to push events (issues, comments, etc.) into secondorder in real-time.

## Table of Contents

- [Quick Start](#quick-start)
- [Authentication](#authentication)
- [API Reference](#api-reference)
- [Integration Guides](#integration-guides)
- [Troubleshooting](#troubleshooting)
- [Examples](#examples)

## Quick Start

### 1. Get Your Webhook Secret

Configure a shared secret for your webhook source in your secondorder environment:

```bash
export WEBHOOK_SECRET_GITHUB="your-secret-key-here"
```

The environment variable format is `WEBHOOK_SECRET_{SOURCE}`, where `{SOURCE}` is uppercase (e.g., `WEBHOOK_SECRET_GITHUB`, `WEBHOOK_SECRET_N8N`).

### 2. Send Your First Webhook

```bash
curl -X POST http://localhost:3001/api/v1/webhooks/github/issues \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature-256: sha256=YOUR_SIGNATURE_HERE" \
  -d '{
    "title": "Bug: Login fails with special characters",
    "description": "When using special characters in password, login fails",
    "type": "bug",
    "priority": 1
  }'
```

## Authentication

### HMAC-SHA256 Signature (Recommended)

All webhook requests should include an `X-Webhook-Signature-256` header containing an HMAC-SHA256 signature of the request body.

**Header Format:**
```
X-Webhook-Signature: sha256=<hex_digest>
X-Webhook-Signature-256: sha256=<hex_digest>
```

**Note:** GitHub webhooks use `X-Hub-Signature-256` which is also accepted.

### Computing the Signature

The signature is computed as HMAC-SHA256 of the raw request body using your webhook secret as the key.

#### Python

```python
import hmac
import hashlib
import json

secret = "your-webhook-secret"
body = json.dumps({
    "title": "Bug: Login fails",
    "description": "User unable to login",
    "type": "bug",
    "priority": 1
})

# Compute HMAC-SHA256 signature
signature = hmac.new(
    secret.encode(),
    body.encode(),
    hashlib.sha256
).hexdigest()

print(f"sha256={signature}")
```

#### JavaScript / Node.js

```javascript
const crypto = require('crypto');

const secret = "your-webhook-secret";
const body = JSON.stringify({
    title: "Bug: Login fails",
    description: "User unable to login",
    type: "bug",
    priority: 1
});

// Compute HMAC-SHA256 signature
const signature = crypto
    .createHmac('sha256', secret)
    .update(body)
    .digest('hex');

console.log(`sha256=${signature}`);
```

#### curl (via bash/openssl)

```bash
SECRET="your-webhook-secret"
BODY='{"title":"Bug: Login fails","description":"User unable to login","type":"bug","priority":1}'

SIGNATURE=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | cut -d' ' -f2)
echo "X-Webhook-Signature-256: sha256=$SIGNATURE"
```

### Token-Based Authentication (Alternative)

If HMAC signing is not feasible, you can use a shared token instead:

```bash
curl -X POST http://localhost:3001/api/v1/webhooks/github/issues \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Token: your-webhook-secret" \
  -d '{...}'
```

Configure the token the same way as the secret above.

## API Reference

### Issues Endpoint

**POST** `/api/v1/webhooks/{source}/issues`

Creates or updates an issue in secondorder from a webhook event.

#### Request Headers

| Header | Required | Example |
|--------|----------|---------|
| `Content-Type` | Yes | `application/json` |
| `X-Webhook-Signature-256` | Yes* | `sha256=abc123...` |
| `X-Webhook-Token` | Yes* | `your-secret-token` |
| `X-Webhook-Delivery-ID` | No | `123e4567-e89b-12d3-a456-426614174000` |

*Either signature or token required

#### Request Body (Generic Format)

```json
{
  "title": "string (required)",
  "description": "string (optional)",
  "type": "task|bug|feature (required)",
  "priority": "number 0-3 (optional, default: 0)",
  "issue_key": "string (optional, for updates)",
  "external_id": "string (optional, for idempotency)"
}
```

#### Request Body (GitHub Format)

When sending GitHub-native webhook payloads, the system automatically maps:
- `opened` action → creates new issue
- Issue labels:
  - `bug` → type: `bug`
  - `enhancement` or `feature` → type: `feature`
  - Otherwise → type: `task`

Example GitHub issue webhook:
```json
{
  "action": "opened",
  "issue": {
    "title": "Login button unresponsive",
    "body": "The login button doesn't respond to clicks",
    "labels": [{"name": "bug"}],
    "user": {"login": "alice"}
  },
  "repository": {"full_name": "myorg/myrepo"}
}
```

#### Response Codes

| Code | Description |
|------|-------------|
| `201` | Issue created successfully |
| `200` | Issue updated successfully |
| `400` | Invalid request body or missing required fields |
| `401` | Authentication failed (invalid signature or token) |
| `409` | Conflict (duplicate external_id for different content) |
| `413` | Payload too large (max 1 MB) |
| `429` | Rate limited (120 requests/min per source) |

#### Success Response

```json
{
  "key": "SO-42",
  "title": "Login button unresponsive",
  "created": true
}
```

### Comments Endpoint

**POST** `/api/v1/webhooks/{source}/comments`

Adds a comment to an existing issue in secondorder.

#### Request Headers

Same as Issues endpoint.

#### Request Body (Generic Format)

```json
{
  "issue_key": "SO-1 (required)",
  "author": "string (required)",
  "body": "string (required)",
  "external_id": "string (optional, for idempotency)"
}
```

#### Request Body (GitHub Format)

When sending GitHub-native comment webhook payloads:
- `created` action → creates new comment
- Author extracted from `comment.user.login`

Example GitHub comment webhook:
```json
{
  "action": "created",
  "issue": {
    "number": 123
  },
  "comment": {
    "user": {"login": "bob"},
    "body": "I've confirmed this on Windows 10"
  },
  "repository": {"full_name": "myorg/myrepo"}
}
```

#### Response Codes

| Code | Description |
|------|-------------|
| `201` | Comment created successfully |
| `200` | Comment already exists (idempotent) |
| `400` | Invalid request body or missing required fields |
| `401` | Authentication failed |
| `404` | Issue not found |
| `413` | Payload too large |
| `429` | Rate limited |

#### Success Response

```json
{
  "id": "comment-uuid",
  "issue_key": "SO-1",
  "author": "bob",
  "created": true
}
```

## Integration Guides

### GitHub → n8n → secondorder

This integration allows you to forward GitHub issues and comments to secondorder with flexible payload transformation via n8n.

#### Prerequisites

- GitHub repository with webhook access
- n8n instance (self-hosted or cloud)
- secondorder webhook secret configured

#### Step 1: Create n8n Webhook Trigger

1. In n8n, create a new workflow
2. Add a **Webhook** trigger node
3. Set the method to POST
4. Copy the webhook URL (e.g., `https://n8n.example.com/webhook/github-webhook`)
5. Note the Authentication type (usually "Header Auth")

#### Step 2: Configure GitHub Webhook

1. Go to your GitHub repository settings → **Webhooks**
2. Click **Add webhook**
3. Paste the n8n webhook URL in **Payload URL**
4. Set **Content type** to `application/json`
5. Select events:
   - **Issues** (to sync new issues)
   - **Issue comment** (to sync comments)
6. Click **Add webhook**

#### Step 3: Transform and Forward in n8n

Add nodes to your n8n workflow:

**For Issues:**
```javascript
// Add a Function node with this code:
return {
  json: {
    title: $input.all()[0].json.issue.title,
    description: $input.all()[0].json.issue.body,
    type: $input.all()[0].json.issue.labels.some(l => l.name === 'bug') ? 'bug' :
          $input.all()[0].json.issue.labels.some(l => ['enhancement', 'feature'].includes(l.name)) ? 'feature' :
          'task',
    priority: 0,
    external_id: `github-${$input.all()[0].json.repository.full_name}-${$input.all()[0].json.issue.number}`
  }
};
```

**For Comments:**
```javascript
// Add a Function node with this code:
return {
  json: {
    issue_key: "SO-1", // You'll need to map this based on your logic
    author: $input.all()[0].json.comment.user.login,
    body: $input.all()[0].json.comment.body,
    external_id: `github-comment-${$input.all()[0].json.comment.id}`
  }
};
```

#### Step 4: Add secondorder Webhook Node

1. Add an **HTTP Request** node
2. Set method to **POST**
3. Set URL to `http://localhost:3001/api/v1/webhooks/github/issues` (or `/comments`)
4. Set **Body** type to JSON and pass the transformed data
5. Add header `X-Webhook-Signature-256: sha256=<your-signature>`

Compute the signature in a previous Function node using Node.js crypto:

```javascript
const crypto = require('crypto');
const body = JSON.stringify($input.all()[0].json);
const secret = process.env.WEBHOOK_SECRET_GITHUB;
const sig = crypto.createHmac('sha256', secret).update(body).digest('hex');
return {
  json: { signature: `sha256=${sig}` }
};
```

#### Step 5: Test

Trigger a test webhook from GitHub. You should see:
- The n8n workflow executes
- The secondorder webhook receives the data
- A new issue appears in secondorder

### GitHub → Zapier → secondorder

For teams using Zapier instead of n8n, the process is similar:

1. Create a **Webhook by Zapier** trigger with POST method
2. Configure GitHub to send webhooks to the Zapier URL
3. In the action step, use **Webhooks by Zapier** to POST to secondorder
4. Map GitHub fields to secondorder payload fields:
   - `issue.title` → `title`
   - `issue.body` → `description`
   - `issue.labels` → `type` (map `bug` label to `bug`, `enhancement`/`feature` to `feature`)
5. Add the HMAC signature header using Zapier's built-in encryption tools

### Direct GitHub Webhook to secondorder

You can send GitHub webhooks directly to secondorder if your network allows.

#### Important Limitation

**GitHub comment webhooks do NOT include the secondorder issue key.** To link comments to the correct secondorder issue, you must use n8n or Zapier to transform the GitHub issue number to a secondorder issue key.

For **direct GitHub issue webhooks**, you can send them directly:

1. In GitHub repository settings → **Webhooks** → **Add webhook**
2. Set **Payload URL** to: `http://secondorder.example.com/api/v1/webhooks/github/issues`
3. Set **Content type** to `application/json`
4. Create a webhook secret in GitHub settings
5. Configure the same secret in secondorder: `export WEBHOOK_SECRET_GITHUB="<github-secret>"`
6. GitHub automatically includes `X-Hub-Signature-256` header with the signature
7. Select **Issues** event and click **Add webhook**

GitHub will send issue events directly to secondorder. The system will automatically:
- Map the `opened` action to issue creation
- Extract title, description, and type from issue and labels
- Use GitHub's delivery ID for idempotency

## Troubleshooting

### "Authentication failed" or "invalid signature"

**Possible causes:**
- Webhook secret mismatch between your tool and secondorder
- Using wrong header name (use `X-Webhook-Signature-256`, not `X-Webhook-Signature`)
- Signature computed from different body (check for whitespace differences)
- Header format incorrect (should be `sha256=<hex>`, not just `<hex>`)

**Solution:**
1. Verify environment variable is set: `echo $WEBHOOK_SECRET_GITHUB`
2. Re-compute signature manually using the exact request body
3. Check header names in your integration tool
4. For GitHub, verify you're reading `X-Hub-Signature-256` not `X-Hub-Signature`

### "Rate limited" (429 response)

**Possible causes:**
- Exceeded 120 requests per minute for this webhook source
- Multiple workflows sending to the same endpoint simultaneously

**Solution:**
- Implement request batching in your workflow
- Spread requests over time with delays
- Use webhook delivery IDs to prevent duplicate processing
- Contact support if you need higher limits

### "Payload too large" (413 response)

**Possible causes:**
- Webhook body exceeds 1 MB
- Sending unnecessary fields or large attachments

**Solution:**
- In n8n/Zapier, transform the payload to include only required fields
- Remove `body`, `description` fields if they're very large
- Split large workflows into multiple smaller webhooks

### Issue created but not appearing in secondorder

**Possible causes:**
- Issue created but with wrong project/workspace
- Missing required fields in payload
- Webhook processed but validation error silently ignored

**Solution:**
1. Check the webhook response code (should be 201)
2. Verify all required fields present: `title`, `type`
3. Use a unique `external_id` to track the webhook through logs
4. Check secondorder logs for validation errors
5. Verify the issue isn't in a different workspace

### GitHub comment webhooks failing

**This is expected behavior.** GitHub comment webhooks don't include the secondorder issue key, so they cannot be sent directly to secondorder.

**Solution:**
- Use n8n or Zapier to add a transformation step
- Use a Function node to:
  1. Extract the GitHub issue number from `issue.number`
  2. Look up the corresponding secondorder issue key (via secondorder API or a mapping table)
  3. Include `issue_key` in the comment payload

## Examples

### Example 1: Create a Bug Issue with curl

```bash
#!/bin/bash

SECRET="my-webhook-secret"
ENDPOINT="http://localhost:3001/api/v1/webhooks/github/issues"
BODY='{"title":"Login fails with OAuth","description":"Users cannot login via OAuth provider","type":"bug","priority":2}'

# Compute signature
SIGNATURE=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | cut -d' ' -f2)

# Send webhook
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature-256: sha256=$SIGNATURE" \
  -d "$BODY"
```

### Example 2: Add Comment to Existing Issue

```bash
#!/bin/bash

SECRET="my-webhook-secret"
ENDPOINT="http://localhost:3001/api/v1/webhooks/github/comments"
BODY='{"issue_key":"SO-42","author":"alice","body":"I can reproduce this on macOS 14.2"}'

SIGNATURE=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | cut -d' ' -f2)

curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature-256: sha256=$SIGNATURE" \
  -d "$BODY"
```

### Example 3: Create Feature Request with Idempotency

```bash
#!/bin/bash

SECRET="my-webhook-secret"
ENDPOINT="http://localhost:3001/api/v1/webhooks/github/issues"
BODY='{"title":"Dark mode support","description":"Add dark theme for night users","type":"feature","priority":1,"external_id":"zapier-dark-mode-001"}'

SIGNATURE=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | cut -d' ' -f2)

curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature-256: sha256=$SIGNATURE" \
  -H "X-Webhook-Delivery-ID: zapier-dark-mode-001" \
  -d "$BODY"
```

### Example 4: GitHub Issue Webhook (Raw)

This is what GitHub sends directly to your webhook endpoint:

```json
{
  "action": "opened",
  "issue": {
    "url": "https://api.github.com/repos/octocat/Hello-World/issues/1",
    "id": 1,
    "number": 1347,
    "title": "Found a bug",
    "user": {
      "login": "octocat",
      "id": 1,
      "type": "User"
    },
    "body": "I'm having a problem with this.",
    "labels": [
      {
        "id": 208045946,
        "name": "bug",
        "color": "f29513"
      }
    ]
  },
  "repository": {
    "id": 1296269,
    "name": "Hello-World",
    "full_name": "octocat/Hello-World",
    "owner": {
      "login": "octocat"
    }
  }
}
```

The secondorder webhook processor automatically transforms this to:
- `title`: "Found a bug"
- `description`: "I'm having a problem with this."
- `type`: "bug" (from the `bug` label)
- `priority`: 0 (default)
- `external_id`: "github-octocat/Hello-World-1347" (derived)

---

## Support

For issues or questions about webhook integration:
1. Check the [Troubleshooting](#troubleshooting) section above
2. Review your webhook request headers and signature computation
3. Check secondorder logs for detailed error messages
4. Ensure webhook secrets match between your tool and secondorder configuration

For n8n-specific issues, see: https://docs.n8n.io/workflows/
For Zapier-specific issues, see: https://zapier.com/help
