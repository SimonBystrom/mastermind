package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/simonbystrom/mastermind/internal/config"
)

// Styles holds all lipgloss styles used by the UI, built from config colors.
type Styles struct {
	Title         lipgloss.Style
	Header        lipgloss.Style
	Selected      lipgloss.Style
	Running       lipgloss.Style
	ReviewReady   lipgloss.Style
	Done          lipgloss.Style
	Waiting       lipgloss.Style
	Permission    lipgloss.Style
	Reviewing     lipgloss.Style
	Reviewed      lipgloss.Style
	Conflicts     lipgloss.Style
	Notification  lipgloss.Style
	Help          lipgloss.Style
	HelpActive    lipgloss.Style
	Border        lipgloss.Style
	Separator     lipgloss.Style
	WizardTitle   lipgloss.Style
	WizardActive  lipgloss.Style
	WizardDim     lipgloss.Style
	Error         lipgloss.Style
	Attention     lipgloss.Style
	Logo          lipgloss.Style
	Previewing    lipgloss.Style
	PreviewBanner lipgloss.Style
}

// NewStyles builds a Styles from config color values. Non-color attributes
// (bold, italic, padding, border) are kept as hardcoded defaults.
func NewStyles(c config.Colors) Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(c.Title)).
			Padding(0, 1),

		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(c.Header)),

		Selected: lipgloss.NewStyle().
			Background(lipgloss.Color(c.SelectedBG)).
			Foreground(lipgloss.Color(c.SelectedFG)),

		Running: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Running)),

		ReviewReady: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.ReviewReady)).
			Bold(true),

		Done: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Done)),

		Waiting: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Waiting)).
			Bold(true),

		Permission: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Permission)).
			Bold(true),

		Reviewing: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Reviewing)),

		Reviewed: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Reviewed)).
			Bold(true),

		Conflicts: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Conflicts)).
			Bold(true),

		Notification: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Notification)).
			Italic(true),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Help)),

		HelpActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.HelpActive)),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(c.Border)).
			Padding(1, 2),

		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Separator)),

		WizardTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(c.WizardTitle)).
			MarginBottom(1),

		WizardActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.WizardActive)),

		WizardDim: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.WizardDim)),

		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Error)).
			Bold(true),

		Attention: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Attention)).
			Italic(true),

		Logo: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Logo)).
			Bold(true),

		Previewing: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.Previewing)).
			Bold(true),

		PreviewBanner: lipgloss.NewStyle().
			Foreground(lipgloss.Color(c.PreviewBanner)).
			Bold(true).
			Italic(true),
	}
}
