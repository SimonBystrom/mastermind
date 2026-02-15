package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/git"
)

// setupIntegrationRepo creates a real git repo for integration testing.
func setupIntegrationRepo(t *testing.T) (repoPath, worktreeDir string) {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %s (%v)", args, out, err)
		}
	}

	wtDir := filepath.Join(dir, ".worktrees")
	os.MkdirAll(wtDir, 0o755)
	return dir, wtDir
}

func TestFullLifecycle_SpawnAndDismiss(t *testing.T) {
	repo, wtDir := setupIntegrationRepo(t)

	// Use real git but mock tmux (no real tmux available in CI)
	mt := &mockTmux{windowIDForPane: "@1", paneExistsResult: false}
	mm := &mockMonitor{}

	store := agent.NewStore()
	o := New(
		context.Background(),
		store,
		repo,
		"test-session",
		wtDir,
		WithGit(git.RealGit{}),
		WithTmux(mt),
		WithMonitor(mm),
	)

	// Spawn with new branch
	err := o.SpawnAgent("integ-agent", "feat/integration", "HEAD", true)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	agents := store.All()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	a := agents[0]
	if a.Branch != "feat/integration" {
		t.Errorf("branch = %q", a.Branch)
	}

	// Verify branch was actually created
	if !git.BranchExists(repo, "feat/integration") {
		t.Error("branch should exist in real repo")
	}

	// Verify worktree was created
	if _, err := os.Stat(a.WorktreePath); os.IsNotExist(err) {
		t.Error("worktree directory should exist")
	}

	// Dismiss with branch deletion
	err = o.DismissAgent(a.ID, true)
	if err != nil {
		t.Fatalf("DismissAgent: %v", err)
	}

	// Verify cleanup
	if len(store.All()) != 0 {
		t.Error("store should be empty after dismiss")
	}
	if git.BranchExists(repo, "feat/integration") {
		t.Error("branch should be deleted after dismiss")
	}
}

func TestFullLifecycle_SpawnMergeCycle(t *testing.T) {
	repo, wtDir := setupIntegrationRepo(t)

	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}

	store := agent.NewStore()
	o := New(
		context.Background(),
		store,
		repo,
		"test-session",
		wtDir,
		WithGit(git.RealGit{}),
		WithTmux(mt),
		WithMonitor(mm),
	)

	defaultBranch, _ := git.CurrentBranch(repo)

	// Spawn agent on new branch
	err := o.SpawnAgent("merge-agent", "feat/merge-test", defaultBranch, true)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	agents := store.All()
	a := agents[0]

	// Create a commit in the worktree
	filePath := filepath.Join(a.WorktreePath, "test.txt")
	os.WriteFile(filePath, []byte("test content"), 0o644)
	for _, args := range [][]string{
		{"git", "-C", a.WorktreePath, "add", "test.txt"},
		{"git", "-C", a.WorktreePath, "commit", "-m", "test commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s (%v)", args, out, err)
		}
	}

	// Record base HEAD before merge
	baseHeadBefore, _ := git.HeadCommit(repo, defaultBranch)

	// Merge
	result := o.MergeAgent(a.ID, true, true)
	if !result.Success {
		t.Fatalf("merge failed: %s", result.Error)
	}

	// Verify base branch advanced
	baseHeadAfter, _ := git.HeadCommit(repo, defaultBranch)
	if baseHeadAfter == baseHeadBefore {
		t.Error("base branch should have advanced after merge")
	}

	// Verify agent is cleaned up
	if len(store.All()) != 0 {
		t.Error("agent should be removed after merge")
	}

	// Verify branch is deleted
	if git.BranchExists(repo, "feat/merge-test") {
		t.Error("branch should be deleted after merge")
	}
}
