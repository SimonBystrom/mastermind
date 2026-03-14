package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/git"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

func newTestSpawn(t *testing.T) spawnModel {
	t.Helper()
	store := agent.NewStore()
	orch := orchestrator.New(context.Background(), store, "/repo", "test", t.TempDir())
	return newSpawn(NewStyles(config.Default().Colors), orch, "/repo", 120, "claude")
}

func TestSpawn_InitialStep(t *testing.T) {
	m := newTestSpawn(t)

	if m.step != stepChooseHarness {
		t.Errorf("initial step = %d, want %d (stepChooseHarness)", m.step, stepChooseHarness)
	}
}

func TestSpawn_EscCancels(t *testing.T) {
	m := newTestSpawn(t)

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected command from Esc")
	}
	msg := cmd()
	if _, ok := msg.(spawnCancelMsg); !ok {
		t.Errorf("expected spawnCancelMsg, got %T", msg)
	}
}

func TestSpawn_ChooseMode_ExistingBranch(t *testing.T) {
	m := newTestSpawn(t)

	// Simulate branches loaded
	m.Update(branchesLoadedMsg{
		branches: []git.Branch{
			{Name: "main", Current: true},
			{Name: "feat/x", Current: false},
		},
	})

	// Select harness first (default is Claude Code at cursor 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Move to existing branch option (default cursor position)
	// Press enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.step != stepPickBranch {
		t.Errorf("step = %d, want %d (stepPickBranch)", m.step, stepPickBranch)
	}
}

func TestSpawn_ChooseMode_NewBranch(t *testing.T) {
	m := newTestSpawn(t)

	// Select harness first (default is Claude Code at cursor 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Move cursor down to "new branch"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Press enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.step != stepNewBranchName {
		t.Errorf("step = %d, want %d (stepNewBranchName)", m.step, stepNewBranchName)
	}
	if m.mode != modeNew {
		t.Errorf("mode = %d, want %d (modeNew)", m.mode, modeNew)
	}
}

func TestSpawn_EscFromPickBranch_GoesBack(t *testing.T) {
	m := newTestSpawn(t)
	m.step = stepPickBranch
	m.mode = modeExisting

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.step != stepChooseMode {
		t.Errorf("step = %d, want %d (stepChooseMode)", m.step, stepChooseMode)
	}
}

func TestSpawn_BranchesLoadedMsg(t *testing.T) {
	m := newTestSpawn(t)

	branches := []git.Branch{
		{Name: "main", Current: true},
		{Name: "feat/x", Current: false},
	}
	m, _ = m.Update(branchesLoadedMsg{branches: branches})

	if len(m.branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(m.branches))
	}
}

func TestSpawn_ViewContent_ChooseMode(t *testing.T) {
	m := newTestSpawn(t)
	
	// Should start at harness selection
	view := m.View()
	if !strings.Contains(view, "Claude Code") {
		t.Error("should show Claude Code option")
	}
	if !strings.Contains(view, "OpenCode") {
		t.Error("should show OpenCode option")
	}

	// Select harness to move to mode selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view = m.View()

	if !strings.Contains(view, "Use existing branch") {
		t.Error("should show existing branch option")
	}
	if !strings.Contains(view, "Create new branch") {
		t.Error("should show new branch option")
	}
}

func TestSpawn_ViewContent_Confirm(t *testing.T) {
	m := newTestSpawn(t)
	m.step = stepConfirm
	m.branch = "feat/test"
	m.baseBranch = "main"
	m.createBranch = true

	content := m.ViewContent()
	if !strings.Contains(content, "feat/test") {
		t.Error("confirm should show branch")
	}
}
