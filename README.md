# Mastermind

A CLI tool that orchestrates multiple [Claude Code](https://docs.anthropic.com/en/docs/claude-code) agents in parallel using tmux and git worktrees, with a Bubble Tea TUI dashboard.

Each agent runs in its own tmux window on a separate git worktree/branch, so multiple Claude instances can work on different tasks simultaneously without conflicting.

## Prerequisites

- **tmux** 3.0+ — terminal multiplexer
- **git** — version control
- **claude** — [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code)
- **lazygit** — terminal UI for git (used for reviewing changes)

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

Mastermind creates a `.worktrees/` directory in your repo for worktrees and logs.

### Options

| Flag | Description |
|------|-------------|
| `--repo <path>` | Path to git repository (defaults to current directory) |
| `--session <name>` | tmux session name (defaults to current session) |

## Keybindings

| Key | Action |
|-----|--------|
| `n` | Spawn a new agent |
| `enter` | Focus running agent / open lazygit for review |
| `d` | Dismiss agent (keep branch) |
| `D` | Dismiss agent + delete branch |
| `j` / `k` | Navigate agent list |
| `s` | Cycle sort mode (id / status / duration) |
| `q` | Quit |

## How It Works

1. **Spawn** — creates a git branch + worktree, opens a tmux window running `claude`
2. **Monitor** — polls pane content every 2s to detect agent state (running, waiting for permission, idle, finished)
3. **Review** — when an agent finishes with changes, open lazygit in a split pane to review
4. **Dismiss** — tears down the tmux window, removes the worktree, optionally deletes the branch

Agent state is persisted to `.worktrees/mastermind-state.json` so agents survive a mastermind restart. Logs are written to `.worktrees/mastermind.log`.
