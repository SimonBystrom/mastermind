package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

const logo = ` ███╗   ███╗ █████╗ ███████╗████████╗███████╗██████╗ ███╗   ███╗██╗███╗   ██╗██████╗
 ████╗ ████║██╔══██╗██╔════╝╚══██╔══╝██╔════╝██╔══██╗████╗ ████║██║████╗  ██║██╔══██╗
 ██╔████╔██║███████║███████╗   ██║   █████╗  ██████╔╝██╔████╔██║██║██╔██╗ ██║██║  ██║
 ██║╚██╔╝██║██╔══██║╚════██║   ██║   ██╔══╝  ██╔══██╗██║╚██╔╝██║██║██║╚██╗██║██║  ██║
 ██║ ╚═╝ ██║██║  ██║███████║   ██║   ███████╗██║  ██║██║ ╚═╝ ██║██║██║ ╚████║██████╔╝
 ╚═╝     ╚═╝╚═╝  ╚═╝╚══════╝   ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═════╝`

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
	styles        Styles
	layout        config.Layout
}

func newDashboard(s Styles, layout config.Layout, orch *orchestrator.Orchestrator, store *agent.Store, repoPath, session string) dashboardModel {
	return dashboardModel{
		store:    store,
		orch:     orch,
		repoPath: repoPath,
		session:  session,
		styles:   s,
		layout:   layout,
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
			style = m.styles.ReviewReady
		} else {
			text = fmt.Sprintf("Agent %s finished with no changes (exit %d)", name, msg.ExitCode)
			style = m.styles.Done
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
			style: m.styles.Done,
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
			style = m.styles.Reviewed
		} else {
			text = fmt.Sprintf("Agent %s review closed — no new commits", name)
			style = m.styles.Done
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
			style = m.styles.Reviewed
		} else if msg.Conflict {
			text = fmt.Sprintf("Agent %s merge has conflicts — resolve in lazygit", name)
			style = m.styles.Conflicts
		} else if msg.Error != "" {
			text = fmt.Sprintf("Agent %s merge failed: %s", name, msg.Error)
			style = m.styles.Error
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

	case orchestrator.PreviewStartedMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		m.notifications = append(m.notifications, notification{
			text:  fmt.Sprintf("Preview started for agent %s", name),
			time:  time.Now(),
			style: m.styles.Previewing,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		return m, nil

	case orchestrator.PreviewStoppedMsg:
		a, ok := m.store.Get(msg.AgentID)
		name := msg.AgentID
		if ok && a.Name != "" {
			name = a.Name
		}
		m.notifications = append(m.notifications, notification{
			text:  fmt.Sprintf("Preview stopped for agent %s", name),
			time:  time.Now(),
			style: m.styles.Done,
		})
		if len(m.notifications) > 10 {
			m.notifications = m.notifications[len(m.notifications)-10:]
		}
		return m, nil

	case orchestrator.PreviewErrorMsg:
		m.err = msg.Error
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
			style = m.styles.Permission
		} else if msg.WaitingFor == "unknown" {
			text = fmt.Sprintf("Agent %s may need attention", name)
			style = m.styles.Attention
		} else {
			text = fmt.Sprintf("Agent %s is waiting for input", name)
			style = m.styles.Waiting
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
				case agent.StatusRunning, agent.StatusWaiting, agent.StatusReviewing, agent.StatusDone, agent.StatusPreviewing:
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
					name := a.Name
					if name == "" {
						name = a.ID
					}
					return m, func() tea.Msg {
						return startMergeMsg{
							agentID:    a.ID,
							agentName:  name,
							branch:     a.Branch,
							baseBranch: a.BaseBranch,
						}
					}
				}
			}
		case "d":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				name := a.Name
				if name == "" {
					name = a.ID
				}
				return m, func() tea.Msg {
					return startDismissMsg{
						agentID:      a.ID,
						agentName:    name,
						branch:       a.Branch,
						deleteBranch: false,
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
						style: m.styles.Done,
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
					style: m.styles.Done,
				})
				if len(m.notifications) > 10 {
					m.notifications = m.notifications[len(m.notifications)-10:]
				}
			}
		case "p":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				previewID := m.orch.GetPreviewAgentID()
				if previewID != "" && previewID == a.ID {
					// Stop preview for this agent
					return m, func() tea.Msg {
						if err := m.orch.StopPreview(); err != nil {
							return orchestrator.PreviewErrorMsg{AgentID: a.ID, Error: err.Error()}
						}
						return nil
					}
				} else if previewID != "" {
					previewAgent, ok := m.store.Get(previewID)
					previewName := previewID
					if ok && previewAgent.Name != "" {
						previewName = previewAgent.Name
					}
					m.err = fmt.Sprintf("preview already active for agent %s — press p on that agent to stop it first", previewName)
				} else {
					return m, func() tea.Msg {
						if err := m.orch.PreviewAgent(a.ID); err != nil {
							return orchestrator.PreviewErrorMsg{AgentID: a.ID, Error: err.Error()}
						}
						return nil
					}
				}
			}
		case "D":
			if len(agents) > 0 && m.cursor < len(agents) {
				a := agents[m.cursor]
				name := a.Name
				if name == "" {
					name = a.ID
				}
				return m, func() tea.Msg {
					return startDismissMsg{
						agentID:      a.ID,
						agentName:    name,
						branch:       a.Branch,
						deleteBranch: true,
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
			agent.StatusPreviewing:  2,
			agent.StatusReviewed:    3,
			agent.StatusReviewReady: 4,
			agent.StatusRunning:     5,
			agent.StatusReviewing:   6,
			agent.StatusDone:        7,
			agent.StatusDismissed:   8,
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

	// Logo
	b.WriteString(m.styles.Logo.Render(logo))
	b.WriteString("\n\n")

	// Title
	title := m.styles.Title.Render(fmt.Sprintf("repo: %s — session: %s", m.repoPath, m.session))
	b.WriteString(title)
	b.WriteString("\n")

	// Preview banner
	if previewID := m.orch.GetPreviewAgentID(); previewID != "" {
		previewAgent, ok := m.store.Get(previewID)
		previewName := previewID
		previewBranch := ""
		if ok {
			if previewAgent.Name != "" {
				previewName = previewAgent.Name
			}
			previewBranch = previewAgent.Branch
		}
		banner := fmt.Sprintf("  PREVIEW ACTIVE: %s (branch %s) — p to stop", previewName, previewBranch)
		b.WriteString(m.styles.PreviewBanner.Render(banner))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Agent table
	agents := m.sortedAgents()
	if len(agents) == 0 {
		b.WriteString(m.styles.WizardDim.Render("  No agents running. Press n to spawn one."))
		b.WriteString("\n")
	} else {
		// Header
		header := fmt.Sprintf("  %-4s %-18s %-22s %-12s %-10s %-8s %-6s %-10s", "ID", "Name", "Branch", "Status", "Duration", "Cost", "Ctx%", "Lines")
		b.WriteString(m.styles.Header.Render(header))
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
				styledStatus = m.styles.Running.Render("running")
			case agent.StatusWaiting:
				if waitingFor == "permission" {
					styledStatus = m.styles.Permission.Render("permission")
				} else if waitingFor == "unknown" {
					styledStatus = m.styles.Attention.Render("attention?")
				} else {
					styledStatus = m.styles.Waiting.Render("waiting")
				}
			case agent.StatusReviewReady:
				styledStatus = m.styles.ReviewReady.Render("review ready")
			case agent.StatusDone:
				styledStatus = m.styles.Done.Render("done")
			case agent.StatusReviewing:
				styledStatus = m.styles.Reviewing.Render("reviewing")
			case agent.StatusReviewed:
				styledStatus = m.styles.Reviewed.Render("reviewed")
			case agent.StatusPreviewing:
				styledStatus = m.styles.Previewing.Render("previewing")
			case agent.StatusConflicts:
				styledStatus = m.styles.Conflicts.Render("conflicts")
			default:
				styledStatus = string(status)
			}

			dur := formatDuration(a.Duration())

			indicator := "  "
			switch status {
			case agent.StatusReviewReady:
				indicator = " " + m.styles.ReviewReady.Render("◀")
			case agent.StatusReviewed:
				indicator = " " + m.styles.Reviewed.Render("◀")
			case agent.StatusPreviewing:
				indicator = " " + m.styles.Previewing.Render("◀")
			case agent.StatusConflicts:
				indicator = " " + m.styles.Conflicts.Render("◀")
			case agent.StatusWaiting:
				if waitingFor == "permission" {
					indicator = " " + m.styles.Permission.Render("◀")
				} else if waitingFor == "unknown" {
					indicator = " " + m.styles.Attention.Render("?")
				} else {
					indicator = " " + m.styles.Waiting.Render("◀")
				}
			}

			// Pad styled status to 12 visual characters (fmt %-12s counts
			// bytes which breaks with ANSI escape codes from lipgloss).
			if w := lipgloss.Width(styledStatus); w < 12 {
				styledStatus += strings.Repeat(" ", 12-w)
			}

			// Statusline data columns
			costStr := "-"
			ctxStr := "-"
			linesStr := "-"
			if sd := a.GetStatuslineData(); sd != nil {
				costStr = fmt.Sprintf("$%.2f", sd.CostUSD)
				ctxPct := int(sd.ContextPct)
				if ctxPct > 80 {
					ctxStr = m.styles.Attention.Render(fmt.Sprintf("%d%%", ctxPct))
				} else {
					ctxStr = fmt.Sprintf("%d%%", ctxPct)
				}
				linesStr = fmt.Sprintf("+%d -%d", sd.LinesAdded, sd.LinesRemoved)
			}

			// Pad ctxStr to 6 visual characters (may contain ANSI codes)
			if w := lipgloss.Width(ctxStr); w < 6 {
				ctxStr += strings.Repeat(" ", 6-w)
			}

			// Build the row content
			row := fmt.Sprintf("  %-4s %-18s %-22s %s%-10s %-8s %s%-10s%s",
				a.ID,
				truncate(name, 18),
				truncate(a.Branch, 22),
				styledStatus,
				dur,
				costStr,
				ctxStr,
				linesStr,
				indicator,
			)

			if i == m.cursor {
				row = m.styles.Selected.Render(row)
			}

			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Notifications (newest first)
	if len(m.notifications) > 0 {
		b.WriteString("\n")
		b.WriteString(m.styles.Header.Render("  ── Notifications ──"))
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
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render(fmt.Sprintf("  n: new agent │ enter: focus/review │ p: preview │ m: merge │ d: dismiss │ D: dismiss+delete │ s: sort (%s) │ q: quit", m.sortLabel())))

	return b.String()
}

func (m dashboardModel) View() string {
	content := m.ViewContent()

	maxWidth := m.width - 4
	if maxWidth < 40 {
		maxWidth = 80
	}

	return m.styles.Border.Width(maxWidth).Render(content)
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
