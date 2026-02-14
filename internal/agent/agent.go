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
	StatusConflicts   Status = "conflicts"
	StatusDismissed   Status = "dismissed"
)

type Agent struct {
	// Immutable fields (safe to read without lock)
	ID           string
	Name         string
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
}

func NewAgent(name, branch, baseBranch, worktreePath, tmuxWindow, tmuxPaneID string) *Agent {
	return &Agent{
		Name:         name,
		Branch:       branch,
		BaseBranch:   baseBranch,
		WorktreePath: worktreePath,
		TmuxWindow:   tmuxWindow,
		TmuxPaneID:   tmuxPaneID,
		StartedAt:    time.Now(),
		status:       StatusRunning,
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
	a.status = s
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

func (a *Agent) Duration() time.Duration {
	a.mu.RLock()
	finished := a.finishedAt
	a.mu.RUnlock()
	if finished.IsZero() {
		return time.Since(a.StartedAt)
	}
	return finished.Sub(a.StartedAt)
}
