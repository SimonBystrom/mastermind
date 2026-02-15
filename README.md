```
                          ╭─────────────────────────────────────╮
                          │                                     │
 ███╗   ███╗  █████╗  ███████╗████████╗███████╗██████╗          │
 ████╗ ████║ ██╔══██╗ ██╔════╝╚══██╔══╝██╔════╝██╔══██╗        │
 ██╔████╔██║ ███████║ ███████╗   ██║   █████╗  ██████╔╝        │
 ██║╚██╔╝██║ ██╔══██║ ╚════██║   ██║   ██╔══╝  ██╔══██╗        │
 ██║ ╚═╝ ██║ ██║  ██║ ███████║   ██║   ███████╗██║  ██║        │
 ╚═╝     ╚═╝ ╚═╝  ╚═╝ ╚══════╝   ╚═╝   ╚══════╝╚═╝  ╚═╝        │
                  ███╗   ███╗██╗███╗   ██╗██████╗               │
                  ████╗ ████║██║████╗  ██║██╔══██╗              │
                  ██╔████╔██║██║██╔██╗ ██║██║  ██║              │
                  ██║╚██╔╝██║██║██║╚██╗██║██║  ██║              │
                  ██║ ╚═╝ ██║██║██║ ╚████║██████╔╝              │
                  ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═════╝               │
                          │                                     │
                          │  orchestrate claude code agents     │
                          ╰─────────────────────────────────────╯
```

A CLI tool that orchestrates multiple [Claude Code](https://docs.anthropic.com/en/docs/claude-code) agents in parallel using tmux and git worktrees, with a Bubble Tea TUI dashboard.

Each agent runs in its own tmux window on a separate git worktree/branch, so multiple Claude instances can work on different tasks simultaneously without conflicting.

## Prerequisites

- **tmux** 3.0+ — terminal multiplexer
- **git** — version control
- **claude** — [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code)
- **lazygit** — terminal UI for git (used for reviewing and merging changes)

## Install

```bash
go install github.com/simonbystrom/mastermind@latest
```

Or build from source:

```bash
git clone https://github.com/simonbystrom/mastermind.git
cd mastermind
go build -o mastermind .
```

## Usage

Run inside a tmux session from your git repository:

```bash
cd /path/to/your/repo
mastermind
```

Mastermind creates a `.worktrees/` directory in your repo for worktrees, state, and logs.

### Options

| Flag               | Description                                            |
| ------------------ | ------------------------------------------------------ |
| `--repo <path>`    | Path to git repository (defaults to current directory) |
| `--session <name>` | tmux session name (defaults to current session)        |

## Features

- **Parallel agents** — run multiple Claude Code instances simultaneously, each isolated in its own git worktree and branch
- **Real-time status monitoring** — polls tmux pane content every 2s with SHA256 stable-content hashing to detect agent state (running, waiting for permission/input, needs attention, review-ready, done)
- **Spawn wizard** — multi-step wizard to select a base branch, create or pick a branch, and name the agent, rendered side-by-side with the dashboard
- **LazyGit integration** — opens lazygit in a split pane for reviewing uncommitted changes, tracks commits made during review
- **Merge workflow** — merge agent branches back into their base branch with fast-forward or full merge, including conflict detection and resolution via lazygit
- **Sortable agent list** — cycle between sorting by ID, status priority, or duration
- **Notifications** — color-coded event feed showing agent state transitions
- **Persistence** — agent state is saved to `.worktrees/mastermind-state.json` so agents survive a mastermind restart (recovered if their tmux windows still exist)
- **Dead agent cleanup** — detect and clean up agents whose tmux windows or worktrees have disappeared, or whose branches have already been merged
- **System monitor** — automatically opens btop (or top) in a split pane for system resource monitoring

## Agent Lifecycle

```
running → waiting (permission/input)
    ↓
review ready → reviewing (lazygit open) → reviewed (commits made)
    ↓                                          ↓
   done                                    merge → conflicts → resolve → done
                                             ↓
                                            done
```

| Status         | Description                                          |
| -------------- | ---------------------------------------------------- |
| **running**    | Claude Code is actively working                      |
| **waiting**    | Agent needs permission approval or user input         |
| **attention?** | Agent may need attention (stable but unrecognized state) |
| **review ready** | Agent finished with uncommitted changes            |
| **reviewing**  | LazyGit is open for review                           |
| **reviewed**   | Review completed, new commits were made              |
| **conflicts**  | Merge conflicts detected, needs resolution           |
| **done**       | Agent finished with no pending changes               |

## Keybindings

| Key        | Action                                                     |
| ---------- | ---------------------------------------------------------- |
| `n`        | Open spawn wizard to create a new agent                    |
| `enter`    | Focus agent window / open lazygit for review-ready, reviewed, or conflicting agents |
| `m`        | Merge agent branch into base branch (review-ready or reviewed, with confirmation) |
| `d`        | Dismiss finished agent (keep branch)                       |
| `D`        | Dismiss finished agent + delete branch (with confirmation) |
| `c`        | Clean up dead agents                                       |
| `j` / `k` / `↓` / `↑` | Navigate agent list                          |
| `s`        | Cycle sort mode (id / status / duration)                   |
| `q` / `ctrl+c` | Quit                                                   |

## How It Works

1. **Spawn** — the spawn wizard walks you through picking a base branch, creating a new branch (or selecting an existing one), and optionally naming the agent. A git worktree is created and Claude Code is launched in a new tmux window.
2. **Monitor** — pane content is polled every 2s. SHA256 content hashing detects when output stabilizes (~4s). Pattern matching classifies the stable state: active work (running spinners), permission prompts, input prompts, or idle.
3. **Review** — when an agent finishes with uncommitted changes, its status becomes "review ready". Pressing `enter` opens lazygit in a split pane. Mastermind tracks the pre-review commit hash and detects whether new commits were made during review.
4. **Merge** — after review, press `m` to merge the agent branch into its base. Fast-forward is used when possible. If merge conflicts occur, lazygit reopens for manual resolution; mastermind monitors and completes the merge once conflicts are resolved.
5. **Dismiss** — tears down the tmux window, removes the worktree, optionally deletes the branch.

Agent state is persisted to `.worktrees/mastermind-state.json` and agents are recovered on restart. Logs are written to `.worktrees/mastermind.log`.

## Testing

Run all tests:

```bash
go test ./...
```

Run with race detection:

```bash
go test -race ./...
```

Run by package:

```bash
go test ./internal/agent/...        # agent, store, persistence
go test ./internal/tmux/...         # pane classification patterns
go test ./internal/git/...          # real git operations (requires git)
go test ./internal/orchestrator/... # orchestrator with mocks + integration
go test ./internal/ui/...           # TUI model tests
```

The test suite uses real git repos (via `t.TempDir()`) for git operations and mock interfaces for tmux — no running tmux or Claude instances are needed.
