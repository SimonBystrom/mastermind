package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	a := NewAgent("myagent", "feat/x", "main", "/tmp/wt", "@1", "%0")
	a.ID = "a1"
	a.SetStatus(StatusReviewReady)
	a.SetWaitingFor("permission")
	a.SetEverActive(true)

	if err := SaveState(path, []*Agent{a}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d agents, want 1", len(loaded))
	}

	pa := loaded[0]
	if pa.ID != "a1" {
		t.Errorf("ID = %q, want %q", pa.ID, "a1")
	}
	if pa.Name != "myagent" {
		t.Errorf("Name = %q, want %q", pa.Name, "myagent")
	}
	if pa.Branch != "feat/x" {
		t.Errorf("Branch = %q, want %q", pa.Branch, "feat/x")
	}
	if pa.Status != StatusReviewReady {
		t.Errorf("Status = %q, want %q", pa.Status, StatusReviewReady)
	}
	if pa.WaitingFor != "permission" {
		t.Errorf("WaitingFor = %q, want %q", pa.WaitingFor, "permission")
	}
	if !pa.EverActive {
		t.Error("EverActive should be true")
	}
}

func TestLoadState_FileNotExist(t *testing.T) {
	loaded, err := LoadState("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil, got %v", loaded)
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSaveState_PreservesAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	started := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	finished := started.Add(5 * time.Minute)

	a := &Agent{
		ID:           "a42",
		Name:         "builder",
		Branch:       "feat/build",
		BaseBranch:   "develop",
		WorktreePath: "/work/trees/feat-build",
		TmuxWindow:   "@7",
		TmuxPaneID:   "%15",
		StartedAt:    started,
	}
	a.SetStatus(StatusReviewing)
	a.SetWaitingFor("input")
	a.SetEverActive(true)
	a.SetFinished(1, finished)
	a.SetLazygitPaneID("%20")
	a.SetPreReviewCommit("deadbeef")

	if err := SaveState(path, []*Agent{a}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d agents, want 1", len(loaded))
	}

	pa := loaded[0]
	if pa.ID != "a42" {
		t.Errorf("ID = %q", pa.ID)
	}
	if pa.Name != "builder" {
		t.Errorf("Name = %q", pa.Name)
	}
	if pa.Branch != "feat/build" {
		t.Errorf("Branch = %q", pa.Branch)
	}
	if pa.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q", pa.BaseBranch)
	}
	if pa.WorktreePath != "/work/trees/feat-build" {
		t.Errorf("WorktreePath = %q", pa.WorktreePath)
	}
	if pa.TmuxWindow != "@7" {
		t.Errorf("TmuxWindow = %q", pa.TmuxWindow)
	}
	if pa.TmuxPaneID != "%15" {
		t.Errorf("TmuxPaneID = %q", pa.TmuxPaneID)
	}
	if pa.Status != StatusReviewing {
		t.Errorf("Status = %q", pa.Status)
	}
	if pa.WaitingFor != "input" {
		t.Errorf("WaitingFor = %q", pa.WaitingFor)
	}
	if !pa.EverActive {
		t.Error("EverActive should be true")
	}
	if pa.ExitCode != 1 {
		t.Errorf("ExitCode = %d", pa.ExitCode)
	}
	if !pa.StartedAt.Equal(started) {
		t.Errorf("StartedAt = %v", pa.StartedAt)
	}
	if !pa.FinishedAt.Equal(finished) {
		t.Errorf("FinishedAt = %v", pa.FinishedAt)
	}
	if pa.LazygitPaneID != "%20" {
		t.Errorf("LazygitPaneID = %q", pa.LazygitPaneID)
	}
	if pa.PreReviewCommit != "deadbeef" {
		t.Errorf("PreReviewCommit = %q", pa.PreReviewCommit)
	}
}
