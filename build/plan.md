# Mastermind — Implementation Plan

## Context

Build a Go CLI tool that orchestrates multiple Claude Code agents in parallel. Each agent runs in its own git worktree and tmux window. A persistent Bubble Tea dashboard provides an overview of all agents, notifications on completion, and triggers lazygit review.

## Architecture Overview

```
mastermind (Go binary)
  ├── Bubble Tea TUI (dashboard) — runs in tmux window 0
  ├── Orchestrator — coordinates agent lifecycle
  ├── tmux — manages windows/panes via CLI
  └── git — manages worktrees/branches via CLI
```

Each agent = **worktree + branch + Claude Code process + tmux window**

## Project Structure

```
/Users/simonbystrom/code/mastermind/
├── go.mod
├── main.go                        # Entry point, flag parsing, startup
├── internal/
│   ├── agent/
│   │   ├── agent.go               # Agent struct, status constants
│   │   └── store.go               # Thread-safe in-memory agent store
│   ├── git/
│   │   ├── worktree.go            # Create/remove/list worktrees
│   │   └── branch.go              # List/create branches
│   ├── tmux/
│   │   ├── session.go             # Session create/detect
│   │   ├── window.go              # Window/pane create/kill/split
│   │   └── monitor.go             # Poll pane status for completion
│   ├── ui/
│   │   ├── app.go                 # Top-level Bubble Tea model (view router)
│   │   ├── dashboard.go           # Agent table, notifications, keybindings
│   │   ├── spawn.go               # Multi-step spawn wizard
│   │   └── styles.go              # Lipgloss styles
│   └── orchestrator/
│       └── orchestrator.go        # Coordinates agent, git, tmux, ui events
└── Makefile
```

## Key Data Structures

**Agent** — ID, Name, Branch, BaseBranch, WorktreePath, TmuxWindow, TmuxPaneID, Status (running/done/reviewing/dismissed), timestamps, exit code.

**Store** — Thread-safe map of agents with Add/Get/All/UpdateStatus/Remove.

## Core Flows

### 1. Spawn Flow
1. User presses `n` in dashboard → spawn wizard opens
2. Step 1: Pick base branch (filterable list)
3. Step 2: Create new branch or pick existing + optional agent name
4. Step 3: Confirm summary → orchestrator creates worktree, launches claude in new tmux window
5. tmux command clears `CLAUDECODE` env var so claude starts cleanly: `tmux new-window -e CLAUDECODE= -e CLAUDE_CODE_ENTRYPOINT= ...`
6. Pane has `remain-on-exit on` so we can detect when claude exits

### 2. Monitor Flow (2-second poll)
- Goroutine polls each running agent's tmux pane via `tmux display-message -t <paneID> -p '#{pane_dead}|#{pane_dead_status}'`
- On death: update agent status to `done`, record exit code, send `AgentFinishedMsg` to Bubble Tea UI
- Dashboard shows notification and highlights the agent

### 3. LazyGit Review Flow
- When agent finishes → notification appears in dashboard, agent row highlighted
- User presses `Enter` on a done agent → orchestrator splits a new pane in the agent's tmux window with lazygit: `tmux split-window -t <paneID> -c <worktreePath> lazygit`
- Agent status → `reviewing`

### 4. Dismiss/Cleanup Flow
- User presses `d` on a done/reviewing agent
- Kill tmux window, remove git worktree, remove from store

## Dashboard UI

```
╭─ Mastermind ─ repo: /path/to/project ─ session: mastermind ──────╮
│                                                                    │
│  ID  Name              Branch            Status     Duration       │
│  a1  oauth-refresh     feat/oauth        running    12m 34s        │
│  a2  fix-nav           fix/nav-bug       done        3m 12s   ◀   │
│  a3  add-tests         test/coverage     reviewing   8m 45s        │
│                                                                    │
│  ── Notifications ──                                               │
│  10:32 Agent a2 finished (exit 0)                                  │
│                                                                    │
│  n: new agent │ enter: focus/review │ d: dismiss │ q: quit         │
╰────────────────────────────────────────────────────────────────────╯
```

## Startup Sequence

1. Parse flags (`--repo`, `--session`)
2. Validate dependencies on PATH: `tmux`, `git`, `claude`, `lazygit`
3. Validate repo is a git repository
4. Ensure `.worktrees/` directory exists
5. Detect if inside tmux → use current session or create+attach
6. Start Bubble Tea in window 0 ("dashboard")
7. Start monitor goroutine
8. Block on Bubble Tea until quit

## Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/bubbles` — list, textinput, spinner components
- `github.com/charmbracelet/lipgloss` — styling
- All git/tmux operations via `os/exec`

## Implementation Order

1. **Skeleton**: `main.go`, flag parsing, dependency validation, go.mod
2. **tmux package**: session, window, pane operations
3. **git package**: branch listing, worktree create/remove
4. **Agent store**: agent struct, thread-safe store
5. **Orchestrator**: SpawnAgent, DismissAgent, FocusAgent, OpenLazyGit
6. **Dashboard UI**: agent table, notifications, keybindings
7. **Spawn wizard UI**: branch picker, name input, confirmation
8. **Monitor**: polling loop, completion detection, lazygit trigger
9. **Integration**: wire everything together, end-to-end test

## Verification

1. Build: `go build -o mastermind .`
2. Run inside tmux in a git repo: `./mastermind --repo /path/to/repo`
3. Press `n` → spawn wizard should list branches
4. Complete spawn → claude should launch in a new tmux window
5. Exit claude (type `/exit`) → dashboard should show notification, agent marked done
6. Press `Enter` on done agent → lazygit should open in a split pane
7. Press `d` → agent dismissed, worktree cleaned up
