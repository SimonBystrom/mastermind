package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/harness"
	"github.com/simonbystrom/mastermind/internal/hook"
)

// Harness implements the Claude Code integration.
type Harness struct{}

func (h *Harness) Type() harness.Type {
	return harness.TypeClaudeCode
}

func (h *Harness) Setup(worktreePath string, opts harness.SetupOptions) error {
	// Write Claude Code project settings with statusline config
	if err := h.writeProjectSettings(worktreePath, opts); err != nil {
		return fmt.Errorf("write project settings: %w", err)
	}

	// Write hook files so Claude Code reports status via hooks
	if err := hook.WriteHookFiles(worktreePath); err != nil {
		return fmt.Errorf("write hook files: %w", err)
	}

	return nil
}

func (h *Harness) Command(opts harness.Options) []string {
	cmd := []string{"claude"}
	if opts.SkipPermissions {
		cmd = append(cmd, "--dangerously-skip-permissions")
	}
	return cmd
}

func (h *Harness) ReadStatus(worktreePath string) (*harness.StatusFile, error) {
	sf, err := hook.ReadStatus(worktreePath)
	if err != nil || sf == nil {
		return nil, err
	}

	// Convert from hook.StatusFile to harness.StatusFile
	return &harness.StatusFile{
		Status:    sf.Status,
		Timestamp: sf.Timestamp,
	}, nil
}

func (h *Harness) ReadMetrics(worktreePath string) (*harness.MetricsData, error) {
	path := filepath.Join(worktreePath, ".claude-status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read claude status: %w", err)
	}

	// Parse the Claude Code statusline JSON structure
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse claude status: %w", err)
	}

	md := &harness.MetricsData{}

	// Extract model name
	if model, ok := raw["model"].(map[string]interface{}); ok {
		if displayName, ok := model["display_name"].(string); ok {
			md.Model = displayName
		}
	}

	// Extract cost data
	if cost, ok := raw["cost"].(map[string]interface{}); ok {
		if totalCost, ok := cost["total_cost_usd"].(float64); ok {
			md.CostUSD = totalCost
		}
		if linesAdded, ok := cost["total_lines_added"].(float64); ok {
			md.LinesAdded = int(linesAdded)
		}
		if linesRemoved, ok := cost["total_lines_removed"].(float64); ok {
			md.LinesRemoved = int(linesRemoved)
		}
	}

	// Extract context usage
	if ctxWindow, ok := raw["context_window"].(map[string]interface{}); ok {
		if usedPct, ok := ctxWindow["used_percentage"].(float64); ok {
			md.ContextPct = usedPct
		}
	}

	// Extract session ID
	if sessionID, ok := raw["session_id"].(string); ok {
		md.SessionID = sessionID
	}

	return md, nil
}

func (h *Harness) StalenessThreshold() time.Duration {
	return hook.StalenessThreshold
}

// writeProjectSettings writes .claude/settings.json in the worktree
// to configure Claude Code's statusline for this agent. It also ensures the
// .claude/ directory and .claude-status.json sidecar are git-ignored.
func (h *Harness) writeProjectSettings(wtPath string, opts harness.SetupOptions) error {
	dir := filepath.Join(wtPath, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Gitignore the .claude directory contents so they don't appear as uncommitted changes
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return err
	}

	// Also gitignore the sidecar file at the worktree root
	_ = appendGitExclude(wtPath, ".claude-status.json")

	settings := map[string]interface{}{
		"statusLine": map[string]string{
			"type":    "command",
			"command": config.StatuslineScriptPath(),
		},
	}

	if opts.AgentTeams {
		settings["env"] = map[string]string{
			"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1",
		}
	}
	if opts.TeammateMode != "" {
		settings["teammateMode"] = opts.TeammateMode
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

// appendGitExclude adds a pattern to .git/info/exclude for the given worktree
// if it's not already present. Uses --git-common-dir so excludes work in
// worktrees (worktree-specific git dirs don't support info/exclude).
func appendGitExclude(wtPath, pattern string) error {
	// Use --git-common-dir which resolves to the main .git dir for worktrees
	out, err := exec.Command("git", "-C", wtPath, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return err
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	content, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}

	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	prefix := ""
	if len(content) > 0 && content[len(content)-1] != '\n' {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s%s\n", prefix, pattern)
	return err
}
