package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	return nil
}

