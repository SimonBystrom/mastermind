package config

import (
	"os"
	"path/filepath"
)

const statuslineScript = `#!/bin/bash
input=$(cat)
CWD=$(echo "$input" | jq -r '.cwd // ""')
[ -n "$CWD" ] && echo "$input" > "$CWD/.claude-status.json"
MODEL=$(echo "$input" | jq -r '.model.display_name // "?"')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
ADDED=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
REMOVED=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')
printf '[%s] %s%% ctx | $%.2f | +%s -%s' "$MODEL" "$PCT" "$COST" "$ADDED" "$REMOVED"
`

// StatuslineScriptPath returns the path where the statusline script is installed.
func StatuslineScriptPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mastermind", "statusline.sh")
}

// WriteStatuslineScript writes the statusline bash script to disk.
// It always overwrites to ensure the latest version is installed.
func WriteStatuslineScript() error {
	path := StatuslineScriptPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(statuslineScript), 0o755)
}
