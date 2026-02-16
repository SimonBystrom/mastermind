package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

func newTestApp(t *testing.T) AppModel {
	t.Helper()
	store := agent.NewStore()
	orch := orchestrator.New(context.Background(), store, "/repo", "test", t.TempDir())
	return NewApp(config.Default(), orch, store, "/repo", "test")
}

func TestAppModel_KeyQ_Quits(t *testing.T) {
	m := newTestApp(t)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	_ = updated

	if cmd == nil {
		t.Fatal("expected a command from 'q' key")
	}
	// Execute the cmd to check if it produces tea.Quit
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestAppModel_KeyN_OpensSpawn(t *testing.T) {
	m := newTestApp(t)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app := updated.(AppModel)

	if app.activeView != viewSpawn {
		t.Errorf("activeView = %d, want %d (viewSpawn)", app.activeView, viewSpawn)
	}
}

func TestAppModel_WindowSizeMsg(t *testing.T) {
	m := newTestApp(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := updated.(AppModel)

	if app.width != 120 {
		t.Errorf("width = %d, want 120", app.width)
	}
	if app.height != 40 {
		t.Errorf("height = %d, want 40", app.height)
	}
}

func TestAppModel_ForwardsAgentMessages(t *testing.T) {
	m := newTestApp(t)

	// Add an agent to the store
	a := agent.NewAgent("test", "feat/x", "main", "/wt", "@1", "%1")
	m.store.Add(a)

	// Send AgentFinishedMsg
	updated, _ := m.Update(orchestrator.AgentFinishedMsg{
		AgentID:    a.ID,
		ExitCode:   0,
		HasChanges: true,
	})
	app := updated.(AppModel)

	// Dashboard should have a notification
	if len(app.dashboard.notifications) == 0 {
		t.Error("expected notification from AgentFinishedMsg")
	}
}

func TestAppModel_SpawnDoneReturns(t *testing.T) {
	m := newTestApp(t)
	m.activeView = viewSpawn

	updated, _ := m.Update(spawnDoneMsg{})
	app := updated.(AppModel)

	if app.activeView != viewDashboard {
		t.Errorf("activeView = %d, want %d (viewDashboard)", app.activeView, viewDashboard)
	}
}

func TestAppModel_MergeDoneReturns(t *testing.T) {
	m := newTestApp(t)
	m.activeView = viewMerge

	updated, _ := m.Update(mergeDoneMsg{})
	app := updated.(AppModel)

	if app.activeView != viewDashboard {
		t.Errorf("activeView = %d, want %d (viewDashboard)", app.activeView, viewDashboard)
	}
}

func TestAppModel_DismissCancelReturns(t *testing.T) {
	m := newTestApp(t)
	m.activeView = viewDismiss

	updated, _ := m.Update(dismissCancelMsg{})
	app := updated.(AppModel)

	if app.activeView != viewDashboard {
		t.Errorf("activeView = %d, want %d (viewDashboard)", app.activeView, viewDashboard)
	}
}
