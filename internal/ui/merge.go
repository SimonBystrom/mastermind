package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type mergeStep int

const (
	mergeStepConfirm mergeStep = iota
	mergeStepMerging
	mergeStepConflicts
)

type mergeModel struct {
	orch     *orchestrator.Orchestrator
	repoPath string
	step     mergeStep
	err      string
	width    int
	styles   Styles

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

	// Spinner shown during merge
	spinner spinner.Model
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

func newMerge(s Styles, orch *orchestrator.Orchestrator, repoPath string, msg startMergeMsg) mergeModel {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
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
		styles:         s,
		spinner:        sp,
	}
}

func (m mergeModel) Update(msg tea.Msg) (mergeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case orchestrator.PruneResultMsg:
		if msg.AgentID != m.agentID {
			return m, nil
		}
		if msg.Success {
			return m, func() tea.Msg { return mergeDoneMsg{} }
		}
		m.step = mergeStepConfirm
		if msg.Error != "" {
			m.err = msg.Error
		}
		return m, nil

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
		m.step = mergeStepConfirm
		if msg.Error != "" {
			m.err = msg.Error
		}
		return m, nil

	case spinner.TickMsg:
		if m.step == mergeStepMerging {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.step == mergeStepMerging {
			return m, nil
		}

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

func (m mergeModel) isExistingBranch() bool {
	return m.baseBranch == ""
}

func (m mergeModel) updateConfirm(msg tea.KeyMsg) (mergeModel, tea.Cmd) {
	if m.isExistingBranch() {
		// Existing branch: no options to toggle, just confirm/cancel
		switch msg.String() {
		case "y", "enter":
			m.step = mergeStepMerging
			pruneID := m.agentID
			pruneCmd := func() tea.Msg {
				return m.orch.PruneAgent(pruneID)
			}
			return m, tea.Batch(m.spinner.Tick, pruneCmd)
		}
		return m, nil
	}

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
		m.step = mergeStepMerging
		mergeID := m.agentID
		delBranch := m.deleteBranch
		removeWT := m.removeWorktree
		mergeCmd := func() tea.Msg {
			return m.orch.MergeAgent(mergeID, delBranch, removeWT)
		}
		return m, tea.Batch(m.spinner.Tick, mergeCmd)
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
	case mergeStepConfirm, mergeStepMerging:
		if m.isExistingBranch() {
			b.WriteString(m.styles.WizardTitle.Render("Detach Agent"))
			b.WriteString("\n\n")

			b.WriteString(fmt.Sprintf("  Agent:       %s\n", m.agentName))
			b.WriteString(fmt.Sprintf("  Branch:      %s\n", m.branch))
			b.WriteString("\n")
			b.WriteString(m.styles.Reviewed.Render("  Branch will be kept as-is"))
			b.WriteString("\n\n")

			b.WriteString(m.styles.WizardActive.Render("  This will:"))
			b.WriteString("\n")
			b.WriteString("    - Stop the Claude process\n")
			b.WriteString("    - Kill the tmux window\n")
			b.WriteString("    - Remove the worktree\n")

			b.WriteString("\n")
			if m.step == mergeStepMerging {
				b.WriteString(m.styles.WizardActive.Render("  " + m.spinner.View() + " Detaching..."))
			} else {
				b.WriteString(m.styles.Help.Render("  y/enter: detach | esc: cancel"))
			}
		} else {
			b.WriteString(m.styles.WizardTitle.Render("Merge Agent"))
			b.WriteString("\n\n")

			b.WriteString(fmt.Sprintf("  Agent:       %s\n", m.agentName))
			b.WriteString(fmt.Sprintf("  Branch:      %s\n", m.branch))
			b.WriteString(fmt.Sprintf("  Into:        %s\n", m.baseBranch))
			b.WriteString("\n")
			b.WriteString(m.styles.WizardActive.Render("  After merge:"))
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
					b.WriteString(m.styles.WizardActive.Render(line))
				} else {
					b.WriteString(line)
				}
				b.WriteString("\n")
			}

			b.WriteString("\n")
			if m.step == mergeStepMerging {
				b.WriteString(m.styles.WizardActive.Render("  " + m.spinner.View() + " Merging..."))
			} else {
				b.WriteString(m.styles.Help.Render("  y/enter: merge | space: toggle | esc: cancel"))
			}
		}

	case mergeStepConflicts:
		b.WriteString(m.styles.WizardTitle.Render("Merge Agent — Conflicts"))
		b.WriteString("\n\n")

		b.WriteString(fmt.Sprintf("  Merging %s into %s\n", m.branch, m.baseBranch))
		b.WriteString("\n")
		b.WriteString(m.styles.WizardActive.Render("  Conflicted files:"))
		b.WriteString("\n")

		if len(m.conflictFiles) == 0 {
			b.WriteString(m.styles.WizardDim.Render("    (no files detected)"))
			b.WriteString("\n")
		} else {
			for _, f := range m.conflictFiles {
				b.WriteString(fmt.Sprintf("    both modified:   %s\n", f))
			}
		}

		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("  enter: open lazygit | esc: cancel"))
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m mergeModel) View() string {
	return m.styles.Border.Render(m.ViewContent())
}
