package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type sortMode int

const (
	sortByID sortMode = iota
	sortByStatus
	sortByDuration
)

type notification struct {
	text  string
	time  time.Time
	style lipgloss.Style
}

type tickMsg time.Time

type dashboardModel struct {
	store         *agent.Store
	orch          *orchestrator.Orchestrator
	repoPath      string
	session       string
	cursor        int
	notifications []notification
	width         int
	height        int
	err           string
	sortBy        sortMode

	// Confirmation state for dismiss+delete
	confirmDelete    bool
	confirmAgentID   string
	confirmAgentName string
	confirmBranch    string

	// Confirmation state for merge
	confirmMerge    bool
	mergeAgentID    string
	mergeAgentName  string
	mergeBranch     string
	mergeBaseBranch string
}

func newDashboard(orch *orchestrator.Orchestrator, store *agent.Store, repoPath, session string) dashboardModel {
	return dashboardModel{
		store:    store,
		orch:     orch,
		repoPath: repoPath,
		session:  session,
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m dashboardModel) Init() tea.Cmd {
	return tickCmd()
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case orchestrator.AgentFinishedMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		var text string
		var style lipgloss.Style
		if msg.HasChanges {
			text = fmt.Sprintf("Agent %s finished with changes — ready for review (exit %d)", name, msg.ExitCode)
			style = reviewReadyStyle
		} else {
			text = fmt.Sprintf("Agent %s finished with no changes (exit %d)", name, msg.ExitCode)
			style = doneStyle
		}
		m.notifications = append(m.notifications, notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		return m, nil

	case orchestrator.AgentGoneMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		m.store.Remove(msg.AgentID)
		m.notifications = append(m.notifications, notification{
			text:  fmt.Sprintf("Agent %s window closed", name),
			time:  time.Now(),
			style: doneStyle,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		agents := m.sortedAgents()
		if m.cursor >= len(agents) && m.cursor > 0 {
			m.cursor = len(agents) - 1
		}
		return m, nil

	case orchestrator.AgentReviewedMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		var text string
		var style lipgloss.Style
		if msg.NewCommits {
			text = fmt.Sprintf("Agent %s review complete — new commits found", name)
			style = reviewedStyle
		} else {
			text = fmt.Sprintf("Agent %s review closed — no new commits", name)
			style = doneStyle
		}
		m.notifications = append(m.notifications, notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		return m, nil

	case orchestrator.MergeResultMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		var text string
		var style lipgloss.Style
		if msg.Success {
			text = fmt.Sprintf("Agent %s merged successfully", name)
			style = reviewedStyle
		} else if msg.Conflict {
			text = fmt.Sprintf("Agent %s merge has conflicts — resolve in lazygit", name)
			style = conflictsStyle
		} else if msg.Error != "" {
			text = fmt.Sprintf("Agent %s merge failed: %s", name, msg.Error)
			style = errorStyle
		}
		m.notifications = append(m.notifications, notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		agents := m.sortedAgents()
		if m.cursor >= len(agents) && m.cursor > 0 {
			m.cursor = len(agents) - 1
		}
		return m, nil

	case orchestrator.AgentWaitingMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		var text string
		var style lipgloss.Style
		if msg.WaitingFor == "permission" {
			text = fmt.Sprintf("Agent %s needs permission approval", name)
			style = permissionStyle
		} else if msg.WaitingFor == "unknown" {
			text = fmt.Sprintf("Agent %s may need attention", name)
			style = attentionStyle
		} else {
			text = fmt.Sprintf("Agent %s is waiting for input", name)
			style = waitingStyle
		}
		m.notifications = append(m.notifications, notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		return m, nil

	case tea.KeyMsg:
		m.err = ""

		// Handle confirmation prompt for dismiss+delete
		if m.confirmDelete {
			switch msg.String() {
			case "y":
				if err := m.orch.DismissAgent(m.confirmAgentID, true); err != nil {
					m.err = err.Error()
				}
				agents := m.sortedAgents()
				if m.cursor > 0 && m.cursor >= len(agents) {
					m.cursor = len(agents) - 1
				}
				m.confirmDelete = false
			case "n", "esc":
				m.confirmDelete = false
			}
			return m, nil
		}

		// Handle confirmation prompt for merge
		if m.confirmMerge {
			switch msg.String() {
			case "y":
				if err := m.orch.MergeAgent(m.mergeAgentID); err != nil {
					m.err = err.Error()
				}
				agents := m.sortedAgents()
				if m.cursor > 0 && m.cursor >= len(agents) {
					m.cursor = len(agents) - 1
				}
				m.confirmMerge = false
			case "n", "esc":
				m.confirmMerge = false
			}
			return m, nil
		}

		agents := m.sortedAgents()

		switch msg.String() {
		case "j", "down":
			if m.cursor < len(agents)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "s":
			m.sortBy = (m.sortBy + 1) % 3
		case "enter":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				status := a.GetStatus()
				switch status {
				case agent.StatusReviewReady:
					if err := m.orch.OpenLazyGit(a.ID); err != nil {
						m.err = err.Error()
					} else {
						m.store.UpdateStatus(a.ID, agent.StatusReviewing)
					}
				case agent.StatusReviewed:
					if err := m.orch.OpenLazyGit(a.ID); err != nil {
						m.err = err.Error()
					} else {
						m.store.UpdateStatus(a.ID, agent.StatusReviewing)
					}
				case agent.StatusConflicts:
					if err := m.orch.OpenLazyGit(a.ID); err != nil {
						m.err = err.Error()
					}
					// Status stays StatusConflicts
				case agent.StatusRunning, agent.StatusWaiting, agent.StatusReviewing, agent.StatusDone:
					if err := m.orch.FocusAgent(a.ID); err != nil {
						m.err = err.Error()
					}
				}
			}
		case "m":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				status := a.GetStatus()
				if status == agent.StatusReviewed || status == agent.StatusReviewReady {
					m.confirmMerge = true
					m.mergeAgentID = a.ID
					m.mergeBranch = a.Branch
					m.mergeBaseBranch = a.BaseBranch
					m.mergeAgentName = a.Name
					if m.mergeAgentName == "" {
						m.mergeAgentName = a.ID
					}
				}
			}
		case "d":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				status := a.GetStatus()
				if status == agent.StatusDone || status == agent.StatusReviewReady || status == agent.StatusReviewing || status == agent.StatusReviewed || status == agent.StatusConflicts {
					if err := m.orch.DismissAgent(a.ID, false); err != nil {
						m.err = err.Error()
					}
					if m.cursor > 0 && m.cursor >= len(agents)-1 {
						m.cursor--
					}
				}
			}
		case "c":
			results := m.orch.CleanupDeadAgents()
			if len(results) > 0 {
				for _, r := range results {
					m.notifications = append(m.notifications, notification{
						text:  fmt.Sprintf("Cleaned up %s (%s)", r.AgentName, r.Reason),
						time:  time.Now(),
						style: doneStyle,
					})
				}
				if len(m.notifications) > 10 {
					m.notifications = m.notifications[len(m.notifications)-10:]
				}
				agents = m.sortedAgents()
				if m.cursor >= len(agents) && m.cursor > 0 {
					m.cursor = len(agents) - 1
				}
			} else {
				m.notifications = append(m.notifications, notification{
					text:  "No dead agents found",
					time:  time.Now(),
					style: doneStyle,
				})
				if len(m.notifications) > 10 {
					m.notifications = m.notifications[len(m.notifications)-10:]
				}
			}
		case "D":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				status := a.GetStatus()
				if status == agent.StatusDone || status == agent.StatusReviewReady || status == agent.StatusReviewing || status == agent.StatusReviewed || status == agent.StatusConflicts {
					m.confirmDelete = true
					m.confirmAgentID = a.ID
					m.confirmBranch = a.Branch
					m.confirmAgentName = a.Name
					if m.confirmAgentName == "" {
						m.confirmAgentName = a.ID
					}
				}
			}
		}
	}

	return m, nil
}

func (m dashboardModel) sortedAgents() []*agent.Agent {
	agents := m.store.All()
	switch m.sortBy {
	case sortByStatus:
		statusOrder := map[agent.Status]int{
			agent.StatusConflicts:   0,
			agent.StatusWaiting:     1,
			agent.StatusReviewed:    2,
			agent.StatusReviewReady: 3,
			agent.StatusRunning:     4,
			agent.StatusReviewing:   5,
			agent.StatusDone:        6,
			agent.StatusDismissed:   7,
		}
		sort.Slice(agents, func(i, j int) bool {
			oi := statusOrder[agents[i].GetStatus()]
			oj := statusOrder[agents[j].GetStatus()]
			if oi != oj {
				return oi < oj
			}
			return agents[i].ID < agents[j].ID
		})
	case sortByDuration:
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Duration() > agents[j].Duration()
		})
	default:
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].ID < agents[j].ID
		})
	}
	return agents
}

func (m dashboardModel) sortLabel() string {
	switch m.sortBy {
	case sortByStatus:
		return "status"
	case sortByDuration:
		return "duration"
	default:
		return "id"
	}
}

func (m dashboardModel) ViewContent() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render(fmt.Sprintf("Mastermind — repo: %s — session: %s", m.repoPath, m.session))
	b.WriteString(title)
	b.WriteString("\n\n")

	// Agent table
	agents := m.sortedAgents()
	if len(agents) == 0 {
		b.WriteString(wizardDimStyle.Render("  No agents running. Press n to spawn one."))
		b.WriteString("\n")
	} else {
		// Header
		header := fmt.Sprintf("  %-4s %-18s %-22s %-12s %-10s", "ID", "Name", "Branch", "Status", "Duration")
		b.WriteString(headerStyle.Render(header))
		b.WriteString("\n")

		for i, a := range agents {
			name := a.Name
			if name == "" {
				name = "-"
			}

			status := a.GetStatus()
			waitingFor := a.GetWaitingFor()

			var styledStatus string
			switch status {
			case agent.StatusRunning:
				styledStatus = runningStyle.Render("running")
			case agent.StatusWaiting:
				if waitingFor == "permission" {
					styledStatus = permissionStyle.Render("permission")
				} else if waitingFor == "unknown" {
					styledStatus = attentionStyle.Render("attention?")
				} else {
					styledStatus = waitingStyle.Render("waiting")
				}
			case agent.StatusReviewReady:
				styledStatus = reviewReadyStyle.Render("review ready")
			case agent.StatusDone:
				styledStatus = doneStyle.Render("done")
			case agent.StatusReviewing:
				styledStatus = reviewingStyle.Render("reviewing")
			case agent.StatusReviewed:
				styledStatus = reviewedStyle.Render("reviewed")
			case agent.StatusConflicts:
				styledStatus = conflictsStyle.Render("conflicts")
			default:
				styledStatus = string(status)
			}

			dur := formatDuration(a.Duration())

			indicator := "  "
			switch status {
			case agent.StatusReviewReady:
				indicator = " " + reviewReadyStyle.Render("◀")
			case agent.StatusReviewed:
				indicator = " " + reviewedStyle.Render("◀")
			case agent.StatusConflicts:
				indicator = " " + conflictsStyle.Render("◀")
			case agent.StatusWaiting:
				if waitingFor == "permission" {
					indicator = " " + permissionStyle.Render("◀")
				} else if waitingFor == "unknown" {
					indicator = " " + attentionStyle.Render("?")
				} else {
					indicator = " " + waitingStyle.Render("◀")
				}
			}

			// Pad styled status to 12 visual characters (fmt %-12s counts
			// bytes which breaks with ANSI escape codes from lipgloss).
			if w := lipgloss.Width(styledStatus); w < 12 {
				styledStatus += strings.Repeat(" ", 12-w)
			}

			// Build the row content
			row := fmt.Sprintf("  %-4s %-18s %-22s %s%-10s%s",
				a.ID,
				truncate(name, 18),
				truncate(a.Branch, 22),
				styledStatus,
				dur,
				indicator,
			)

			if i == m.cursor {
				row = selectedStyle.Render(row)
			}

			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Notifications (newest first)
	if len(m.notifications) > 0 {
		b.WriteString("\n")
		b.WriteString(headerStyle.Render("  ── Notifications ──"))
		b.WriteString("\n")
		for i := len(m.notifications) - 1; i >= 0; i-- {
			n := m.notifications[i]
			ts := n.time.Format("15:04")
			line := fmt.Sprintf("  %s %s", ts, n.text)
			b.WriteString(n.style.Render(line))
			b.WriteString("\n")
		}
	}

	// Error
	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  Error: " + m.err))
		b.WriteString("\n")
	}

	// Confirm delete prompt
	if m.confirmDelete {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(fmt.Sprintf("  Delete branch %q for agent %s? (y/n)", m.confirmBranch, m.confirmAgentName)))
		b.WriteString("\n")
	}

	// Confirm merge prompt
	if m.confirmMerge {
		b.WriteString("\n")
		b.WriteString(reviewedStyle.Render(fmt.Sprintf("  Merge %s into %s? (y/n)", m.mergeBranch, m.mergeBaseBranch)))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("  n: new agent │ enter: focus/review │ m: merge │ d: dismiss │ D: dismiss+delete branch │ s: sort (%s) │ q: quit", m.sortLabel())))

	return b.String()
}

func (m dashboardModel) View() string {
	content := m.ViewContent()

	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 80
	}

	return borderStyle.Width(maxWidth).Render(content)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %02ds", m, s)
}

func truncate(s string, max int) string {
	if lipgloss.Width(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
