package agent

import (
	"sync"
	"testing"
	"time"
)

func TestNewAgent(t *testing.T) {
	a := NewAgent("feat/x", "main", "/tmp/wt", "@1", "%0")

	if a.Branch != "feat/x" {
		t.Errorf("Branch = %q, want %q", a.Branch, "feat/x")
	}
	if a.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", a.BaseBranch, "main")
	}
	if a.WorktreePath != "/tmp/wt" {
		t.Errorf("WorktreePath = %q, want %q", a.WorktreePath, "/tmp/wt")
	}
	if a.TmuxWindow != "@1" {
		t.Errorf("TmuxWindow = %q, want %q", a.TmuxWindow, "@1")
	}
	if a.TmuxPaneID != "%0" {
		t.Errorf("TmuxPaneID = %q, want %q", a.TmuxPaneID, "%0")
	}
	if a.GetStatus() != StatusRunning {
		t.Errorf("initial status = %q, want %q", a.GetStatus(), StatusRunning)
	}
	if a.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}
	if a.ID != "" {
		t.Errorf("ID should be empty before Store.Add, got %q", a.ID)
	}
}

func TestAgent_GetSetStatus(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	statuses := []Status{
		StatusRunning, StatusWaiting, StatusReviewReady,
		StatusDone, StatusReviewing, StatusReviewed,
		StatusConflicts, StatusDismissed,
	}
	for _, s := range statuses {
		a.SetStatus(s)
		if got := a.GetStatus(); got != s {
			t.Errorf("SetStatus(%q) then GetStatus() = %q", s, got)
		}
	}
}

func TestAgent_GetSetWaitingFor(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	a.SetWaitingFor("permission")
	if got := a.GetWaitingFor(); got != "permission" {
		t.Errorf("GetWaitingFor() = %q, want %q", got, "permission")
	}

	a.SetWaitingFor("")
	if got := a.GetWaitingFor(); got != "" {
		t.Errorf("GetWaitingFor() = %q, want empty", got)
	}
}

func TestAgent_GetSetEverActive(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	if a.GetEverActive() {
		t.Error("GetEverActive() should be false initially")
	}
	a.SetEverActive(true)
	if !a.GetEverActive() {
		t.Error("GetEverActive() should be true after SetEverActive(true)")
	}
}

func TestAgent_SetFinished(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	now := time.Now()
	a.SetFinished(42, now)

	if got := a.GetExitCode(); got != 42 {
		t.Errorf("GetExitCode() = %d, want 42", got)
	}
	if got := a.GetFinishedAt(); !got.Equal(now) {
		t.Errorf("GetFinishedAt() = %v, want %v", got, now)
	}
}

func TestAgent_LazygitPaneID(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	if got := a.GetLazygitPaneID(); got != "" {
		t.Errorf("GetLazygitPaneID() = %q, want empty", got)
	}
	a.SetLazygitPaneID("%5")
	if got := a.GetLazygitPaneID(); got != "%5" {
		t.Errorf("GetLazygitPaneID() = %q, want %%5", got)
	}
}

func TestAgent_PreReviewCommit(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	a.SetPreReviewCommit("abc123")
	if got := a.GetPreReviewCommit(); got != "abc123" {
		t.Errorf("GetPreReviewCommit() = %q, want %q", got, "abc123")
	}
}

func TestAgent_MergePreferences(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	if a.GetMergeDeleteBranch() {
		t.Error("GetMergeDeleteBranch() should be false initially")
	}
	if a.GetMergeRemoveWorktree() {
		t.Error("GetMergeRemoveWorktree() should be false initially")
	}

	a.SetMergeDeleteBranch(true)
	a.SetMergeRemoveWorktree(true)

	if !a.GetMergeDeleteBranch() {
		t.Error("GetMergeDeleteBranch() should be true")
	}
	if !a.GetMergeRemoveWorktree() {
		t.Error("GetMergeRemoveWorktree() should be true")
	}
}

func TestAgent_Duration_Running(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	// Running agent: Duration = time.Since(StartedAt)
	time.Sleep(10 * time.Millisecond)
	d := a.Duration()
	if d < 10*time.Millisecond {
		t.Errorf("Duration() = %v, expected >= 10ms for running agent", d)
	}
}

func TestAgent_Duration_Finished(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	finishTime := a.StartedAt.Add(5 * time.Second)
	a.SetFinished(0, finishTime)

	d := a.Duration()
	if d != 5*time.Second {
		t.Errorf("Duration() = %v, want 5s for finished agent", d)
	}
}

func TestAgent_ConcurrentAccess(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.SetStatus(StatusRunning)
			a.GetStatus()
			a.SetWaitingFor("permission")
			a.GetWaitingFor()
			a.SetEverActive(true)
			a.GetEverActive()
			a.SetFinished(0, time.Now())
			a.GetExitCode()
			a.GetFinishedAt()
			a.SetLazygitPaneID("%1")
			a.GetLazygitPaneID()
			a.SetPreReviewCommit("abc")
			a.GetPreReviewCommit()
			a.SetMergeDeleteBranch(true)
			a.GetMergeDeleteBranch()
			a.SetMergeRemoveWorktree(true)
			a.GetMergeRemoveWorktree()
			a.Duration()
		}()
	}
	wg.Wait()
}
