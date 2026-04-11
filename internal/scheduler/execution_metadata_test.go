package scheduler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- strPtr tests ---

func TestStrPtrNonEmpty(t *testing.T) {
	v := "hello"
	got := strPtr(v)
	if got == nil {
		t.Fatal("expected non-nil pointer for non-empty string")
	}
	if *got != v {
		t.Fatalf("strPtr(%q) = %q, want %q", v, *got, v)
	}
}

func TestStrPtrEmpty(t *testing.T) {
	got := strPtr("")
	if got != nil {
		t.Fatalf("strPtr(\"\") = %v, want nil", got)
	}
}

func TestStrPtrWhitespaceOnly(t *testing.T) {
	got := strPtr("   ")
	if got != nil {
		t.Fatalf("strPtr(whitespace) = %v, want nil", got)
	}
}

// --- gateTargetSnapshot tests ---

func TestGateTargetSnapshotWithIssueKey(t *testing.T) {
	got := gateTargetSnapshot("issue", "SO-66")
	if got == nil {
		t.Fatal("expected non-nil gate target for issue key")
	}
	if *got != "issue:SO-66" {
		t.Fatalf("gateTargetSnapshot = %q, want %q", *got, "issue:SO-66")
	}
}

func TestGateTargetSnapshotIssueKeyTakesPrecedenceOverMode(t *testing.T) {
	got := gateTargetSnapshot("heartbeat", "SO-10")
	if got == nil {
		t.Fatal("expected non-nil gate target")
	}
	if *got != "issue:SO-10" {
		t.Fatalf("gateTargetSnapshot = %q, want issue:SO-10", *got)
	}
}

func TestGateTargetSnapshotModeOnlyNoIssueKey(t *testing.T) {
	got := gateTargetSnapshot("heartbeat", "")
	if got == nil {
		t.Fatal("expected non-nil gate target for mode-only run")
	}
	if *got != "heartbeat" {
		t.Fatalf("gateTargetSnapshot = %q, want heartbeat", *got)
	}
}

func TestGateTargetSnapshotBothEmpty(t *testing.T) {
	got := gateTargetSnapshot("", "")
	if got != nil {
		t.Fatalf("gateTargetSnapshot(\"\", \"\") = %v, want nil", got)
	}
}

// --- captureGitSnapshot tests ---

func TestCaptureGitSnapshotEmptyWorkingDir(t *testing.T) {
	worktree, branch, commit := captureGitSnapshot("")
	if worktree != nil || branch != nil || commit != nil {
		t.Fatalf("captureGitSnapshot(\"\") = (%v, %v, %v), want all nil", worktree, branch, commit)
	}
}

func TestCaptureGitSnapshotWhitespaceWorkingDir(t *testing.T) {
	worktree, branch, commit := captureGitSnapshot("   ")
	if worktree != nil || branch != nil || commit != nil {
		t.Fatalf("captureGitSnapshot(whitespace) = (%v, %v, %v), want all nil", worktree, branch, commit)
	}
}

func TestCaptureGitSnapshotNonGitDirectory(t *testing.T) {
	dir := t.TempDir()
	worktree, branch, commit := captureGitSnapshot(dir)
	if worktree == nil {
		t.Fatal("expected non-nil worktree for a valid directory (even non-git)")
	}
	if *worktree != dir {
		t.Fatalf("worktree = %q, want %q", *worktree, dir)
	}
	// branch and commit should be nil for a non-git dir
	if branch != nil {
		t.Fatalf("branch should be nil for non-git dir, got %q", *branch)
	}
	if commit != nil {
		t.Fatalf("commit should be nil for non-git dir, got %q", *commit)
	}
}

func TestCaptureGitSnapshotGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	dir := t.TempDir()

	// initialize a git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}

	worktree, branch, commit := captureGitSnapshot(dir)

	if worktree == nil {
		t.Fatal("expected non-nil worktree for git repo")
	}
	if *worktree != dir {
		t.Fatalf("worktree = %q, want %q", *worktree, dir)
	}
	if commit == nil {
		t.Fatal("expected non-nil commit SHA for initialized git repo")
	}
	if len(*commit) != 40 {
		t.Fatalf("commit SHA length = %d, want 40 (full SHA): %q", len(*commit), *commit)
	}
	// branch may be "main" or "master" depending on git config
	if branch == nil {
		t.Fatal("expected non-nil branch for git repo with commit")
	}
	if strings.TrimSpace(*branch) == "" {
		t.Fatal("branch should not be empty for non-detached HEAD")
	}
}

func TestCaptureGitSnapshotDetachedHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	dir := t.TempDir()

	// init repo, make two commits, then detach HEAD
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
		{"git", "commit", "--allow-empty", "-m", "second"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}

	// get the first commit hash
	hashCmd := exec.Command("git", "rev-parse", "HEAD~1")
	hashCmd.Dir = dir
	hashOut, err := hashCmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD~1: %v", err)
	}
	firstHash := strings.TrimSpace(string(hashOut))

	// detach HEAD at first commit
	detachCmd := exec.Command("git", "checkout", "--detach", firstHash)
	detachCmd.Dir = dir
	if out, err := detachCmd.CombinedOutput(); err != nil {
		t.Fatalf("detach HEAD: %v\n%s", err, out)
	}

	worktree, branch, commit := captureGitSnapshot(dir)

	if worktree == nil {
		t.Fatal("expected non-nil worktree in detached HEAD state")
	}
	if commit == nil {
		t.Fatal("expected non-nil commit SHA in detached HEAD state")
	}
	if *commit != firstHash {
		t.Fatalf("commit = %q, want %q", *commit, firstHash)
	}
	// branch should be nil for detached HEAD
	if branch != nil && *branch != "" {
		t.Fatalf("branch should be nil or empty for detached HEAD, got %q", *branch)
	}
}

func TestCaptureGitSnapshotDirtyTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}

	// create an untracked file to make the tree dirty
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	// captureGitSnapshot does not track dirty state in the current implementation,
	// but it should still return worktree, branch, and commit successfully
	worktree, branch, commit := captureGitSnapshot(dir)

	if worktree == nil {
		t.Fatal("expected non-nil worktree for dirty git repo")
	}
	if commit == nil {
		t.Fatal("expected non-nil commit for dirty git repo with HEAD commit")
	}
	if branch == nil {
		t.Fatal("expected non-nil branch for dirty git repo")
	}
}
