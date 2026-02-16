package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

func newTestDismiss(t *testing.T, deleteBranch bool) dismissModel {
	t.Helper()
	store := agent.NewStore()
	orch := orchestrator.New(context.Background(), store, "/repo", "test", t.TempDir())
	return newDismiss(NewStyles(config.Default().Colors), orch, startDismissMsg{
		agentID:      "a1",
		agentName:    "test-agent",
		branch:       "feat/x",
		deleteBranch: deleteBranch,
	})
}

func TestDismiss_EscCancels(t *testing.T) {
	m := newTestDismiss(t, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc")
	}
	msg := cmd()
	if _, ok := msg.(dismissCancelMsg); !ok {
		t.Errorf("expected dismissCancelMsg, got %T", msg)
	}
}

func TestDismiss_NCancels(t *testing.T) {
	m := newTestDismiss(t, false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil {
		t.Fatal("expected command from 'n'")
	}
	msg := cmd()
	if _, ok := msg.(dismissCancelMsg); !ok {
		t.Errorf("expected dismissCancelMsg, got %T", msg)
	}
}

func TestDismiss_ViewContent_WithoutDelete(t *testing.T) {
	m := newTestDismiss(t, false)

	content := m.ViewContent()
	if !strings.Contains(content, "Dismiss Agent") {
		t.Error("should show dismiss title")
	}
	if !strings.Contains(content, "test-agent") {
		t.Error("should show agent name")
	}
	if !strings.Contains(content, "feat/x") {
		t.Error("should show branch")
	}
	if strings.Contains(content, "Delete the branch") {
		t.Error("should NOT show delete branch when deleteBranch=false")
	}
}

func TestDismiss_ViewContent_WithDelete(t *testing.T) {
	m := newTestDismiss(t, true)

	content := m.ViewContent()
	if !strings.Contains(content, "Dismiss & Delete") {
		t.Error("should show delete title")
	}
	if !strings.Contains(content, "Delete the branch") {
		t.Error("should show delete branch action")
	}
	if !strings.Contains(content, "committed and uncommitted") {
		t.Error("should warn about all data loss")
	}
}

func TestDismiss_ErrorMsg(t *testing.T) {
	m := newTestDismiss(t, false)

	m, _ = m.Update(dismissErrorMsg{err: "something went wrong"})
	if m.err != "something went wrong" {
		t.Errorf("err = %q, want %q", m.err, "something went wrong")
	}

	content := m.ViewContent()
	if !strings.Contains(content, "something went wrong") {
		t.Error("should display error")
	}
}
