package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestVerifyHMAC(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"title":"test"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name   string
		sig    string
		want   bool
	}{
		{"valid signature", validSig, true},
		{"valid without prefix", strings.TrimPrefix(validSig, "sha256="), true},
		{"invalid signature", "sha256=deadbeef", false},
		{"empty signature", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyHMAC(body, tt.sig, secret)
			if got != tt.want {
				t.Errorf("verifyHMAC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdaptGenericIssue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid issue",
			input: `{"title":"Bug report","description":"Something broke","type":"bug","priority":2}`,
			want:  "Bug report",
		},
		{
			name:    "invalid json",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:  "minimal fields",
			input: `{"title":"Minimal"}`,
			want:  "Minimal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGenericIssue([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGenericIssue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.Title != tt.want {
				t.Errorf("adaptGenericIssue() title = %v, want %v", got.Title, tt.want)
			}
		})
	}
}

func TestAdaptGenericComment(t *testing.T) {
	input := `{"issue_key":"SO-1","author":"webhook-user","body":"A comment from webhook"}`
	got, err := adaptGenericComment([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IssueKey != "SO-1" {
		t.Errorf("issue_key = %v, want SO-1", got.IssueKey)
	}
	if got.Author != "webhook-user" {
		t.Errorf("author = %v, want webhook-user", got.Author)
	}
	if got.Body != "A comment from webhook" {
		t.Errorf("body = %v, want 'A comment from webhook'", got.Body)
	}
}

func TestAdaptGitHubIssue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name: "opened issue",
			input: `{
				"action": "opened",
				"issue": {
					"number": 42,
					"title": "GitHub Issue Title",
					"body": "Issue description from GitHub",
					"state": "open",
					"labels": [{"name": "bug"}],
					"user": {"login": "octocat"}
				}
			}`,
			want: "GitHub Issue Title",
		},
		{
			name: "closed issue ignored",
			input: `{
				"action": "closed",
				"issue": {
					"number": 42,
					"title": "Closed Issue",
					"body": "Body",
					"state": "closed",
					"labels": [],
					"user": {"login": "octocat"}
				}
			}`,
			wantErr: true,
		},
		{
			name: "feature label",
			input: `{
				"action": "opened",
				"issue": {
					"number": 1,
					"title": "New Feature",
					"body": "Please add this",
					"state": "open",
					"labels": [{"name": "enhancement"}],
					"user": {"login": "user"}
				}
			}`,
			want: "New Feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGitHubIssue([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGitHubIssue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got.Title != tt.want {
				t.Errorf("adaptGitHubIssue() title = %v, want %v", got.Title, tt.want)
			}
		})
	}
}

func TestAdaptGitHubIssue_LabelMapping(t *testing.T) {
	payload := `{
		"action": "opened",
		"issue": {
			"number": 1,
			"title": "Bug",
			"body": "",
			"state": "open",
			"labels": [{"name": "bug"}],
			"user": {"login": "u"}
		}
	}`
	got, err := adaptGitHubIssue([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "bug" {
		t.Errorf("type = %v, want bug", got.Type)
	}
}

func TestAdaptGitHubComment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "created comment",
			input: `{
				"action": "created",
				"issue": {"number": 1, "title": "Test Issue"},
				"comment": {
					"body": "This is a comment",
					"user": {"login": "octocat"}
				}
			}`,
		},
		{
			name: "edited comment ignored",
			input: `{
				"action": "edited",
				"issue": {"number": 1, "title": "Test Issue"},
				"comment": {
					"body": "Edited comment",
					"user": {"login": "octocat"}
				}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adaptGitHubComment([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("adaptGitHubComment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if got.Author != "octocat" {
					t.Errorf("author = %v, want octocat", got.Author)
				}
			}
		})
	}
}

func TestWebhookAuth_MissingSignature(t *testing.T) {
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", "secret123")
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create request with valid path value but no signature
	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(`{"title":"test"}`))
	req.SetPathValue("source", "testauth")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "missing signature" {
		t.Errorf("expected 'missing signature' error, got %q", resp["error"])
	}
}

func TestWebhookAuth_InvalidSignature(t *testing.T) {
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", "secret123")
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(`{"title":"test"}`))
	req.SetPathValue("source", "testauth")
	req.Header.Set("X-Webhook-Signature-256", "sha256=invalid")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWebhookAuth_ValidSignature(t *testing.T) {
	secret := "secret123"
	os.Setenv("WEBHOOK_SECRET_TESTAUTH", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_TESTAUTH")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	called := false
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	body := `{"title":"test"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/v1/webhooks/testauth/issues", strings.NewReader(body))
	req.SetPathValue("source", "testauth")
	req.Header.Set("X-Webhook-Signature-256", sig)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler was not called with valid signature")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWebhookAuth_GitHubSignatureHeader(t *testing.T) {
	secret := "gh-secret"
	os.Setenv("WEBHOOK_SECRET_GITHUB", secret)
	defer os.Unsetenv("WEBHOOK_SECRET_GITHUB")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	called := false
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	body := `{"action":"opened","issue":{"title":"test"}}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/api/v1/webhooks/github/issues", strings.NewReader(body))
	req.SetPathValue("source", "github")
	// Use GitHub-specific header
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("handler was not called with valid GitHub signature")
	}
}

func TestWebhookAuth_UnconfiguredSource(t *testing.T) {
	// Make sure env is not set
	os.Unsetenv("WEBHOOK_SECRET_UNKNOWN")

	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}
	handler := wh.WebhookAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/v1/webhooks/unknown/issues", strings.NewReader(`{}`))
	req.SetPathValue("source", "unknown")
	req.Header.Set("X-Webhook-Signature-256", "sha256=abc")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unconfigured source, got %d", w.Code)
	}
}

func TestRateLimiting(t *testing.T) {
	wh := &WebhookAPI{rateCount: make(map[string]*rateBucket)}

	// Should allow up to rateLimitMax requests
	for i := 0; i < rateLimitMax; i++ {
		if !wh.allowRequest("test-source") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Next request should be denied
	if wh.allowRequest("test-source") {
		t.Error("request beyond rate limit should be denied")
	}

	// Different source should still be allowed
	if !wh.allowRequest("other-source") {
		t.Error("different source should not be rate limited")
	}
}
