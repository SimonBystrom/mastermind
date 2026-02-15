package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestRepo creates a git repo in a temp dir with an initial empty commit.
func setupTestRepo(t *testing.T) string {
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
	return dir
}

// commitFile creates a file and commits it in the given repo/worktree.
func commitFile(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", dir, "add", filename},
		{"git", "-C", dir, "commit", "-m", message},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s (%v)", args, out, err)
		}
	}
}

func TestCreateBranch(t *testing.T) {
	repo := setupTestRepo(t)

	if err := CreateBranch(repo, "feat/x", "HEAD"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	if !BranchExists(repo, "feat/x") {
		t.Error("branch should exist after creation")
	}
}

func TestBranchExists(t *testing.T) {
	repo := setupTestRepo(t)

	if BranchExists(repo, "nonexistent") {
		t.Error("nonexistent branch should not exist")
	}

	// The default branch (master or main) should exist
	branches, _ := ListBranches(repo)
	if len(branches) == 0 {
		t.Fatal("expected at least one branch")
	}
	if !BranchExists(repo, branches[0].Name) {
		t.Errorf("branch %q should exist", branches[0].Name)
	}
}

func TestDeleteBranch(t *testing.T) {
	repo := setupTestRepo(t)

	CreateBranch(repo, "to-delete", "HEAD")
	if err := DeleteBranch(repo, "to-delete"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if BranchExists(repo, "to-delete") {
		t.Error("branch should not exist after deletion")
	}
}

func TestListBranches(t *testing.T) {
	repo := setupTestRepo(t)
	CreateBranch(repo, "feat/a", "HEAD")
	CreateBranch(repo, "feat/b", "HEAD")

	branches, err := ListBranches(repo)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) < 3 {
		t.Errorf("expected at least 3 branches, got %d", len(branches))
	}

	// Check that exactly one is current
	currentCount := 0
	for _, b := range branches {
		if b.Current {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Errorf("expected exactly 1 current branch, got %d", currentCount)
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := setupTestRepo(t)

	branch, err := CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch == "" {
		t.Error("CurrentBranch should not be empty")
	}
}

func TestHeadCommit(t *testing.T) {
	repo := setupTestRepo(t)

	hash, err := HeadCommit(repo, "HEAD")
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if len(hash) != 40 {
		t.Errorf("expected 40-char hash, got %q (%d chars)", hash, len(hash))
	}
}

func TestIsAncestor(t *testing.T) {
	repo := setupTestRepo(t)

	base, _ := HeadCommit(repo, "HEAD")
	commitFile(t, repo, "f.txt", "data", "second commit")
	head, _ := HeadCommit(repo, "HEAD")

	if !IsAncestor(repo, base, head) {
		t.Error("base should be ancestor of head")
	}
	if IsAncestor(repo, head, base) {
		t.Error("head should not be ancestor of base")
	}
}

func TestUpdateBranchRef(t *testing.T) {
	repo := setupTestRepo(t)
	CreateBranch(repo, "target", "HEAD")

	commitFile(t, repo, "f.txt", "data", "advance")
	newHead, _ := HeadCommit(repo, "HEAD")

	if err := UpdateBranchRef(repo, "target", newHead); err != nil {
		t.Fatalf("UpdateBranchRef: %v", err)
	}

	targetHead, _ := HeadCommit(repo, "target")
	if targetHead != newHead {
		t.Errorf("target HEAD = %q, want %q", targetHead, newHead)
	}
}

func TestIsBranchMerged(t *testing.T) {
	repo := setupTestRepo(t)

	CreateBranch(repo, "feat", "HEAD")
	// feat is at the same commit as the default branch — it IS merged (ancestor)
	defaultBranch, _ := CurrentBranch(repo)
	if !IsBranchMerged(repo, "feat", defaultBranch) {
		t.Error("branch at same commit should be considered merged")
	}

	// Advance feat past the default branch
	commitFile(t, repo, "f.txt", "data", "advance on default")
	UpdateBranchRef(repo, "feat", mustHeadCommit(t, repo, "HEAD"))

	// Now create a commit on default that diverges
	// Actually let's just check: feat is ahead of the default branch baseline
	// but since we updated feat to the same commit, let's make a new commit on feat only
	// Reset: let's use a cleaner approach
	repo2 := setupTestRepo(t)
	defaultBranch2, _ := CurrentBranch(repo2)
	CreateBranch(repo2, "feat2", "HEAD")

	// Advance feat2 with a commit via update-ref so it diverges
	commitFile(t, repo2, "f.txt", "data", "feat commit")
	featHead, _ := HeadCommit(repo2, "HEAD")
	// Go back to default
	exec.Command("git", "-C", repo2, "checkout", defaultBranch2).Run()
	UpdateBranchRef(repo2, "feat2", featHead)

	// feat2 is ahead of default — it is NOT merged into default
	// Actually IsBranchMerged checks if branch is ancestor of base
	// feat2 has commits not in default, so it's not an ancestor
	// But wait, feat2 was set to featHead which was committed on default...
	// Let me simplify:
	repo3 := setupTestRepo(t)
	defaultBranch3, _ := CurrentBranch(repo3)
	CreateBranch(repo3, "unmerged", defaultBranch3)
	// Advance unmerged
	wtDir := filepath.Join(t.TempDir(), "unmerged-wt")
	exec.Command("git", "-C", repo3, "worktree", "add", wtDir, "unmerged").Run()
	commitFile(t, wtDir, "feat.txt", "feat", "feat commit")
	exec.Command("git", "-C", repo3, "worktree", "remove", wtDir, "--force").Run()

	if IsBranchMerged(repo3, "unmerged", defaultBranch3) {
		t.Error("unmerged branch should not be merged")
	}
}

func TestMergeInWorktree_NoConflict(t *testing.T) {
	repo := setupTestRepo(t)
	defaultBranch, _ := CurrentBranch(repo)

	CreateBranch(repo, "feat", defaultBranch)

	// Create worktree for feat
	wtDir := filepath.Join(t.TempDir(), "feat-wt")
	exec.Command("git", "-C", repo, "worktree", "add", wtDir, "feat").Run()
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtDir, "--force").Run()

	// Commit on feat worktree
	commitFile(t, wtDir, "feat.txt", "feature", "feat change")

	// Merge default into feat (no conflicts since no changes on default)
	conflicted, err := MergeInWorktree(wtDir, defaultBranch)
	if err != nil {
		t.Fatalf("MergeInWorktree: %v", err)
	}
	if conflicted {
		t.Error("expected no conflicts")
	}
}

func TestMergeInWorktree_WithConflict(t *testing.T) {
	repo := setupTestRepo(t)
	defaultBranch, _ := CurrentBranch(repo)

	CreateBranch(repo, "feat", defaultBranch)

	// Commit on default
	commitFile(t, repo, "shared.txt", "default version", "default change")

	// Create worktree for feat and commit conflicting change
	wtDir := filepath.Join(t.TempDir(), "feat-wt")
	exec.Command("git", "-C", repo, "worktree", "add", wtDir, "feat").Run()
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtDir, "--force").Run()

	commitFile(t, wtDir, "shared.txt", "feat version", "feat change")

	conflicted, err := MergeInWorktree(wtDir, defaultBranch)
	if err != nil {
		t.Fatalf("MergeInWorktree: %v", err)
	}
	if !conflicted {
		t.Error("expected conflicts")
	}
}

func TestMergeFFOnly(t *testing.T) {
	repo := setupTestRepo(t)
	defaultBranch, _ := CurrentBranch(repo)

	CreateBranch(repo, "feat", defaultBranch)

	// Advance feat
	wtDir := filepath.Join(t.TempDir(), "feat-wt")
	exec.Command("git", "-C", repo, "worktree", "add", wtDir, "feat").Run()
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtDir, "--force").Run()

	commitFile(t, wtDir, "feat.txt", "feature", "feat commit")

	// FF-only merge feat into default (should work since default hasn't moved)
	if err := MergeFFOnly(repo, "feat"); err != nil {
		t.Fatalf("MergeFFOnly: %v", err)
	}

	// Verify default advanced
	defaultHead, _ := HeadCommit(repo, defaultBranch)
	featHead, _ := HeadCommit(wtDir, "HEAD")
	if defaultHead != featHead {
		t.Errorf("default HEAD %q != feat HEAD %q after ff merge", defaultHead, featHead)
	}
}

func TestConflictFiles(t *testing.T) {
	repo := setupTestRepo(t)
	defaultBranch, _ := CurrentBranch(repo)

	CreateBranch(repo, "feat", defaultBranch)

	// Commit conflicting changes
	commitFile(t, repo, "a.txt", "default", "default a")
	commitFile(t, repo, "b.txt", "default", "default b")

	wtDir := filepath.Join(t.TempDir(), "feat-wt")
	exec.Command("git", "-C", repo, "worktree", "add", wtDir, "feat").Run()
	defer exec.Command("git", "-C", repo, "worktree", "remove", wtDir, "--force").Run()

	commitFile(t, wtDir, "a.txt", "feat", "feat a")
	commitFile(t, wtDir, "b.txt", "feat", "feat b")

	conflicted, _ := MergeInWorktree(wtDir, defaultBranch)
	if !conflicted {
		t.Fatal("expected conflicts")
	}

	files, err := ConflictFiles(wtDir)
	if err != nil {
		t.Fatalf("ConflictFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 conflict files, got %d: %v", len(files), files)
	}
}

func mustHeadCommit(t *testing.T, repo, ref string) string {
	t.Helper()
	h, err := HeadCommit(repo, ref)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
