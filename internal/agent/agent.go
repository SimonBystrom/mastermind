package agent

import (
	"sync"
	"time"
)

type Status string

const (
	StatusRunning     Status = "running"
	StatusWaiting     Status = "waiting"
	StatusReviewReady Status = "review ready"
	StatusDone        Status = "done"
	StatusReviewing   Status = "reviewing"
	StatusReviewed    Status = "reviewed"
	StatusPreviewing  Status = "previewing"
	StatusConflicts   Status = "conflicts"
	StatusDismissed   Status = "dismissed"
)

type Agent struct {
	// Immutable fields (safe to read without lock)
	ID           string
	Branch       string
	BaseBranch   string
	WorktreePath string
	TmuxWindow   string
	TmuxPaneID   string
	StartedAt    time.Time

	// Mutable fields (protected by mu)
	mu              sync.RWMutex
	status          Status
	waitingFor      string // "permission" or "input" when status == StatusWaiting
	everActive      bool   // true once the agent has been seen actively working
	exitCode        int
	finishedAt      time.Time
	lazygitPaneID   string // tracks the lazygit split pane
	preReviewCommit string // HEAD hash before review started

	// Merge cleanup preferences (set by merge wizard, read after conflict resolution)
	mergeDeleteBranch   bool
	mergeRemoveWorktree bool

	// Duration tracking: only counts time spent in StatusRunning.
	accumulatedDuration time.Duration // total time accumulated in previous running periods
	runningStartedAt    time.Time     // when the current running period started (zero if not running)

	// Claude Code statusline data (read from sidecar file)
	statuslineData *StatuslineData
}

func NewAgent(branch, baseBranch, worktreePath, tmuxWindow, tmuxPaneID string) *Agent {
	now := time.Now()
	return &Agent{
		Branch:           branch,
		BaseBranch:       baseBranch,
		WorktreePath:     worktreePath,
		TmuxWindow:       tmuxWindow,
		TmuxPaneID:       tmuxPaneID,
		StartedAt:        now,
		status:           StatusRunning,
		runningStartedAt: now, // starts in running state
	}
}

func (a *Agent) GetStatus() Status {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *Agent) SetStatus(s Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.status
	a.status = s

	// Pause timer when leaving running state.
	if prev == StatusRunning && s != StatusRunning {
		if !a.runningStartedAt.IsZero() {
			a.accumulatedDuration += time.Since(a.runningStartedAt)
			a.runningStartedAt = time.Time{}
		}
	}

	// Resume timer when entering running state.
	if s == StatusRunning && prev != StatusRunning {
		a.runningStartedAt = time.Now()
	}
}

func (a *Agent) GetWaitingFor() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.waitingFor
}

func (a *Agent) SetWaitingFor(wf string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.waitingFor = wf
}

func (a *Agent) GetEverActive() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.everActive
}

func (a *Agent) SetEverActive(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.everActive = v
}

func (a *Agent) GetExitCode() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.exitCode
}

func (a *Agent) GetFinishedAt() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.finishedAt
}

func (a *Agent) SetFinished(exitCode int, t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.finishedAt.IsZero() {
		return // first call wins
	}
	a.exitCode = exitCode
	a.finishedAt = t
}

func (a *Agent) GetLazygitPaneID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lazygitPaneID
}

func (a *Agent) SetLazygitPaneID(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lazygitPaneID = id
}

func (a *Agent) GetPreReviewCommit() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.preReviewCommit
}

func (a *Agent) SetPreReviewCommit(commit string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.preReviewCommit = commit
}

func (a *Agent) GetMergeDeleteBranch() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mergeDeleteBranch
}

func (a *Agent) SetMergeDeleteBranch(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mergeDeleteBranch = v
}

func (a *Agent) GetMergeRemoveWorktree() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mergeRemoveWorktree
}

func (a *Agent) SetMergeRemoveWorktree(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mergeRemoveWorktree = v
}

func (a *Agent) GetStatuslineData() *StatuslineData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.statuslineData
}

func (a *Agent) SetStatuslineData(sd *StatuslineData) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuslineData = sd
}

func (a *Agent) Duration() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.runningStartedAt.IsZero() {
		// Currently running: add live elapsed time.
		return a.accumulatedDuration + time.Since(a.runningStartedAt)
	}
	return a.accumulatedDuration
}

func (a *Agent) GetAccumulatedDuration() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.accumulatedDuration
}

func (a *Agent) GetRunningStartedAt() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.runningStartedAt
}

// AgentSnapshot holds a consistent point-in-time view of all mutable fields.
type AgentSnapshot struct {
	Status              Status
	WaitingFor          string
	EverActive          bool
	ExitCode            int
	FinishedAt          time.Time
	LazygitPaneID       string
	PreReviewCommit     string
	AccumulatedDuration time.Duration
	RunningStartedAt    time.Time
}

// Snapshot reads all mutable fields under a single lock acquisition.
func (a *Agent) Snapshot() AgentSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return AgentSnapshot{
		Status:              a.status,
		WaitingFor:          a.waitingFor,
		EverActive:          a.everActive,
		ExitCode:            a.exitCode,
		FinishedAt:          a.finishedAt,
		LazygitPaneID:       a.lazygitPaneID,
		PreReviewCommit:     a.preReviewCommit,
		AccumulatedDuration: a.accumulatedDuration,
		RunningStartedAt:    a.runningStartedAt,
	}
}

// SetDurationState restores duration tracking fields (used during recovery).
func (a *Agent) SetDurationState(accumulated time.Duration, runningStarted time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accumulatedDuration = accumulated
	a.runningStartedAt = runningStarted
}
