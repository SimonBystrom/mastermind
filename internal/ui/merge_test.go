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

func newTestMerge(t *testing.T) mergeModel {
	t.Helper()
	store := agent.NewStore()
	orch := orchestrator.New(context.Background(), store, "/repo", "test", t.TempDir())
	return newMerge(NewStyles(config.Default().Colors), orch, "/repo", startMergeMsg{
		agentID:    "a1",
		agentName:  "test-agent",
		branch:     "feat/x",
		baseBranch: "main",
	})
}

func TestMerge_InitialState(t *testing.T) {
	m := newTestMerge(t)

	if m.step != mergeStepConfirm {
		t.Errorf("initial step = %d, want mergeStepConfirm", m.step)
	}
	if !m.deleteBranch {
		t.Error("deleteBranch should default to true")
	}
	if !m.removeWorktree {
		t.Error("removeWorktree should default to true")
	}
}

func TestMerge_ToggleOptions(t *testing.T) {
	m := newTestMerge(t)

	// Toggle removeWorktree (cursor at 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.removeWorktree {
		t.Error("removeWorktree should be false after toggle")
	}

	// Move to deleteBranch
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.deleteBranch {
		t.Error("deleteBranch should be false after toggle")
	}
}

func TestMerge_EscCancels(t *testing.T) {
	m := newTestMerge(t)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc")
	}
	msg := cmd()
	if _, ok := msg.(mergeCancelMsg); !ok {
		t.Errorf("expected mergeCancelMsg, got %T", msg)
	}
}

func TestMerge_SuccessMsg_Done(t *testing.T) {
	m := newTestMerge(t)

	_, cmd := m.Update(orchestrator.MergeResultMsg{
		AgentID: "a1",
		Success: true,
	})
	if cmd == nil {
		t.Fatal("expected command from success")
	}
	msg := cmd()
	if _, ok := msg.(mergeDoneMsg); !ok {
		t.Errorf("expected mergeDoneMsg, got %T", msg)
	}
}

func TestMerge_ConflictMsg_ShowsConflicts(t *testing.T) {
	m := newTestMerge(t)

	m, _ = m.Update(orchestrator.MergeResultMsg{
		AgentID:       "a1",
		Conflict:      true,
		ConflictFiles: []string{"a.txt", "b.txt"},
	})

	if m.step != mergeStepConflicts {
		t.Errorf("step = %d, want mergeStepConflicts", m.step)
	}
	if len(m.conflictFiles) != 2 {
		t.Errorf("expected 2 conflict files, got %d", len(m.conflictFiles))
	}
}

func TestMerge_ViewContent_Confirm(t *testing.T) {
	m := newTestMerge(t)

	content := m.ViewContent()
	if !strings.Contains(content, "test-agent") {
		t.Error("should show agent name")
	}
	if !strings.Contains(content, "feat/x") {
		t.Error("should show branch")
	}
	if !strings.Contains(content, "main") {
		t.Error("should show base branch")
	}
	if !strings.Contains(content, "Remove worktree") {
		t.Error("should show worktree toggle")
	}
	if !strings.Contains(content, "Delete branch") {
		t.Error("should show branch toggle")
	}
}

func TestMerge_ViewContent_Conflicts(t *testing.T) {
	m := newTestMerge(t)
	m.step = mergeStepConflicts
	m.conflictFiles = []string{"file1.go", "file2.go"}

	content := m.ViewContent()
	if !strings.Contains(content, "Conflicts") {
		t.Error("should show conflicts title")
	}
	if !strings.Contains(content, "file1.go") {
		t.Error("should show conflict file")
	}
}
