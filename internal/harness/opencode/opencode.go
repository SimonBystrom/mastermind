package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simonbystrom/mastermind/internal/harness"
)

// Harness implements the OpenCode integration.
type Harness struct{}

func (h *Harness) Type() harness.Type {
	return harness.TypeOpenCode
}

func (h *Harness) Setup(worktreePath string, opts harness.SetupOptions) error {
	// 1. Write .opencode/plugins/mastermind-status.ts
	if err := h.writeStatusPlugin(worktreePath); err != nil {
		return fmt.Errorf("write status plugin: %w", err)
	}

	// 2. Write opencode.json with plugin config
	if err := h.writeConfig(worktreePath, opts); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// 3. Gitignore OpenCode artifacts
	if err := h.setupGitIgnore(worktreePath); err != nil {
		return fmt.Errorf("setup gitignore: %w", err)
	}

	return nil
}

func (h *Harness) Command(opts harness.Options) []string {
	// OpenCode doesn't have a direct --skip-permissions equivalent
	// Permissions are configured via opencode.json
	return []string{"opencode"}
}

func (h *Harness) ReadStatus(worktreePath string) (*harness.StatusFile, error) {
	// Same .mastermind-status file format as Claude Code
	path := filepath.Join(worktreePath, ".mastermind-status")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read status: %w", err)
	}

	var sf harness.StatusFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	return &sf, nil
}

func (h *Harness) ReadMetrics(worktreePath string) (*harness.MetricsData, error) {
	// Read .opencode-status.json (written by our plugin)
	path := filepath.Join(worktreePath, ".opencode-status.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metrics: %w", err)
	}

	var md harness.MetricsData
	if err := json.Unmarshal(data, &md); err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}

	return &md, nil
}

func (h *Harness) StalenessThreshold() time.Duration {
	return 30 * time.Second
}

// writeStatusPlugin writes the TypeScript plugin to .opencode/plugins/mastermind-status.ts
func (h *Harness) writeStatusPlugin(worktreePath string) error {
	pluginDir := filepath.Join(worktreePath, ".opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	pluginPath := filepath.Join(pluginDir, "mastermind-status.ts")
	return os.WriteFile(pluginPath, []byte(statusPluginScript), 0o644)
}

// writeConfig writes opencode.json with plugin configuration
func (h *Harness) writeConfig(worktreePath string, opts harness.SetupOptions) error {
	opencodeDir := filepath.Join(worktreePath, ".opencode")
	if err := os.MkdirAll(opencodeDir, 0o755); err != nil {
		return err
	}

	config := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"permission": map[string]string{
			"edit": "allow",
			"bash": "allow",
		},
	}

	// Add opencode-worktree plugin if specified
	if len(opts.Plugins) > 0 {
		config["plugin"] = opts.Plugins
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(opencodeDir, "opencode.json")
	return os.WriteFile(configPath, data, 0o644)
}

// setupGitIgnore ensures OpenCode artifacts are git-ignored
func (h *Harness) setupGitIgnore(worktreePath string) error {
	// Gitignore the .opencode directory contents
	opencodeDir := filepath.Join(worktreePath, ".opencode")
	gitignorePath := filepath.Join(opencodeDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("*\n"), 0o644); err != nil {
		return err
	}

	// Also gitignore the sidecar files at the worktree root
	_ = appendGitExclude(worktreePath, ".opencode-status.json")
	_ = appendGitExclude(worktreePath, ".mastermind-status")

	return nil
}

// appendGitExclude adds a pattern to .git/info/exclude for the given worktree
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

// statusPluginScript is the embedded TypeScript plugin for OpenCode
const statusPluginScript = `import type { Plugin } from "@opencode-ai/plugin"

export const MastermindStatusPlugin: Plugin = async ({ directory }) => {
  const statusFile = ` + "`${directory}/.mastermind-status`" + `
  const metricsFile = ` + "`${directory}/.opencode-status.json`" + `
  
  const writeStatus = async (status: string) => {
    const ts = Math.floor(Date.now() / 1000)
    const data = JSON.stringify({ status, ts })
    await Bun.write(statusFile, data + "\n")
  }
  
  const writeMetrics = async (session: any) => {
    if (!session) return
    
    const metrics = {
      model: session.model?.display_name || session.model || "",
      cost_usd: session.cost?.total_cost_usd || 0,
      context_pct: session.context?.used_percentage || 0,
      lines_added: session.cost?.total_lines_added || 0,
      lines_removed: session.cost?.total_lines_removed || 0,
      session_id: session.id || "",
    }
    await Bun.write(metricsFile, JSON.stringify(metrics))
  }
  
  return {
    // Tool events → running
    "tool.execute.before": async () => {
      await writeStatus("running")
    },
    "tool.execute.after": async () => {
      await writeStatus("running")
    },
    
    // Permission events → waiting_permission
    "permission.asked": async () => {
      await writeStatus("waiting_permission")
    },
    "permission.replied": async () => {
      await writeStatus("running")
    },
    
    // Session events
    "session.created": async () => {
      await writeStatus("running")
    },
    "session.idle": async ({ event }) => {
      await writeStatus("idle")
      
      // Write final metrics on idle
      if (event.properties?.session) {
        await writeMetrics(event.properties.session)
      }
    },
    "session.updated": async ({ event }) => {
      // Periodically update metrics during session
      if (event.properties?.session) {
        await writeMetrics(event.properties.session)
      }
    },
  }
}
`
