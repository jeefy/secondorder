package db

import (
	"testing"

	"github.com/msoedov/secondorder/internal/models"
)

func ptrStr(s string) *string { return &s }

// TestRunExecutionMetadataPersisted verifies that execution metadata fields
// (runner_snapshot, model_snapshot, git_worktree_snapshot, git_branch_snapshot,
// git_commit_sha_snapshot, gate_target_snapshot) are written to the DB on
// CreateRun and read back correctly by GetRun.
func TestRunExecutionMetadataPersisted(t *testing.T) {
	d := testDB(t)
	a := makeAgent("meta-agent")
	d.CreateAgent(a)

	issueKey := "SO-66"
	issue := makeIssue(issueKey)
	d.CreateIssue(issue)

	runner := "claude_code"
	model := "sonnet"
	worktree := "/workspace/secondorder"
	branch := "so-66-execution-metadata-qa"
	sha := "0123456789abcdef0123456789abcdef01234567"
	gateTarget := "issue:SO-66"

	r := &models.Run{
		AgentID:        a.ID,
		IssueKey:       &issueKey,
		Mode:           "issue",
		Status:         models.RunStatusRunning,
		RunnerSnapshot: &runner,
		ModelSnapshot:  &model,
		GitWorktree:    &worktree,
		GitBranch:      &branch,
		GitCommitSHA:   &sha,
		GateTarget:     &gateTarget,
	}

	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected non-empty ID after CreateRun")
	}

	got, err := d.GetRun(r.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}

	checkPtr := func(field string, got, want *string) {
		t.Helper()
		if want == nil {
			if got != nil {
				t.Errorf("%s: got %q, want nil", field, *got)
			}
			return
		}
		if got == nil {
			t.Errorf("%s: got nil, want %q", field, *want)
			return
		}
		if *got != *want {
			t.Errorf("%s: got %q, want %q", field, *got, *want)
		}
	}

	checkPtr("RunnerSnapshot", got.RunnerSnapshot, &runner)
	checkPtr("ModelSnapshot", got.ModelSnapshot, &model)
	checkPtr("GitWorktree", got.GitWorktree, &worktree)
	checkPtr("GitBranch", got.GitBranch, &branch)
	checkPtr("GitCommitSHA", got.GitCommitSHA, &sha)
	checkPtr("GateTarget", got.GateTarget, &gateTarget)
}

// TestRunExecutionMetadataLegacyRunNullFields verifies that historical runs
// without execution metadata are readable and return nil for all snapshot fields.
// This is the backward compatibility regression test.
func TestRunExecutionMetadataLegacyRunNullFields(t *testing.T) {
	d := testDB(t)
	a := makeAgent("legacy-meta-agent")
	d.CreateAgent(a)

	// Create a run without any snapshot fields (simulates pre-SO-65 behavior)
	r := &models.Run{
		AgentID: a.ID,
		Mode:    "issue",
		Status:  models.RunStatusCompleted,
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := d.GetRun(r.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}

	if got.RunnerSnapshot != nil {
		t.Errorf("RunnerSnapshot should be nil for legacy run, got %q", *got.RunnerSnapshot)
	}
	if got.ModelSnapshot != nil {
		t.Errorf("ModelSnapshot should be nil for legacy run, got %q", *got.ModelSnapshot)
	}
	if got.GitWorktree != nil {
		t.Errorf("GitWorktree should be nil for legacy run, got %q", *got.GitWorktree)
	}
	if got.GitBranch != nil {
		t.Errorf("GitBranch should be nil for legacy run, got %q", *got.GitBranch)
	}
	if got.GitCommitSHA != nil {
		t.Errorf("GitCommitSHA should be nil for legacy run, got %q", *got.GitCommitSHA)
	}
	if got.GateTarget != nil {
		t.Errorf("GateTarget should be nil for legacy run, got %q", *got.GateTarget)
	}
}

// TestCompleteRunDoesNotOverwriteExecutionMetadata verifies that CompleteRun
// does not mutate run-time snapshot fields (immutability requirement from SO-61 design).
func TestCompleteRunDoesNotOverwriteExecutionMetadata(t *testing.T) {
	d := testDB(t)
	a := makeAgent("immutable-meta-agent")
	d.CreateAgent(a)

	runner := "copilot"
	model := "claude-sonnet-4.6"
	branch := "so-65-backend"
	sha := "abcdef1234567890abcdef1234567890abcdef12"
	gateTarget := "issue:SO-65"

	r := &models.Run{
		AgentID:        a.ID,
		Mode:           "issue",
		Status:         models.RunStatusRunning,
		RunnerSnapshot: &runner,
		ModelSnapshot:  &model,
		GitBranch:      &branch,
		GitCommitSHA:   &sha,
		GateTarget:     &gateTarget,
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	tokens := models.Run{InputTokens: 500, OutputTokens: 200, TotalCostUSD: 0.05}
	if err := d.CompleteRun(r.ID, models.RunStatusCompleted, "done output", "diff content", tokens); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got, err := d.GetRun(r.ID)
	if err != nil {
		t.Fatalf("GetRun after complete: %v", err)
	}

	// Verify execution metadata is unchanged after completion
	if got.RunnerSnapshot == nil || *got.RunnerSnapshot != runner {
		t.Errorf("RunnerSnapshot after complete = %v, want %q", got.RunnerSnapshot, runner)
	}
	if got.ModelSnapshot == nil || *got.ModelSnapshot != model {
		t.Errorf("ModelSnapshot after complete = %v, want %q", got.ModelSnapshot, model)
	}
	if got.GitBranch == nil || *got.GitBranch != branch {
		t.Errorf("GitBranch after complete = %v, want %q", got.GitBranch, branch)
	}
	if got.GitCommitSHA == nil || *got.GitCommitSHA != sha {
		t.Errorf("GitCommitSHA after complete = %v, want %q", got.GitCommitSHA, sha)
	}
	if got.GateTarget == nil || *got.GateTarget != gateTarget {
		t.Errorf("GateTarget after complete = %v, want %q", got.GateTarget, gateTarget)
	}

	// Verify run was actually completed
	if got.Status != models.RunStatusCompleted {
		t.Errorf("Status after complete = %q, want %q", got.Status, models.RunStatusCompleted)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set after CompleteRun")
	}
}

// TestListRunsForIssueIncludesExecutionMetadata verifies that ListRunsForIssue
// correctly scans execution metadata fields from the DB.
func TestListRunsForIssueIncludesExecutionMetadata(t *testing.T) {
	d := testDB(t)
	a := makeAgent("list-meta-agent")
	d.CreateAgent(a)

	issueKey := "SO-66-list"
	issue := makeIssue(issueKey)
	d.CreateIssue(issue)

	runner := "opencode"
	model := "github-copilot/claude-sonnet-4.6"
	sha := "fedcba9876543210fedcba9876543210fedcba98"
	gateTarget := "issue:SO-66-list"

	r := &models.Run{
		AgentID:        a.ID,
		IssueKey:       &issueKey,
		Mode:           "issue",
		Status:         models.RunStatusCompleted,
		RunnerSnapshot: &runner,
		ModelSnapshot:  &model,
		GitCommitSHA:   &sha,
		GateTarget:     &gateTarget,
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	runs, err := d.ListRunsForIssue(issueKey)
	if err != nil {
		t.Fatalf("ListRunsForIssue: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}

	got := runs[0]
	if got.RunnerSnapshot == nil || *got.RunnerSnapshot != runner {
		t.Errorf("RunnerSnapshot = %v, want %q", got.RunnerSnapshot, runner)
	}
	if got.ModelSnapshot == nil || *got.ModelSnapshot != model {
		t.Errorf("ModelSnapshot = %v, want %q", got.ModelSnapshot, model)
	}
	if got.GitCommitSHA == nil || *got.GitCommitSHA != sha {
		t.Errorf("GitCommitSHA = %v, want %q", got.GitCommitSHA, sha)
	}
	if got.GateTarget == nil || *got.GateTarget != gateTarget {
		t.Errorf("GateTarget = %v, want %q", got.GateTarget, gateTarget)
	}
}

// TestListRunsForAgentIncludesExecutionMetadata verifies ListRunsForAgent
// correctly scans execution metadata.
func TestListRunsForAgentIncludesExecutionMetadata(t *testing.T) {
	d := testDB(t)
	a := makeAgent("agent-meta-agent")
	d.CreateAgent(a)

	runner := "gemini"
	model := "gemini-2.0-flash"
	worktree := "/home/user/projects/secondorder"

	r := &models.Run{
		AgentID:        a.ID,
		Mode:           "heartbeat",
		Status:         models.RunStatusCompleted,
		RunnerSnapshot: &runner,
		ModelSnapshot:  &model,
		GitWorktree:    &worktree,
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	runs, err := d.ListRunsForAgent(a.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsForAgent: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}

	got := runs[0]
	if got.RunnerSnapshot == nil || *got.RunnerSnapshot != runner {
		t.Errorf("RunnerSnapshot = %v, want %q", got.RunnerSnapshot, runner)
	}
	if got.ModelSnapshot == nil || *got.ModelSnapshot != model {
		t.Errorf("ModelSnapshot = %v, want %q", got.ModelSnapshot, model)
	}
	if got.GitWorktree == nil || *got.GitWorktree != worktree {
		t.Errorf("GitWorktree = %v, want %q", got.GitWorktree, worktree)
	}
}

// TestRunExecutionMetadataPartialCapture verifies that a run can be created
// with only some snapshot fields set (partial capture scenario), and that
// unset fields come back as nil.
func TestRunExecutionMetadataPartialCapture(t *testing.T) {
	d := testDB(t)
	a := makeAgent("partial-meta-agent")
	d.CreateAgent(a)

	// Simulate partial capture: runner/model set but no git context
	runner := "ollama"
	model := "llama3@localhost:11434"

	r := &models.Run{
		AgentID:        a.ID,
		Mode:           "issue",
		Status:         models.RunStatusRunning,
		RunnerSnapshot: &runner,
		ModelSnapshot:  &model,
		// GitWorktree, GitBranch, GitCommitSHA, GateTarget intentionally nil
	}
	if err := d.CreateRun(r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	got, err := d.GetRun(r.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}

	if got.RunnerSnapshot == nil || *got.RunnerSnapshot != runner {
		t.Errorf("RunnerSnapshot = %v, want %q", got.RunnerSnapshot, runner)
	}
	if got.ModelSnapshot == nil || *got.ModelSnapshot != model {
		t.Errorf("ModelSnapshot = %v, want %q", got.ModelSnapshot, model)
	}
	if got.GitWorktree != nil {
		t.Errorf("GitWorktree should be nil for partial capture, got %q", *got.GitWorktree)
	}
	if got.GitBranch != nil {
		t.Errorf("GitBranch should be nil for partial capture, got %q", *got.GitBranch)
	}
	if got.GitCommitSHA != nil {
		t.Errorf("GitCommitSHA should be nil for partial capture, got %q", *got.GitCommitSHA)
	}
	if got.GateTarget != nil {
		t.Errorf("GateTarget should be nil for partial capture, got %q", *got.GateTarget)
	}
}
