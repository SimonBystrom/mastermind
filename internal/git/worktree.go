package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func CreateWorktree(repoPath, worktreeDir, branch string) (string, error) {
	wtPath := filepath.Join(worktreeDir, branch)
	err := exec.Command("git", "-C", repoPath, "worktree", "add", wtPath, branch).Run()
	if err != nil {
		return "", fmt.Errorf("failed to create worktree at %s for branch %s: %w", wtPath, branch, err)
	}
	return wtPath, nil
}

func RemoveWorktree(repoPath, wtPath string) error {
	err := exec.Command("git", "-C", repoPath, "worktree", "remove", wtPath, "--force").Run()
	if err != nil {
		return fmt.Errorf("failed to remove worktree %s: %w", wtPath, err)
	}
	return nil
}

type Worktree struct {
	Path   string
	Branch string
}

// HasChanges returns true if the worktree at wtPath has any uncommitted changes
// (staged, unstaged, or untracked files).
func HasChanges(wtPath string) bool {
	out, err := exec.Command("git", "-C", wtPath, "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func ListWorktrees(repoPath string) ([]Worktree, error) {
	out, err := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []Worktree
	var current Worktree
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		} else if line == "" && current.Path != "" {
			worktrees = append(worktrees, current)
			current = Worktree{}
		}
	}
	return worktrees, nil
}
