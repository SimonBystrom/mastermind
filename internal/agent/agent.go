package agent

import "time"

type Status string

const (
	StatusRunning     Status = "running"
	StatusWaiting     Status = "waiting"
	StatusReviewReady Status = "review ready"
	StatusDone        Status = "done"
	StatusReviewing   Status = "reviewing"
	StatusDismissed   Status = "dismissed"
)

type Agent struct {
	ID           string
	Name         string
	Branch       string
	BaseBranch   string
	WorktreePath string
	TmuxWindow   string
	TmuxPaneID   string
	Status       Status
	WaitingFor   string // "permission" or "input" when Status == StatusWaiting
	EverActive   bool   // true once the agent has been seen actively working
	ExitCode     int
	StartedAt    time.Time
	FinishedAt   time.Time
}

func (a *Agent) Duration() time.Duration {
	if a.FinishedAt.IsZero() {
		return time.Since(a.StartedAt)
	}
	return a.FinishedAt.Sub(a.StartedAt)
}
