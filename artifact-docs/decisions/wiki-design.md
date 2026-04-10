# Decision: Wiki / Knowledge Base Feature Design

Date: 2026-04-09
Issue: SO-29
Status: PROPOSED

---

## Overview

A built-in wiki / knowledge base that allows both agents and human operators to create, read, update, and delete pages of markdown content. Pages are identified by URL-safe slugs, support hierarchical nesting via `parent_page_id`, and can cross-reference each other with `[[PageSlug]]` link syntax.

---

## 1. Database Schema

### Migration: `023_wiki_pages.sql`

```sql
-- Wiki pages for knowledge base
CREATE TABLE IF NOT EXISTS wiki_pages (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    created_by_type TEXT NOT NULL DEFAULT 'user',  -- 'agent' or 'user'
    created_by_id TEXT,                             -- agent id (nullable for user-created)
    parent_page_id TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (parent_page_id) REFERENCES wiki_pages(id),
    FOREIGN KEY (created_by_id) REFERENCES agents(id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_slug ON wiki_pages(slug);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_parent ON wiki_pages(parent_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_created_at ON wiki_pages(created_at);
```

### Field notes

| Column | Type | Purpose |
|--------|------|---------|
| `id` | TEXT (UUID) | Primary key, consistent with all other tables |
| `slug` | TEXT UNIQUE | URL-safe identifier (e.g. `onboarding-guide`). Used in routes and `[[slug]]` links |
| `title` | TEXT | Human-readable page title |
| `content` | TEXT | Markdown body. Supports `[[PageSlug]]` inter-page links |
| `created_by_type` | TEXT | Discriminator: `agent` or `user` |
| `created_by_id` | TEXT | FK to `agents.id` when `created_by_type = 'agent'`; NULL for user-created pages |
| `parent_page_id` | TEXT | Self-referential FK for hierarchy (nullable = top-level page) |
| `created_at` | DATETIME | Immutable creation timestamp |
| `updated_at` | DATETIME | Updated on every write via application code |

### Slug rules

- Lowercase alphanumeric + hyphens only: `^[a-z0-9]+(?:-[a-z0-9]+)*$`
- Auto-generated from title on create (user can override)
- Must be unique across all pages

---

## 2. REST API Contract

All endpoints require `Authorization: Bearer <token>` (same `api.Auth` middleware as existing API routes).

### `GET /api/v1/wiki`

List all wiki pages (summary, no content body).

**Response** `200 OK`:
```json
[
  {
    "id": "uuid",
    "slug": "onboarding-guide",
    "title": "Onboarding Guide",
    "created_by_type": "agent",
    "created_by_id": "agent-uuid",
    "parent_page_id": null,
    "created_at": "2026-04-09T12:00:00Z",
    "updated_at": "2026-04-09T12:00:00Z"
  }
]
```

### `GET /api/v1/wiki/{slug}`

Get a single page by slug, including full content.

**Response** `200 OK`:
```json
{
  "id": "uuid",
  "slug": "onboarding-guide",
  "title": "Onboarding Guide",
  "content": "# Welcome\n\nSee also [[architecture-overview]].",
  "created_by_type": "agent",
  "created_by_id": "agent-uuid",
  "parent_page_id": null,
  "created_at": "2026-04-09T12:00:00Z",
  "updated_at": "2026-04-09T12:00:00Z"
}
```

**Response** `404 Not Found`:
```json
{"error": "wiki page not found"}
```

### `POST /api/v1/wiki`

Create a new wiki page.

**Request body**:
```json
{
  "title": "Onboarding Guide",
  "slug": "onboarding-guide",
  "content": "# Welcome\n\nThis is the onboarding guide.",
  "parent_page_id": null
}
```

- `slug` is optional; if omitted, auto-generated from `title`
- `created_by_type` and `created_by_id` are set from the authenticated agent context

**Response** `201 Created`: full page object (same shape as GET).

**Response** `409 Conflict`:
```json
{"error": "slug already exists"}
```

### `PATCH /api/v1/wiki/{slug}`

Update an existing page. All fields optional; only provided fields are updated.

**Request body**:
```json
{
  "title": "Updated Title",
  "content": "Updated content...",
  "slug": "new-slug",
  "parent_page_id": "parent-uuid"
}
```

- `updated_at` is set to `CURRENT_TIMESTAMP` on every update

**Response** `200 OK`: full updated page object.

**Response** `404 Not Found`:
```json
{"error": "wiki page not found"}
```

**Response** `409 Conflict` (if new slug collides):
```json
{"error": "slug already exists"}
```

### `DELETE /api/v1/wiki/{slug}`

Delete a page. Child pages (those with `parent_page_id` pointing to this page) are orphaned (their `parent_page_id` set to NULL), not cascade-deleted.

**Response** `200 OK`:
```json
{"ok": true}
```

**Response** `404 Not Found`:
```json
{"error": "wiki page not found"}
```

---

## 3. UI Routes

Follow the existing pattern: `handlers.NewUI` owns these handlers, registered in `main.go`.

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/wiki` | `ui.WikiList` | List all pages with hierarchy |
| `GET` | `/wiki/new` | `ui.WikiNew` | Create form |
| `POST` | `/wiki` | `ui.WikiList` | Handle create (POST body, redirect on success) |
| `GET` | `/wiki/{slug}` | `ui.WikiPage` | View page (rendered markdown) |
| `GET` | `/wiki/{slug}/edit` | `ui.WikiEdit` | Edit form |
| `POST` | `/wiki/{slug}` | `ui.WikiPage` | Handle update (POST body, redirect on success) |

### Notes on UI routing

- `GET /wiki/new` must be registered **before** `GET /wiki/{slug}` in the mux to avoid `new` being captured as a slug. Go 1.22+ `ServeMux` matches more-specific patterns first, so the explicit `/wiki/new` route takes priority.
- Delete is handled via `POST /wiki/{slug}` with a form field `_action=delete` (consistent with how existing entities handle destructive actions via HTMX).
- Templates: `wiki_list.html`, `wiki_page.html`, `wiki_edit.html` (following existing naming in `internal/templates/`).

---

## 4. Inter-Page Link Strategy

### Syntax

Use double-bracket wiki-link syntax inside markdown content:

```
See [[architecture-overview]] for details.
Also check [[onboarding-guide|the onboarding doc]].
```

- `[[slug]]` renders as a link with the page title as display text
- `[[slug|display text]]` renders with custom display text

### Rendering

Links are resolved at **render time**, not at write time. This keeps stored content simple and avoids dangling-reference maintenance.

**Implementation**: a template function or pre-render pass that applies a regex replacement:

```
\[\[([a-z0-9-]+)(?:\|([^\]]+))?\]\]
```

- Look up the slug in the database
- If found: render `<a href="/wiki/{slug}">{display text or page title}</a>`
- If not found: render `<a href="/wiki/{slug}" class="wiki-link-missing">{slug}</a>` (red/dimmed link, signals the page needs to be created)

### API behavior

- The API returns raw markdown with `[[...]]` syntax intact; the caller is responsible for rendering
- The UI templates handle rendering via a Go template function `resolveWikiLinks`

---

## 5. Data Model (Go struct)

Add to `internal/models/models.go`:

```go
type WikiPage struct {
    ID            string    `json:"id"`
    Slug          string    `json:"slug"`
    Title         string    `json:"title"`
    Content       string    `json:"content,omitempty"`
    CreatedByType string    `json:"created_by_type"`
    CreatedByID   *string   `json:"created_by_id"`
    ParentPageID  *string   `json:"parent_page_id"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}
```

---

## 6. Database Methods

Add to `internal/db/db.go` (or a new `wiki.go` file in that package):

| Method | Signature | SQL |
|--------|-----------|-----|
| `ListWikiPages` | `() ([]WikiPage, error)` | `SELECT id,slug,title,created_by_type,created_by_id,parent_page_id,created_at,updated_at FROM wiki_pages ORDER BY title` |
| `GetWikiPage` | `(slug string) (*WikiPage, error)` | `SELECT * FROM wiki_pages WHERE slug=?` |
| `CreateWikiPage` | `(page *WikiPage) error` | `INSERT INTO wiki_pages ...` |
| `UpdateWikiPage` | `(slug string, fields map[string]any) error` | Dynamic `UPDATE wiki_pages SET ... WHERE slug=?` |
| `DeleteWikiPage` | `(slug string) error` | `UPDATE wiki_pages SET parent_page_id=NULL WHERE parent_page_id=(SELECT id FROM wiki_pages WHERE slug=?); DELETE FROM wiki_pages WHERE slug=?` |
| `GetWikiPagesByParent` | `(parentID *string) ([]WikiPage, error)` | `SELECT ... FROM wiki_pages WHERE parent_page_id IS ? ORDER BY title` |

---

## 7. Access Model

- **API access**: All wiki endpoints are behind `api.Auth`, meaning any agent with a valid API key can read/write wiki pages. This is intentional -- agents should be able to document their work, create runbooks, and share knowledge.
- **UI access**: No authentication (consistent with existing UI routes). The operator has full access.
- `created_by_type` tracks provenance: `"agent"` (with `created_by_id` set) or `"user"` (with `created_by_id` NULL).

---

## 8. Future Considerations (out of scope for v1)

- **Page versioning / history**: could add a `wiki_page_revisions` table later
- **Full-text search**: SQLite FTS5 on title + content
- **Tags / categories**: separate `wiki_tags` join table
- **Permissions**: per-page read/write ACLs
- **Markdown extensions**: task lists, tables, syntax highlighting beyond basic rendering
