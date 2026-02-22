package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type sortMode int

const (
	sortByID sortMode = iota
	sortByStatus
	sortByDuration
)

type dashboardKeyMap struct {
	New        key.Binding
	Focus      key.Binding
	Preview    key.Binding
	Merge      key.Binding
	Dismiss    key.Binding
	DismissDel key.Binding
	Sort       key.Binding
	Quit       key.Binding
}

func newDashboardKeyMap() dashboardKeyMap {
	return dashboardKeyMap{
		New:        key.NewBinding(key.WithKeys("n"), key.WithHelp("n:", "new")),
		Focus:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter:", "focus")),
		Preview:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p:", "preview")),
		Merge:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m:", "merge")),
		Dismiss:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d:", "dismiss")),
		DismissDel: key.NewBinding(key.WithKeys("D"), key.WithHelp("D:", "dismiss+del")),
		Sort:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s:", "sort (id)")),
		Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q:", "quit")),
	}
}

func (k dashboardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.New, k.Focus, k.Preview, k.Merge, k.Dismiss, k.DismissDel, k.Sort, k.Quit}
}

func (k dashboardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.New, k.Focus, k.Preview, k.Merge},
		{k.Dismiss, k.DismissDel, k.Sort, k.Quit},
	}
}

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
	keys          dashboardKeyMap
	help          help.Model
}

func newDashboard(s Styles, layout config.Layout, orch *orchestrator.Orchestrator, store *agent.Store, repoPath, session string) dashboardModel {
	keys := newDashboardKeyMap()
	h := help.New()
	h.ShortSeparator = " │ "
	h.Styles.ShortKey = s.HelpActive
	h.Styles.ShortDesc = s.Help
	h.Styles.ShortSeparator = s.Help
	h.Styles.FullKey = s.HelpActive
	h.Styles.FullDesc = s.Help
	h.Styles.FullSeparator = s.Help
	h.Styles.Ellipsis = s.Help
	return dashboardModel{
		store:    store,
		orch:     orch,
		repoPath: repoPath,
		session:  session,
		styles:   s,
		layout:   layout,
		keys:     keys,
		help:     h,
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *dashboardModel) addNotification(n notification) {
	m.notifications = append(m.notifications, n)
	if len(m.notifications) > 10 {
		m.notifications = m.notifications[len(m.notifications)-10:]
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tickCmd()
}

func (m dashboardModel) Update(msg tea.Msg) (dashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case orchestrator.AgentFinishedMsg:
		name := msg.AgentID
		var text string
		var style lipgloss.Style
		if msg.HasChanges {
			text = fmt.Sprintf("Agent %s finished with changes — ready for review (exit %d)", name, msg.ExitCode)
			style = m.styles.ReviewReady
		} else {
			text = fmt.Sprintf("Agent %s finished with no changes (exit %d)", name, msg.ExitCode)
			style = m.styles.Done
		}
		m.addNotification(notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		return m, nil

	case orchestrator.AgentGoneMsg:
		name := msg.AgentID
		m.store.Remove(msg.AgentID)
		m.addNotification(notification{
			text:  fmt.Sprintf("Agent %s window closed", name),
			time:  time.Now(),
			style: m.styles.Done,
		})
		agents := m.sortedAgents()
		if m.cursor >= len(agents) && m.cursor > 0 {
			m.cursor = len(agents) - 1
		}
		return m, nil

	case orchestrator.AgentReviewedMsg:
		name := msg.AgentID
		var text string
		var style lipgloss.Style
		if msg.NewCommits {
			text = fmt.Sprintf("Agent %s review complete — new commits found", name)
			style = m.styles.Reviewed
		} else {
			text = fmt.Sprintf("Agent %s review closed — no new commits", name)
			style = m.styles.Done
		}
		m.addNotification(notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		return m, nil

	case orchestrator.MergeResultMsg:
		name := msg.AgentID
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
		m.addNotification(notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
		agents := m.sortedAgents()
		if m.cursor >= len(agents) && m.cursor > 0 {
			m.cursor = len(agents) - 1
		}
		return m, nil

	case orchestrator.PreviewStartedMsg:
		name := msg.AgentID
		m.addNotification(notification{
			text:  fmt.Sprintf("Preview started for agent %s", name),
			time:  time.Now(),
			style: m.styles.Previewing,
		})
		return m, nil

	case orchestrator.PreviewStoppedMsg:
		name := msg.AgentID
		m.addNotification(notification{
			text:  fmt.Sprintf("Preview stopped for agent %s", name),
			time:  time.Now(),
			style: m.styles.Done,
		})
		return m, nil

	case orchestrator.PreviewErrorMsg:
		m.err = msg.Error
		return m, nil

	case orchestrator.AgentWaitingMsg:
		name := msg.AgentID
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
		m.addNotification(notification{
			text:  text,
			time:  time.Now(),
			style: style,
		})
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
					name := a.ID
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
				name := a.ID
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
					m.addNotification(notification{
						text:  fmt.Sprintf("Cleaned up %s (%s)", r.AgentName, r.Reason),
						time:  time.Now(),
						style: m.styles.Done,
					})
				}
				agents = m.sortedAgents()
				if m.cursor >= len(agents) && m.cursor > 0 {
					m.cursor = len(agents) - 1
				}
			} else {
				m.addNotification(notification{
					text:  "No dead agents found",
					time:  time.Now(),
					style: m.styles.Done,
				})
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
					m.err = fmt.Sprintf("preview already active for agent %s — press p on that agent to stop it first", previewID)
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
				name := a.ID
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

// contentWidth returns the usable content width inside the border.
func (m dashboardModel) contentWidth() int {
	// Border has 2 horizontal padding + 2 border chars = 4 total overhead,
	// plus we subtract 4 more from terminal width (see View).
	w := m.width - 8
	if w < 20 {
		w = 20
	}
	return w
}

func (m dashboardModel) ViewContent() string {
	var b strings.Builder

	cw := m.contentWidth()

	chosenLogo := renderLogo(cw)
	b.WriteString(m.styles.Logo.Render(chosenLogo))
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
			previewBranch = previewAgent.Branch
		}
		banner := fmt.Sprintf("  PREVIEW ACTIVE: %s (branch %s) — p to stop", previewName, previewBranch)
		b.WriteString(m.styles.PreviewBanner.Render(banner))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Agent table — flex column layout.
	// Each column has a minimum width and a flex weight. After giving every
	// column its minimum, remaining space is distributed proportionally.
	type col struct {
		min, weight int
	}
	cols := [8]col{
		{3, 1},  // 0: ID
		{8, 2},  // 1: Model
		{10, 3}, // 2: Branch
		{10, 2}, // 3: Status
		{7, 2},  // 4: Duration
		{6, 1},  // 5: Cost
		{4, 1},  // 6: Ctx%
		{8, 2},  // 7: Lines
	}
	const indent = 2
	const gaps = 8   // 1-char gap between each of 8 cols + indicator
	const indic = 2  // indicator width
	totalMin := indent + gaps + indic
	totalWeight := 0
	for _, c := range cols {
		totalMin += c.min
		totalWeight += c.weight
	}
	extra := cw - totalMin
	if extra < 0 {
		extra = 0
	}
	// Compute actual widths
	var colW [8]int
	for i, c := range cols {
		colW[i] = c.min + extra*c.weight/totalWeight
	}
	// Distribute rounding remainder to Branch (largest flex col)
	used := indent + gaps + indic
	for _, w := range colW {
		used += w
	}
	if rem := cw - used; rem > 0 {
		colW[2] += rem
	}

	agents := m.sortedAgents()
	if len(agents) == 0 {
		b.WriteString(m.styles.WizardDim.Render("  No agents running. Press n to spawn one."))
		b.WriteString("\n")
	} else {
		// Header
		header := fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %-*s %-*s %-*s",
			colW[0], "ID", colW[1], "Model", colW[2], "Branch", colW[3], "Status",
			colW[4], "Duration", colW[5], "Cost", colW[6], "Ctx%", colW[7], "Lines")
		b.WriteString(m.styles.Header.Render(header))
		b.WriteString("\n")

		for i, a := range agents {
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

			// Statusline data columns
			modelStr := "-"
			costStr := "-"
			ctxPctStr := "-"
			linesStr := "-"
			ctxPct := 0
			if sd := a.GetStatuslineData(); sd != nil {
				if sd.Model != "" {
					modelStr = sd.Model
				}
				costStr = fmt.Sprintf("$%.2f", sd.CostUSD)
				ctxPct = int(sd.ContextPct)
				ctxPctStr = fmt.Sprintf("%d%%", ctxPct)
				linesStr = fmt.Sprintf("+%d -%d", sd.LinesAdded, sd.LinesRemoved)
			}

			isSelected := i == m.cursor

			var row string
			if isSelected {
				// Selected row: plain text only, single outer style.
				// Avoids ANSI nesting conflicts that cause background gaps.
				plainStatus := string(status)
				if status == agent.StatusWaiting {
					if waitingFor == "permission" {
						plainStatus = "permission"
					} else if waitingFor == "unknown" {
						plainStatus = "attention?"
					} else {
						plainStatus = "waiting"
					}
				}

				row = fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %-*s %-*s %-*s  ",
					colW[0], a.ID,
					colW[1], truncate(modelStr, colW[1]),
					colW[2], truncate(a.Branch, colW[2]),
					colW[3], plainStatus,
					colW[4], dur,
					colW[5], costStr,
					colW[6], ctxPctStr,
					colW[7], linesStr,
				)

				// Pad to full content width using visual width for safety
				if w := lipgloss.Width(row); w < cw {
					row += strings.Repeat(" ", cw-w)
				}
				row = m.styles.Selected.Render(row)
			} else {
				// Non-selected row: styled status, ctx%, and indicator.
				displayStatus := styledStatus
				// Pad styled status to colW[3] visual characters (fmt %-*s counts
				// bytes which breaks with ANSI escape codes from lipgloss).
				if w := lipgloss.Width(displayStatus); w < colW[3] {
					displayStatus += strings.Repeat(" ", colW[3]-w)
				}

				displayCtx := ctxPctStr
				if ctxPct > 80 {
					displayCtx = m.styles.Attention.Render(ctxPctStr)
				}
				if w := lipgloss.Width(displayCtx); w < colW[6] {
					displayCtx += strings.Repeat(" ", colW[6]-w)
				}

				row = fmt.Sprintf("  %-*s %-*s %-*s %s %-*s %-*s %s %-*s %s",
					colW[0], a.ID,
					colW[1], truncate(modelStr, colW[1]),
					colW[2], truncate(a.Branch, colW[2]),
					displayStatus,
					colW[4], dur,
					colW[5], costStr,
					displayCtx,
					colW[7], linesStr,
					indicator,
				)

				if w := lipgloss.Width(row); w < cw {
					row += strings.Repeat(" ", cw-w)
				}
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

	// Help — show available actions, hiding unavailable ones
	b.WriteString("\n")

	var selectedStatus agent.Status
	hasSelection := false
	if len(agents) > 0 && m.cursor < len(agents) {
		hasSelection = true
		selectedStatus = agents[m.cursor].GetStatus()
	}

	canPreview := hasSelection && (selectedStatus == agent.StatusReviewReady ||
		selectedStatus == agent.StatusReviewed ||
		selectedStatus == agent.StatusReviewing ||
		selectedStatus == agent.StatusPreviewing)
	canMerge := hasSelection && (selectedStatus == agent.StatusReviewed ||
		selectedStatus == agent.StatusReviewReady)

	m.keys.Focus.SetEnabled(hasSelection)
	m.keys.Preview.SetEnabled(canPreview)
	m.keys.Merge.SetEnabled(canMerge)
	m.keys.Dismiss.SetEnabled(hasSelection)
	m.keys.DismissDel.SetEnabled(hasSelection)
	m.keys.Sort.SetHelp("s:", fmt.Sprintf("sort (%s)", m.sortLabel()))

	m.help.Width = cw - 2

	var helpLine string
	if cw < 80 {
		m.keys.DismissDel.SetHelp("D:", "del")
		line1 := m.help.ShortHelpView([]key.Binding{m.keys.New, m.keys.Focus, m.keys.Preview, m.keys.Merge})
		line2 := m.help.ShortHelpView([]key.Binding{m.keys.Dismiss, m.keys.DismissDel, m.keys.Sort, m.keys.Quit})
		helpLine = "  " + line1 + "\n  " + line2
	} else {
		m.keys.DismissDel.SetHelp("D:", "dismiss+del")
		helpLine = "  " + m.help.ShortHelpView(m.keys.ShortHelp())
	}
	b.WriteString(helpLine)

	return b.String()
}

func (m dashboardModel) View() string {
	content := m.ViewContent()

	maxWidth := m.width - 4
	if maxWidth < 20 {
		maxWidth = 20
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
