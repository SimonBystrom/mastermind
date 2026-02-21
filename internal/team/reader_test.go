package team

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindTeamForSession_NoTeamsDir(t *testing.T) {
	r := NewReaderWithDirs("/nonexistent/teams", "/nonexistent/tasks")
	info, err := r.FindTeamForSession("abc123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil info, got %+v", info)
	}
}

func TestFindTeamForSession_MatchingTeam(t *testing.T) {
	tmp := t.TempDir()
	teamsDir := filepath.Join(tmp, "teams")
	tasksDir := filepath.Join(tmp, "tasks")

	// Create team config
	cfg := TeamConfig{
		TeamName: "my-team",
		Members: []Member{
			{Name: "lead-agent", AgentID: "session-123", AgentType: "lead"},
			{Name: "helper-1", AgentID: "session-456", AgentType: "teammate"},
		},
	}
	writeJSON(t, filepath.Join(teamsDir, "my-team", "config.json"), cfg)

	// Create tasks
	writeJSON(t, filepath.Join(tasksDir, "my-team", "task-1.json"), Task{
		ID: "1", Subject: "Do thing", Status: TaskCompleted,
	})
	writeJSON(t, filepath.Join(tasksDir, "my-team", "task-2.json"), Task{
		ID: "2", Subject: "Do other thing", Status: TaskInProgress, Owner: "helper-1",
	})
	writeJSON(t, filepath.Join(tasksDir, "my-team", "task-3.json"), Task{
		ID: "3", Subject: "Pending thing", Status: TaskPending,
	})

	r := NewReaderWithDirs(teamsDir, tasksDir)
	info, err := r.FindTeamForSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected team info, got nil")
	}

	if info.TeamName != "my-team" {
		t.Errorf("team name = %q, want %q", info.TeamName, "my-team")
	}
	if info.MemberCount != 2 {
		t.Errorf("member count = %d, want 2", info.MemberCount)
	}
	if info.TotalTasks != 3 {
		t.Errorf("total tasks = %d, want 3", info.TotalTasks)
	}
	if info.CompletedTasks != 1 {
		t.Errorf("completed = %d, want 1", info.CompletedTasks)
	}
	if info.InProgressTasks != 1 {
		t.Errorf("in progress = %d, want 1", info.InProgressTasks)
	}
	if info.PendingTasks != 1 {
		t.Errorf("pending = %d, want 1", info.PendingTasks)
	}
}

func TestFindTeamForSession_NonMatchingTeam(t *testing.T) {
	tmp := t.TempDir()
	teamsDir := filepath.Join(tmp, "teams")
	tasksDir := filepath.Join(tmp, "tasks")

	cfg := TeamConfig{
		TeamName: "other-team",
		Members: []Member{
			{Name: "lead", AgentID: "different-session", AgentType: "lead"},
		},
	}
	writeJSON(t, filepath.Join(teamsDir, "other-team", "config.json"), cfg)

	r := NewReaderWithDirs(teamsDir, tasksDir)
	info, err := r.FindTeamForSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil for non-matching session, got %+v", info)
	}
}

func TestFindTeamForSession_MalformedJSON(t *testing.T) {
	tmp := t.TempDir()
	teamsDir := filepath.Join(tmp, "teams")
	tasksDir := filepath.Join(tmp, "tasks")

	// Write invalid JSON
	teamDir := filepath.Join(teamsDir, "bad-team")
	if err := os.MkdirAll(teamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(teamDir, "config.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewReaderWithDirs(teamsDir, tasksDir)
	info, err := r.FindTeamForSession("session-123")
	if err != nil {
		t.Fatalf("expected no error for malformed JSON, got %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil, got %+v", info)
	}
}

func TestFindTeamForSession_NoTasks(t *testing.T) {
	tmp := t.TempDir()
	teamsDir := filepath.Join(tmp, "teams")
	tasksDir := filepath.Join(tmp, "tasks")

	cfg := TeamConfig{
		TeamName: "no-tasks-team",
		Members: []Member{
			{Name: "lead", AgentID: "session-789", AgentType: "lead"},
		},
	}
	writeJSON(t, filepath.Join(teamsDir, "no-tasks-team", "config.json"), cfg)

	r := NewReaderWithDirs(teamsDir, tasksDir)
	info, err := r.FindTeamForSession("session-789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected team info, got nil")
	}
	if info.TotalTasks != 0 {
		t.Errorf("total tasks = %d, want 0", info.TotalTasks)
	}
	if info.MemberCount != 1 {
		t.Errorf("member count = %d, want 1", info.MemberCount)
	}
}

func TestFindTeamForSession_TeammateDoesNotMatch(t *testing.T) {
	tmp := t.TempDir()
	teamsDir := filepath.Join(tmp, "teams")
	tasksDir := filepath.Join(tmp, "tasks")

	// Session ID matches a teammate, not a lead
	cfg := TeamConfig{
		TeamName: "team",
		Members: []Member{
			{Name: "lead", AgentID: "lead-session", AgentType: "lead"},
			{Name: "helper", AgentID: "session-123", AgentType: "teammate"},
		},
	}
	writeJSON(t, filepath.Join(teamsDir, "team", "config.json"), cfg)

	r := NewReaderWithDirs(teamsDir, tasksDir)
	info, err := r.FindTeamForSession("session-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil for teammate session, got %+v", info)
	}
}
