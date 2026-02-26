package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
	dismissing   bool

	spinner spinner.Model
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
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	return dismissModel{
		orch:         orch,
		agentID:      msg.agentID,
		agentName:    msg.agentName,
		branch:       msg.branch,
		deleteBranch: msg.deleteBranch,
		styles:       s,
		spinner:      sp,
	}
}

func (m dismissModel) Update(msg tea.Msg) (dismissModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.dismissing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.dismissing {
			return m, nil
		}

		m.err = ""

		switch msg.String() {
		case "esc", "n":
			return m, func() tea.Msg { return dismissCancelMsg{} }
		case "y", "enter":
			m.dismissing = true
			id := m.agentID
			del := m.deleteBranch
			dismissCmd := func() tea.Msg {
				if err := m.orch.DismissAgent(id, del); err != nil {
					return dismissErrorMsg{err: err.Error()}
				}
				return dismissDoneMsg{}
			}
			return m, tea.Batch(m.spinner.Tick, dismissCmd)
		}

	case dismissErrorMsg:
		m.dismissing = false
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
	if m.dismissing {
		b.WriteString(m.styles.WizardActive.Render("  " + m.spinner.View() + " Dismissing..."))
	} else {
		b.WriteString(m.styles.Help.Render("  y/enter: confirm | esc/n: cancel"))
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m dismissModel) View() string {
	return m.styles.Border.Render(m.ViewContent())
}
