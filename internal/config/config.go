package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Colors holds color values for every UI style.
// Values can be xterm-256 codes (0-255) or hex colors (#rrggbb).
type Colors struct {
	Title         string `toml:"title"`
	Header        string `toml:"header"`
	SelectedBG    string `toml:"selected_bg"`
	SelectedFG    string `toml:"selected_fg"`
	Running       string `toml:"running"`
	ReviewReady   string `toml:"review_ready"`
	Done          string `toml:"done"`
	Waiting       string `toml:"waiting"`
	Permission    string `toml:"permission"`
	Reviewing     string `toml:"reviewing"`
	Reviewed      string `toml:"reviewed"`
	Conflicts     string `toml:"conflicts"`
	Notification  string `toml:"notification"`
	Help          string `toml:"help"`
	HelpActive    string `toml:"help_active"`
	Border        string `toml:"border"`
	Separator     string `toml:"separator"`
	WizardTitle   string `toml:"wizard_title"`
	WizardActive  string `toml:"wizard_active"`
	WizardDim     string `toml:"wizard_dim"`
	Error         string `toml:"error"`
	Attention     string `toml:"attention"`
	Logo          string `toml:"logo"`
	Previewing    string `toml:"previewing"`
	PreviewBanner string `toml:"preview_banner"`
	Team          string `toml:"team"`
}

// Layout holds pane sizing percentages.
type Layout struct {
	DashboardWidth int `toml:"dashboard_width"`
	LazygitSplit   int `toml:"lazygit_split"`
}

// Claude holds settings for Claude Code agent behavior.
type Claude struct {
	AgentTeams   bool   `toml:"agent_teams"`
	TeammateMode string `toml:"teammate_mode"`
}

// Config is the top-level configuration.
type Config struct {
	Colors Colors `toml:"colors"`
	Layout Layout `toml:"layout"`
	Claude Claude `toml:"claude"`
}

// Default returns a Config populated with the current hardcoded defaults.
func Default() Config {
	return Config{
		Colors: Colors{
			Title:         "#cba6f7", // Mauve
			Header:        "#89b4fa", // Blue
			SelectedBG:    "#313244", // Surface 0
			SelectedFG:    "#cdd6f4", // Text
			Running:       "#89b4fa", // Blue
			ReviewReady:   "#94e2d5", // Teal
			Done:          "#7f849c", // Overlay 1
			Waiting:       "#f9e2af", // Yellow
			Permission:    "#fab387", // Peach
			Reviewing:     "#b4befe", // Lavender
			Reviewed:      "#a6e3a1", // Green
			Conflicts:     "#f38ba8", // Red
			Notification:  "#a6adc8", // Subtext 0
			Help:          "#7f849c", // Overlay 1
			HelpActive:    "#bac2de", // Subtext 1
			Border:        "#585b70", // Surface 2
			Separator:     "#585b70", // Surface 2
			WizardTitle:   "#cba6f7", // Mauve
			WizardActive:  "#cba6f7", // Mauve
			WizardDim:     "#7f849c", // Overlay 1
			Error:         "#f38ba8", // Red
			Attention:     "#fab387", // Peach
			Logo:          "#cba6f7", // Mauve
			Previewing:    "#f5c2e7", // Pink
			PreviewBanner: "#f5c2e7", // Pink
			Team:          "#74c7ec", // Sapphire
		},
		Layout: Layout{
			DashboardWidth: 55,
			LazygitSplit:   80,
		},
		Claude: Claude{
			AgentTeams:   true,
			TeammateMode: "in-process",
		},
	}
}

// Path returns the config file path, respecting XDG_CONFIG_HOME.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mastermind", "mastermind.conf")
}

// Load reads the config file and returns a Config. Omitted fields keep
// their default values. If the file does not exist, defaults are returned
// with no error.
func Load() (Config, error) {
	cfg := Default()
	path := Path()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

const defaultFileContent = `# Mastermind configuration
# Uncomment and modify values to customize. All values are optional.
# Colors can be hex (#rrggbb) or xterm-256 codes (0-255).
# Defaults use the Catppuccin Mocha palette.

[colors]
# title          = "#cba6f7"  # Mauve
# header         = "#89b4fa"  # Blue
# selected_bg    = "#313244"  # Surface 0
# selected_fg    = "#cdd6f4"  # Text
# running        = "#89b4fa"  # Blue
# review_ready   = "#94e2d5"  # Teal
# done           = "#7f849c"  # Overlay 1
# waiting        = "#f9e2af"  # Yellow
# permission     = "#fab387"  # Peach
# reviewing      = "#b4befe"  # Lavender
# reviewed       = "#a6e3a1"  # Green
# conflicts      = "#f38ba8"  # Red
# notification   = "#a6adc8"  # Subtext 0
# help           = "#7f849c"  # Overlay 1
# help_active    = "#bac2de"  # Subtext 1
# border         = "#585b70"  # Surface 2
# separator      = "#585b70"  # Surface 2
# wizard_title   = "#cba6f7"  # Mauve
# wizard_active  = "#cba6f7"  # Mauve
# wizard_dim     = "#7f849c"  # Overlay 1
# error          = "#f38ba8"  # Red
# attention      = "#fab387"  # Peach
# logo           = "#cba6f7"  # Mauve
# previewing     = "#f5c2e7"  # Pink
# preview_banner = "#f5c2e7"  # Pink
# team           = "#74c7ec"  # Sapphire

[layout]
# dashboard_width = 55   # percentage of terminal width for left panel
# lazygit_split   = 80   # percentage for lazygit pane size

[claude]
# agent_teams   = true   # enable Claude Code agent teams (CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS)
# teammate_mode = "in-process"  # teammate mode for agent team collaboration
`

// WriteDefault writes the default config file with all values commented out.
// It no-ops if the file already exists. Parent directories are created as needed.
func WriteDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // file already exists
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(defaultFileContent), 0o644)
}
