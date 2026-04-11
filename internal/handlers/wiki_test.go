package handlers

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

// --- Wiki API Handler Tests ---

func createTestWikiAgent(t *testing.T, d *db.DB, slug string) *models.Agent {
	agent := &models.Agent{
		Name:          "Wiki Test Agent " + slug,
		Slug:          slug,
		ArchetypeSlug: "worker",
		Model:         "sonnet",
		WorkingDir:    "/tmp",
		MaxTurns:      50,
		TimeoutSec:    600,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create test agent: %v", err)
	}
	return agent
}

func createTestWikiAPIKey(t *testing.T, d *db.DB, agent *models.Agent) string {
	rawKey := "so_wiki_test_" + agent.Slug
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])
	if err := d.CreateAPIKey(agent.ID, "wiki-test-"+agent.Slug, keyHash, "so_wiki_test", 60*time.Minute); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return rawKey
}

func setupWikiTest(t *testing.T) (*db.DB, *API, *models.Agent, string) {
	d := testDB(t)
	hub := NewSSEHub()
	t.Cleanup(func() { hub.Close() })
	api := NewAPI(d, hub, nil, nil, &stubTelegram{}, nil)

	agent := createTestWikiAgent(t, d, "wiki-agent")
	apiKey := createTestWikiAPIKey(t, d, agent)

	return d, api, agent, apiKey
}

// --- Acceptance Criteria Tests ---

func TestListWikiPagesEmpty(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result []models.WikiPageSummary
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 pages, got %d", len(result))
	}
}

func TestListWikiPages(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	// Create some wiki pages
	page1 := &models.WikiPage{
		Slug:             "page1",
		Title:            "First Page",
		Content:          "Content 1",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	page2 := &models.WikiPage{
		Slug:             "page2",
		Title:            "Second Page",
		Content:          "Content 2",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page1)
	d.CreateWikiPage(page2)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result []models.WikiPageSummary
	json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 2 {
		t.Errorf("expected 2 pages, got %d", len(result))
	}
}

func TestCreateWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title":   "New Page",
		"content": "This is new content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "new-page" {
		t.Errorf("slug = %q, want new-page", result.Slug)
	}
	if result.Title != "New Page" {
		t.Errorf("title = %q, want New Page", result.Title)
	}
	if result.ID == "" {
		t.Error("expected ID to be set")
	}

	// Verify agent identity
	if result.CreatedByAgentID == nil || *result.CreatedByAgentID != agent.ID {
		t.Errorf("CreatedByAgentID = %v, want %s", result.CreatedByAgentID, agent.ID)
	}

	// Double check in DB
	dbPage, _ := d.GetWikiPageBySlug("new-page")
	if dbPage.CreatedByAgentID == nil || *dbPage.CreatedByAgentID != agent.ID {
		t.Errorf("DB CreatedByAgentID = %v, want %s", dbPage.CreatedByAgentID, agent.ID)
	}
}

func TestWikiHandlersNoAuth(t *testing.T) {
	_, api, _, _ := setupWikiTest(t)

	endpoints := []struct {
		method string
		url    string
		slug   string
		h      http.HandlerFunc
	}{
		{"GET", "/api/v1/wiki", "", api.ListWikiPages},
		{"POST", "/api/v1/wiki", "", api.CreateWikiPage},
		{"GET", "/api/v1/wiki/slug", "slug", api.GetWikiPage},
		{"PATCH", "/api/v1/wiki/slug", "slug", api.UpdateWikiPage},
		{"DELETE", "/api/v1/wiki/slug", "slug", api.DeleteWikiPage},
		{"GET", "/api/v1/wiki/search", "", api.SearchWikiPages},
	}

	for _, tt := range endpoints {
		t.Run(tt.method+" "+tt.url, func(t *testing.T) {
			// 1. Missing header
			req := httptest.NewRequest(tt.method, tt.url, nil)
			if tt.slug != "" {
				req.SetPathValue("slug", tt.slug)
			}
			w := httptest.NewRecorder()
			http.HandlerFunc(api.Auth(tt.h))(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("missing header: status = %d, want 401", w.Code)
			}

			// 2. Invalid token
			req = httptest.NewRequest(tt.method, tt.url, nil)
			if tt.slug != "" {
				req.SetPathValue("slug", tt.slug)
			}
			req.Header.Set("Authorization", "Bearer invalid-token")
			w = httptest.NewRecorder()
			http.HandlerFunc(api.Auth(tt.h))(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("invalid token: status = %d, want 401", w.Code)
			}
		})
	}
}

func TestUpdateWikiPageAgentIdentity(t *testing.T) {
	d, api, agent1, _ := setupWikiTest(t)

	// Create a second agent
	agent2 := createTestWikiAgent(t, d, "wiki-agent-2")
	apiKey2 := createTestWikiAPIKey(t, d, agent2)

	// Create page with agent 1
	page := &models.WikiPage{
		Slug:             "identity-test",
		Title:            "Agent 1 Title",
		Content:          "Agent 1 Content",
		CreatedByAgentID: &agent1.ID,
		UpdatedByAgentID: &agent1.ID,
	}
	d.CreateWikiPage(page)

	// Update with agent 2
	body := map[string]string{
		"title": "Agent 2 Update",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/identity-test", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey2)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "identity-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	// Verify identity in DB
	dbPage, _ := d.GetWikiPageBySlug("identity-test")
	if dbPage.CreatedByAgentID == nil || *dbPage.CreatedByAgentID != agent1.ID {
		t.Errorf("CreatedByAgentID should still be %s, got %v", agent1.ID, dbPage.CreatedByAgentID)
	}
	if dbPage.UpdatedByAgentID == nil || *dbPage.UpdatedByAgentID != agent2.ID {
		t.Errorf("UpdatedByAgentID should be %s, got %v", agent2.ID, dbPage.UpdatedByAgentID)
	}
}

func TestCreateWikiPageAutoGeneratesSlug(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title": "No Slug",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestCreateWikiPageMissingTitle(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"content": "Missing title",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateWikiPageDuplicateSlug(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	// Create first page directly in DB
	firstPage := &models.WikiPage{
		Slug:             "duplicate",
		Title:            "First",
		Content:          "Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(firstPage)

	// Try to create second with same slug via API
	body := map[string]string{
		"title": "Duplicate",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
}

func TestGetWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "get-test",
		Title:            "Get Test",
		Content:          "Test content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	req := httptest.NewRequest("GET", "/api/v1/wiki/get-test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "get-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.GetWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "get-test" {
		t.Errorf("slug = %q, want get-test", result.Slug)
	}
	if result.Title != "Get Test" {
		t.Errorf("title = %q, want Get Test", result.Title)
	}
}

func TestGetWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("GET", "/api/v1/wiki/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.GetWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestUpdateWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "update-test",
		Title:            "Original Title",
		Content:          "Original Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	body := map[string]string{
		"title":   "Updated Title",
		"content": "Updated Content",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/update-test", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "update-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Title != "Updated Title" {
		t.Errorf("title = %q, want Updated Title", result.Title)
	}
	if result.Content != "Updated Content" {
		t.Errorf("content = %q, want Updated Content", result.Content)
	}
}

func TestUpdateWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"title": "New Title",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/nonexistent", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteWikiPageSuccess(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "delete-test",
		Title:            "To Delete",
		Content:          "Delete me",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	// Verify it exists
	_, err := d.GetWikiPageBySlug("delete-test")
	if err != nil {
		t.Fatalf("page should exist before delete: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/wiki/delete-test", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "delete-test")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.DeleteWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Verify it's deleted
	_, err = d.GetWikiPageBySlug("delete-test")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestDeleteWikiPageNotFound(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("DELETE", "/api/v1/wiki/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.SetPathValue("slug", "nonexistent")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.DeleteWikiPage))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCreateWikiPageSlugNormalization(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	cases := []struct {
		title    string
		wantSlug string
	}{
		{"What's New?", "what-s-new"},
		{"Hello   World", "hello-world"},
		{"  Leading Trailing  ", "leading-trailing"},
		{"API v2.0 Release!", "api-v2-0-release"},
	}

	for _, tc := range cases {
		body := map[string]string{"title": tc.title}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("title=%q: status = %d, want 201, body=%s", tc.title, w.Code, w.Body.String())
			continue
		}

		var result models.WikiPage
		json.NewDecoder(w.Body).Decode(&result)
		if result.Slug != tc.wantSlug {
			t.Errorf("title=%q: slug = %q, want %q", tc.title, result.Slug, tc.wantSlug)
		}
	}
}

func TestCreateWikiPageExplicitSlugNormalized(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	body := map[string]string{
		"slug":  "My Custom Slug!",
		"title": "Test Page",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/wiki", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.CreateWikiPage))(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", w.Code, w.Body.String())
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "my-custom-slug" {
		t.Errorf("slug = %q, want my-custom-slug", result.Slug)
	}
}

func TestUpdateWikiPageSlug(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "old-slug",
		Title:            "Old Title",
		Content:          "Content",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	newSlug := "new-slug"
	body := map[string]*string{
		"slug": &newSlug,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("PATCH", "/api/v1/wiki/old-slug", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("slug", "old-slug")
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.UpdateWikiPage))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var result models.WikiPage
	json.NewDecoder(w.Body).Decode(&result)
	if result.Slug != "new-slug" {
		t.Errorf("slug = %q, want new-slug", result.Slug)
	}

	// Old slug should 404
	_, err := d.GetWikiPageBySlug("old-slug")
	if err != sql.ErrNoRows {
		t.Errorf("old slug should not exist, got %v", err)
	}
}

func TestListWikiPagesSummaryOmitsContent(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	page := &models.WikiPage{
		Slug:             "summary-test",
		Title:            "Summary Test",
		Content:          "This content should NOT appear in list response",
		CreatedByAgentID: &agent.ID,
		UpdatedByAgentID: &agent.ID,
	}
	d.CreateWikiPage(page)

	req := httptest.NewRequest("GET", "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()

	http.HandlerFunc(api.Auth(api.ListWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Decode as raw JSON to verify content field is absent
	var raw []map[string]any
	json.NewDecoder(w.Body).Decode(&raw)
	if len(raw) != 1 {
		t.Fatalf("expected 1 page, got %d", len(raw))
	}

	if _, hasContent := raw[0]["content"]; hasContent {
		t.Error("list response should not include content field")
	}
	if _, hasSlug := raw[0]["slug"]; !hasSlug {
		t.Error("list response should include slug field")
	}
	if _, hasTitle := raw[0]["title"]; !hasTitle {
		t.Error("list response should include title field")
	}
}

func TestFzfScore(t *testing.T) {
	tests := []struct {
		text    string
		pattern string
		wantHit bool
	}{
		{"deployment-guide", "dpg", true},
		{"deployment-guide", "dep", true},
		{"deployment-guide", "xyz", false},
		{"API Key Rotation", "akr", true},
		{"API Key Rotation", "api", true},
		{"API Key Rotation", "rotation", true},
		{"API Key Rotation", "zzz", false},
		{"hello", "hello", true},
		{"", "a", false},
		{"abc", "", false},
	}
	for _, tt := range tests {
		score, pos := fzfScore(tt.text, tt.pattern)
		got := score > 0
		if got != tt.wantHit {
			t.Errorf("fzfScore(%q, %q) hit=%v want=%v (score=%d pos=%v)", tt.text, tt.pattern, got, tt.wantHit, score, pos)
		}
	}
}

func TestFzfScoreRanking(t *testing.T) {
	// Exact prefix should score higher than scattered match
	prefixScore, _ := fzfScore("api-key-rotation", "api")
	scatterScore, _ := fzfScore("archetype-patches-implementation", "api")
	if prefixScore <= scatterScore {
		t.Errorf("prefix match (%d) should rank higher than scattered (%d)", prefixScore, scatterScore)
	}

	// Exact match should be highest
	exactScore, _ := fzfScore("hello", "hello")
	partialScore, _ := fzfScore("hello world", "hello")
	if exactScore <= partialScore {
		t.Errorf("exact match (%d) should rank higher than partial (%d)", exactScore, partialScore)
	}
}

func TestSearchWikiPages(t *testing.T) {
	d, api, agent, apiKey := setupWikiTest(t)

	pages := []models.WikiPage{
		{Slug: "api-key-rotation", Title: "API Key Rotation", Content: "How to rotate keys", CreatedByAgentID: &agent.ID, UpdatedByAgentID: &agent.ID},
		{Slug: "deployment-guide", Title: "Deployment Guide", Content: "Deploy steps", CreatedByAgentID: &agent.ID, UpdatedByAgentID: &agent.ID},
		{Slug: "security-model", Title: "Security Model", Content: "Our security approach", CreatedByAgentID: &agent.ID, UpdatedByAgentID: &agent.ID},
	}
	for i := range pages {
		if err := d.CreateWikiPage(&pages[i]); err != nil {
			t.Fatalf("create page: %v", err)
		}
	}

	tests := []struct {
		query     string
		wantCount int
		wantFirst string
	}{
		{"api", 1, "api-key-rotation"},
		{"sec", 1, "security-model"},
		{"de", 2, ""},    // matches deployment + security-model (model has no 'de'... actually deployment-guide)
		{"zzz", 0, ""},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/api/v1/wiki/search?q="+tt.query, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		w := httptest.NewRecorder()
		http.HandlerFunc(api.Auth(api.SearchWikiPages))(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("q=%q: status=%d want=200", tt.query, w.Code)
			continue
		}

		var results []struct {
			Slug  string `json:"slug"`
			Score int    `json:"score"`
		}
		if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
			t.Fatalf("q=%q: decode: %v", tt.query, err)
		}
		if len(results) < tt.wantCount {
			t.Errorf("q=%q: got %d results, want >= %d", tt.query, len(results), tt.wantCount)
		}
		if tt.wantFirst != "" && len(results) > 0 && results[0].Slug != tt.wantFirst {
			t.Errorf("q=%q: first result slug=%q want=%q", tt.query, results[0].Slug, tt.wantFirst)
		}
	}
}

func TestSearchWikiPagesEmpty(t *testing.T) {
	_, api, _, apiKey := setupWikiTest(t)

	req := httptest.NewRequest("GET", "/api/v1/wiki/search?q=", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	w := httptest.NewRecorder()
	http.HandlerFunc(api.Auth(api.SearchWikiPages))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status=%d want=200", w.Code)
	}
	var results []any
	json.NewDecoder(w.Body).Decode(&results)
	if len(results) != 0 {
		t.Errorf("empty query should return 0 results, got %d", len(results))
	}
}
