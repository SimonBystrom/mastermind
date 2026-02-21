package config

import (
	"os"
	"path/filepath"
)

const statuslineScript = `#!/bin/sh
input=$(cat)

dir=$(echo "$input" | jq -r '.workspace.current_dir // .cwd // ""')
[ -n "$dir" ] && echo "$input" > "$dir/.claude-status.json"

model=$(echo "$input" | jq -r '.model.display_name // ""')
used=$(echo "$input" | jq -r '.context_window.used_percentage // empty')

# Short directory name (basename)
short_dir=$(basename "$dir")

# Context usage indicator
if [ -n "$used" ]; then
  used_int=$(printf "%.0f" "$used")
  ctx=" [ctx: ${used_int}%]"
else
  ctx=""
fi

# Git branch (skip optional locks to avoid blocking)
git_branch=""
if [ -d "$dir/.git" ] || git -C "$dir" rev-parse --git-dir > /dev/null 2>&1; then
  branch=$(git -C "$dir" --no-optional-locks symbolic-ref --short HEAD 2>/dev/null)
  if [ -n "$branch" ]; then
    git_branch=" git:(${branch})"
  fi
fi

# Code changes from Claude Code statusline data
added=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
removed=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')
diff_parts=""
if [ "$added" -gt 0 ]; then
  diff_parts="\033[0;32m+${added}\033[0m"
fi
if [ "$removed" -gt 0 ]; then
  [ -n "$diff_parts" ] && diff_parts="${diff_parts} "
  diff_parts="${diff_parts}\033[0;31m-${removed}\033[0m"
fi
diff_stat=""
if [ -n "$diff_parts" ]; then
  diff_stat=" ${diff_parts}"
fi

# Session cost from Claude Code statusline data
total_cost=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
cost_str=$(printf " \033[2m\$%.4f\033[0m" "$total_cost")

printf "\033[1;32m➜\033[0m  \033[0;36m%s\033[0m\033[1;34m%s\033[0m\033[0;33m%s\033[0m%b%b\033[2m  %s\033[0m" \
  "$short_dir" \
  "$git_branch" \
  "$ctx" \
  "$diff_stat" \
  "$cost_str" \
  "$model"
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
