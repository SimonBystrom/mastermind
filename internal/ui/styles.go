package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255"))

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("34"))

	reviewReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("49")).
				Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	permissionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	reviewingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	reviewedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("76")).
			Bold(true)

	conflictsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	notificationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Italic(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)

	wizardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("170")).
				MarginBottom(1)

	wizardActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170"))

	wizardDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62"))
)
