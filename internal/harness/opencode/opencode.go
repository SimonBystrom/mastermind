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

// statusPluginScript is the embedded TypeScript plugin for OpenCode.
//
// OpenCode event schemas (from the source):
//   - session.updated:  { info: Session.Info }  (Info has id, title, summary, etc. — no cost/tokens)
//   - session.idle:     { sessionID: string }    (deprecated, no session data)
//   - session.status:   { sessionID, status: { type: "idle"|"busy"|"retry" } }
//   - message.updated:  { info: Message }        (AssistantMessage has cost, tokens, modelID)
//
// Cost/token data lives on individual AssistantMessages, not on Session.Info.
// We use the SDK client (from PluginInput) to fetch session messages for full metrics,
// and accumulate cost incrementally from message.updated events.
const statusPluginScript = `import type { Plugin } from "@opencode-ai/plugin"

export const MastermindStatusPlugin: Plugin = async ({ client, directory }) => {
  const statusFile = ` + "`${directory}/.mastermind-status`" + `
  const metricsFile = ` + "`${directory}/.opencode-status.json`" + `

  // In-memory accumulator for incremental metrics updates
  let currentSessionID = ""
  let totalCost = 0
  let totalInputTokens = 0
  let totalOutputTokens = 0
  let lastModelID = ""
  let linesAdded = 0
  let linesRemoved = 0

  const writeStatus = async (status: string) => {
    const ts = Math.floor(Date.now() / 1000)
    const data = JSON.stringify({ status, ts })
    await Bun.write(statusFile, data + "\n")
  }

  const writeMetricsFile = async () => {
    const metrics = {
      model: lastModelID,
      cost_usd: totalCost,
      context_pct: 0,
      lines_added: linesAdded,
      lines_removed: linesRemoved,
      session_id: currentSessionID,
    }
    await Bun.write(metricsFile, JSON.stringify(metrics))
  }

  // Fetch full session metrics via SDK client and write to sidecar file
  const fetchAndWriteMetrics = async (sessionID: string) => {
    try {
      const { data: messages } = await client.session.messages({ path: { id: sessionID } })
      if (!messages) return

      let cost = 0
      let inputTokens = 0
      let outputTokens = 0
      let modelID = ""

      for (const msg of messages) {
        const info = msg.info as any
        if (info?.role === "assistant") {
          cost += info.cost || 0
          inputTokens += info.tokens?.input || 0
          outputTokens += info.tokens?.output || 0
          modelID = info.modelID || modelID
        }
      }

      // Get session info for lines added/removed from summary
      const { data: sessionInfo } = await client.session.get({ path: { id: sessionID } })
      const summary = (sessionInfo as any)?.summary

      totalCost = cost
      totalInputTokens = inputTokens
      totalOutputTokens = outputTokens
      lastModelID = modelID
      currentSessionID = sessionID
      if (summary) {
        linesAdded = summary.additions || 0
        linesRemoved = summary.deletions || 0
      }

      await writeMetricsFile()
    } catch {
      // SDK call failed — write whatever we have accumulated
      await writeMetricsFile()
    }
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
    "session.created": async ({ event }: any) => {
      const info = event?.properties?.info
      if (info?.id) {
        currentSessionID = info.id
        // Reset accumulators for new session
        totalCost = 0
        totalInputTokens = 0
        totalOutputTokens = 0
        lastModelID = ""
        linesAdded = 0
        linesRemoved = 0
      }
      await writeStatus("running")
    },

    // session.updated carries { info: Session.Info } — use for lines added/removed
    "session.updated": async ({ event }: any) => {
      const info = event?.properties?.info
      if (info) {
        if (info.id) currentSessionID = info.id
        if (info.summary) {
          linesAdded = info.summary.additions || 0
          linesRemoved = info.summary.deletions || 0
        }
        await writeMetricsFile()
      }
    },

    // message.updated carries { info: Message } — accumulate cost/tokens from assistant messages
    "message.updated": async ({ event }: any) => {
      const info = event?.properties?.info
      if (info?.role === "assistant") {
        // Track the latest model
        if (info.modelID) lastModelID = info.modelID
        if (info.sessionID) currentSessionID = info.sessionID

        // Accumulate cost (message.updated fires multiple times per message,
        // so we fetch full totals periodically via fetchAndWriteMetrics on idle)
        totalCost += info.cost || 0
        totalInputTokens += info.tokens?.input || 0
        totalOutputTokens += info.tokens?.output || 0

        await writeMetricsFile()
      }
    },

    // session.idle carries { sessionID } — fetch full metrics via SDK
    "session.idle": async ({ event }: any) => {
      await writeStatus("idle")
      const sessionID = event?.properties?.sessionID || currentSessionID
      if (sessionID) {
        await fetchAndWriteMetrics(sessionID)
      }
    },

    // session.status is the non-deprecated replacement for session.idle
    "session.status": async ({ event }: any) => {
      const status = event?.properties?.status
      const sessionID = event?.properties?.sessionID || currentSessionID
      if (status?.type === "idle") {
        await writeStatus("idle")
        if (sessionID) {
          await fetchAndWriteMetrics(sessionID)
        }
      } else if (status?.type === "busy") {
        await writeStatus("running")
      }
    },
  }
}
`
