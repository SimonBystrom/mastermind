package git

import (
	"fmt"
	"os/exec"
	"strings"
)

type Branch struct {
	Name    string
	Current bool
}

func ListBranches(repoPath string) ([]Branch, error) {
	out, err := exec.Command("git", "-C", repoPath, "branch", "--format=%(HEAD)|%(refname:short)").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	var branches []Branch
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		branches = append(branches, Branch{
			Name:    parts[1],
			Current: parts[0] == "*",
		})
	}
	return branches, nil
}

func CreateBranch(repoPath, branchName, baseBranch string) error {
	err := exec.Command("git", "-C", repoPath, "branch", branchName, baseBranch).Run()
	if err != nil {
		return fmt.Errorf("failed to create branch %s from %s: %w", branchName, baseBranch, err)
	}
	return nil
}

func BranchExists(repoPath, branchName string) bool {
	err := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", branchName).Run()
	return err == nil
}

func DeleteBranch(repoPath, branchName string) error {
	out, err := exec.Command("git", "-C", repoPath, "branch", "-D", branchName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %s (%w)", branchName, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func CurrentBranch(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func HeadCommit(repoOrWtPath, ref string) (string, error) {
	out, err := exec.Command("git", "-C", repoOrWtPath, "rev-parse", ref).Output()
	if err != nil {
		return "", fmt.Errorf("failed to rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func IsAncestor(repoPath, ancestor, descendant string) bool {
	err := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", ancestor, descendant).Run()
	return err == nil
}

func UpdateBranchRef(repoPath, branch, targetCommit string) error {
	err := exec.Command("git", "-C", repoPath, "update-ref", "refs/heads/"+branch, targetCommit).Run()
	if err != nil {
		return fmt.Errorf("failed to update-ref %s to %s: %w", branch, targetCommit, err)
	}
	return nil
}

func CheckoutBranch(wtPath, branch string) error {
	out, err := exec.Command("git", "-C", wtPath, "checkout", branch).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout %s: %s (%w)", branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func MergeInWorktree(wtPath, mergeBranch string) (conflicted bool, err error) {
	out, err := exec.Command("git", "-C", wtPath, "merge", mergeBranch).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "CONFLICT") {
			return true, nil
		}
		return false, fmt.Errorf("failed to merge %s: %s (%w)", mergeBranch, strings.TrimSpace(string(out)), err)
	}
	return false, nil
}

func IsBranchCheckedOut(repoPath, branch string) (bool, error) {
	worktrees, err := ListWorktrees(repoPath)
	if err != nil {
		return false, err
	}
	for _, wt := range worktrees {
		if wt.Branch == branch {
			return true, nil
		}
	}
	return false, nil
}
