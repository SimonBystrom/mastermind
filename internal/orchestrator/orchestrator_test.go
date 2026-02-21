package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/git"
	"github.com/simonbystrom/mastermind/internal/tmux"
)

// --- Mock implementations ---

type mockGit struct {
	mu    sync.Mutex
	calls []string

	createBranchErr      error
	createWorktreeResult string
	createWorktreeErr    error
	removeWorktreeErr    error
	isBranchCheckedOut   bool
	isBranchMergedResult bool
	hasChangesResult     bool
	headCommitResult     string
	headCommitErr        error
	mergeInWorktreeConflict bool
	mergeInWorktreeErr   error
	conflictFilesResult  []string
	worktreeForBranch    string
	listBranchesResult   []git.Branch
	checkoutBranchErr    error
	currentBranchResult  string
	currentBranchErr     error
	branchExistsResult   bool
	mergeAbortErr        error
}

func (m *mockGit) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *mockGit) hasCalled(call string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (m *mockGit) CreateBranch(repoPath, branchName, baseBranch string) error {
	m.record("CreateBranch:" + branchName)
	return m.createBranchErr
}

func (m *mockGit) DeleteBranch(repoPath, branchName string) error {
	m.record("DeleteBranch:" + branchName)
	return nil
}

func (m *mockGit) IsBranchCheckedOut(repoPath, branch string) (bool, error) {
	m.record("IsBranchCheckedOut:" + branch)
	return m.isBranchCheckedOut, nil
}

func (m *mockGit) IsBranchMerged(repoPath, branch, baseBranch string) bool {
	m.record("IsBranchMerged:" + branch)
	return m.isBranchMergedResult
}

func (m *mockGit) CreateWorktree(repoPath, worktreeDir, branch string) (string, error) {
	m.record("CreateWorktree:" + branch)
	if m.createWorktreeErr != nil {
		return "", m.createWorktreeErr
	}
	result := m.createWorktreeResult
	if result == "" {
		result = worktreeDir + "/" + branch
	}
	return result, nil
}

func (m *mockGit) RemoveWorktree(repoPath, wtPath string) error {
	m.record("RemoveWorktree:" + wtPath)
	return m.removeWorktreeErr
}

func (m *mockGit) HasChanges(wtPath string) bool {
	m.record("HasChanges")
	return m.hasChangesResult
}

func (m *mockGit) HeadCommit(repoOrWtPath, ref string) (string, error) {
	m.record("HeadCommit:" + ref)
	if m.headCommitErr != nil {
		return "", m.headCommitErr
	}
	result := m.headCommitResult
	if result == "" {
		result = "abc123"
	}
	return result, nil
}

func (m *mockGit) UpdateBranchRef(repoPath, branch, targetCommit string) error {
	m.record("UpdateBranchRef:" + branch)
	return nil
}

func (m *mockGit) MergeInWorktree(wtPath, mergeBranch string) (bool, error) {
	m.record("MergeInWorktree:" + mergeBranch)
	return m.mergeInWorktreeConflict, m.mergeInWorktreeErr
}

func (m *mockGit) MergeFFOnly(wtPath, branch string) error {
	m.record("MergeFFOnly:" + branch)
	return nil
}

func (m *mockGit) ConflictFiles(wtPath string) ([]string, error) {
	m.record("ConflictFiles")
	return m.conflictFilesResult, nil
}

func (m *mockGit) WorktreeForBranch(repoPath, branch string) string {
	m.record("WorktreeForBranch:" + branch)
	return m.worktreeForBranch
}

func (m *mockGit) MergeAbort(wtPath string) error {
	m.record("MergeAbort")
	return m.mergeAbortErr
}

func (m *mockGit) CheckoutBranch(wtPath, branch string) error {
	m.record("CheckoutBranch:" + branch)
	return m.checkoutBranchErr
}

func (m *mockGit) CurrentBranch(repoPath string) (string, error) {
	m.record("CurrentBranch")
	if m.currentBranchErr != nil {
		return "", m.currentBranchErr
	}
	result := m.currentBranchResult
	if result == "" {
		result = "main"
	}
	return result, nil
}

func (m *mockGit) BranchExists(repoPath, branchName string) bool {
	m.record("BranchExists:" + branchName)
	return m.branchExistsResult
}

func (m *mockGit) ListBranches(repoPath string) ([]git.Branch, error) {
	m.record("ListBranches")
	return m.listBranchesResult, nil
}

func (m *mockGit) CopyUncommittedChanges(srcWT, dstWT string) error {
	m.record("CopyUncommittedChanges")
	return nil
}

type mockTmux struct {
	mu    sync.Mutex
	calls []string

	newWindowResult    string
	newWindowErr       error
	splitWindowResult  string
	splitWindowErr     error
	paneExistsResult   bool
	windowIDForPane    string
}

func (m *mockTmux) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *mockTmux) hasCalled(call string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (m *mockTmux) NewWindow(session, name, dir string, command []string) (string, error) {
	m.record("NewWindow:" + name)
	if m.newWindowErr != nil {
		return "", m.newWindowErr
	}
	result := m.newWindowResult
	if result == "" {
		result = "%1"
	}
	return result, nil
}

func (m *mockTmux) SplitWindow(paneID, dir string, horizontal bool, sizePercent int, command []string) (string, error) {
	m.record("SplitWindow:" + paneID)
	if m.splitWindowErr != nil {
		return "", m.splitWindowErr
	}
	result := m.splitWindowResult
	if result == "" {
		result = "%2"
	}
	return result, nil
}

func (m *mockTmux) KillWindow(target string) error {
	m.record("KillWindow:" + target)
	return nil
}

func (m *mockTmux) KillPane(paneID string) error {
	m.record("KillPane:" + paneID)
	return nil
}

func (m *mockTmux) SendKeys(paneID string, keys ...string) error {
	m.record("SendKeys:" + paneID)
	return nil
}

func (m *mockTmux) SelectWindow(target string) error {
	m.record("SelectWindow:" + target)
	return nil
}

func (m *mockTmux) SelectPane(paneID string) error {
	m.record("SelectPane:" + paneID)
	return nil
}

func (m *mockTmux) PaneExistsInWindow(paneID, windowID string) bool {
	m.record("PaneExistsInWindow:" + paneID)
	return m.paneExistsResult
}

func (m *mockTmux) WindowIDForPane(paneID string) (string, error) {
	m.record("WindowIDForPane:" + paneID)
	return m.windowIDForPane, nil
}

func (m *mockTmux) ListPanesInWindow(windowID string) ([]string, error) {
	m.record("ListPanesInWindow:" + windowID)
	return nil, nil
}

type mockMonitor struct {
	mu    sync.Mutex
	calls []string

	paneStatus    tmux.PaneStatus
	paneStatusErr error
}

func (m *mockMonitor) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *mockMonitor) GetPaneStatus(paneID string) (tmux.PaneStatus, error) {
	m.record("GetPaneStatus:" + paneID)
	return m.paneStatus, m.paneStatusErr
}

func (m *mockMonitor) Remove(paneID string) {
	m.record("Remove:" + paneID)
}

// --- Helper ---

func newTestOrch(t *testing.T, mg *mockGit, mt *mockTmux, mm *mockMonitor) *Orchestrator {
	t.Helper()
	dir := t.TempDir()
	store := agent.NewStore()
	return New(
		context.Background(),
		store,
		"/repo",
		"test-session",
		dir,
		WithGit(mg),
		WithTmux(mt),
		WithMonitor(mm),
	)
}

// --- Tests ---

func TestSpawnAgent_Success(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	err := o.SpawnAgent("feat/x", "main", true)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	if !mg.hasCalled("CreateBranch:feat/x") {
		t.Error("expected CreateBranch call")
	}
	if !mg.hasCalled("CreateWorktree:feat/x") {
		t.Error("expected CreateWorktree call")
	}
	if !mt.hasCalled("NewWindow:feat/x") {
		t.Error("expected NewWindow call")
	}

	agents := o.store.All()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Branch != "feat/x" {
		t.Errorf("agent branch = %q", agents[0].Branch)
	}
	if agents[0].GetStatus() != agent.StatusRunning {
		t.Errorf("agent status = %q", agents[0].GetStatus())
	}
}

func TestSpawnAgent_DuplicateBranch(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	err := o.SpawnAgent("feat/x", "main", true)
	if err == nil {
		t.Fatal("expected error for duplicate branch")
	}
}

func TestSpawnAgent_BranchCheckedOut(t *testing.T) {
	mg := &mockGit{isBranchCheckedOut: true}
	mt := &mockTmux{}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	err := o.SpawnAgent("feat/x", "", false)
	if err == nil {
		t.Fatal("expected error for checked-out branch")
	}
}

func TestSpawnAgent_TmuxFails_CleansUpWorktree(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{newWindowErr: fmt.Errorf("tmux error")}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	err := o.SpawnAgent("feat/x", "main", true)
	if err == nil {
		t.Fatal("expected error")
	}

	// Should have cleaned up the worktree
	found := false
	mg.mu.Lock()
	for _, c := range mg.calls {
		if len(c) > 15 && c[:15] == "RemoveWorktree:" {
			found = true
		}
	}
	mg.mu.Unlock()
	if !found {
		t.Error("expected RemoveWorktree call for cleanup")
	}

	if len(o.store.All()) != 0 {
		t.Error("store should be empty after failed spawn")
	}
}

func TestDismissAgent_Success(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{windowIDForPane: "@1", paneExistsResult: true}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	agents := o.store.All()
	id := agents[0].ID

	err := o.DismissAgent(id, false)
	if err != nil {
		t.Fatalf("DismissAgent: %v", err)
	}

	if len(o.store.All()) != 0 {
		t.Error("store should be empty after dismiss")
	}
	if !mt.hasCalled("SendKeys:%1") {
		t.Error("expected SendKeys for graceful shutdown")
	}
}

func TestDismissAgent_NotFound(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	err := o.DismissAgent("nonexistent", false)
	if err == nil {
		t.Fatal("expected error for not-found agent")
	}
}

func TestDismissAgent_WithDeleteBranch(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	agents := o.store.All()
	id := agents[0].ID

	o.DismissAgent(id, true)
	if !mg.hasCalled("DeleteBranch:feat/x") {
		t.Error("expected DeleteBranch call")
	}
}

func TestMergeAgent_NoConflicts(t *testing.T) {
	mg := &mockGit{headCommitResult: "abc123"}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	agents := o.store.All()
	id := agents[0].ID

	result := o.MergeAgent(id, true, true)
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Conflict {
		t.Error("expected no conflict")
	}

	// Agent should be removed from store
	if len(o.store.All()) != 0 {
		t.Error("agent should be removed after merge")
	}
}

func TestMergeAgent_WithConflicts(t *testing.T) {
	mg := &mockGit{
		mergeInWorktreeConflict: true,
		conflictFilesResult:     []string{"a.txt", "b.txt"},
	}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	agents := o.store.All()
	id := agents[0].ID

	result := o.MergeAgent(id, true, true)
	if result.Success {
		t.Error("should not succeed with conflicts")
	}
	if !result.Conflict {
		t.Error("expected conflict flag")
	}
	if len(result.ConflictFiles) != 2 {
		t.Errorf("expected 2 conflict files, got %d", len(result.ConflictFiles))
	}

	// Agent should still be in store with StatusConflicts
	a, ok := o.store.Get(id)
	if !ok {
		t.Fatal("agent should still be in store")
	}
	if a.GetStatus() != agent.StatusConflicts {
		t.Errorf("status = %q, want %q", a.GetStatus(), agent.StatusConflicts)
	}
}

func TestMergeAgent_UncommittedChanges(t *testing.T) {
	mg := &mockGit{hasChangesResult: true}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	o.SpawnAgent("feat/x", "main", true)
	agents := o.store.All()
	id := agents[0].ID

	result := o.MergeAgent(id, true, true)
	if result.Error == "" {
		t.Error("expected error for uncommitted changes")
	}
}

func TestHandleAgentFinished_WithChanges(t *testing.T) {
	mg := &mockGit{hasChangesResult: true}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	a := agent.NewAgent("feat/x", "main", "/wt", "@1", "%1")
	o.store.Add(a)

	o.handleAgentFinished(a, 0)

	if a.GetStatus() != agent.StatusReviewReady {
		t.Errorf("status = %q, want %q", a.GetStatus(), agent.StatusReviewReady)
	}
}

func TestHandleAgentFinished_NoChanges(t *testing.T) {
	mg := &mockGit{hasChangesResult: false}
	mt := &mockTmux{windowIDForPane: "@1"}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	a := agent.NewAgent("feat/x", "main", "/wt", "@1", "%1")
	o.store.Add(a)

	o.handleAgentFinished(a, 0)

	if a.GetStatus() != agent.StatusDone {
		t.Errorf("status = %q, want %q", a.GetStatus(), agent.StatusDone)
	}
}

func TestHandleLazygitClosed_NewCommits(t *testing.T) {
	mg := &mockGit{headCommitResult: "newcommit"}
	mt := &mockTmux{}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	a := agent.NewAgent("feat/x", "main", "/wt", "@1", "%1")
	a.SetPreReviewCommit("oldcommit")
	a.SetLazygitPaneID("%2")
	o.store.Add(a)

	o.handleLazygitClosed(a, agent.StatusReviewing)

	if a.GetStatus() != agent.StatusReviewed {
		t.Errorf("status = %q, want %q", a.GetStatus(), agent.StatusReviewed)
	}
	if a.GetLazygitPaneID() != "" {
		t.Error("lazygit pane ID should be cleared")
	}
}

func TestHandleLazygitClosed_NoNewCommits(t *testing.T) {
	mg := &mockGit{headCommitResult: "samecommit"}
	mt := &mockTmux{}
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	a := agent.NewAgent("feat/x", "main", "/wt", "@1", "%1")
	a.SetPreReviewCommit("samecommit")
	a.SetLazygitPaneID("%2")
	o.store.Add(a)

	o.handleLazygitClosed(a, agent.StatusReviewing)

	if a.GetStatus() != agent.StatusReviewReady {
		t.Errorf("status = %q, want %q", a.GetStatus(), agent.StatusReviewReady)
	}
}

func TestCleanupDeadAgents(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{paneExistsResult: false} // panes don't exist
	mm := &mockMonitor{}
	o := newTestOrch(t, mg, mt, mm)

	// Manually add agents (bypass SpawnAgent since we don't want real tmux)
	a1 := agent.NewAgent("feat/a", "main", "/nonexistent", "@1", "%1")
	a1.ID = "a1"
	a2 := agent.NewAgent("feat/b", "main", "/nonexistent", "@2", "%2")
	a2.ID = "a2"
	o.store.Add(a1)
	o.store.Add(a2)

	results := o.CleanupDeadAgents()
	if len(results) != 2 {
		t.Errorf("expected 2 cleanup results, got %d", len(results))
	}
	if len(o.store.All()) != 0 {
		t.Error("store should be empty after cleanup")
	}
}

func TestRecoverAgents(t *testing.T) {
	mg := &mockGit{}
	mt := &mockTmux{paneExistsResult: true}
	mm := &mockMonitor{}

	dir := t.TempDir()
	store := agent.NewStore()
	o := New(
		context.Background(),
		store,
		"/repo",
		"test-session",
		dir,
		WithGit(mg),
		WithTmux(mt),
		WithMonitor(mm),
	)

	// Create persisted state with an agent whose worktree dir exists
	a := &agent.Agent{
		ID:           "a1",
		Branch:       "feat/r",
		BaseBranch:   "main",
		WorktreePath: dir, // use tempdir as worktree (it exists)
		TmuxWindow:   "@1",
		TmuxPaneID:   "%1",
	}
	a.SetStatus(agent.StatusRunning)
	if err := agent.SaveState(dir+"/mastermind-state.json", []*agent.Agent{a}); err != nil {
		t.Fatal(err)
	}

	o.RecoverAgents()

	agents := o.store.All()
	if len(agents) != 1 {
		t.Fatalf("expected 1 recovered agent, got %d", len(agents))
	}
}
