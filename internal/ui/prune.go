package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type pruneModel struct {
	orch   *orchestrator.Orchestrator
	err    string
	width  int
	styles Styles

	agentID   string
	agentName string
	branch    string
	pruning   bool

	hasUncommitted bool

	spinner spinner.Model
}

type pruneDoneMsg struct{}
type pruneCancelMsg struct{}

type startPruneMsg struct {
	agentID   string
	agentName string
	branch    string
}

func newPrune(s Styles, orch *orchestrator.Orchestrator, msg startPruneMsg) pruneModel {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	return pruneModel{
		orch:      orch,
		agentID:   msg.agentID,
		agentName: msg.agentName,
		branch:    msg.branch,
		styles:    s,
		spinner:   sp,
	}
}

func (m pruneModel) Update(msg tea.Msg) (pruneModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.pruning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case orchestrator.PruneResultMsg:
		if msg.AgentID != m.agentID {
			return m, nil
		}
		if msg.Success {
			return m, func() tea.Msg { return pruneDoneMsg{} }
		}
		m.pruning = false
		m.hasUncommitted = msg.HasUncommitted
		if msg.Error != "" {
			m.err = msg.Error
		}
		return m, nil

	case tea.KeyMsg:
		if m.pruning {
			return m, nil
		}

		m.err = ""

		switch msg.String() {
		case "esc", "n":
			return m, func() tea.Msg { return pruneCancelMsg{} }
		case "enter":
			if m.hasUncommitted {
				// Open lazygit to let user commit
				if err := m.orch.OpenLazyGit(m.agentID); err != nil {
					m.err = err.Error()
					return m, nil
				}
				return m, func() tea.Msg { return pruneCancelMsg{} }
			}
			return m.startPrune()
		case "y":
			if !m.hasUncommitted {
				return m.startPrune()
			}
		}
	}

	return m, nil
}

func (m pruneModel) startPrune() (pruneModel, tea.Cmd) {
	m.pruning = true
	id := m.agentID
	pruneCmd := func() tea.Msg {
		return m.orch.PruneAgent(id)
	}
	return m, tea.Batch(m.spinner.Tick, pruneCmd)
}

func (m pruneModel) ViewContent() string {
	var b strings.Builder

	b.WriteString(m.styles.WizardTitle.Render("Prune Worktree"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  Agent:       %s\n", m.agentName))
	b.WriteString(fmt.Sprintf("  Branch:      %s\n", m.branch))
	b.WriteString("\n")

	b.WriteString(m.styles.WizardActive.Render("  This will:"))
	b.WriteString("\n")
	b.WriteString("    - Stop the Claude process\n")
	b.WriteString("    - Kill the tmux window\n")
	b.WriteString("    - Remove the worktree\n")
	b.WriteString("\n")
	b.WriteString(m.styles.Reviewed.Render("  The branch and all committed changes will be kept."))
	b.WriteString("\n")

	b.WriteString("\n")
	if m.pruning {
		b.WriteString(m.styles.WizardActive.Render("  " + m.spinner.View() + " Pruning..."))
	} else if m.hasUncommitted {
		b.WriteString(m.styles.Error.Render("  Uncommitted changes in worktree"))
		b.WriteString("\n\n")
		b.WriteString(m.styles.Help.Render("  enter: open lazygit | esc: cancel"))
	} else {
		b.WriteString(m.styles.Help.Render("  y/enter: confirm | esc/n: cancel"))
	}

	if m.err != "" && !m.hasUncommitted {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m pruneModel) View() string {
	return m.styles.Border.Render(m.ViewContent())
}
