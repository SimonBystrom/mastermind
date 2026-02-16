package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0m 00s"},
		{65 * time.Second, "1m 05s"},
		{3661 * time.Second, "61m 01s"},
		{30 * time.Second, "0m 30s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"a", 1, "a"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}

func newTestDashboard(t *testing.T) (dashboardModel, *agent.Store) {
	t.Helper()
	store := agent.NewStore()
	cfg := config.Default()
	orch := orchestrator.New(context.Background(), store, "/repo", "test", t.TempDir())
	d := newDashboard(NewStyles(cfg.Colors), cfg.Layout, orch, store, "/repo", "test")
	d.width = 120
	d.height = 40
	return d, store
}

func TestSortedAgents_ByID(t *testing.T) {
	d, store := newTestDashboard(t)
	d.sortBy = sortByID

	a1 := agent.NewAgent("first", "b1", "main", "/wt1", "@1", "%1")
	a1.ID = "a1"
	a2 := agent.NewAgent("second", "b2", "main", "/wt2", "@2", "%2")
	a2.ID = "a2"
	store.Add(a2) // add in reverse order
	store.Add(a1)

	sorted := d.sortedAgents()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(sorted))
	}
	if sorted[0].ID != "a1" {
		t.Errorf("first agent ID = %q, want %q", sorted[0].ID, "a1")
	}
}

func TestSortedAgents_ByStatus(t *testing.T) {
	d, store := newTestDashboard(t)
	d.sortBy = sortByStatus

	running := agent.NewAgent("running", "b1", "main", "/wt1", "@1", "%1")
	running.ID = "r1"
	running.SetStatus(agent.StatusRunning)

	waiting := agent.NewAgent("waiting", "b2", "main", "/wt2", "@2", "%2")
	waiting.ID = "w1"
	waiting.SetStatus(agent.StatusWaiting)

	store.Add(running)
	store.Add(waiting)

	sorted := d.sortedAgents()
	// StatusWaiting (order=1) should come before StatusRunning (order=4)
	if sorted[0].GetStatus() != agent.StatusWaiting {
		t.Errorf("first agent status = %q, want %q", sorted[0].GetStatus(), agent.StatusWaiting)
	}
}

func TestSortedAgents_ByDuration(t *testing.T) {
	d, store := newTestDashboard(t)
	d.sortBy = sortByDuration

	newer := agent.NewAgent("newer", "b1", "main", "/wt1", "@1", "%1")
	newer.ID = "n1"

	older := agent.NewAgent("older", "b2", "main", "/wt2", "@2", "%2")
	older.ID = "o1"
	// Finish older agent with a known duration
	older.SetFinished(0, older.StartedAt.Add(10*time.Minute))

	store.Add(newer)
	store.Add(older)

	sorted := d.sortedAgents()
	// Longer duration first
	if sorted[0].ID != "o1" {
		t.Errorf("first agent ID = %q, want %q (longer duration)", sorted[0].ID, "o1")
	}
}

func TestDashboard_ViewContent_NoAgents(t *testing.T) {
	d, _ := newTestDashboard(t)

	content := d.ViewContent()
	if !strings.Contains(content, "No agents running") {
		t.Error("empty dashboard should show 'No agents running'")
	}
}

func TestDashboard_ViewContent_WithAgents(t *testing.T) {
	d, store := newTestDashboard(t)

	a := agent.NewAgent("builder", "feat/build", "main", "/wt", "@1", "%1")
	store.Add(a)

	content := d.ViewContent()
	if !strings.Contains(content, "builder") {
		t.Error("dashboard should show agent name")
	}
	if !strings.Contains(content, "feat/build") {
		t.Error("dashboard should show agent branch")
	}
}

func TestDashboard_CursorNavigation(t *testing.T) {
	d, store := newTestDashboard(t)

	a1 := agent.NewAgent("a1", "b1", "main", "/wt1", "@1", "%1")
	a1.ID = "a1"
	a2 := agent.NewAgent("a2", "b2", "main", "/wt2", "@2", "%2")
	a2.ID = "a2"
	store.Add(a1)
	store.Add(a2)

	// Start at cursor 0
	if d.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", d.cursor)
	}

	// Move down with j
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if d.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", d.cursor)
	}

	// Move up with k
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.cursor != 0 {
		t.Errorf("cursor after k = %d, want 0", d.cursor)
	}

	// Don't go below 0
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if d.cursor != 0 {
		t.Errorf("cursor should not go below 0, got %d", d.cursor)
	}
}

func TestDashboard_SortCycle(t *testing.T) {
	d, _ := newTestDashboard(t)

	if d.sortBy != sortByID {
		t.Errorf("initial sort = %d, want sortByID", d.sortBy)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if d.sortBy != sortByStatus {
		t.Errorf("sort after s = %d, want sortByStatus", d.sortBy)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if d.sortBy != sortByDuration {
		t.Errorf("sort after s = %d, want sortByDuration", d.sortBy)
	}

	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if d.sortBy != sortByID {
		t.Errorf("sort after s = %d, want sortByID (wrap around)", d.sortBy)
	}
}

func TestDashboard_Notifications(t *testing.T) {
	d, store := newTestDashboard(t)

	a := agent.NewAgent("notifier", "feat/n", "main", "/wt", "@1", "%1")
	store.Add(a)

	d, _ = d.Update(orchestrator.AgentFinishedMsg{
		AgentID:    a.ID,
		ExitCode:   0,
		HasChanges: true,
	})

	if len(d.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(d.notifications))
	}
	if !strings.Contains(d.notifications[0].text, "finished") {
		t.Errorf("notification text = %q, expected 'finished'", d.notifications[0].text)
	}
}
