package main

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/msoedov/secondorder/internal/db"
	"github.com/msoedov/secondorder/internal/models"
)

func TestApplyStartupTemplateUsesDefaultAgentTimeout(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	applyStartupTemplate(database, "startup", "claude")

	agents, err := database.ListAgents()
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if len(agents) == 0 {
		t.Fatal("expected seeded agents")
	}
	for _, agent := range agents {
		if agent.TimeoutSec != models.DefaultAgentTimeoutSec {
			t.Fatalf("agent %s timeout = %d, want %d", agent.Slug, agent.TimeoutSec, models.DefaultAgentTimeoutSec)
		}
	}
}

func TestLogStartupDiagnosticsLogsAgentCountAndCommit(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	applyStartupTemplate(database, "startup", "claude")

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	logStartupDiagnostics(database)

	output := buf.String()
	if !strings.Contains(output, "msg=\"loaded agents\"") {
		t.Fatalf("expected loaded agents log, got: %s", output)
	}
	if !strings.Contains(output, "msg=\"startup git commit\"") {
		t.Fatalf("expected startup git commit log, got: %s", output)
	}
	if !strings.Contains(output, "commit="+models.CommitHash) {
		t.Fatalf("expected commit hash log field, got: %s", output)
	}
}
