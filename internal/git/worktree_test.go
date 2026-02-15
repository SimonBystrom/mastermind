package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreateAndListWorktrees(t *testing.T) {
	repo := setupTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	os.MkdirAll(wtDir, 0o755)

	CreateBranch(repo, "feat/wt-test", "HEAD")

	wtPath, err := CreateWorktree(repo, wtDir, "feat/wt-test")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtPath, "--force").Run()

	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Error("worktree directory should exist")
	}

	worktrees, err := ListWorktrees(repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}

	found := false
	for _, wt := range worktrees {
		if wt.Branch == "feat/wt-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("worktree for feat/wt-test not found in listing")
	}
}

func TestRemoveWorktree(t *testing.T) {
	repo := setupTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	os.MkdirAll(wtDir, 0o755)

	CreateBranch(repo, "feat/rm-test", "HEAD")
	wtPath, _ := CreateWorktree(repo, wtDir, "feat/rm-test")

	if err := RemoveWorktree(repo, wtPath); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory should be removed")
	}
}

func TestHasChanges_Clean(t *testing.T) {
	repo := setupTestRepo(t)

	if HasChanges(repo) {
		t.Error("clean repo should have no changes")
	}
}

func TestHasChanges_Dirty(t *testing.T) {
	repo := setupTestRepo(t)

	os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty"), 0o644)
	if !HasChanges(repo) {
		t.Error("repo with untracked file should have changes")
	}
}

func TestWorktreeForBranch(t *testing.T) {
	repo := setupTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	os.MkdirAll(wtDir, 0o755)

	CreateBranch(repo, "feat/find-me", "HEAD")
	wtPath, _ := CreateWorktree(repo, wtDir, "feat/find-me")
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtPath, "--force").Run()

	found := WorktreeForBranch(repo, "feat/find-me")
	// Resolve symlinks for macOS /var -> /private/var
	resolvedFound, _ := filepath.EvalSymlinks(found)
	resolvedWant, _ := filepath.EvalSymlinks(wtPath)
	if resolvedFound != resolvedWant {
		t.Errorf("WorktreeForBranch = %q, want %q", found, wtPath)
	}

	notFound := WorktreeForBranch(repo, "nonexistent")
	if notFound != "" {
		t.Errorf("WorktreeForBranch for missing branch = %q, want empty", notFound)
	}
}

func TestIsBranchCheckedOut(t *testing.T) {
	repo := setupTestRepo(t)
	defaultBranch, _ := CurrentBranch(repo)

	checked, err := IsBranchCheckedOut(repo, defaultBranch)
	if err != nil {
		t.Fatalf("IsBranchCheckedOut: %v", err)
	}
	if !checked {
		t.Error("current branch should be checked out")
	}

	CreateBranch(repo, "not-checked-out", "HEAD")
	checked, err = IsBranchCheckedOut(repo, "not-checked-out")
	if err != nil {
		t.Fatal(err)
	}
	if checked {
		t.Error("branch that's not checked out should return false")
	}
}
