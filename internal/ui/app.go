package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type view int

const (
	viewDashboard view = iota
	viewSpawn
	viewMerge
	viewDismiss
)

type AppModel struct {
	orch      *orchestrator.Orchestrator
	store     *agent.Store
	repoPath  string
	session   string
	activeView view

	styles Styles
	layout config.Layout

	dashboard dashboardModel
	spawn     spawnModel
	merge     mergeModel
	dismiss   dismissModel

	width  int
	height int
}

func NewApp(cfg config.Config, orch *orchestrator.Orchestrator, store *agent.Store, repoPath, session string) AppModel {
	s := NewStyles(cfg.Colors)
	return AppModel{
		orch:       orch,
		store:      store,
		repoPath:   repoPath,
		session:    session,
		activeView: viewDashboard,
		styles:     s,
		layout:     cfg.Layout,
		dashboard:  newDashboard(s, cfg.Layout, orch, store, repoPath, session),
	}
}

func (m AppModel) Init() tea.Cmd {
	return m.dashboard.Init()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.dashboard.width = msg.Width
		m.dashboard.height = msg.Height
		m.spawn.width = msg.Width
		if m.activeView == viewSpawn {
			m.spawn.branchList.SetSize(max(msg.Width-8, 20), 15)
		}
		m.merge.width = msg.Width
		m.dismiss.width = msg.Width
		return m, nil

	case tea.FocusMsg:
		// When the tmux pane regains focus, force a full repaint so the
		// screen is correct after tmux restores its buffer, and schedule
		// an immediate tick so durations update without waiting.
		return m, tea.Batch(tea.ClearScreen, tickCmd())

	case tickMsg:
		// Always keep the tick chain alive regardless of active view,
		// and always forward to dashboard so it can update durations.
		var dashCmd tea.Cmd
		m.dashboard, dashCmd = m.dashboard.Update(msg)
		return m, tea.Batch(dashCmd, tickCmd())

	case orchestrator.AgentFinishedMsg:
		// Always forward agent-finished notifications to dashboard.
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.AgentWaitingMsg:
		// Always forward agent-waiting notifications to dashboard.
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.AgentGoneMsg:
		// Window was closed externally — forward to dashboard and clean up.
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.AgentReviewedMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.PreviewStartedMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.PreviewStoppedMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.PreviewErrorMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case orchestrator.MergeResultMsg:
		var dashCmd tea.Cmd
		m.dashboard, dashCmd = m.dashboard.Update(msg)
		if m.activeView == viewMerge {
			var mergeCmd tea.Cmd
			m.merge, mergeCmd = m.merge.Update(msg)
			return m, tea.Batch(dashCmd, mergeCmd)
		}
		return m, dashCmd

	case orchestrator.CleanupMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case spawnDoneMsg:
		m.activeView = viewDashboard
		return m, nil

	case spawnCancelMsg:
		m.activeView = viewDashboard
		return m, nil

	case startMergeMsg:
		m.activeView = viewMerge
		m.merge = newMerge(m.styles, m.orch, m.repoPath, msg)
		return m, nil

	case mergeDoneMsg:
		m.activeView = viewDashboard
		return m, nil

	case mergeCancelMsg:
		m.activeView = viewDashboard
		return m, nil

	case startDismissMsg:
		m.activeView = viewDismiss
		m.dismiss = newDismiss(m.styles, m.orch, msg)
		return m, nil

	case dismissDoneMsg:
		m.activeView = viewDashboard
		// Adjust cursor after agent removal
		agents := m.dashboard.sortedAgents()
		if m.dashboard.cursor >= len(agents) && m.dashboard.cursor > 0 {
			m.dashboard.cursor = len(agents) - 1
		}
		return m, nil

	case dismissCancelMsg:
		m.activeView = viewDashboard
		return m, nil

	case dismissErrorMsg:
		if m.activeView == viewDismiss {
			var cmd tea.Cmd
			m.dismiss, cmd = m.dismiss.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.activeView {
	case viewDashboard:
		return m.updateDashboard(msg)
	case viewSpawn:
		return m.updateSpawn(msg)
	case viewMerge:
		return m.updateMerge(msg)
	case viewDismiss:
		return m.updateDismiss(msg)
	}

	return m, nil
}

func (m AppModel) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.activeView = viewSpawn
			m.spawn = newSpawn(m.styles, m.orch, m.repoPath, m.width)
			return m, m.spawn.Init()
		}
	}

	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.Update(msg)
	return m, cmd
}

func (m AppModel) updateSpawn(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spawn, cmd = m.spawn.Update(msg)
	return m, cmd
}

func (m AppModel) updateMerge(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.merge, cmd = m.merge.Update(msg)
	return m, cmd
}

func (m AppModel) updateDismiss(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dismiss, cmd = m.dismiss.Update(msg)
	return m, cmd
}

func (m AppModel) View() string {
	switch m.activeView {
	case viewSpawn:
		return m.viewSideBySide(m.spawn.ViewContent())
	case viewMerge:
		return m.viewSideBySide(m.merge.ViewContent())
	case viewDismiss:
		return m.viewSideBySide(m.dismiss.ViewContent())
	default:
		return m.dashboard.View()
	}
}

// minSideBySideWidth is the minimum terminal width needed to show
// dashboard and sidebar side-by-side. Below this, panels stack vertically.
const minSideBySideWidth = 100

func (m AppModel) viewSideBySide(rightPanel string) string {
	maxWidth := m.width - 4
	if maxWidth < 20 {
		maxWidth = 20
	}

	// Narrow terminal: stack panels vertically
	if m.width < minSideBySideWidth {
		// Dashboard gets the full width in stacked mode
		dash := m.dashboard
		dash.width = maxWidth + 8 // contentWidth() subtracts 8 for border+padding
		dashContent := lipgloss.NewStyle().Width(maxWidth).Render(dash.ViewContent())
		sep := m.styles.Separator.Render(strings.Repeat("─", maxWidth))
		panelContent := lipgloss.NewStyle().Width(maxWidth).Render(rightPanel)
		joined := lipgloss.JoinVertical(lipgloss.Left, dashContent, sep, panelContent)
		return m.styles.Border.Width(maxWidth).Render(joined)
	}

	// Wide terminal: side-by-side
	dashWidth := maxWidth * m.layout.DashboardWidth / 100
	panelWidth := maxWidth - dashWidth - 1

	// Give dashboard the constrained width so logo/columns adapt
	dash := m.dashboard
	dash.width = dashWidth + 8 // contentWidth() subtracts 8 for border+padding
	dashContent := lipgloss.NewStyle().Width(dashWidth).Render(dash.ViewContent())
	panelContent := lipgloss.NewStyle().Width(panelWidth).Render(rightPanel)

	// Build a vertical separator matching the height of the taller panel
	dashHeight := lipgloss.Height(dashContent)
	panelHeight := lipgloss.Height(panelContent)
	sepHeight := dashHeight
	if panelHeight > sepHeight {
		sepHeight = panelHeight
	}
	sepLines := make([]string, sepHeight)
	for i := range sepLines {
		sepLines[i] = "│"
	}
	sep := m.styles.Separator.Render(strings.Join(sepLines, "\n"))

	joined := lipgloss.JoinHorizontal(lipgloss.Top, dashContent, sep, panelContent)

	return m.styles.Border.Width(maxWidth).Render(joined)
}
