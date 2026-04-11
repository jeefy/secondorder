package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

type capabilityStubTelegram struct{}

func (s *capabilityStubTelegram) SendWorkBlockApproval(_, _, _, _ string) error { return nil }
func (s *capabilityStubTelegram) SendMessage(_ string) error                    { return nil }

func capabilityTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestAgentCapabilityMatrixHappyPath(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	workingDir := t.TempDir()
	t.Setenv("BACKEND_AGENT_TOKEN", "present")

	agent := &models.Agent{
		Name:          "Backend Engineer",
		Slug:          "backend-engineer",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "BACKEND_AGENT_TOKEN",
		WorkingDir:    workingDir,
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: true,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := d.SetSetting("instance_name", "test-instance"); err != nil {
		t.Fatalf("set instance setting: %v", err)
	}

	rawKey := "so_test_capability_key"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-capability-1", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Run.InstanceName != "test-instance" {
		t.Fatalf("instance_name = %q, want test-instance", resp.Run.InstanceName)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(resp.Agents))
	}

	row := resp.Agents[0]
	if row.AgentSlug != "backend-engineer" {
		t.Fatalf("agent_slug = %q, want backend-engineer", row.AgentSlug)
	}

	cred := findCredential(t, row.Credentials, "cred:backend-engineer:primary_api_key")
	if cred.Status != "verified" {
		t.Fatalf("credential status = %q, want verified", cred.Status)
	}

	workspace := findCapability(t, row.EnvironmentCapabilities, "workspace_access")
	if workspace.Status != "verified" {
		t.Fatalf("workspace status = %q, want verified", workspace.Status)
	}

	patchAction := findCapability(t, row.Capabilities, "archetype_patch_submission")
	if patchAction.Status != "verified" {
		t.Fatalf("patch submission status = %q, want verified", patchAction.Status)
	}

	mergeAction := findCapability(t, row.Capabilities, "merge_pull_request")
	if mergeAction.Status != "unknown" {
		t.Fatalf("merge status = %q, want unknown", mergeAction.Status)
	}
}

func TestAgentCapabilityMatrixUnknownAndUnavailableStates(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "QA Engineer",
		Slug:          "qa-engineer",
		ArchetypeSlug: "qa-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "QA_AGENT_TOKEN",
		WorkingDir:    "/does/not/exist",
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	requester := &models.Agent{
		Name:          "Requester",
		Slug:          "requester",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "REQUESTER_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		ChromeEnabled: false,
		Active:        true,
	}
	if err := d.CreateAgent(requester); err != nil {
		t.Fatalf("create requester: %v", err)
	}

	rawKey := "so_test_capability_key_unknown"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(requester.ID, "run-capability-2", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrix)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp capabilityMatrixResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}

	qaRow := findAgentRow(t, resp.Agents, "qa-engineer")
	cred := findCredential(t, qaRow.Credentials, "cred:qa-engineer:primary_api_key")
	if cred.Status != "unknown" {
		t.Fatalf("credential status = %q, want unknown", cred.Status)
	}

	workspace := findCapability(t, qaRow.EnvironmentCapabilities, "workspace_access")
	if workspace.Status != "unavailable" {
		t.Fatalf("workspace status = %q, want unavailable", workspace.Status)
	}

	chrome := findCapability(t, qaRow.EnvironmentCapabilities, "chrome_mcp_access")
	if chrome.Status != "unavailable" {
		t.Fatalf("chrome status = %q, want unavailable", chrome.Status)
	}
}

func TestAgentCapabilityMatrixContractEndpoint(t *testing.T) {
	d := capabilityTestDB(t)
	hub := NewSSEHub()
	defer hub.Close()

	agent := &models.Agent{
		Name:          "Contract Reader",
		Slug:          "contract-reader",
		ArchetypeSlug: "backend-engineer",
		Runner:        models.RunnerOpenCode,
		Model:         "default",
		ApiKeyEnv:     "CONTRACT_READER_TOKEN",
		WorkingDir:    t.TempDir(),
		MaxTurns:      50,
		TimeoutSec:    1200,
		Active:        true,
	}
	if err := d.CreateAgent(agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rawKey := "so_test_capability_key_contract"
	h := sha256.Sum256([]byte(rawKey))
	if err := d.CreateAPIKey(agent.ID, "run-capability-3", hex.EncodeToString(h[:]), "so_test", time.Hour); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	api := NewAPI(d, hub, nil, nil, &capabilityStubTelegram{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/capability-matrix/contract", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	handler := api.Auth(api.AgentCapabilityMatrixContract)
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	if _, ok := payload["status_values"]["unknown"]; !ok {
		t.Fatalf("contract missing unknown status description")
	}
}

func findAgentRow(t *testing.T, rows []agentCapabilityMatrixRow, slug string) agentCapabilityMatrixRow {
	t.Helper()
	for _, row := range rows {
		if row.AgentSlug == slug {
			return row
		}
	}
	t.Fatalf("agent row not found: %s", slug)
	return agentCapabilityMatrixRow{}
}

func findCapability(t *testing.T, items []capabilityItem, key string) capabilityItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("capability not found: %s", key)
	return capabilityItem{}
}

func findCredential(t *testing.T, items []credentialItem, ref string) credentialItem {
	t.Helper()
	for _, item := range items {
		if item.Ref == ref {
			return item
		}
	}
	t.Fatalf("credential not found: %s", ref)
	return credentialItem{}
}
