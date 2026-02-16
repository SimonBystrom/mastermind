package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/git"
	"github.com/simonbystrom/mastermind/internal/tmux"
)

type AgentFinishedMsg struct {
	AgentID    string
	ExitCode   int
	HasChanges bool
}

type AgentWaitingMsg struct {
	AgentID    string
	WaitingFor string // "permission", "input", or "" (no longer waiting)
}

type AgentGoneMsg struct {
	AgentID string
}

type AgentReviewedMsg struct {
	AgentID    string
	NewCommits bool
}

type MergeResultMsg struct {
	AgentID       string
	Success       bool
	Conflict      bool
	Error         string
	ConflictFiles []string
}

type CleanupResult struct {
	AgentName string
	Reason    string // "pane gone", "worktree missing", "branch merged"
}

type CleanupMsg struct {
	Results []CleanupResult
}

type PreviewStartedMsg struct{ AgentID string }
type PreviewStoppedMsg  struct{ AgentID string }
type PreviewErrorMsg    struct{ AgentID string; Error string }

type Orchestrator struct {
	ctx         context.Context
	store       *agent.Store
	repoPath    string
	session     string
	worktreeDir string
	program     *tea.Program
	monitor     tmux.PaneStatusChecker
	statePath   string
	git         git.GitOps
	tmux        tmux.TmuxOps
	lazygitSplit int

	previewAgentID    string // ID of agent being previewed (empty = no preview)
	previewPrevBranch string // branch the main worktree was on before preview
	previewPrevStatus agent.Status // agent's status before preview started
}

// Option configures an Orchestrator.
type Option func(*Orchestrator)

// WithGit overrides the default git implementation.
func WithGit(g git.GitOps) Option {
	return func(o *Orchestrator) { o.git = g }
}

// WithTmux overrides the default tmux implementation.
func WithTmux(t tmux.TmuxOps) Option {
	return func(o *Orchestrator) { o.tmux = t }
}

// WithMonitor overrides the default pane monitor.
func WithMonitor(m tmux.PaneStatusChecker) Option {
	return func(o *Orchestrator) { o.monitor = m }
}

// WithLazygitSplit sets the lazygit pane size percentage.
func WithLazygitSplit(pct int) Option {
	return func(o *Orchestrator) { o.lazygitSplit = pct }
}

func New(ctx context.Context, store *agent.Store, repoPath, session, worktreeDir string, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		ctx:          ctx,
		store:        store,
		repoPath:     repoPath,
		session:      session,
		worktreeDir:  worktreeDir,
		monitor:      tmux.NewPaneMonitor(),
		statePath:    worktreeDir + "/mastermind-state.json",
		git:          git.RealGit{},
		tmux:         tmux.RealTmux{},
		lazygitSplit: 80,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *Orchestrator) SetProgram(p *tea.Program) {
	o.program = p
}

func (o *Orchestrator) SpawnAgent(name, branch, baseBranch string, createBranch bool) error {
	// Guard against worktree name collision
	for _, existing := range o.store.All() {
		if existing.Branch == branch {
			return fmt.Errorf("branch %q already in use by agent %s", branch, existing.ID)
		}
	}

	// Guard against branch already checked out in another worktree (e.g. the main working tree)
	if !createBranch {
		if checkedOut, err := o.git.IsBranchCheckedOut(o.repoPath, branch); err == nil && checkedOut {
			return fmt.Errorf("branch %q is already checked out in another worktree", branch)
		}
	}

	if createBranch {
		if err := o.git.CreateBranch(o.repoPath, branch, baseBranch); err != nil {
			return fmt.Errorf("create branch: %w", err)
		}
	}

	wtPath, err := o.git.CreateWorktree(o.repoPath, o.worktreeDir, branch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	windowName := name
	if windowName == "" {
		windowName = branch
	}

	paneID, err := o.tmux.NewWindow(o.session, windowName, wtPath, []string{"claude"})
	if err != nil {
		o.git.RemoveWorktree(o.repoPath, wtPath)
		return fmt.Errorf("create tmux window: %w", err)
	}

	windowID, _ := o.tmux.WindowIDForPane(paneID)

	a := agent.NewAgent(name, branch, baseBranch, wtPath, windowID, paneID)
	o.store.Add(a)

	slog.Info("agent spawned", "id", a.ID, "branch", branch, "name", name)
	o.saveState()

	return nil
}

func (o *Orchestrator) DismissAgent(id string, deleteBranch bool) error {
	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if a.TmuxPaneID != "" {
		o.monitor.Remove(a.TmuxPaneID)
	}

	// Gracefully stop Claude if the pane is still alive
	if a.TmuxPaneID != "" && o.tmux.PaneExistsInWindow(a.TmuxPaneID, a.TmuxWindow) {
		status := a.GetStatus()
		if status == agent.StatusRunning || status == agent.StatusWaiting {
			// Send Ctrl+C to interrupt, then /exit to quit cleanly
			o.tmux.SendKeys(a.TmuxPaneID, "C-c")
			o.tmux.SendKeys(a.TmuxPaneID, "/exit", "Enter")
			// Give Claude a moment to shut down
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Kill lazygit pane if open
	if lgPane := a.GetLazygitPaneID(); lgPane != "" {
		o.tmux.KillPane(lgPane)
	}

	if a.TmuxWindow != "" {
		o.tmux.KillWindow(a.TmuxWindow)
	}

	if a.WorktreePath != "" {
		o.git.RemoveWorktree(o.repoPath, a.WorktreePath)
	}

	if deleteBranch && a.Branch != "" {
		o.git.DeleteBranch(o.repoPath, a.Branch)
	}

	o.store.Remove(id)

	slog.Info("agent dismissed", "id", id, "deleteBranch", deleteBranch)
	o.saveState()

	return nil
}

func (o *Orchestrator) FocusAgent(id string) error {
	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if err := o.tmux.SelectWindow(a.TmuxWindow); err != nil {
		return fmt.Errorf("select window: %w", err)
	}
	return o.tmux.SelectPane(a.TmuxPaneID)
}

func (o *Orchestrator) OpenLazyGit(id string) error {
	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if err := o.tmux.SelectWindow(a.TmuxWindow); err != nil {
		return fmt.Errorf("select window: %w", err)
	}

	// Record HEAD before review starts
	head, err := o.git.HeadCommit(a.WorktreePath, "HEAD")
	if err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}
	a.SetPreReviewCommit(head)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	paneID, err := o.tmux.SplitWindow(a.TmuxPaneID, a.WorktreePath, true, o.lazygitSplit, []string{shell, "-lc", "export GPG_TTY=$(tty); exec lazygit"})
	if err != nil {
		return fmt.Errorf("split window for lazygit: %w", err)
	}

	a.SetLazygitPaneID(paneID)
	// Callers set the status themselves (allows StatusConflicts to open lazygit without change)
	return nil
}

func (o *Orchestrator) StartMonitor() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			slog.Info("monitor stopped: context cancelled")
			return
		case <-ticker.C:
		}

		agents := o.store.All()
		for _, a := range agents {
			status := a.GetStatus()

			// Handle lazygit pane detection for reviewing/conflicts agents
			if (status == agent.StatusReviewing || status == agent.StatusConflicts) && a.GetLazygitPaneID() != "" {
				lgGone := !o.tmux.PaneExistsInWindow(a.GetLazygitPaneID(), a.TmuxWindow)
				if !lgGone {
					// Pane exists but may be dead (remain-on-exit keeps it around).
					lgStatus, err := o.monitor.GetPaneStatus(a.GetLazygitPaneID())
					lgGone = err != nil || lgStatus.Dead
				}
				if lgGone {
					// Kill the dead lazygit pane if it's still lingering
					o.tmux.KillPane(a.GetLazygitPaneID())
					o.handleLazygitClosed(a, status)
				}
				continue
			}

			switch status {
			case agent.StatusRunning, agent.StatusWaiting,
				agent.StatusReviewReady, agent.StatusDone,
				agent.StatusReviewed:
				// These statuses need monitoring
			default:
				continue
			}

			paneStatus, err := o.monitor.GetPaneStatus(a.TmuxPaneID)
			if err != nil {
				// Pane no longer exists — window was closed externally
				slog.Debug("pane gone, marking dismissed", "id", a.ID, "pane", a.TmuxPaneID)
				o.monitor.Remove(a.TmuxPaneID)
				a.SetStatus(agent.StatusDismissed)
				if o.program != nil {
					o.program.Send(AgentGoneMsg{AgentID: a.ID})
				}
				continue
			}

			if paneStatus.Dead {
				o.handleAgentFinished(a, paneStatus.ExitCode)
				continue
			}

			// No statuses are fully "settled" — the user can always send
			// another prompt to any agent, so we always need to re-classify
			// pane content to detect when an idle agent starts working again.

			if paneStatus.WaitingFor != "" && a.GetEverActive() && o.git.HasChanges(a.WorktreePath) {
				// Agent was active and has changes — review ready, regardless
				// of what the pattern matcher thinks (avoids false "permission"
				// detection when Claude is actually idle with changes).
				o.handleAgentIdle(a)
			} else if paneStatus.WaitingFor == "permission" {
				// Claude needs permission approval
				a.SetEverActive(true)
				if status != agent.StatusWaiting || a.GetWaitingFor() != "permission" {
					a.SetStatus(agent.StatusWaiting)
					a.SetWaitingFor("permission")
					slog.Debug("agent status change", "id", a.ID, "status", "waiting", "waitingFor", "permission")
					if o.program != nil {
						o.program.Send(AgentWaitingMsg{
							AgentID:    a.ID,
							WaitingFor: "permission",
						})
					}
				}
			} else if paneStatus.WaitingFor == "unknown" {
				a.SetEverActive(true)
				if status != agent.StatusWaiting || a.GetWaitingFor() != "unknown" {
					a.SetStatus(agent.StatusWaiting)
					a.SetWaitingFor("unknown")
					slog.Debug("agent status change", "id", a.ID, "status", "waiting", "waitingFor", "unknown")
					if o.program != nil {
						o.program.Send(AgentWaitingMsg{AgentID: a.ID, WaitingFor: "unknown"})
					}
				}
			} else if paneStatus.WaitingFor == "input" {
				// Claude is idle at prompt — only treat as finished if agent was ever active
				if a.GetEverActive() {
					o.handleAgentIdle(a)
				}
			} else {
				// Claude is actively working
				a.SetEverActive(true)
				if status != agent.StatusRunning {
					a.SetStatus(agent.StatusRunning)
					a.SetWaitingFor("")
					slog.Debug("agent status change", "id", a.ID, "status", "running")
				}
			}
		}
		o.saveState()
	}
}

func (o *Orchestrator) handleAgentFinished(a *agent.Agent, exitCode int) {
	a.SetFinished(exitCode, time.Now())

	hasChanges := o.git.HasChanges(a.WorktreePath)
	if hasChanges {
		a.SetStatus(agent.StatusReviewReady)
	} else {
		a.SetStatus(agent.StatusDone)
	}

	slog.Info("agent finished", "id", a.ID, "exitCode", exitCode, "hasChanges", hasChanges)

	if o.program != nil {
		o.program.Send(AgentFinishedMsg{
			AgentID:    a.ID,
			ExitCode:   exitCode,
			HasChanges: hasChanges,
		})
	}
}

func (o *Orchestrator) handleAgentIdle(a *agent.Agent) {
	hasChanges := o.git.HasChanges(a.WorktreePath)
	if hasChanges {
		if a.GetStatus() != agent.StatusReviewReady {
			a.SetStatus(agent.StatusReviewReady)
			a.SetFinished(a.GetExitCode(), time.Now())
			slog.Info("agent idle with changes", "id", a.ID)
			if o.program != nil {
				o.program.Send(AgentFinishedMsg{
					AgentID:    a.ID,
					HasChanges: true,
				})
			}
		}
	} else {
		if a.GetStatus() != agent.StatusDone {
			a.SetStatus(agent.StatusDone)
			a.SetFinished(a.GetExitCode(), time.Now())
			slog.Info("agent idle without changes", "id", a.ID)
			if o.program != nil {
				o.program.Send(AgentFinishedMsg{
					AgentID:    a.ID,
					HasChanges: false,
				})
			}
		}
	}
}

func (o *Orchestrator) handleLazygitClosed(a *agent.Agent, status agent.Status) {
	a.SetLazygitPaneID("")

	if status == agent.StatusReviewing {
		currentHead, err := o.git.HeadCommit(a.WorktreePath, "HEAD")
		if err != nil {
			slog.Error("failed to get head after review", "id", a.ID, "error", err)
			a.SetStatus(agent.StatusReviewReady)
			return
		}

		preReview := a.GetPreReviewCommit()
		if currentHead != preReview {
			a.SetStatus(agent.StatusReviewed)
			if o.program != nil {
				o.program.Send(AgentReviewedMsg{AgentID: a.ID, NewCommits: true})
			}
		} else {
			a.SetStatus(agent.StatusReviewReady)
			if o.program != nil {
				o.program.Send(AgentReviewedMsg{AgentID: a.ID, NewCommits: false})
			}
		}
	} else if status == agent.StatusConflicts {
		if !o.git.HasChanges(a.WorktreePath) {
			// Conflicts were resolved and committed on agent's branch.
			// Fast-forward base to the agent's HEAD before cleanup.
			if err := o.ffMergeBase(a); err != nil {
				slog.Error("ff merge base after conflict resolution failed", "id", a.ID, "error", err)
			}
			if err := o.cleanupAfterMerge(a); err != nil {
				slog.Error("cleanup after merge failed", "id", a.ID, "error", err)
			}
			if o.program != nil {
				o.program.Send(MergeResultMsg{AgentID: a.ID, Success: true})
			}
		}
		// If still dirty, stay in StatusConflicts
	}
}

func (o *Orchestrator) MergeAgent(id string, deleteBranch, removeWorktree bool) MergeResultMsg {
	a, ok := o.store.Get(id)
	if !ok {
		return MergeResultMsg{AgentID: id, Error: "agent not found"}
	}

	// Store cleanup preferences on the agent so conflict resolution path can read them
	a.SetMergeDeleteBranch(deleteBranch)
	a.SetMergeRemoveWorktree(removeWorktree)

	if o.git.HasChanges(a.WorktreePath) {
		return MergeResultMsg{AgentID: id, Error: "uncommitted changes in worktree — commit or discard them first"}
	}

	// Merge base into the agent's branch. If base is already an ancestor
	// this is a no-op ("Already up to date"). Otherwise it creates a merge
	// commit on the agent's branch, making it a superset of base. Either
	// way the agent branch ends up FF-able onto base.
	conflicted, err := o.git.MergeInWorktree(a.WorktreePath, a.BaseBranch)
	if err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("merge: %v", err)}
	}

	if conflicted {
		a.SetStatus(agent.StatusConflicts)
		conflictFiles, _ := o.git.ConflictFiles(a.WorktreePath)
		return MergeResultMsg{AgentID: id, Conflict: true, ConflictFiles: conflictFiles}
	}

	// Fast-forward base to the agent's HEAD.
	if err := o.ffMergeBase(a); err != nil {
		return MergeResultMsg{AgentID: id, Error: err.Error()}
	}

	slog.Info("merge completed", "id", a.ID, "branch", a.Branch, "base", a.BaseBranch)
	if err := o.cleanupAfterMerge(a); err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("cleanup: %v", err)}
	}
	return MergeResultMsg{AgentID: id, Success: true}
}

// ffMergeBase fast-forwards the base branch to the agent's current HEAD.
// This is used after the agent's branch has incorporated base (via merge),
// making it a strict superset that can be fast-forwarded.
func (o *Orchestrator) ffMergeBase(a *agent.Agent) error {
	agentHead, err := o.git.HeadCommit(a.WorktreePath, "HEAD")
	if err != nil {
		return fmt.Errorf("get agent HEAD: %v", err)
	}
	if wtPath := o.git.WorktreeForBranch(o.repoPath, a.BaseBranch); wtPath != "" {
		if err := o.git.MergeFFOnly(wtPath, a.Branch); err != nil {
			return fmt.Errorf("fast-forward merge: %v", err)
		}
	} else {
		if err := o.git.UpdateBranchRef(o.repoPath, a.BaseBranch, agentHead); err != nil {
			return fmt.Errorf("fast-forward update: %v", err)
		}
	}
	return nil
}

func (o *Orchestrator) cleanupAfterMerge(a *agent.Agent) error {
	removeWorktree := a.GetMergeRemoveWorktree()
	deleteBranch := a.GetMergeDeleteBranch()

	if a.TmuxPaneID != "" {
		o.monitor.Remove(a.TmuxPaneID)
	}
	if removeWorktree {
		if a.TmuxWindow != "" {
			o.tmux.KillWindow(a.TmuxWindow)
		}
		if a.WorktreePath != "" {
			o.git.RemoveWorktree(o.repoPath, a.WorktreePath)
		}
	}
	if deleteBranch && a.Branch != "" {
		o.git.DeleteBranch(o.repoPath, a.Branch)
	}
	o.store.Remove(a.ID)
	slog.Info("agent cleaned up after merge", "id", a.ID, "removeWorktree", removeWorktree, "deleteBranch", deleteBranch)
	o.saveState()
	return nil
}

func (o *Orchestrator) CleanupDeadAgents() []CleanupResult {
	var results []CleanupResult
	for _, a := range o.store.All() {
		name := a.Name
		if name == "" {
			name = a.ID
		}

		var reason string
		if !o.tmux.PaneExistsInWindow(a.TmuxPaneID, a.TmuxWindow) {
			reason = "pane gone"
		} else if _, err := os.Stat(a.WorktreePath); os.IsNotExist(err) {
			reason = "worktree missing"
		} else if a.BaseBranch != "" && o.git.IsBranchMerged(o.repoPath, a.Branch, a.BaseBranch) {
			reason = "branch merged"
		}

		if reason != "" {
			o.DismissAgent(a.ID, false)
			results = append(results, CleanupResult{AgentName: name, Reason: reason})
		}
	}
	return results
}

func (o *Orchestrator) RepoPath() string {
	return o.repoPath
}

func (o *Orchestrator) Session() string {
	return o.session
}

// --- Preview ---

// previewState is persisted to disk so preview can be cleaned up on restart.
type previewState struct {
	AgentID    string       `json:"agent_id"`
	PrevBranch string       `json:"prev_branch"`
	PrevStatus agent.Status `json:"prev_status"`
}

func (o *Orchestrator) previewStatePath() string {
	return filepath.Join(o.worktreeDir, "mastermind-preview.json")
}

func (o *Orchestrator) savePreviewState() {
	ps := previewState{
		AgentID:    o.previewAgentID,
		PrevBranch: o.previewPrevBranch,
		PrevStatus: o.previewPrevStatus,
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		slog.Error("failed to marshal preview state", "error", err)
		return
	}
	if err := os.WriteFile(o.previewStatePath(), data, 0o644); err != nil {
		slog.Error("failed to save preview state", "error", err)
	}
}

func (o *Orchestrator) deletePreviewState() {
	os.Remove(o.previewStatePath())
}

func (o *Orchestrator) loadPreviewState() *previewState {
	data, err := os.ReadFile(o.previewStatePath())
	if err != nil {
		return nil
	}
	var ps previewState
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil
	}
	return &ps
}

func (o *Orchestrator) GetPreviewAgentID() string {
	return o.previewAgentID
}

func (o *Orchestrator) PreviewAgent(id string) error {
	if o.previewAgentID != "" {
		return fmt.Errorf("preview already active for agent %s — stop it first", o.previewAgentID)
	}

	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	status := a.GetStatus()
	if status != agent.StatusReviewReady && status != agent.StatusReviewed && status != agent.StatusReviewing {
		return fmt.Errorf("agent %s is not reviewable (status: %s)", id, status)
	}

	if o.git.HasChanges(o.repoPath) {
		return fmt.Errorf("main worktree has uncommitted changes — commit or stash them first")
	}

	prevBranch, err := o.git.CurrentBranch(o.repoPath)
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	previewBranch := "preview/" + id
	if err := o.git.CreateBranch(o.repoPath, previewBranch, a.BaseBranch); err != nil {
		return fmt.Errorf("create preview branch: %w", err)
	}

	if err := o.git.CheckoutBranch(o.repoPath, previewBranch); err != nil {
		o.git.DeleteBranch(o.repoPath, previewBranch)
		return fmt.Errorf("checkout preview branch: %w", err)
	}

	conflicted, err := o.git.MergeInWorktree(o.repoPath, a.Branch)
	if err != nil {
		o.git.CheckoutBranch(o.repoPath, prevBranch)
		o.git.DeleteBranch(o.repoPath, previewBranch)
		return fmt.Errorf("merge agent branch: %w", err)
	}
	if conflicted {
		o.git.MergeAbort(o.repoPath)
		o.git.CheckoutBranch(o.repoPath, prevBranch)
		o.git.DeleteBranch(o.repoPath, previewBranch)
		return fmt.Errorf("merge conflicts between %s and %s — cannot preview", a.BaseBranch, a.Branch)
	}

	o.previewAgentID = id
	o.previewPrevBranch = prevBranch
	o.previewPrevStatus = status
	a.SetStatus(agent.StatusPreviewing)
	o.savePreviewState()

	slog.Info("preview started", "agent", id, "branch", previewBranch, "prevBranch", prevBranch)
	if o.program != nil {
		o.program.Send(PreviewStartedMsg{AgentID: id})
	}
	return nil
}

func (o *Orchestrator) StopPreview() error {
	if o.previewAgentID == "" {
		return fmt.Errorf("no preview is active")
	}

	agentID := o.previewAgentID
	previewBranch := "preview/" + agentID

	if err := o.git.CheckoutBranch(o.repoPath, o.previewPrevBranch); err != nil {
		return fmt.Errorf("checkout previous branch: %w", err)
	}

	if err := o.git.DeleteBranch(o.repoPath, previewBranch); err != nil {
		slog.Warn("failed to delete preview branch", "branch", previewBranch, "error", err)
	}

	// Restore agent's previous status
	if a, ok := o.store.Get(agentID); ok {
		a.SetStatus(o.previewPrevStatus)
	}

	o.previewAgentID = ""
	o.previewPrevBranch = ""
	o.previewPrevStatus = ""
	o.deletePreviewState()

	slog.Info("preview stopped", "agent", agentID)
	if o.program != nil {
		o.program.Send(PreviewStoppedMsg{AgentID: agentID})
	}
	return nil
}

// CleanupPreview stops any active preview, restoring the main worktree.
// Called on shutdown to ensure no orphaned preview branches.
func (o *Orchestrator) CleanupPreview() {
	// Try to restore from persisted state if not already loaded
	if o.previewAgentID == "" {
		if ps := o.loadPreviewState(); ps != nil {
			o.previewAgentID = ps.AgentID
			o.previewPrevBranch = ps.PrevBranch
			o.previewPrevStatus = ps.PrevStatus
		}
	}

	if o.previewAgentID == "" {
		return
	}

	previewBranch := "preview/" + o.previewAgentID

	if err := o.git.CheckoutBranch(o.repoPath, o.previewPrevBranch); err != nil {
		slog.Error("cleanup: failed to checkout previous branch", "branch", o.previewPrevBranch, "error", err)
	}

	if o.git.BranchExists(o.repoPath, previewBranch) {
		if err := o.git.DeleteBranch(o.repoPath, previewBranch); err != nil {
			slog.Error("cleanup: failed to delete preview branch", "branch", previewBranch, "error", err)
		}
	}

	if a, ok := o.store.Get(o.previewAgentID); ok {
		a.SetStatus(o.previewPrevStatus)
	}

	o.previewAgentID = ""
	o.previewPrevBranch = ""
	o.previewPrevStatus = ""
	o.deletePreviewState()
	slog.Info("preview cleaned up on shutdown")
}

// RecoverAgents restores agents from persisted state, validating that
// their tmux panes and worktree directories still exist.
func (o *Orchestrator) RecoverAgents() {
	persisted, err := agent.LoadState(o.statePath)
	if err != nil {
		slog.Error("failed to load persisted state", "error", err)
		return
	}
	if persisted == nil {
		return
	}

	recovered := 0
	for _, pa := range persisted {
		// Check if the tmux pane still exists
		if !o.tmux.PaneExistsInWindow(pa.TmuxPaneID, pa.TmuxWindow) {
			slog.Debug("skipping stale agent, pane gone", "id", pa.ID, "pane", pa.TmuxPaneID)
			continue
		}

		// Check if the worktree directory still exists
		if _, err := os.Stat(pa.WorktreePath); os.IsNotExist(err) {
			slog.Debug("skipping stale agent, worktree gone", "id", pa.ID, "path", pa.WorktreePath)
			continue
		}

		a := &agent.Agent{
			ID:           pa.ID,
			Name:         pa.Name,
			Branch:       pa.Branch,
			BaseBranch:   pa.BaseBranch,
			WorktreePath: pa.WorktreePath,
			TmuxWindow:   pa.TmuxWindow,
			TmuxPaneID:   pa.TmuxPaneID,
			StartedAt:    pa.StartedAt,
		}
		a.SetStatus(pa.Status)
		a.SetWaitingFor(pa.WaitingFor)
		a.SetEverActive(pa.EverActive)
		if !pa.FinishedAt.IsZero() {
			a.SetFinished(pa.ExitCode, pa.FinishedAt)
		}
		if pa.LazygitPaneID != "" {
			a.SetLazygitPaneID(pa.LazygitPaneID)
		}
		if pa.PreReviewCommit != "" {
			a.SetPreReviewCommit(pa.PreReviewCommit)
		}

		o.store.Add(a)
		recovered++
		slog.Info("recovered agent", "id", a.ID, "branch", a.Branch, "status", pa.Status)
	}

	if recovered > 0 {
		slog.Info("agent recovery complete", "recovered", recovered, "total", len(persisted))
	}

	// Recover preview state
	if ps := o.loadPreviewState(); ps != nil && ps.AgentID != "" {
		o.previewAgentID = ps.AgentID
		o.previewPrevBranch = ps.PrevBranch
		o.previewPrevStatus = ps.PrevStatus
		slog.Info("recovered preview state", "agent", ps.AgentID, "prevBranch", ps.PrevBranch)
	}
}

func (o *Orchestrator) saveState() {
	agents := o.store.All()
	if err := agent.SaveState(o.statePath, agents); err != nil {
		slog.Error("failed to save state", "error", err)
	}
}
