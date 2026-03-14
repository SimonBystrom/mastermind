# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
make build          # Compile binary (go build with version ldflags)
make test           # Run tests with gotestsum (auto-installs if missing)
make run            # Build and run
make install        # Build, install to /usr/local/bin, init config
go test ./internal/agent/...           # Run tests for a single package
go test -run TestName ./internal/...   # Run a single test by name
```

## What This Project Is

Mastermind is a Go CLI tool that orchestrates multiple AI coding agents in parallel. It uses tmux windows and git worktrees to isolate each agent, with a Bubble Tea TUI dashboard for managing them. Each agent gets its own branch/worktree so multiple instances can work simultaneously without conflicts.

Mastermind supports multiple AI coding assistant harnesses:
- **Claude Code** (default) — Anthropic's official Claude CLI
- **OpenCode** — Alternative open-source AI coding assistant

- **Go 1.26** — module path `github.com/simonbystrom/mastermind`
- **Required runtime dependencies:** tmux 3.0+, git, lazygit, jq
- **Optional runtime dependencies:** claude (Claude Code CLI), opencode (OpenCode CLI)

## Architecture

**Entry point:** `main.go` — flag parsing, dependency validation, orchestrator/UI setup.

The two central packages are **orchestrator** (async engine) and **ui** (Bubble Tea TUI). The orchestrator exposes `tea.Cmd` functions; the UI calls them and reacts to the resulting `tea.Msg` values. All other packages are support libraries used by one or both.

**`internal/` packages:**

- **`orchestrator/`** — Core engine. Spawns agents (worktree + tmux window + harness launch), dismisses, merges, previews, and recovers. Returns `tea.Cmd`s that produce typed messages (`AgentFinishedMsg`, `MergeResultMsg`, etc.). Maintains a registry of harness implementations and dispatches to the correct harness per agent.
- **`ui/`** — Bubble Tea TUI. `AppModel` routes between views: dashboard, spawn wizard, merge confirmation, dismiss dialog. Consumes orchestrator messages to update state. Dashboard displays harness badges (`[C]` for Claude Code, `[O]` for OpenCode).
- **`agent/`** — Agent data model + thread-safe `Store` (RWMutex-guarded map with atomic ID counter). Persistence to `.worktrees/mastermind-state.json` includes harness type for recovery. Statusline parsing for cost/model/context data.
- **`harness/`** — Harness abstraction layer. Defines `Harness` interface with methods for agent lifecycle (Spawn, Attach, Monitor, Stop) and status reporting. Each harness implementation provides:
  - **`claudecode/`** — Claude Code implementation. Uses `.claude/hooks/mastermind-status.sh` hook for status updates and `.claude-status.json` for metrics.
  - **`opencode/`** — OpenCode implementation. Embeds TypeScript plugin (written to `.opencode/plugins/mastermind-status.ts`) that writes `.mastermind-status` and `.opencode-status.json` for metrics.
- **`git/`** — Git operations behind a `GitOps` interface. Branch CRUD, worktree management, merge (fast-forward + full), conflict detection. Worktrees stored in `.worktrees/`.
- **`tmux/`** — Tmux operations behind a `TmuxOps` interface. Window/pane management, status monitoring via pane content polling (SHA256 hashing for stability), pane death detection.
- **`hook/`** — Claude Code hook registration. Generates `.claude/settings.local.json` and `.claude/hooks/mastermind-status.sh` in each worktree for instant status updates. The hook script writes `{"status":"...","ts":...}` to `.mastermind-status` atomically.
- **`config/`** — TOML config parsing from `~/.config/mastermind/mastermind.conf`. 25 color slots (Catppuccin Mocha defaults), layout sizing, `[claude]` section (`agent_teams`, `teammate_mode`), and `[harness]` section (default harness selection). Also installs `~/.config/mastermind/statusline.sh` — the Claude Code statusline script that writes `.claude-status.json` sidecar files.
- **`team/`** — Reads Claude Code's native team/task data from `~/.claude/teams/` and `~/.claude/tasks/`. `TeamReader` interface matches a team to a mastermind session by the lead agent's session ID, with 10s TTL caching.

## Key Patterns

- **Dependency injection via interfaces:** `git.GitOps`, `tmux.TmuxOps`, and `harness.Harness` are interfaces implemented by real structs and mocked in tests. Always use the interface, not the concrete type, when adding parameters/fields.

- **Harness abstraction:** Each agent has an immutable `Harness` field (type `harness.Type`) set at spawn time. The orchestrator maintains a registry of harness implementations and dispatches lifecycle operations (spawn, attach, monitor, stop) to the correct harness. This allows mixing Claude Code and OpenCode agents in the same session.

- **Bubble Tea message-passing:** All async work (spawning, merging, monitoring) is done via `tea.Cmd` functions that return typed `tea.Msg` values. The UI `Update` method pattern-matches on message types. Never do blocking I/O in `Update`.

- **Thread safety:** Agent fields are guarded by `RWMutex` on the `Agent` struct. The `Store` has its own `RWMutex`. Use the accessor methods, not direct field access.

- **Agent status lifecycle:** `running` → `waiting` ⟷ `running` → `review_ready` → `reviewing` → `reviewed` → merge → `done`. Additional statuses: `previewing`, `conflicts`, `dismissed`. Status transitions happen in both the monitor (pane polling/hooks/plugins) and UI (user actions). Review lifecycle states are **derived**, not harness-specific: `review_ready` = agent goes `idle` AND `git.HasChanges()` returns true.

- **Dual sidecar files:** `.mastermind-status` (written by hook/plugin — agent state like running/waiting/idle) and `.claude-status.json` / `.opencode-status.json` (written by statusline/plugin — cost/model/context data). These serve different purposes and are read by different subsystems. Both use mtime-based caching to avoid redundant reads.

- **Hybrid status monitoring:** Prefers hook/plugin data (`.mastermind-status`, <30s staleness threshold). Falls back to tmux pane content polling (every 2s, SHA256 stability hashing, configurable patterns) when hook/plugin data is stale. Always reads metrics regardless of which status method worked. Pane content parsing supports both Claude Code's statusline format (`➜ dirname [ctx: X%] $X.XX model`) and OpenCode's "Project overview" format (`Context\nX% used\n$X.XX spent`).

- **OpenCode plugin structure:** The OpenCode harness embeds a TypeScript plugin as a string constant in Go. On spawn, it writes the plugin to `.opencode/plugins/mastermind-status.ts` in the worktree. The plugin listens for OpenCode events (`tool.execute.before/after`, `permission.asked`, `session.idle`, `session.updated`) and maps them to mastermind status values, writing `.mastermind-status` and `.opencode-status.json`.

- **Logging:** Uses `log/slog` with structured logging to `.worktrees/mastermind.log`. New code should use `slog.Info`/`slog.Error`/etc. — not `fmt.Println` or `log.Println`.

- **Git ignores:** All generated/runtime files (`.worktrees/`, `.claude/settings.local.json`, `.claude/hooks/`, `.opencode/plugins/`, `.mastermind-status`, `.claude-status.json`, `.opencode-status.json`) are excluded via `.gitignore`. The orchestrator also adds harness-specific metrics files to per-worktree `.git/info/exclude`.

## Required Final Steps

After making **any** code changes in this repo, you MUST always run these steps before considering your work done:

1. **Run tests:** `make test` — all tests must pass. If your changes break existing tests, update them. If you add new functionality, add corresponding tests.
2. **Build:** `make build` — the binary must compile cleanly.

You are always allowed to run these commands without asking for permission. Do not skip these steps.
