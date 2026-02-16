package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Colors holds xterm-256 color codes for every UI style.
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
}

// Layout holds pane sizing percentages.
type Layout struct {
	DashboardWidth int `toml:"dashboard_width"`
	LazygitSplit   int `toml:"lazygit_split"`
}

// Config is the top-level configuration.
type Config struct {
	Colors Colors `toml:"colors"`
	Layout Layout `toml:"layout"`
}

// Default returns a Config populated with the current hardcoded defaults.
func Default() Config {
	return Config{
		Colors: Colors{
			Title:         "170",
			Header:        "39",
			SelectedBG:    "236",
			SelectedFG:    "255",
			Running:       "34",
			ReviewReady:   "49",
			Done:          "241",
			Waiting:       "214",
			Permission:    "220",
			Reviewing:     "99",
			Reviewed:      "76",
			Conflicts:     "196",
			Notification:  "245",
			Help:          "241",
			Border:        "62",
			Separator:     "62",
			WizardTitle:   "170",
			WizardActive:  "170",
			WizardDim:     "241",
			Error:         "196",
			Attention:     "208",
			Logo:          "170",
			Previewing:    "213",
			PreviewBanner: "213",
		},
		Layout: Layout{
			DashboardWidth: 55,
			LazygitSplit:   80,
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
# Colors use xterm-256 color codes (0-255).

[colors]
# title          = "170"
# header         = "39"
# selected_bg    = "236"
# selected_fg    = "255"
# running        = "34"
# review_ready   = "49"
# done           = "241"
# waiting        = "214"
# permission     = "220"
# reviewing      = "99"
# reviewed       = "76"
# conflicts      = "196"
# notification   = "245"
# help           = "241"
# border         = "62"
# separator      = "62"
# wizard_title   = "170"
# wizard_active  = "170"
# wizard_dim     = "241"
# error          = "196"
# attention      = "208"
# logo           = "170"
# previewing     = "213"
# preview_banner = "213"

[layout]
# dashboard_width = 55   # percentage of terminal width for left panel
# lazygit_split   = 80   # percentage for lazygit pane size
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
