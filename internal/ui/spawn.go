package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/git"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
)

type spawnStep int

const (
	stepChooseMode spawnStep = iota
	stepPickBranch
	stepNewBranchName
	stepConfirm
)

type spawnMode int

const (
	modeExisting spawnMode = iota
	modeNew
)

// branchItem implements list.DefaultItem for the branch picker list.
type branchItem struct {
	name    string
	current bool
}

func (b branchItem) Title() string {
	if b.current {
		return b.name + " (current)"
	}
	return b.name
}

func (b branchItem) Description() string { return "" }
func (b branchItem) FilterValue() string { return b.name }

type spawnModel struct {
	orch     *orchestrator.Orchestrator
	repoPath string
	step     spawnStep
	mode     spawnMode
	err      string
	width    int
	styles   Styles

	// Mode selection
	modeCursor int

	// Branch picker
	branches           []git.Branch
	checkedOutBranches map[string]bool
	branchList         list.Model

	// New branch name input
	branchInput textinput.Model

	// Computed
	baseBranch   string
	branch       string
	createBranch bool
}

type spawnDoneMsg struct{}
type spawnCancelMsg struct{}

func newSpawn(s Styles, orch *orchestrator.Orchestrator, repoPath string, width int) spawnModel {
	bi := textinput.New()
	bi.Placeholder = "new branch name (e.g. feat/my-feature)"

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(s.WizardActive.GetForeground()).
		Foreground(s.WizardActive.GetForeground()).
		Padding(0, 0, 0, 1)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().Padding(0, 0, 0, 2)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(s.WizardDim.GetForeground()).
		Padding(0, 0, 0, 2)

	listWidth := max(width-8, 20)
	bl := list.New([]list.Item{}, delegate, listWidth, 15)
	bl.SetShowTitle(false)
	bl.SetShowStatusBar(false)
	bl.SetShowHelp(false)
	bl.SetFilteringEnabled(true)
	bl.DisableQuitKeybindings()
	bl.KeyMap.ShowFullHelp.SetEnabled(false)
	bl.KeyMap.CloseFullHelp.SetEnabled(false)
	bl.FilterInput.Prompt = "Filter: "
	bl.FilterInput.PromptStyle = s.WizardActive

	return spawnModel{
		orch:        orch,
		repoPath:    repoPath,
		step:        stepChooseMode,
		branchInput: bi,
		branchList:  bl,
		styles:      s,
		width:       width,
	}
}

func (m spawnModel) Init() tea.Cmd {
	return m.loadBranches()
}

type branchesLoadedMsg struct {
	branches []git.Branch
	err      error
}

func (m spawnModel) loadBranches() tea.Cmd {
	return func() tea.Msg {
		branches, err := git.ListBranches(m.repoPath)
		return branchesLoadedMsg{branches: branches, err: err}
	}
}

func (m *spawnModel) setBranchListItems() tea.Cmd {
	var items []list.Item
	for _, b := range m.branches {
		if m.mode == modeExisting && m.checkedOutBranches[b.Name] {
			continue
		}
		items = append(items, branchItem{name: b.Name, current: b.Current})
	}
	cmd := m.branchList.SetItems(items)
	m.branchList.ResetFilter()
	m.branchList.Select(0)
	return cmd
}

func (m spawnModel) Update(msg tea.Msg) (spawnModel, tea.Cmd) {
	switch msg := msg.(type) {
	case branchesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.branches = msg.branches
		m.checkedOutBranches = make(map[string]bool)
		if worktrees, err := git.ListWorktrees(m.repoPath); err == nil {
			for _, wt := range worktrees {
				if wt.Branch != "" {
					m.checkedOutBranches[wt.Branch] = true
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		m.err = ""

		if msg.String() == "esc" {
			// If in branch picker with active filter, let the list handle esc
			if m.step == stepPickBranch && (m.branchList.SettingFilter() || m.branchList.IsFiltered()) {
				return m.updatePickBranch(msg)
			}
			if m.step == stepChooseMode {
				return m, func() tea.Msg { return spawnCancelMsg{} }
			}
			// Go back to mode selection
			m.step = stepChooseMode
			m.branchList.ResetFilter()
			m.branchList.Select(0)
			m.branchInput.SetValue("")
			return m, nil
		}

		switch m.step {
		case stepChooseMode:
			return m.updateChooseMode(msg)
		case stepPickBranch:
			return m.updatePickBranch(msg)
		case stepNewBranchName:
			return m.updateNewBranchName(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		}
	}

	return m, nil
}

func (m spawnModel) updateChooseMode(msg tea.KeyMsg) (spawnModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.modeCursor > 0 {
			m.modeCursor--
		}
	case "down", "j":
		if m.modeCursor < 1 {
			m.modeCursor++
		}
	case "enter":
		if m.modeCursor == 0 {
			m.mode = modeExisting
			m.step = stepPickBranch
			cmd := m.setBranchListItems()
			return m, cmd
		}
		m.mode = modeNew
		m.step = stepNewBranchName
		m.branchInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m spawnModel) updatePickBranch(msg tea.KeyMsg) (spawnModel, tea.Cmd) {
	wasFiltering := m.branchList.SettingFilter()

	var cmd tea.Cmd
	m.branchList, cmd = m.branchList.Update(msg)

	isFiltering := m.branchList.SettingFilter()

	// Only handle enter for branch selection when the list isn't filtering
	if msg.String() == "enter" && !wasFiltering && !isFiltering {
		item := m.branchList.SelectedItem()
		if item == nil {
			return m, cmd
		}
		selected := item.(branchItem)
		if m.mode == modeExisting {
			m.branch = selected.name
			m.baseBranch = ""
			m.createBranch = false
			m.step = stepConfirm
			return m, nil
		}
		// New branch mode — this is the base branch
		m.baseBranch = selected.name
		m.createBranch = true
		m.step = stepConfirm
		return m, nil
	}

	return m, cmd
}

func (m spawnModel) updateNewBranchName(msg tea.KeyMsg) (spawnModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.branchInput.Value())
		if name == "" {
			m.err = "branch name is required"
			return m, nil
		}
		if git.BranchExists(m.repoPath, name) {
			m.err = fmt.Sprintf("branch %q already exists — use existing branch mode", name)
			return m, nil
		}
		m.branch = name
		m.step = stepPickBranch
		cmd := m.setBranchListItems()
		return m, cmd
	default:
		var cmd tea.Cmd
		m.branchInput, cmd = m.branchInput.Update(msg)
		return m, cmd
	}
}

func (m spawnModel) updateConfirm(msg tea.KeyMsg) (spawnModel, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		err := m.orch.SpawnAgent(m.branch, m.baseBranch, m.createBranch)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m, func() tea.Msg { return spawnDoneMsg{} }
	case "n":
		m.step = stepPickBranch
		return m, nil
	}
	return m, nil
}


func (m spawnModel) ViewContent() string {
	var b strings.Builder

	b.WriteString(m.styles.WizardTitle.Render("Spawn New Agent"))
	b.WriteString("\n\n")

	switch m.step {
	case stepChooseMode:
		b.WriteString(m.styles.WizardActive.Render("How do you want to set up the branch?"))
		b.WriteString("\n\n")

		options := []struct {
			label string
			desc  string
		}{
			{"Use existing branch", "Check out an existing branch into a new worktree"},
			{"Create new branch", "Create a new branch from a base branch"},
		}
		for i, opt := range options {
			cursor := "  "
			if i == m.modeCursor {
				cursor = "> "
			}
			line := fmt.Sprintf("%s%s", cursor, opt.label)
			if i == m.modeCursor {
				b.WriteString(m.styles.WizardActive.Render(line))
				b.WriteString("\n")
				b.WriteString(m.styles.WizardDim.Render("    " + opt.desc))
			} else {
				b.WriteString("  " + opt.label)
				b.WriteString("\n")
				b.WriteString(m.styles.WizardDim.Render("    " + opt.desc))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("  enter: select │ esc: cancel"))

	case stepPickBranch:
		if m.mode == modeExisting {
			b.WriteString(m.styles.WizardDim.Render("Mode: Use existing branch"))
			b.WriteString("\n")
			b.WriteString(m.styles.WizardActive.Render("Pick branch to use"))
		} else {
			b.WriteString(m.styles.WizardDim.Render(fmt.Sprintf("New branch: %s", m.branch)))
			b.WriteString("\n")
			b.WriteString(m.styles.WizardActive.Render("Pick base branch to create from"))
		}
		b.WriteString("\n\n")
		b.WriteString(m.branchList.View())
		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("  /: filter │ enter: select │ esc: back"))

	case stepNewBranchName:
		b.WriteString(m.styles.WizardDim.Render("Mode: Create new branch"))
		b.WriteString("\n")
		b.WriteString(m.styles.WizardActive.Render("Enter new branch name"))
		b.WriteString("\n\n")
		b.WriteString("  " + m.branchInput.View())
		b.WriteString("\n\n")
		b.WriteString(m.styles.Help.Render("  enter: continue │ esc: back"))

	case stepConfirm:
		b.WriteString(m.styles.WizardActive.Render("Confirm"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  Branch:    %s\n", m.branch))
		if m.createBranch {
			b.WriteString(fmt.Sprintf("  Base:      %s (will create)\n", m.baseBranch))
		} else {
			b.WriteString("  Base:      — (existing branch)\n")
		}
		b.WriteString("\n")
		b.WriteString(m.styles.Help.Render("  y/enter: spawn │ n: go back │ esc: back"))
	}

	if m.err != "" {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("  Error: " + m.err))
	}

	return b.String()
}

func (m spawnModel) View() string {
	return m.styles.Border.Render(m.ViewContent())
}
