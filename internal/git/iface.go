package git

// GitOps abstracts git operations so the orchestrator can be tested with mocks.
type GitOps interface {
	CreateBranch(repoPath, branchName, baseBranch string) error
	DeleteBranch(repoPath, branchName string) error
	IsBranchCheckedOut(repoPath, branch string) (bool, error)
	IsBranchMerged(repoPath, branch, baseBranch string) bool
	CreateWorktree(repoPath, worktreeDir, branch string) (string, error)
	RemoveWorktree(repoPath, wtPath string) error
	HasChanges(wtPath string) bool
	HeadCommit(repoOrWtPath, ref string) (string, error)
	UpdateBranchRef(repoPath, branch, targetCommit string) error
	MergeInWorktree(wtPath, mergeBranch string) (bool, error)
	MergeFFOnly(wtPath, branch string) error
	ConflictFiles(wtPath string) ([]string, error)
	WorktreeForBranch(repoPath, branch string) string
	ListBranches(repoPath string) ([]Branch, error)
}

// RealGit delegates to the package-level functions.
type RealGit struct{}

func (RealGit) CreateBranch(repoPath, branchName, baseBranch string) error {
	return CreateBranch(repoPath, branchName, baseBranch)
}

func (RealGit) DeleteBranch(repoPath, branchName string) error {
	return DeleteBranch(repoPath, branchName)
}

func (RealGit) IsBranchCheckedOut(repoPath, branch string) (bool, error) {
	return IsBranchCheckedOut(repoPath, branch)
}

func (RealGit) IsBranchMerged(repoPath, branch, baseBranch string) bool {
	return IsBranchMerged(repoPath, branch, baseBranch)
}

func (RealGit) CreateWorktree(repoPath, worktreeDir, branch string) (string, error) {
	return CreateWorktree(repoPath, worktreeDir, branch)
}

func (RealGit) RemoveWorktree(repoPath, wtPath string) error {
	return RemoveWorktree(repoPath, wtPath)
}

func (RealGit) HasChanges(wtPath string) bool {
	return HasChanges(wtPath)
}

func (RealGit) HeadCommit(repoOrWtPath, ref string) (string, error) {
	return HeadCommit(repoOrWtPath, ref)
}

func (RealGit) UpdateBranchRef(repoPath, branch, targetCommit string) error {
	return UpdateBranchRef(repoPath, branch, targetCommit)
}

func (RealGit) MergeInWorktree(wtPath, mergeBranch string) (bool, error) {
	return MergeInWorktree(wtPath, mergeBranch)
}

func (RealGit) MergeFFOnly(wtPath, branch string) error {
	return MergeFFOnly(wtPath, branch)
}

func (RealGit) ConflictFiles(wtPath string) ([]string, error) {
	return ConflictFiles(wtPath)
}

func (RealGit) WorktreeForBranch(repoPath, branch string) string {
	return WorktreeForBranch(repoPath, branch)
}

func (RealGit) ListBranches(repoPath string) ([]Branch, error) {
	return ListBranches(repoPath)
}
