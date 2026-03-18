package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/git"
	"github.com/simonbystrom/mastermind/internal/harness"
	"github.com/simonbystrom/mastermind/internal/harness/claudecode"
	"github.com/simonbystrom/mastermind/internal/harness/opencode"
	"github.com/simonbystrom/mastermind/internal/hook"
	"github.com/simonbystrom/mastermind/internal/notify"
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

type PruneResultMsg struct {
	AgentID        string
	Success        bool
	Error          string
	HasUncommitted bool
}

type PreviewStartedMsg struct{ AgentID string }
type PreviewStoppedMsg struct{ AgentID string }
type PreviewErrorMsg struct {
	AgentID string
	Error   string
}

// mtimeEntry caches the result of a file read keyed by its mtime.
type mtimeEntry struct {
	mtime  time.Time
	result interface{}
}

type Orchestrator struct {
	ctx              context.Context
	store            *agent.Store
	repoPath         string
	session          string
	worktreeDir      string
	program          *tea.Program
	monitor          tmux.PaneStatusChecker
	statePath        string
	git              git.GitOps
	tmux             tmux.TmuxOps
	lazygitSplit     int
	agentTeams       bool
	teammateMode     string
	skipPermissions  bool
	promptEditor     bool
	promptEditorSize int

	// Harness support
	harnesses      map[harness.Type]harness.Harness
	defaultHarness harness.Type

	// Performance caches (monitor loop only, no mutex needed)
	idleHasChanges       map[string]*bool      // agentID → cached HasChanges result for idle agents
	hookMtimeCache       map[string]mtimeEntry // worktreePath → cached hook status
	statuslineMtimeCache map[string]mtimeEntry // worktreePath → cached statusline data
	todosMtimeCache      map[string]mtimeEntry // worktreePath → cached todos data
	lastSaveTime         time.Time             // debounce state persistence

	previewMu         sync.RWMutex
	previewAgentID    string       // ID of agent being previewed (empty = no preview)
	previewPrevBranch string       // branch the main worktree was on before preview
	previewPrevStatus agent.Status // agent's status before preview started

	previewCleanupOnce sync.Once // ensures shutdown cleanup runs exactly once

	// Notification support
	notifier           notify.Notifier
	overviewWindowID   string // tmux window ID of the TUI window (e.g. "@0")
	overviewWindowName string // original window name (without " *" suffix)
	attentionActive    bool   // true when " *" suffix is currently appended
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

// WithAgentTeams enables or disables Claude Code agent teams.
func WithAgentTeams(enabled bool) Option {
	return func(o *Orchestrator) { o.agentTeams = enabled }
}

// WithTeammateMode sets the teammate mode for Claude Code split-pane collaboration.
func WithTeammateMode(mode string) Option {
	return func(o *Orchestrator) { o.teammateMode = mode }
}

// WithSkipPermissions passes --dangerously-skip-permissions to all spawned agents.
func WithSkipPermissions(enabled bool) Option {
	return func(o *Orchestrator) { o.skipPermissions = enabled }
}

// WithPromptEditor opens an nvim split pane for drafting prompts when spawning agents.
func WithPromptEditor(enabled bool) Option {
	return func(o *Orchestrator) { o.promptEditor = enabled }
}

// WithPromptEditorSize sets the prompt editor pane size percentage.
func WithPromptEditorSize(pct int) Option {
	return func(o *Orchestrator) { o.promptEditorSize = pct }
}

// WithDefaultHarness sets the default harness type for new agents.
func WithDefaultHarness(ht harness.Type) Option {
	return func(o *Orchestrator) { o.defaultHarness = ht }
}

// WithNotifier sets the OS notification implementation.
func WithNotifier(n notify.Notifier) Option {
	return func(o *Orchestrator) { o.notifier = n }
}

// WithOverviewWindow sets the tmux window ID and original name of the
// mastermind TUI window, enabling the " *" attention indicator.
func WithOverviewWindow(windowID, windowName string) Option {
	return func(o *Orchestrator) {
		o.overviewWindowID = windowID
		o.overviewWindowName = windowName
	}
}

func New(ctx context.Context, store *agent.Store, repoPath, session, worktreeDir string, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		ctx:              ctx,
		store:            store,
		repoPath:         repoPath,
		session:          session,
		worktreeDir:      worktreeDir,
		monitor:          tmux.NewPaneMonitor(),
		statePath:        worktreeDir + "/mastermind-state.json",
		git:              git.RealGit{},
		tmux:             tmux.RealTmux{},
		lazygitSplit:     80,
		promptEditorSize: 50,
		agentTeams:       true,
		teammateMode:     "in-process",
		harnesses: map[harness.Type]harness.Harness{
			harness.TypeClaudeCode: &claudecode.Harness{},
			harness.TypeOpenCode:   &opencode.Harness{},
		},
		defaultHarness:       harness.TypeClaudeCode,
		notifier:             notify.NoopNotifier{},
		idleHasChanges:       make(map[string]*bool),
		hookMtimeCache:       make(map[string]mtimeEntry),
		statuslineMtimeCache: make(map[string]mtimeEntry),
		todosMtimeCache:      make(map[string]mtimeEntry),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *Orchestrator) SetProgram(p *tea.Program) {
	o.program = p
}

func (o *Orchestrator) DefaultHarness() harness.Type {
	return o.defaultHarness
}

func (o *Orchestrator) SpawnAgent(branch, baseBranch string, createBranch bool, harnessType harness.Type) error {
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

	// Get the harness implementation
	h, ok := o.harnesses[harnessType]
	if !ok {
		o.git.RemoveWorktree(o.repoPath, wtPath)
		return fmt.Errorf("unknown harness type: %s", harnessType)
	}

	// Setup harness (writes hooks/plugins/config)
	setupOpts := harness.SetupOptions{
		AgentTeams:   o.agentTeams,
		TeammateMode: o.teammateMode,
	}
	if err := h.Setup(wtPath, setupOpts); err != nil {
		slog.Warn("failed to setup harness", "harness", harnessType, "error", err)
	}

	// Build command
	cmdOpts := harness.Options{
		SkipPermissions: o.skipPermissions,
	}
	cmd := h.Command(cmdOpts)

	// Launch in tmux
	paneID, err := o.tmux.NewWindow(o.session, branch, wtPath, cmd)
	if err != nil {
		o.git.RemoveWorktree(o.repoPath, wtPath)
		return fmt.Errorf("create tmux window: %w", err)
	}

	windowID, _ := o.tmux.WindowIDForPane(paneID)

	a := agent.NewAgent(branch, baseBranch, wtPath, windowID, paneID, harnessType)
	o.store.Add(a)

	// Open prompt editor split pane if enabled
	if o.promptEditor {
		promptFile := filepath.Join(wtPath, "prompt.txt")
		if err := os.WriteFile(promptFile, []byte{}, 0o644); err != nil {
			slog.Warn("failed to create prompt file", "path", promptFile, "error", err)
		} else {
			if err := appendGitExclude(wtPath, "prompt.txt", ""); err != nil {
				slog.Warn("failed to exclude prompt.txt from git", "path", wtPath, "error", err)
			}
			_, err := o.tmux.SplitWindow(paneID, wtPath, false, o.promptEditorSize, []string{"nvim", promptFile})
			if err != nil {
				slog.Warn("failed to open prompt editor pane", "error", err)
			}
		}
	}

	// Write agent metadata so orphaned worktrees can be rediscovered
	writeAgentMetadata(wtPath, baseBranch, "", harnessType)
	if err := appendGitExclude(wtPath, agentMetadataFile, ""); err != nil {
		slog.Warn("failed to exclude agent metadata from git", "path", wtPath, "error", err)
	}

	slog.Info("agent spawned", "id", a.ID, "branch", branch)
	o.saveState()

	return nil
}

// ResumeAgent reopens a tmux window for an orphaned agent and resumes
// the Claude Code session using the stored session ID.
func (o *Orchestrator) ResumeAgent(id string) error {
	a, ok := o.store.Get(id)
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if a.GetStatus() != agent.StatusOrphaned {
		return fmt.Errorf("agent %s is not orphaned (status: %s)", id, a.GetStatus())
	}

	// Verify worktree still exists
	if _, err := os.Stat(a.WorktreePath); os.IsNotExist(err) {
		return fmt.Errorf("worktree directory no longer exists: %s", a.WorktreePath)
	}

	// Write Claude Code project settings and hooks
	if err := o.writeClaudeProjectSettings(a.WorktreePath); err != nil {
		slog.Warn("failed to write claude project settings", "error", err)
	}
	if err := hook.WriteHookFiles(a.WorktreePath); err != nil {
		slog.Warn("failed to write hook files", "error", err)
	}

	// Build claude command with optional resume flag
	claudeCmd := []string{"claude"}
	if o.skipPermissions {
		claudeCmd = append(claudeCmd, "--dangerously-skip-permissions")
	}
	sessionID := a.GetSessionID()
	if sessionID != "" {
		claudeCmd = append(claudeCmd, "--resume", sessionID)
	}

	paneID, err := o.tmux.NewWindow(o.session, a.Branch, a.WorktreePath, claudeCmd)
	if err != nil {
		return fmt.Errorf("create tmux window: %w", err)
	}

	windowID, _ := o.tmux.WindowIDForPane(paneID)

	// Update the agent's tmux references (these are immutable fields, but
	// for orphan recovery we need to set them on the existing agent)
	a.TmuxWindow = windowID
	a.TmuxPaneID = paneID
	a.SetStatus(agent.StatusRunning)

	o.store.MarkDirty()
	o.saveState()
	slog.Info("resumed orphaned agent", "id", a.ID, "branch", a.Branch, "sessionID", sessionID)

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
		if err := o.tmux.KillPane(lgPane); err != nil {
			slog.Warn("failed to kill lazygit pane", "id", id, "pane", lgPane, "error", err)
		}
	}

	if a.TmuxWindow != "" {
		if err := o.tmux.KillWindow(a.TmuxWindow); err != nil {
			slog.Warn("failed to kill tmux window", "id", id, "window", a.TmuxWindow, "error", err)
		}
	}

	if a.WorktreePath != "" {
		if err := o.git.RemoveWorktree(o.repoPath, a.WorktreePath); err != nil {
			slog.Warn("failed to remove worktree", "id", id, "path", a.WorktreePath, "error", err)
		}
	}

	if deleteBranch && a.Branch != "" {
		if err := o.git.DeleteBranch(o.repoPath, a.Branch); err != nil {
			slog.Warn("failed to delete branch", "id", id, "branch", a.Branch, "error", err)
		}
	}

	o.store.Remove(id)

	slog.Info("agent dismissed", "id", id, "deleteBranch", deleteBranch)
	o.saveState()

	return nil
}

func (o *Orchestrator) PruneAgent(id string) PruneResultMsg {
	a, ok := o.store.Get(id)
	if !ok {
		return PruneResultMsg{AgentID: id, Error: "agent not found"}
	}

	if o.git.HasChanges(a.WorktreePath) {
		return PruneResultMsg{AgentID: id, Error: "uncommitted changes in worktree", HasUncommitted: true}
	}

	if a.TmuxPaneID != "" {
		o.monitor.Remove(a.TmuxPaneID)
	}

	// Gracefully stop Claude if the pane is still alive
	if a.TmuxPaneID != "" && o.tmux.PaneExistsInWindow(a.TmuxPaneID, a.TmuxWindow) {
		status := a.GetStatus()
		if status == agent.StatusRunning || status == agent.StatusWaiting {
			o.tmux.SendKeys(a.TmuxPaneID, "C-c")
			o.tmux.SendKeys(a.TmuxPaneID, "/exit", "Enter")
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Kill lazygit pane if open
	if lgPane := a.GetLazygitPaneID(); lgPane != "" {
		if err := o.tmux.KillPane(lgPane); err != nil {
			slog.Warn("failed to kill lazygit pane", "id", id, "pane", lgPane, "error", err)
		}
	}

	if a.TmuxWindow != "" {
		if err := o.tmux.KillWindow(a.TmuxWindow); err != nil {
			slog.Warn("failed to kill tmux window", "id", id, "window", a.TmuxWindow, "error", err)
		}
	}

	if a.WorktreePath != "" {
		if err := o.git.RemoveWorktree(o.repoPath, a.WorktreePath); err != nil {
			slog.Warn("failed to remove worktree", "id", id, "path", a.WorktreePath, "error", err)
		}
	}

	o.store.Remove(id)

	slog.Info("agent pruned (branch kept)", "id", id, "branch", a.Branch)
	o.saveState()

	return PruneResultMsg{AgentID: id, Success: true}
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
			// Force-save on shutdown regardless of debounce
			if o.store.IsDirty() {
				o.doSaveState()
			}
			slog.Info("monitor stopped: context cancelled")
			return
		case <-ticker.C:
		}

		agents := o.store.All()

		// Batch-fetch all panes in the session (1 subprocess) — now includes dead/exit status
		allPanes, paneListErr := o.tmux.ListAllPanes(o.session)
		if paneListErr != nil {
			slog.Debug("ListAllPanes failed, falling back to per-agent checks", "error", paneListErr)
			allPanes = nil // nil signals fallback
		}

		// paneInWindow checks if a pane exists in the expected window,
		// using the batch result when available.
		paneInWindow := func(paneID, windowID string) bool {
			if allPanes != nil {
				info, ok := allPanes[paneID]
				return ok && info.WindowID == windowID
			}
			return o.tmux.PaneExistsInWindow(paneID, windowID)
		}

		// paneDeadFromBatch returns dead status from batch result, or falls back to GetPaneStatus.
		paneDeadFromBatch := func(paneID string) (dead bool, exitCode int, err error) {
			if allPanes != nil {
				if info, ok := allPanes[paneID]; ok {
					return info.Dead, info.ExitCode, nil
				}
				// Pane not in batch = gone
				return false, 0, fmt.Errorf("pane not in session")
			}
			// Fallback: individual subprocess call
			ps, err := o.monitor.GetPaneStatus(paneID)
			if err != nil {
				return false, 0, err
			}
			return ps.Dead, ps.ExitCode, nil
		}

		for _, a := range agents {
			snap := a.Snapshot()

			// Handle lazygit pane detection for reviewing/conflicts agents
			if (snap.Status == agent.StatusReviewing || snap.Status == agent.StatusConflicts) && snap.LazygitPaneID != "" {
				lgGone := !paneInWindow(snap.LazygitPaneID, a.TmuxWindow)
				if !lgGone {
					// Pane exists but may be dead (remain-on-exit keeps it around).
					dead, _, err := paneDeadFromBatch(snap.LazygitPaneID)
					lgGone = err != nil || dead
				}
				if lgGone {
					o.tmux.KillPane(snap.LazygitPaneID)
					o.handleLazygitClosed(a, snap.Status)
				}
				continue
			}

			switch snap.Status {
			case agent.StatusRunning, agent.StatusWaiting,
				agent.StatusReviewReady, agent.StatusDone:
				// These statuses need monitoring
			default:
				continue
			}

			// Check if pane still exists
			if !paneInWindow(a.TmuxPaneID, a.TmuxWindow) {
				slog.Debug("pane gone, marking dismissed", "id", a.ID, "pane", a.TmuxPaneID)
				o.monitor.Remove(a.TmuxPaneID)
				a.SetStatus(agent.StatusDismissed)
				o.store.MarkDirty()
				delete(o.idleHasChanges, a.ID)
				if o.program != nil {
					o.program.Send(AgentGoneMsg{AgentID: a.ID})
				}
				continue
			}

			// Check for dead pane from batch result (no extra subprocess)
			dead, exitCode, err := paneDeadFromBatch(a.TmuxPaneID)
			if err != nil {
				slog.Debug("pane gone, marking dismissed", "id", a.ID, "pane", a.TmuxPaneID)
				o.monitor.Remove(a.TmuxPaneID)
				a.SetStatus(agent.StatusDismissed)
				o.store.MarkDirty()
				delete(o.idleHasChanges, a.ID)
				if o.program != nil {
					o.program.Send(AgentGoneMsg{AgentID: a.ID})
				}
				continue
			}

			if dead {
				o.handleAgentFinished(a, exitCode)
				continue
			}

			// Try hook-based status detection first (skip tmux capture if fresh)
			if o.handleHookStatus(a, snap.Status) {
				o.readStatuslineCached(a)
				o.readTodosCached(a)
				continue
			}

			// Fall back to tmux content polling
			paneStatus, err := o.monitor.GetPaneStatus(a.TmuxPaneID)
			if err != nil {
				slog.Debug("pane status error, marking dismissed", "id", a.ID, "pane", a.TmuxPaneID)
				o.monitor.Remove(a.TmuxPaneID)
				a.SetStatus(agent.StatusDismissed)
				o.store.MarkDirty()
				delete(o.idleHasChanges, a.ID)
				if o.program != nil {
					o.program.Send(AgentGoneMsg{AgentID: a.ID})
				}
				continue
			}

			if paneStatus.WaitingFor == "" {
				// Claude is actively working
				a.SetEverActive(true)
				delete(o.idleHasChanges, a.ID)
				if snap.Status != agent.StatusRunning {
					a.SetStatus(agent.StatusRunning)
					a.SetWaitingFor("")
					o.store.MarkDirty()
					slog.Debug("agent status change (tmux)", "id", a.ID, "status", "running")
				}
			} else if paneStatus.WaitingFor == "permission" {
				a.SetEverActive(true)
				if snap.Status != agent.StatusWaiting || snap.WaitingFor != "permission" {
					a.SetStatus(agent.StatusWaiting)
					a.SetWaitingFor("permission")
					o.store.MarkDirty()
					slog.Debug("agent status change (tmux)", "id", a.ID, "status", "waiting", "waitingFor", "permission")
					o.triggerAttention(a.ID, fmt.Sprintf("Agent %s needs permission", a.ID))
					if o.program != nil {
						o.program.Send(AgentWaitingMsg{
							AgentID:    a.ID,
							WaitingFor: "permission",
						})
					}
				}
			} else if snap.EverActive {
				o.handleAgentIdle(a)
			}

			o.readStatuslineCached(a)
			o.readTodosCached(a)
		}

		if o.store.IsDirty() {
			o.saveStateDebounced()
			o.store.ClearDirty()
		}
	}
}

// handleHookStatus reads the hook status file for the agent and updates
// state accordingly. Returns true if hook status was available and handled,
// false if we should fall back to tmux polling.
func (o *Orchestrator) handleHookStatus(a *agent.Agent, status agent.Status) bool {
	sf := o.readHookStatusCached(a.WorktreePath)
	if sf == nil || sf.IsStale() {
		return false
	}

	switch sf.Status {
	case hook.StatusRunning:
		a.SetEverActive(true)
		delete(o.idleHasChanges, a.ID)
		if status != agent.StatusRunning {
			a.SetStatus(agent.StatusRunning)
			a.SetWaitingFor("")
			o.store.MarkDirty()
			slog.Debug("agent status change (hook)", "id", a.ID, "status", "running")
		}

	case hook.StatusWaitingPermission:
		a.SetEverActive(true)
		if status != agent.StatusWaiting || a.GetWaitingFor() != "permission" {
			a.SetStatus(agent.StatusWaiting)
			a.SetWaitingFor("permission")
			o.store.MarkDirty()
			slog.Debug("agent status change (hook)", "id", a.ID, "status", "waiting", "waitingFor", "permission")
			o.triggerAttention(a.ID, fmt.Sprintf("Agent %s needs permission", a.ID))
			if o.program != nil {
				o.program.Send(AgentWaitingMsg{
					AgentID:    a.ID,
					WaitingFor: "permission",
				})
			}
		}

	case hook.StatusWaitingInput:
		if a.GetEverActive() {
			o.handleAgentIdle(a)
		}

	case hook.StatusIdle:
		if a.GetEverActive() {
			o.handleAgentIdle(a)
		}

	case hook.StatusStopped:
		if a.GetEverActive() {
			o.handleAgentIdle(a)
		}

	default:
		return false
	}

	return true
}

// readHookStatusCached reads the hook status file, using mtime to skip re-reads.
func (o *Orchestrator) readHookStatusCached(worktreePath string) *hook.StatusFile {
	path := filepath.Join(worktreePath, ".mastermind-status")
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	mtime := info.ModTime()
	if cached, ok := o.hookMtimeCache[worktreePath]; ok && cached.mtime.Equal(mtime) {
		if sf, ok := cached.result.(*hook.StatusFile); ok {
			return sf
		}
		return nil
	}
	sf, err := hook.ReadStatus(worktreePath)
	if err != nil {
		slog.Debug("hook status read error", "path", worktreePath, "error", err)
		o.hookMtimeCache[worktreePath] = mtimeEntry{mtime: mtime, result: (*hook.StatusFile)(nil)}
		return nil
	}
	o.hookMtimeCache[worktreePath] = mtimeEntry{mtime: mtime, result: sf}
	return sf
}

// readStatuslineCached reads the metrics sidecar file, using mtime to skip re-reads.
// The sidecar filename depends on the agent's harness type.
func (o *Orchestrator) readStatuslineCached(a *agent.Agent) {
	// Determine metrics file path based on harness type
	var metricsFile string
	switch a.Harness {
	case harness.TypeOpenCode:
		metricsFile = ".opencode-status.json"
	default:
		metricsFile = ".claude-status.json"
	}
	metricsPath := filepath.Join(a.WorktreePath, metricsFile)

	info, err := os.Stat(metricsPath)
	if err != nil {
		return
	}
	mtime := info.ModTime()
	if cached, ok := o.statuslineMtimeCache[a.WorktreePath]; ok && cached.mtime.Equal(mtime) {
		if sd, ok := cached.result.(*agent.StatuslineData); ok && sd != nil {
			a.SetStatuslineData(sd)
		}
		return
	}

	// Use harness ReadMetrics to parse the correct sidecar file
	h, ok := o.harnesses[a.Harness]
	if !ok {
		return
	}
	md, err := h.ReadMetrics(a.WorktreePath)
	if err != nil || md == nil {
		o.statuslineMtimeCache[a.WorktreePath] = mtimeEntry{mtime: mtime, result: (*agent.StatuslineData)(nil)}
		return
	}

	sd := &agent.StatuslineData{
		Model:        md.Model,
		CostUSD:      md.CostUSD,
		ContextPct:   md.ContextPct,
		LinesAdded:   md.LinesAdded,
		LinesRemoved: md.LinesRemoved,
		SessionID:    md.SessionID,
	}

	prevSessionID := a.GetSessionID()
	a.SetStatuslineData(sd)
	// Update agent metadata file with session ID for orphan recovery
	if sd.SessionID != "" && sd.SessionID != prevSessionID {
		writeAgentMetadata(a.WorktreePath, a.BaseBranch, sd.SessionID, a.Harness)
	}
	o.store.MarkDirty()
	o.statuslineMtimeCache[a.WorktreePath] = mtimeEntry{mtime: mtime, result: sd}
}

// readTodosCached reads the todos sidecar file, using mtime to skip re-reads.
func (o *Orchestrator) readTodosCached(a *agent.Agent) {
	path := filepath.Join(a.WorktreePath, ".mastermind-todos")
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mtime := info.ModTime()
	if cached, ok := o.todosMtimeCache[a.WorktreePath]; ok && cached.mtime.Equal(mtime) {
		if todos, ok := cached.result.([]hook.TodoItem); ok && todos != nil {
			a.SetTodos(todos)
		}
		return
	}
	todos, err := hook.ReadTodos(a.WorktreePath)
	if err != nil {
		o.todosMtimeCache[a.WorktreePath] = mtimeEntry{mtime: mtime, result: ([]hook.TodoItem)(nil)}
		return
	}
	a.SetTodos(todos)
	o.todosMtimeCache[a.WorktreePath] = mtimeEntry{mtime: mtime, result: todos}
}

func (o *Orchestrator) handleAgentFinished(a *agent.Agent, exitCode int) {
	a.SetFinished(exitCode, time.Now())

	hasChanges := o.git.HasChanges(a.WorktreePath)
	// Cache the result for subsequent idle checks
	hc := hasChanges
	o.idleHasChanges[a.ID] = &hc

	if hasChanges {
		a.SetStatus(agent.StatusReviewReady)
	} else {
		a.SetStatus(agent.StatusDone)
	}
	o.store.MarkDirty()

	slog.Info("agent finished", "id", a.ID, "exitCode", exitCode, "hasChanges", hasChanges)

	if hasChanges {
		o.triggerAttention(a.ID, fmt.Sprintf("Agent %s finished with changes", a.ID))
	} else {
		o.triggerAttention(a.ID, fmt.Sprintf("Agent %s finished", a.ID))
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
	// Don't overwrite reviewed status — it must stick until merge or manual change
	if a.GetStatus() == agent.StatusReviewed {
		return
	}

	// Use cached HasChanges result for idle agents to avoid redundant git status calls
	var hasChanges bool
	if cached := o.idleHasChanges[a.ID]; cached != nil {
		hasChanges = *cached
	} else {
		hasChanges = o.git.HasChanges(a.WorktreePath)
		hc := hasChanges
		o.idleHasChanges[a.ID] = &hc
	}

	if hasChanges {
		if a.GetStatus() != agent.StatusReviewReady {
			a.SetStatus(agent.StatusReviewReady)
			a.SetFinished(a.GetExitCode(), time.Now())
			o.store.MarkDirty()
			slog.Info("agent idle with changes", "id", a.ID)
			o.triggerAttention(a.ID, fmt.Sprintf("Agent %s ready for review", a.ID))
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
			o.store.MarkDirty()
			slog.Info("agent idle without changes", "id", a.ID)
			o.triggerAttention(a.ID, fmt.Sprintf("Agent %s finished", a.ID))
			if o.program != nil {
				o.program.Send(AgentFinishedMsg{
					AgentID:    a.ID,
					HasChanges: false,
				})
			}
		}
	}
}

// ClearAttentionMsg is sent to the TUI when the " *" window indicator is cleared.
type ClearAttentionMsg struct{}

// triggerAttention fires an OS notification and appends " *" to the overview
// window name. It is safe to call from the monitor goroutine.
func (o *Orchestrator) triggerAttention(agentID, message string) {
	o.notifier.Notify("Mastermind", message)

	if o.overviewWindowID != "" && !o.attentionActive {
		o.attentionActive = true
		if err := o.tmux.RenameWindow(o.overviewWindowID, o.overviewWindowName+" *"); err != nil {
			slog.Error("rename window failed", "error", err)
		}
	}
}

// ClearAttentionIndicator returns a tea.Cmd that removes the " *" suffix from
// the overview window name. The UI should call this on any keypress.
func (o *Orchestrator) ClearAttentionIndicator() tea.Cmd {
	return func() tea.Msg {
		if o.overviewWindowID != "" && o.attentionActive {
			o.attentionActive = false
			if err := o.tmux.RenameWindow(o.overviewWindowID, o.overviewWindowName); err != nil {
				slog.Error("clear attention indicator failed", "error", err)
			}
		}
		return ClearAttentionMsg{}
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
			if err := o.tmux.KillWindow(a.TmuxWindow); err != nil {
				slog.Warn("cleanup: failed to kill tmux window", "id", a.ID, "window", a.TmuxWindow, "error", err)
			}
		}
		if a.WorktreePath != "" {
			if err := o.git.RemoveWorktree(o.repoPath, a.WorktreePath); err != nil {
				slog.Warn("cleanup: failed to remove worktree", "id", a.ID, "path", a.WorktreePath, "error", err)
			}
		}
	}
	if deleteBranch && a.Branch != "" {
		if err := o.git.DeleteBranch(o.repoPath, a.Branch); err != nil {
			slog.Warn("cleanup: failed to delete branch", "id", a.ID, "branch", a.Branch, "error", err)
		}
	}
	o.store.Remove(a.ID)
	slog.Info("agent cleaned up after merge", "id", a.ID, "removeWorktree", removeWorktree, "deleteBranch", deleteBranch)
	o.saveState()
	return nil
}

func (o *Orchestrator) CleanupDeadAgents() []CleanupResult {
	var results []CleanupResult
	for _, a := range o.store.All() {
		name := a.ID

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
	o.previewMu.RLock()
	ps := previewState{
		AgentID:    o.previewAgentID,
		PrevBranch: o.previewPrevBranch,
		PrevStatus: o.previewPrevStatus,
	}
	o.previewMu.RUnlock()
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
	o.previewMu.RLock()
	defer o.previewMu.RUnlock()
	return o.previewAgentID
}

func (o *Orchestrator) PreviewAgent(id string) error {
	o.previewMu.Lock()
	if o.previewAgentID != "" {
		o.previewMu.Unlock()
		return fmt.Errorf("preview already active for agent %s — stop it first", o.previewAgentID)
	}
	o.previewAgentID = "__starting__"
	o.previewMu.Unlock()

	resetSentinel := func() {
		o.previewMu.Lock()
		o.previewAgentID = ""
		o.previewMu.Unlock()
	}

	a, ok := o.store.Get(id)
	if !ok {
		resetSentinel()
		return fmt.Errorf("agent %s not found", id)
	}

	status := a.GetStatus()
	if status != agent.StatusReviewReady && status != agent.StatusReviewed && status != agent.StatusReviewing {
		resetSentinel()
		return fmt.Errorf("agent %s is not reviewable (status: %s)", id, status)
	}

	if o.git.HasChanges(o.repoPath) {
		resetSentinel()
		return fmt.Errorf("main worktree has uncommitted changes — commit or stash them first")
	}

	prevBranch, err := o.git.CurrentBranch(o.repoPath)
	if err != nil {
		resetSentinel()
		return fmt.Errorf("get current branch: %w", err)
	}

	previewBranch := "preview/" + id
	if err := o.git.CreateBranch(o.repoPath, previewBranch, a.BaseBranch); err != nil {
		resetSentinel()
		return fmt.Errorf("create preview branch: %w", err)
	}

	if err := o.git.CheckoutBranch(o.repoPath, previewBranch); err != nil {
		o.git.DeleteBranch(o.repoPath, previewBranch)
		resetSentinel()
		return fmt.Errorf("checkout preview branch: %w", err)
	}

	conflicted, err := o.git.MergeInWorktree(o.repoPath, a.Branch)
	if err != nil {
		o.git.CheckoutBranch(o.repoPath, prevBranch)
		o.git.DeleteBranch(o.repoPath, previewBranch)
		resetSentinel()
		return fmt.Errorf("merge agent branch: %w", err)
	}
	if conflicted {
		o.git.MergeAbort(o.repoPath)
		o.git.CheckoutBranch(o.repoPath, prevBranch)
		o.git.DeleteBranch(o.repoPath, previewBranch)
		resetSentinel()
		return fmt.Errorf("merge conflicts between %s and %s — cannot preview", a.BaseBranch, a.Branch)
	}

	// Copy any uncommitted changes from the agent's worktree so the preview
	// reflects work-in-progress, not just committed code.
	if o.git.HasChanges(a.WorktreePath) {
		if err := o.git.CopyUncommittedChanges(a.WorktreePath, o.repoPath); err != nil {
			slog.Warn("failed to copy uncommitted changes to preview", "agent", id, "error", err)
		}
	}

	o.previewMu.Lock()
	o.previewAgentID = id
	o.previewPrevBranch = prevBranch
	o.previewPrevStatus = status
	o.previewMu.Unlock()
	a.SetStatus(agent.StatusPreviewing)
	o.savePreviewState()

	slog.Info("preview started", "agent", id, "branch", previewBranch, "prevBranch", prevBranch)
	if o.program != nil {
		o.program.Send(PreviewStartedMsg{AgentID: id})
	}
	return nil
}

func (o *Orchestrator) StopPreview() error {
	o.previewMu.Lock()
	if o.previewAgentID == "" {
		o.previewMu.Unlock()
		return fmt.Errorf("no preview is active")
	}

	agentID := o.previewAgentID
	prevBranch := o.previewPrevBranch
	prevStatus := o.previewPrevStatus
	o.previewMu.Unlock()

	previewBranch := "preview/" + agentID

	// Discard any uncommitted changes that were applied during preview,
	// otherwise checkout back to the previous branch may fail.
	if o.git.HasChanges(o.repoPath) {
		exec.Command("git", "-C", o.repoPath, "checkout", ".").Run()
	}

	if err := o.git.CheckoutBranch(o.repoPath, prevBranch); err != nil {
		return fmt.Errorf("checkout previous branch: %w", err)
	}

	if err := o.git.DeleteBranch(o.repoPath, previewBranch); err != nil {
		slog.Warn("failed to delete preview branch", "branch", previewBranch, "error", err)
	}

	// Restore agent's previous status
	if a, ok := o.store.Get(agentID); ok {
		a.SetStatus(prevStatus)
	}

	o.previewMu.Lock()
	o.previewAgentID = ""
	o.previewPrevBranch = ""
	o.previewPrevStatus = ""
	o.previewMu.Unlock()
	o.deletePreviewState()

	slog.Info("preview stopped", "agent", agentID)
	if o.program != nil {
		o.program.Send(PreviewStoppedMsg{AgentID: agentID})
	}
	return nil
}

// CleanupPreview stops any active preview, restoring the main worktree.
// It is safe to call multiple times — the first call performs the cleanup
// and subsequent calls are no-ops. This allows it to be called from both
// normal shutdown and signal handlers without racing.
func (o *Orchestrator) CleanupPreview() {
	o.previewCleanupOnce.Do(func() {
		o.doCleanupPreview()
	})
}

// ResetPreviewCleanup resets the once guard so CleanupPreview can fire
// again. Call this after startup cleanup so the shutdown path still works.
func (o *Orchestrator) ResetPreviewCleanup() {
	o.previewCleanupOnce = sync.Once{}
}

func (o *Orchestrator) doCleanupPreview() {
	o.previewMu.Lock()
	// Try to restore from persisted state if not already loaded
	if o.previewAgentID == "" {
		if ps := o.loadPreviewState(); ps != nil {
			o.previewAgentID = ps.AgentID
			o.previewPrevBranch = ps.PrevBranch
			o.previewPrevStatus = ps.PrevStatus
		}
	}

	if o.previewAgentID == "" {
		o.previewMu.Unlock()
		return
	}

	agentID := o.previewAgentID
	prevBranch := o.previewPrevBranch
	prevStatus := o.previewPrevStatus
	o.previewMu.Unlock()

	previewBranch := "preview/" + agentID

	// Discard uncommitted preview changes before switching back.
	if o.git.HasChanges(o.repoPath) {
		exec.Command("git", "-C", o.repoPath, "checkout", ".").Run()
	}

	if err := o.git.CheckoutBranch(o.repoPath, prevBranch); err != nil {
		slog.Error("cleanup: failed to checkout previous branch", "branch", prevBranch, "error", err)
	}

	if o.git.BranchExists(o.repoPath, previewBranch) {
		if err := o.git.DeleteBranch(o.repoPath, previewBranch); err != nil {
			slog.Error("cleanup: failed to delete preview branch", "branch", previewBranch, "error", err)
		}
	}

	if a, ok := o.store.Get(agentID); ok {
		a.SetStatus(prevStatus)
	}

	o.previewMu.Lock()
	o.previewAgentID = ""
	o.previewPrevBranch = ""
	o.previewPrevStatus = ""
	o.previewMu.Unlock()
	o.deletePreviewState()
	o.saveState()
	slog.Info("preview cleaned up")
}

// RecoverAgents restores agents from persisted state, validating that
// their tmux panes and worktree directories still exist.
func (o *Orchestrator) RecoverAgents() {
	persisted, err := agent.LoadState(o.statePath)
	if err != nil {
		slog.Error("failed to load persisted state", "error", err)
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
		if pa.SessionID != "" {
			a.SetSessionID(pa.SessionID)
		}
		a.SetDurationState(pa.AccumulatedDuration, pa.RunningStartedAt)

		// Read sidecar files immediately so recovered agents have
		// statusline data and todos available before the first monitor tick.
		o.readStatuslineCached(a)
		o.readTodosCached(a)

		o.store.Add(a)
		recovered++
		slog.Info("recovered agent", "id", a.ID, "branch", a.Branch, "status", pa.Status)
	}

	if recovered > 0 {
		slog.Info("agent recovery complete", "recovered", recovered, "total", len(persisted))
	}

	// Discover orphaned worktrees that have tmux windows but aren't in state
	if discovered := o.discoverOrphanedAgents(); discovered > 0 {
		slog.Info("orphan discovery complete", "discovered", discovered)
		o.saveState()
	}

	// Recover preview state
	if ps := o.loadPreviewState(); ps != nil && ps.AgentID != "" {
		o.previewMu.Lock()
		o.previewAgentID = ps.AgentID
		o.previewPrevBranch = ps.PrevBranch
		o.previewPrevStatus = ps.PrevStatus
		o.previewMu.Unlock()
		slog.Info("recovered preview state", "agent", ps.AgentID, "prevBranch", ps.PrevBranch)
	}
}

// agentMetadata is written to each worktree so orphaned agents can be rediscovered.
type agentMetadata struct {
	BaseBranch  string       `json:"base_branch"`
	SessionID   string       `json:"session_id,omitempty"`
	HarnessType harness.Type `json:"harness,omitempty"`
}

const agentMetadataFile = ".mastermind-agent.json"

func writeAgentMetadata(wtPath, baseBranch, sessionID string, harnessType harness.Type) {
	data, err := json.Marshal(agentMetadata{
		BaseBranch:  baseBranch,
		SessionID:   sessionID,
		HarnessType: harnessType,
	})
	if err != nil {
		slog.Warn("failed to marshal agent metadata", "error", err)
		return
	}
	if err := os.WriteFile(filepath.Join(wtPath, agentMetadataFile), data, 0o644); err != nil {
		slog.Warn("failed to write agent metadata", "path", wtPath, "error", err)
	}
}

func readAgentMetadata(wtPath string) *agentMetadata {
	data, err := os.ReadFile(filepath.Join(wtPath, agentMetadataFile))
	if err != nil {
		return nil
	}
	var m agentMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// discoverOrphanedAgents scans for worktrees that have a matching tmux window
// but are not tracked in the store. It also discovers worktrees without tmux
// windows (e.g. after a tmux crash) and marks them as orphaned.
func (o *Orchestrator) discoverOrphanedAgents() int {
	// Build set of branches already tracked in the store
	tracked := make(map[string]bool)
	for _, a := range o.store.All() {
		tracked[a.Branch] = true
	}

	// Build set of branches that were previously completed/dismissed in persisted state.
	// These are NOT orphans — they are leftovers from agents whose cleanup hasn't finished.
	completedBranches := make(map[string]bool)
	persisted, _ := agent.LoadState(o.statePath)
	for _, pa := range persisted {
		if pa.Status == agent.StatusDone || pa.Status == agent.StatusDismissed {
			completedBranches[pa.Branch] = true
		}
	}

	// Get all tmux windows in this session
	windows, err := o.tmux.ListWindows(o.session)
	if err != nil {
		slog.Debug("ListWindows failed, skipping orphan discovery", "error", err)
		return 0
	}

	// Scan worktree directory for subdirectories
	entries, err := os.ReadDir(o.worktreeDir)
	if err != nil {
		slog.Debug("failed to read worktree dir", "error", err)
		return 0
	}

	discovered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		branch := entry.Name()
		if tracked[branch] {
			continue
		}

		wtPath := filepath.Join(o.worktreeDir, branch)

		// Read metadata — if no metadata file exists, this isn't a mastermind-managed worktree
		meta := readAgentMetadata(wtPath)
		if meta == nil {
			continue
		}
		baseBranch := meta.BaseBranch

		// Determine harness type from metadata, default to Claude Code for backwards compat
		harnessType := meta.HarnessType
		if harnessType == "" {
			harnessType = harness.TypeClaudeCode
		}

		// Check if a tmux window with this branch name exists
		winInfo, ok := windows[branch]
		if ok {
			// Has tmux window — existing orphan discovery logic
			status := agent.StatusRunning
			paneID := winInfo.PaneID

			// Check if pane is dead
			allPanes, paneErr := o.tmux.ListAllPanes(o.session)
			if paneErr == nil {
				if info, ok := allPanes[paneID]; ok && info.Dead {
					if o.git.HasChanges(wtPath) {
						status = agent.StatusReviewReady
					} else {
						status = agent.StatusDone
					}
				}
			}

			// If still running, check hook status
			if status == agent.StatusRunning {
				if sf, err := hook.ReadStatus(wtPath); err == nil && sf != nil && !sf.IsStale() {
					switch sf.Status {
					case hook.StatusIdle, hook.StatusStopped, hook.StatusWaitingInput:
						if o.git.HasChanges(wtPath) {
							status = agent.StatusReviewReady
						} else {
							status = agent.StatusDone
						}
					case hook.StatusWaitingPermission:
						status = agent.StatusWaiting
					}
				}
			}

			a := agent.NewAgent(branch, baseBranch, wtPath, winInfo.ID, paneID, harnessType)
			a.SetStatus(status)
			if status == agent.StatusWaiting {
				a.SetWaitingFor("permission")
			}
			a.SetEverActive(true)
			if meta.SessionID != "" {
				a.SetSessionID(meta.SessionID)
			}

			o.readStatuslineCached(a)
			o.readTodosCached(a)

			o.store.Add(a)
			discovered++
			slog.Info("discovered orphaned agent", "id", a.ID, "branch", branch, "status", status)
		} else {
			// No tmux window — truly orphaned (e.g. tmux crash)
			// Skip if this branch was previously completed/dismissed
			if completedBranches[branch] {
				continue
			}

			// Only recover if the worktree has uncommitted changes (work worth saving)
			if !o.git.HasChanges(wtPath) {
				continue
			}

			a := agent.NewAgent(branch, baseBranch, wtPath, "", "", harnessType)
			a.SetStatus(agent.StatusOrphaned)
			a.SetEverActive(true)
			if meta.SessionID != "" {
				a.SetSessionID(meta.SessionID)
			}

			o.readStatuslineCached(a)
			o.readTodosCached(a)

			o.store.Add(a)
			discovered++
			slog.Info("discovered orphaned agent (no tmux window)", "id", a.ID, "branch", branch, "sessionID", meta.SessionID)
		}
	}

	return discovered
}

func (o *Orchestrator) saveState() {
	o.doSaveState()
}

// saveStateDebounced only writes if at least 5s have passed since last save.
// Used in the monitor loop to reduce I/O. Force-saves happen via doSaveState.
func (o *Orchestrator) saveStateDebounced() {
	if time.Since(o.lastSaveTime) < 5*time.Second {
		return
	}
	o.doSaveState()
}

func (o *Orchestrator) doSaveState() {
	agents := o.store.All()
	if err := agent.SaveState(o.statePath, agents); err != nil {
		slog.Error("failed to save state", "error", err)
	}
	o.lastSaveTime = time.Now()
}

// writeClaudeProjectSettings writes .claude/settings.json in the worktree
// to configure Claude Code's statusline for this agent. It also ensures the
// .claude/ directory and .claude-status.json sidecar are git-ignored.
func (o *Orchestrator) writeClaudeProjectSettings(wtPath string) error {
	dir := filepath.Join(wtPath, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Gitignore the .claude directory contents so they don't appear as uncommitted changes
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return err
	}

	// Also gitignore the sidecar file at the worktree root
	statusIgnorePath := filepath.Join(wtPath, ".claude-status.json")
	_ = appendGitExclude(wtPath, ".claude-status.json", statusIgnorePath)

	settings := map[string]interface{}{
		"statusLine": map[string]string{
			"type":    "command",
			"command": config.StatuslineScriptPath(),
		},
	}

	if o.agentTeams {
		settings["env"] = map[string]string{
			"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1",
		}
	}
	if o.teammateMode != "" {
		settings["teammateMode"] = o.teammateMode
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

// appendGitExclude adds a pattern to .git/info/exclude for the given worktree
// if it's not already present. Uses --git-common-dir so excludes work in
// worktrees (worktree-specific git dirs don't support info/exclude).
func appendGitExclude(wtPath, pattern, _ string) error {
	// Use --git-common-dir which resolves to the main .git dir for worktrees
	out, err := exec.Command("git", "-C", wtPath, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return err
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	content, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}

	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	prefix := ""
	if len(content) > 0 && content[len(content)-1] != '\n' {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s%s\n", prefix, pattern)
	return err
}
