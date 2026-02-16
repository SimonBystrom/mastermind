package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type dismissModel struct {
	orch   *orchestrator.Orchestrator
	err    string
	width  int
	styles Styles

	agentID      string
	agentName    string
	branch       string
	deleteBranch bool
}

type dismissDoneMsg struct{}
type dismissCancelMsg struct{}

type startDismissMsg struct {
	agentID      string
	agentName    string
	branch       string
	deleteBranch bool
}

func newDismiss(s Styles, orch *orchestrator.Orchestrator, msg startDismissMsg) dismissModel {
	return dismissModel{
		orch:         orch,
		agentID:      msg.agentID,
		agentName:    msg.agentName,
		branch:       msg.branch,
		deleteBranch: msg.deleteBranch,
		styles:       s,
	}
}

func (m dismissModel) Update(msg tea.Msg) (dismissModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.err = ""

		switch msg.String() {
		case "esc", "n":
			return m, func() tea.Msg { return dismissCancelMsg{} }
		case "y", "enter":
			id := m.agentID
			del := m.deleteBranch
			return m, func() tea.Msg {
				if err := m.orch.DismissAgent(id, del); err != nil {
					return dismissErrorMsg{err: err.Error()}
				}
				return dismissDoneMsg{}
			}
		}
	case dismissErrorMsg:
		m.err = msg.err
		return m, nil
	}

	return m, nil
}

type dismissErrorMsg struct {
	err string
}

func (m dismissModel) ViewContent() string {
	var b strings.Builder

	if m.deleteBranch {
		b.WriteString(m.styles.WizardTitle.Render("Dismiss & Delete Agent"))
	} else {
		b.WriteString(m.styles.WizardTitle.Render("Dismiss Agent"))
	}
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  Agent:       %s\n", m.agentName))
	b.WriteString(fmt.Sprintf("  Branch:      %s\n", m.branch))
	b.WriteString("\n")

	b.WriteString(m.styles.WizardActive.Render("  This will:"))
	b.WriteString("\n")
	b.WriteString("    - Stop the Claude process\n")
	b.WriteString("    - Kill the tmux window\n")
	b.WriteString("    - Remove the worktree\n")
	if m.deleteBranch {
		b.WriteString("    - Delete the branch\n")
	}

	b.WriteString("\n")
	if m.deleteBranch {
		b.WriteString(m.styles.Error.Render("  All changes (committed and uncommitted) will be lost."))
	} else {
		b.WriteString(m.styles.Error.Render("  Any uncommitted changes will be lost."))
	}
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render("  y/enter: confirm | esc/n: cancel"))

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m dismissModel) View() string {
	return m.styles.Border.Render(m.ViewContent())
}
