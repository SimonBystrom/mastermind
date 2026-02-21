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

	// Running agent: Duration increments via wall clock
	time.Sleep(10 * time.Millisecond)
	d := a.Duration()
	if d < 10*time.Millisecond {
		t.Errorf("Duration() = %v, expected >= 10ms for running agent", d)
	}
}

func TestAgent_Duration_PausesWhenNotRunning(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	// Run for a bit
	time.Sleep(20 * time.Millisecond)

	// Transition to waiting — timer pauses
	a.SetStatus(StatusWaiting)
	d1 := a.Duration()

	// Sleep while not running — duration should NOT increase
	time.Sleep(20 * time.Millisecond)
	d2 := a.Duration()

	if d2 != d1 {
		t.Errorf("Duration increased while paused: %v -> %v", d1, d2)
	}

	// Resume running — timer resumes
	a.SetStatus(StatusRunning)
	time.Sleep(20 * time.Millisecond)
	d3 := a.Duration()

	if d3 < d1+20*time.Millisecond {
		t.Errorf("Duration() = %v after resume, expected >= %v", d3, d1+20*time.Millisecond)
	}
}

func TestAgent_Duration_Finished(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	// Run for a bit, then pause, then finish
	time.Sleep(10 * time.Millisecond)
	a.SetStatus(StatusReviewReady)
	d := a.Duration()

	// Duration should be frozen after leaving running state
	time.Sleep(10 * time.Millisecond)
	d2 := a.Duration()
	if d2 != d {
		t.Errorf("Duration changed after leaving running: %v -> %v", d, d2)
	}
}

func TestAgent_Snapshot(t *testing.T) {
	a := NewAgent("feat/snap", "main", "/tmp/wt", "@1", "%0")
	a.SetStatus(StatusWaiting)
	a.SetWaitingFor("permission")
	a.SetEverActive(true)
	a.SetFinished(1, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	a.SetLazygitPaneID("%5")
	a.SetPreReviewCommit("abc123")

	snap := a.Snapshot()

	if snap.Status != StatusWaiting {
		t.Errorf("Snapshot().Status = %q, want %q", snap.Status, StatusWaiting)
	}
	if snap.WaitingFor != "permission" {
		t.Errorf("Snapshot().WaitingFor = %q, want %q", snap.WaitingFor, "permission")
	}
	if !snap.EverActive {
		t.Error("Snapshot().EverActive should be true")
	}
	if snap.ExitCode != 1 {
		t.Errorf("Snapshot().ExitCode = %d, want 1", snap.ExitCode)
	}
	if snap.FinishedAt.IsZero() {
		t.Error("Snapshot().FinishedAt should not be zero")
	}
	if snap.LazygitPaneID != "%5" {
		t.Errorf("Snapshot().LazygitPaneID = %q, want %%5", snap.LazygitPaneID)
	}
	if snap.PreReviewCommit != "abc123" {
		t.Errorf("Snapshot().PreReviewCommit = %q, want %q", snap.PreReviewCommit, "abc123")
	}
}

func TestAgent_SetFinished_OnlyOnce(t *testing.T) {
	a := NewAgent("b", "main", "/wt", "@1", "%0")

	first := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	second := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	a.SetFinished(1, first)
	a.SetFinished(2, second)

	if got := a.GetExitCode(); got != 1 {
		t.Errorf("GetExitCode() = %d after second SetFinished, want 1 (first call wins)", got)
	}
	if got := a.GetFinishedAt(); !got.Equal(first) {
		t.Errorf("GetFinishedAt() = %v, want %v (first call wins)", got, first)
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
