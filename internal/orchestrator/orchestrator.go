package orchestrator

import (
	"fmt"
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

type Orchestrator struct {
	store       *agent.Store
	repoPath    string
	session     string
	worktreeDir string
	program     *tea.Program
	monitor     *tmux.PaneMonitor
}

func New(store *agent.Store, repoPath, session, worktreeDir string) *Orchestrator {
	return &Orchestrator{
		store:       store,
		repoPath:    repoPath,
		session:     session,
		worktreeDir: worktreeDir,
		monitor:     tmux.NewPaneMonitor(),
	}
}

func (o *Orchestrator) SetProgram(p *tea.Program) {
	o.program = p
}

func (o *Orchestrator) SpawnAgent(name, branch, baseBranch string, createBranch bool) error {
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

	a := &agent.Agent{
		Name:         name,
		Branch:       branch,
		BaseBranch:   baseBranch,
		WorktreePath: wtPath,
		TmuxWindow:   windowID,
		TmuxPaneID:   paneID,
		Status:       agent.StatusRunning,
		StartedAt:    time.Now(),
	}
	o.store.Add(a)

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

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	_, err := tmux.SplitWindow(a.TmuxPaneID, a.WorktreePath, true, 80, []string{shell, "-lc", "export GPG_TTY=$(tty); exec lazygit"})
	if err != nil {
		return fmt.Errorf("split window for lazygit: %w", err)
	}

	o.store.UpdateStatus(id, agent.StatusReviewing)
	return nil
}

func (o *Orchestrator) StartMonitor() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		agents := o.store.All()
		for _, a := range agents {
			switch a.Status {
			case agent.StatusRunning, agent.StatusWaiting,
				agent.StatusReviewReady, agent.StatusDone,
				agent.StatusReviewing:
				// These statuses need monitoring
			default:
				continue
			}

			status, err := o.monitor.GetPaneStatus(a.TmuxPaneID)
			if err != nil {
				// Pane no longer exists — window was closed externally
				o.monitor.Remove(a.TmuxPaneID)
				a.Status = agent.StatusDismissed
				if o.program != nil {
					o.program.Send(AgentGoneMsg{AgentID: a.ID})
				}
				continue
			}

			if status.Dead {
				o.handleAgentFinished(a, status.ExitCode)
				continue
			}

			if status.WaitingFor == "permission" {
				// Claude needs permission approval
				a.EverActive = true
				if a.Status != agent.StatusWaiting || a.WaitingFor != "permission" {
					a.Status = agent.StatusWaiting
					a.WaitingFor = "permission"
					if o.program != nil {
						o.program.Send(AgentWaitingMsg{
							AgentID:    a.ID,
							WaitingFor: "permission",
						})
					}
				}
			} else if status.WaitingFor == "input" {
				// Claude is idle at prompt — only treat as finished if agent was ever active
				if a.EverActive {
					o.handleAgentIdle(a)
				}
			} else {
				// Claude is actively working
				a.EverActive = true
				if a.Status != agent.StatusRunning {
					a.Status = agent.StatusRunning
					a.WaitingFor = ""
				}
			}
		}
	}
}

func (o *Orchestrator) handleAgentFinished(a *agent.Agent, exitCode int) {
	a.ExitCode = exitCode
	a.FinishedAt = time.Now()

	hasChanges := git.HasChanges(a.WorktreePath)
	if hasChanges {
		a.Status = agent.StatusReviewReady
	} else {
		a.Status = agent.StatusDone
	}

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
		if a.Status != agent.StatusReviewReady {
			a.Status = agent.StatusReviewReady
			a.FinishedAt = time.Now()
			if o.program != nil {
				o.program.Send(AgentFinishedMsg{
					AgentID:    a.ID,
					HasChanges: true,
				})
			}
		}
	} else {
		if a.Status != agent.StatusDone {
			a.Status = agent.StatusDone
			a.FinishedAt = time.Now()
			if o.program != nil {
				o.program.Send(AgentFinishedMsg{
					AgentID:    a.ID,
					HasChanges: false,
				})
			}
		}
	}
}

func (o *Orchestrator) RepoPath() string {
	return o.repoPath
}

func (o *Orchestrator) Session() string {
	return o.session
}
