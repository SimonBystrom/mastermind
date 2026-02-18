package tmux

// PatternRule defines a single pattern for classifying pane content.
type PatternRule struct {
	Contains     string // Required substring
	Suffix       string // Optional: line must also end with this
	RequiresAlso string // Optional: joined bottom content must also contain this
}

// MonitorPatterns defines the string patterns used to classify pane state.
type MonitorPatterns struct {
	// WorkingIndicators are patterns that, if found in any bottom line,
	// indicate Claude is still working (even if content is stable).
	WorkingIndicators []PatternRule

	// EarlyPermissionPatterns are simple substring patterns checked before
	// content stabilizes. Only include strings that unambiguously indicate
	// a permission prompt to avoid false positives on changing content.
	EarlyPermissionPatterns []string

	// PermissionPatterns are patterns that indicate a permission prompt.
	PermissionPatterns []PatternRule

	// InputPatterns are patterns that indicate Claude is at the input prompt.
	InputPatterns []PatternRule
}

// PaneStatus represents the current state of a tmux pane.
type PaneStatus struct {
	Dead            bool
	ExitCode        int
	WaitingFor      string // "permission", "input", "unknown", or "" (working)
	HasNumberedList bool   // bottom of pane contains numbered options (1. X  2. Y  3. Z)
}

// DefaultPatterns contains the default detection patterns for Claude Code.
var DefaultPatterns = MonitorPatterns{
	WorkingIndicators: []PatternRule{
		{Contains: "Running", Suffix: "…"},
	},
	EarlyPermissionPatterns: []string{
		"Do you want to proceed?",
		"Esc to cancel",
	},
	PermissionPatterns: []PatternRule{
		// Tool permission prompts
		{Contains: "Yes", RequiresAlso: "No"},
		{Contains: "Allow", RequiresAlso: "Deny"},
		{Contains: "allow for"},
		{Contains: "Always allow"},
		// AskUserQuestion prompts (numbered options + "Chat about this")
		{Contains: "Chat about this"},
	},
	InputPatterns: []PatternRule{
		{Contains: "for shortcuts"},
	},
}
