package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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

type Orchestrator struct {
	ctx         context.Context
	store       *agent.Store
	repoPath    string
	session     string
	worktreeDir string
	program     *tea.Program
	monitor     *tmux.PaneMonitor
	statePath   string
}

func New(ctx context.Context, store *agent.Store, repoPath, session, worktreeDir string) *Orchestrator {
	return &Orchestrator{
		ctx:         ctx,
		store:       store,
		repoPath:    repoPath,
		session:     session,
		worktreeDir: worktreeDir,
		monitor:     tmux.NewPaneMonitor(),
		statePath:   worktreeDir + "/mastermind-state.json",
	}
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

	if createBranch {
		if err := git.CreateBranch(o.repoPath, branch, baseBranch); err != nil {
			return fmt.Errorf("create branch: %w", err)
		}
	}

	wtPath, err := git.CreateWorktree(o.repoPath, o.worktreeDir, branch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	windowName := name
	if windowName == "" {
		windowName = branch
	}

	paneID, err := tmux.NewWindow(o.session, windowName, wtPath, []string{"claude"})
	if err != nil {
		git.RemoveWorktree(o.repoPath, wtPath)
		return fmt.Errorf("create tmux window: %w", err)
	}

	windowID, _ := tmux.WindowIDForPane(paneID)

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
	if a.TmuxPaneID != "" && tmux.PaneExistsInWindow(a.TmuxPaneID, a.TmuxWindow) {
		status := a.GetStatus()
		if status == agent.StatusRunning || status == agent.StatusWaiting {
			// Send Ctrl+C to interrupt, then /exit to quit cleanly
			tmux.SendKeys(a.TmuxPaneID, "C-c")
			tmux.SendKeys(a.TmuxPaneID, "/exit", "Enter")
			// Give Claude a moment to shut down
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Kill lazygit pane if open
	if lgPane := a.GetLazygitPaneID(); lgPane != "" {
		tmux.KillPane(lgPane)
	}

	if a.TmuxWindow != "" {
		tmux.KillWindow(a.TmuxWindow)
	}

	if a.WorktreePath != "" {
		git.RemoveWorktree(o.repoPath, a.WorktreePath)
	}

	if deleteBranch && a.Branch != "" {
		git.DeleteBranch(o.repoPath, a.Branch)
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

	if err := tmux.SelectWindow(a.TmuxWindow); err != nil {
		return fmt.Errorf("select window: %w", err)
	}
	return tmux.SelectPane(a.TmuxPaneID)
}

func (o *Orchestrator) OpenLazyGit(id string) error {
	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if err := tmux.SelectWindow(a.TmuxWindow); err != nil {
		return fmt.Errorf("select window: %w", err)
	}

	// Record HEAD before review starts
	head, err := git.HeadCommit(a.WorktreePath, "HEAD")
	if err != nil {
		return fmt.Errorf("get head commit: %w", err)
	}
	a.SetPreReviewCommit(head)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	paneID, err := tmux.SplitWindow(a.TmuxPaneID, a.WorktreePath, true, 80, []string{shell, "-lc", "export GPG_TTY=$(tty); exec lazygit"})
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
				lgGone := !tmux.PaneExistsInWindow(a.GetLazygitPaneID(), a.TmuxWindow)
				if !lgGone {
					// Pane exists but may be dead (remain-on-exit keeps it around).
					lgStatus, err := o.monitor.GetPaneStatus(a.GetLazygitPaneID())
					lgGone = err != nil || lgStatus.Dead
				}
				if lgGone {
					// Kill the dead lazygit pane if it's still lingering
					tmux.KillPane(a.GetLazygitPaneID())
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

			// Settled statuses only need pane-gone/dead detection above.
			// Don't re-classify them based on pane content — the pattern
			// matcher can produce false positives that demote these states.
			if status == agent.StatusReviewed || status == agent.StatusReviewReady || status == agent.StatusDone {
				continue
			}

			if paneStatus.WaitingFor != "" && a.GetEverActive() && git.HasChanges(a.WorktreePath) {
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

	hasChanges := git.HasChanges(a.WorktreePath)
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
	hasChanges := git.HasChanges(a.WorktreePath)
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
		currentHead, err := git.HeadCommit(a.WorktreePath, "HEAD")
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
		if !git.HasChanges(a.WorktreePath) {
			// Conflicts were resolved and committed
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

	if git.HasChanges(a.WorktreePath) {
		return MergeResultMsg{AgentID: id, Error: "uncommitted changes in worktree — commit or discard them first"}
	}

	agentHead, err := git.HeadCommit(a.WorktreePath, "HEAD")
	if err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("get agent HEAD: %v", err)}
	}

	baseHead, err := git.HeadCommit(o.repoPath, a.BaseBranch)
	if err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("get base HEAD: %v", err)}
	}

	// Fast-forward: base is ancestor of agent
	if git.IsAncestor(o.repoPath, baseHead, agentHead) {
		// If the base branch is checked out somewhere, merge there so the
		// working tree gets updated. Otherwise just move the ref.
		if wtPath := git.WorktreeForBranch(o.repoPath, a.BaseBranch); wtPath != "" {
			if err := git.MergeFFOnly(wtPath, a.Branch); err != nil {
				return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("fast-forward merge: %v", err)}
			}
		} else {
			if err := git.UpdateBranchRef(o.repoPath, a.BaseBranch, agentHead); err != nil {
				return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("fast-forward update: %v", err)}
			}
		}
		slog.Info("fast-forward merge", "id", a.ID, "branch", a.Branch, "base", a.BaseBranch)
		if err := o.cleanupAfterMerge(a); err != nil {
			return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("cleanup: %v", err)}
		}
		return MergeResultMsg{AgentID: id, Success: true}
	}

	// Non-fast-forward: need to do a real merge
	checkedOut, err := git.IsBranchCheckedOut(o.repoPath, a.BaseBranch)
	if err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("check branch checkout: %v", err)}
	}
	if checkedOut {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("base branch %q is checked out in another worktree — switch away first", a.BaseBranch)}
	}

	if err := git.CheckoutBranch(a.WorktreePath, a.BaseBranch); err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("checkout base: %v", err)}
	}

	conflicted, err := git.MergeInWorktree(a.WorktreePath, a.Branch)
	if err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("merge: %v", err)}
	}

	if conflicted {
		a.SetStatus(agent.StatusConflicts)
		conflictFiles, _ := git.ConflictFiles(a.WorktreePath)
		return MergeResultMsg{AgentID: id, Conflict: true, ConflictFiles: conflictFiles}
	}

	slog.Info("merge completed", "id", a.ID, "branch", a.Branch, "base", a.BaseBranch)
	if err := o.cleanupAfterMerge(a); err != nil {
		return MergeResultMsg{AgentID: id, Error: fmt.Sprintf("cleanup: %v", err)}
	}
	return MergeResultMsg{AgentID: id, Success: true}
}

func (o *Orchestrator) cleanupAfterMerge(a *agent.Agent) error {
	removeWorktree := a.GetMergeRemoveWorktree()
	deleteBranch := a.GetMergeDeleteBranch()

	if a.TmuxPaneID != "" {
		o.monitor.Remove(a.TmuxPaneID)
	}
	if removeWorktree {
		if a.TmuxWindow != "" {
			tmux.KillWindow(a.TmuxWindow)
		}
		if a.WorktreePath != "" {
			git.RemoveWorktree(o.repoPath, a.WorktreePath)
		}
	}
	if deleteBranch && a.Branch != "" {
		git.DeleteBranch(o.repoPath, a.Branch)
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
		if !tmux.PaneExistsInWindow(a.TmuxPaneID, a.TmuxWindow) {
			reason = "pane gone"
		} else if _, err := os.Stat(a.WorktreePath); os.IsNotExist(err) {
			reason = "worktree missing"
		} else if a.BaseBranch != "" && git.IsBranchMerged(o.repoPath, a.Branch, a.BaseBranch) {
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
		if !tmux.PaneExistsInWindow(pa.TmuxPaneID, pa.TmuxWindow) {
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
}

func (o *Orchestrator) saveState() {
	agents := o.store.All()
	if err := agent.SaveState(o.statePath, agents); err != nil {
		slog.Error("failed to save state", "error", err)
	}
}
