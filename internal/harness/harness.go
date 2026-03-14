package harness

import "time"

// Type identifies which AI coding assistant is used.
type Type string

const (
	TypeClaudeCode Type = "claude"
	TypeOpenCode   Type = "opencode"
)

// StatusFile is the unified status format written by both harnesses.
// This maps to the agent's lifecycle: running, waiting, idle (finished), stopped.
type StatusFile struct {
	Status    string `json:"status"` // running, waiting_permission, waiting_input, idle, stopped
	Timestamp int64  `json:"ts"`
}

// IsStale returns true if the status timestamp is older than the given threshold.
func (sf *StatusFile) IsStale(threshold time.Duration) bool {
	if sf == nil {
		return true
	}
	return time.Since(time.Unix(sf.Timestamp, 0)) > threshold
}

// MetricsData holds session metrics (cost, context, model, etc.).
// This is read from sidecar files written by the harness's statusline/plugin.
type MetricsData struct {
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
	ContextPct   float64 `json:"context_pct"`
	LinesAdded   int     `json:"lines_added"`
	LinesRemoved int     `json:"lines_removed"`
	SessionID    string  `json:"session_id"`
}

// SetupOptions configure harness setup behavior.
type SetupOptions struct {
	// Claude Code specific
	AgentTeams   bool
	TeammateMode string

	// OpenCode specific
	Plugins []string // additional plugins to enable beyond mastermind-status
}

// Options passed when launching the harness command.
type Options struct {
	SkipPermissions bool
	// Future: model selection, resume session, etc.
}

// Harness abstracts the AI coding assistant integration.
// Each implementation (Claude Code, OpenCode) provides:
// - Setup: Write config files, hooks, and plugins to the worktree
// - Command: Generate the CLI command to launch the assistant
// - ReadStatus: Parse the status sidecar file
// - ReadMetrics: Parse the metrics sidecar file
type Harness interface {
	// Type returns the harness identifier.
	Type() Type

	// Setup prepares the worktree for this harness (writes config/plugins/hooks).
	// This is called once when spawning a new agent.
	Setup(worktreePath string, opts SetupOptions) error

	// Command returns the command + args to launch the harness.
	Command(opts Options) []string

	// ReadStatus reads the status sidecar file written by the harness.
	// Returns nil, nil if the file doesn't exist (not an error).
	ReadStatus(worktreePath string) (*StatusFile, error)

	// ReadMetrics reads session metrics (cost, model, context) from the sidecar file.
	// Returns nil, nil if the file doesn't exist (not an error).
	ReadMetrics(worktreePath string) (*MetricsData, error)

	// StalenessThreshold returns how old status data can be before falling back to tmux polling.
	StalenessThreshold() time.Duration
}
