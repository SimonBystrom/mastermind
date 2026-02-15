package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type mergeStep int

const (
	mergeStepConfirm mergeStep = iota
	mergeStepConflicts
)

type mergeModel struct {
	orch     *orchestrator.Orchestrator
	repoPath string
	step     mergeStep
	err      string
	width    int

	agentID   string
	agentName string
	branch    string
	baseBranch string

	// Cleanup options (toggled by user)
	deleteBranch   bool // default: true
	removeWorktree bool // default: true
	optionCursor   int  // 0 = removeWorktree, 1 = deleteBranch

	// Conflict info
	conflictFiles []string
}

type mergeDoneMsg struct{}
type mergeCancelMsg struct{}

// startMergeMsg is emitted by the dashboard when user presses 'm' on an agent.
type startMergeMsg struct {
	agentID    string
	agentName  string
	branch     string
	baseBranch string
}

func newMerge(orch *orchestrator.Orchestrator, repoPath string, msg startMergeMsg) mergeModel {
	return mergeModel{
		orch:           orch,
		repoPath:       repoPath,
		step:           mergeStepConfirm,
		agentID:        msg.agentID,
		agentName:      msg.agentName,
		branch:         msg.branch,
		baseBranch:     msg.baseBranch,
		deleteBranch:   true,
		removeWorktree: true,
	}
}

func (m mergeModel) Update(msg tea.Msg) (mergeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case orchestrator.MergeResultMsg:
		if msg.AgentID != m.agentID {
			return m, nil
		}
		if msg.Success {
			return m, func() tea.Msg { return mergeDoneMsg{} }
		}
		if msg.Conflict {
			m.step = mergeStepConflicts
			m.conflictFiles = msg.ConflictFiles
			return m, nil
		}
		if msg.Error != "" {
			m.err = msg.Error
		}
		return m, nil

	case tea.KeyMsg:
		m.err = ""

		if msg.String() == "esc" {
			return m, func() tea.Msg { return mergeCancelMsg{} }
		}

		switch m.step {
		case mergeStepConfirm:
			return m.updateConfirm(msg)
		case mergeStepConflicts:
			return m.updateConflicts(msg)
		}
	}

	return m, nil
}

func (m mergeModel) updateConfirm(msg tea.KeyMsg) (mergeModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.optionCursor < 1 {
			m.optionCursor++
		}
	case "k", "up":
		if m.optionCursor > 0 {
			m.optionCursor--
		}
	case " ":
		if m.optionCursor == 0 {
			m.removeWorktree = !m.removeWorktree
		} else {
			m.deleteBranch = !m.deleteBranch
		}
	case "y", "enter":
		mergeID := m.agentID
		delBranch := m.deleteBranch
		removeWT := m.removeWorktree
		return m, func() tea.Msg {
			return m.orch.MergeAgent(mergeID, delBranch, removeWT)
		}
	}
	return m, nil
}

func (m mergeModel) updateConflicts(msg tea.KeyMsg) (mergeModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if err := m.orch.OpenLazyGit(m.agentID); err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m, func() tea.Msg { return mergeDoneMsg{} }
	}
	return m, nil
}

func (m mergeModel) ViewContent() string {
	var b strings.Builder

	switch m.step {
	case mergeStepConfirm:
		b.WriteString(wizardTitleStyle.Render("Merge Agent"))
		b.WriteString("\n\n")

		b.WriteString(fmt.Sprintf("  Agent:       %s\n", m.agentName))
		b.WriteString(fmt.Sprintf("  Branch:      %s\n", m.branch))
		b.WriteString(fmt.Sprintf("  Into:        %s\n", m.baseBranch))
		b.WriteString("\n")
		b.WriteString(wizardActiveStyle.Render("  After merge:"))
		b.WriteString("\n")

		options := []struct {
			label   string
			checked bool
		}{
			{"Remove worktree", m.removeWorktree},
			{"Delete branch", m.deleteBranch},
		}
		for i, opt := range options {
			cursor := "  "
			if i == m.optionCursor {
				cursor = "> "
			}
			check := " "
			if opt.checked {
				check = "x"
			}
			line := fmt.Sprintf("  %s[%s] %s", cursor, check, opt.label)
			if i == m.optionCursor {
				b.WriteString(wizardActiveStyle.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  y/enter: merge | space: toggle | esc: cancel"))

	case mergeStepConflicts:
		b.WriteString(wizardTitleStyle.Render("Merge Agent — Conflicts"))
		b.WriteString("\n\n")

		b.WriteString(fmt.Sprintf("  Merging %s into %s\n", m.branch, m.baseBranch))
		b.WriteString("\n")
		b.WriteString(wizardActiveStyle.Render("  Conflicted files:"))
		b.WriteString("\n")

		if len(m.conflictFiles) == 0 {
			b.WriteString(wizardDimStyle.Render("    (no files detected)"))
			b.WriteString("\n")
		} else {
			for _, f := range m.conflictFiles {
				b.WriteString(fmt.Sprintf("    both modified:   %s\n", f))
			}
		}

		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  enter: open lazygit | esc: cancel"))
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(errorStyle.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m mergeModel) View() string {
	return borderStyle.Render(m.ViewContent())
}
