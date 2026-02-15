package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type view int

const (
	viewDashboard view = iota
	viewSpawn
	viewMerge
)

type AppModel struct {
	orch      *orchestrator.Orchestrator
	store     *agent.Store
	repoPath  string
	session   string
	activeView view

	dashboard dashboardModel
	spawn     spawnModel
	merge     mergeModel

	width  int
	height int
}

func NewApp(orch *orchestrator.Orchestrator, store *agent.Store, repoPath, session string) AppModel {
	return AppModel{
		orch:       orch,
		store:      store,
		repoPath:   repoPath,
		session:    session,
		activeView: viewDashboard,
		dashboard:  newDashboard(orch, store, repoPath, session),
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
		m.merge.width = msg.Width
		return m, nil

	case tea.FocusMsg:
		// When the tmux pane regains focus, force an immediate tick so
		// durations are up-to-date without waiting for the next 1-second tick.
		return m, tickCmd()

	case tickMsg:
		// Always keep the tick chain alive regardless of active view,
		// and always forward to dashboard so it can update durations.
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, tickCmd()

	case orchestrator.AgentFinishedMsg:
		// Always forward agent-finished notifications to dashboard.
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, nil

	case orchestrator.AgentWaitingMsg:
		// Always forward agent-waiting notifications to dashboard.
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, nil

	case orchestrator.AgentGoneMsg:
		// Window was closed externally — forward to dashboard and clean up.
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, nil

	case orchestrator.AgentReviewedMsg:
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, nil

	case orchestrator.MergeResultMsg:
		m.dashboard, _ = m.dashboard.Update(msg)
		if m.activeView == viewMerge {
			var cmd tea.Cmd
			m.merge, cmd = m.merge.Update(msg)
			return m, cmd
		}
		return m, nil

	case orchestrator.CleanupMsg:
		m.dashboard, _ = m.dashboard.Update(msg)
		return m, nil

	case spawnDoneMsg:
		m.activeView = viewDashboard
		return m, nil

	case spawnCancelMsg:
		m.activeView = viewDashboard
		return m, nil

	case startMergeMsg:
		m.activeView = viewMerge
		m.merge = newMerge(m.orch, m.repoPath, msg)
		return m, nil

	case mergeDoneMsg:
		m.activeView = viewDashboard
		return m, nil

	case mergeCancelMsg:
		m.activeView = viewDashboard
		return m, nil
	}

	switch m.activeView {
	case viewDashboard:
		return m.updateDashboard(msg)
	case viewSpawn:
		return m.updateSpawn(msg)
	case viewMerge:
		return m.updateMerge(msg)
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
			m.spawn = newSpawn(m.orch, m.repoPath)
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

func (m AppModel) View() string {
	switch m.activeView {
	case viewSpawn:
		return m.viewSideBySide(m.spawn.ViewContent())
	case viewMerge:
		return m.viewSideBySide(m.merge.ViewContent())
	default:
		return m.dashboard.View()
	}
}

func (m AppModel) viewSideBySide(rightPanel string) string {
	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 80
	}

	// 55% for dashboard, 45% for right panel, minus 1 for separator
	dashWidth := maxWidth * 55 / 100
	panelWidth := maxWidth - dashWidth - 1

	dashContent := lipgloss.NewStyle().Width(dashWidth).Render(m.dashboard.ViewContent())
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
	sep := separatorStyle.Render(strings.Join(sepLines, "\n"))

	joined := lipgloss.JoinHorizontal(lipgloss.Top, dashContent, sep, panelContent)

	return borderStyle.Width(maxWidth).Render(joined)
}
