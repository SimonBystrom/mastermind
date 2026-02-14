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

func IsBranchMerged(repoPath, branch, baseBranch string) bool {
	err := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", branch, baseBranch).Run()
	return err == nil
}

func CurrentBranch(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
