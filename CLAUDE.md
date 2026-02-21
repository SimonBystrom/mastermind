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

Mastermind is a Go CLI tool that orchestrates multiple Claude Code agents in parallel. It uses tmux windows and git worktrees to isolate each agent, with a Bubble Tea TUI dashboard for managing them. Each agent gets its own branch/worktree so multiple Claude instances can work simultaneously without conflicts.

- **Go 1.26** — module path `github.com/simonbystrom/mastermind`
- **Required runtime dependencies:** tmux 3.0+, git, claude (Claude Code CLI), lazygit, jq
- **Optional dev dependency:** goreleaser (for `make snapshot` local release testing)

## Architecture

**Entry point:** `main.go` — flag parsing, dependency validation, orchestrator/UI setup.

The two central packages are **orchestrator** (async engine) and **ui** (Bubble Tea TUI). The orchestrator exposes `tea.Cmd` functions; the UI calls them and reacts to the resulting `tea.Msg` values. All other packages are support libraries used by one or both.

**`internal/` packages:**

- **`orchestrator/`** — Core engine. Spawns agents (worktree + tmux window + claude launch), dismisses, merges, previews, and recovers. Returns `tea.Cmd`s that produce typed messages (`AgentFinishedMsg`, `MergeResultMsg`, etc.).
- **`ui/`** — Bubble Tea TUI. `AppModel` routes between views: dashboard, spawn wizard, merge confirmation, dismiss dialog. Consumes orchestrator messages to update state.
- **`agent/`** — Agent data model + thread-safe `Store` (RWMutex-guarded map with atomic ID counter). Persistence to `.worktrees/mastermind-state.json`. Statusline parsing for cost/model/context data.
- **`git/`** — Git operations behind a `GitOps` interface. Branch CRUD, worktree management, merge (fast-forward + full), conflict detection. Worktrees stored in `.worktrees/`.
- **`tmux/`** — Tmux operations behind a `TmuxOps` interface. Window/pane management, status monitoring via pane content polling (SHA256 hashing for stability), pane death detection.
- **`hook/`** — Claude Code hook registration. Generates `.claude/settings.local.json` and `.claude/hooks/mastermind-status.sh` in each worktree for instant status updates.
- **`config/`** — TOML config parsing from `~/.config/mastermind/mastermind.conf`. 24 color slots (Catppuccin Mocha defaults) and layout sizing.

## Key Patterns

- **Dependency injection via interfaces:** `git.GitOps` and `tmux.TmuxOps` are interfaces implemented by real structs and mocked in tests. Always use the interface, not the concrete type, when adding parameters/fields.

- **Bubble Tea message-passing:** All async work (spawning, merging, monitoring) is done via `tea.Cmd` functions that return typed `tea.Msg` values. The UI `Update` method pattern-matches on message types. Never do blocking I/O in `Update`.

- **Thread safety:** Agent fields are guarded by `RWMutex` on the `Agent` struct. The `Store` has its own `RWMutex`. Use the accessor methods, not direct field access.

- **Agent status lifecycle:** `running` → `waiting` ⟷ `running` → `review_ready` → `reviewing` → `reviewed` → merge → `done`. Status transitions happen in both the monitor (pane polling/hooks) and UI (user actions).

- **Hybrid status monitoring:** Prefers Claude Code hook data (`.mastermind-status` JSON file written by hook script). Falls back to tmux pane content polling (every 2s) when hook data is stale (>30s).

- **Logging:** Uses `log/slog` with structured logging to `.worktrees/mastermind.log`. New code should use `slog.Info`/`slog.Error`/etc. — not `fmt.Println` or `log.Println`.

- **Git ignores:** All generated/runtime files (`.worktrees/`, `.claude/settings.local.json`, `.claude/hooks/`, `.mastermind-status`) are excluded via `.gitignore`. Do not use `.git/info/exclude`.

## Required Final Steps

After making **any** code changes in this repo, you MUST always run these steps before considering your work done:

1. **Run tests:** `make test` — all tests must pass. If your changes break existing tests, update them. If you add new functionality, add corresponding tests.
2. **Build:** `make build` — the binary must compile cleanly.

You are always allowed to run these commands without asking for permission. Do not skip these steps.
