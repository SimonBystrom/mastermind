package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hookScript is the shell script that Claude Code hooks invoke.
// It reads hook event JSON from stdin and writes a status file.
const hookScript = `#!/bin/sh
set -e

# Read hook event JSON from stdin
INPUT=$(cat)

# Extract hook event name from CLAUDE_HOOK_EVENT_NAME env var
EVENT="$CLAUDE_HOOK_EVENT_NAME"

# Determine status based on hook event
STATUS=""
case "$EVENT" in
  PreToolUse|PostToolUse|SessionStart)
    STATUS="running"
    ;;
  Notification)
    # Check the notification type from the JSON payload
    TYPE=$(echo "$INPUT" | grep -o '"type"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"type"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    case "$TYPE" in
      permission_prompt)
        STATUS="waiting_permission"
        ;;
      idle_prompt)
        STATUS="waiting_input"
        ;;
      *)
        # Unknown notification type, ignore
        exit 0
        ;;
    esac
    ;;
  Stop)
    STATUS="idle"
    ;;
  SessionEnd)
    STATUS="stopped"
    ;;
  *)
    # Unknown event, ignore
    exit 0
    ;;
esac

if [ -z "$STATUS" ]; then
  exit 0
fi

# Write status file to the working directory
TS=$(date +%s)
STATUS_FILE="${CLAUDE_WORKING_DIRECTORY:-.}/.mastermind-status"
printf '{"status":"%s","ts":%s}\n' "$STATUS" "$TS" > "$STATUS_FILE"
`

// settingsJSON is the .claude/settings.local.json content that registers hooks.
var settingsJSON = map[string]interface{}{
	"hooks": map[string]interface{}{
		"PreToolUse": []map[string]interface{}{
			{"hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
		"PostToolUse": []map[string]interface{}{
			{"hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
		"Notification": []map[string]interface{}{
			{"matcher": "permission_prompt", "hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
			{"matcher": "idle_prompt", "hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
		"Stop": []map[string]interface{}{
			{"hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
		"SessionStart": []map[string]interface{}{
			{"hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
		"SessionEnd": []map[string]interface{}{
			{"hooks": []map[string]interface{}{
				{"type": "command", "command": `"$CLAUDE_PROJECT_DIR"/.claude/hooks/mastermind-status.sh`},
			}},
		},
	},
}

// WriteHookFiles writes the hook script and settings.local.json into the
// worktree so that Claude Code instances spawned there report status via hooks.
func WriteHookFiles(worktreePath string) error {
	// Write hook script
	hooksDir := filepath.Join(worktreePath, ".claude", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	scriptPath := filepath.Join(hooksDir, "mastermind-status.sh")
	if err := os.WriteFile(scriptPath, []byte(hookScript), 0o755); err != nil {
		return fmt.Errorf("write hook script: %w", err)
	}

	// Write settings.local.json
	claudeDir := filepath.Join(worktreePath, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	data, err := json.MarshalIndent(settingsJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	// Ensure hook files and status file are gitignored via git's exclude mechanism
	// (uses .git/info/exclude so we don't create untracked .gitignore files)
	if err := ensureGitExclude(worktreePath); err != nil {
		return fmt.Errorf("write git exclude: %w", err)
	}

	return nil
}

// ensureGitExclude adds mastermind-related paths to the git exclude file
// for the worktree. This uses .git/info/exclude (or the worktree-specific
// equivalent) so no untracked files are created.
func ensureGitExclude(worktreePath string) error {
	// Find the common git dir (shared across worktrees) so exclude entries
	// are respected by all worktrees.
	out, err := exec.Command("git", "-C", worktreePath, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return fmt.Errorf("find git common dir: %w", err)
	}
	gitCommonDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitCommonDir) {
		gitCommonDir = filepath.Join(worktreePath, gitCommonDir)
	}

	excludePath := filepath.Join(gitCommonDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	existing, _ := os.ReadFile(excludePath)
	content := string(existing)

	entries := []string{
		".claude/settings.local.json",
		".claude/hooks/",
		".mastermind-status",
	}
	// Note: we keep fine-grained entries rather than blanket ".claude/" so
	// other .claude files (like .claude/settings.json) remain tracked.

	var toAdd []string
	for _, entry := range entries {
		if !containsLine(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	for _, entry := range toAdd {
		if _, err := f.WriteString(entry + "\n"); err != nil {
			return err
		}
	}

	return nil
}

// containsLine checks if the content contains a line matching the given entry.
func containsLine(content, entry string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}
