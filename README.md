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

Mastermind creates a `.worktrees/` directory in your repo for worktrees, state, and logs.

### Options

| Flag               | Description                                            |
| ------------------ | ------------------------------------------------------ |
| `--repo <path>`    | Path to git repository (defaults to current directory) |
| `--session <name>` | tmux session name (defaults to current session)        |

## Features

- **Parallel agents** — run multiple Claude Code instances simultaneously, each isolated in its own git worktree and branch
- **Real-time status monitoring** — polls tmux pane content every 2s with stable-content hashing to detect agent state (running, waiting for permission/input, review-ready, done)
- **Spawn wizard** — multi-step wizard to select a base branch, create or pick a branch, and name the agent, rendered side-by-side with the dashboard
- **LazyGit integration** — automatically detects when agents finish with uncommitted changes and opens lazygit in a split pane for review
- **Notifications** — color-coded event feed showing agent state transitions
- **Persistence** — agent state is saved to `.worktrees/mastermind-state.json` so agents survive a mastermind restart (recovered if their tmux windows still exist)
- **Dead agent cleanup** — detect and clean up agents whose tmux windows or worktrees have disappeared

## Keybindings

| Key       | Action                                                     |
| --------- | ---------------------------------------------------------- |
| `n`       | Open spawn wizard to create a new agent                    |
| `enter`   | Focus agent window / open lazygit for review-ready agents  |
| `d`       | Dismiss finished agent (keep branch)                       |
| `D`       | Dismiss finished agent + delete branch (with confirmation) |
| `c`       | Clean up dead agents                                       |
| `j` / `k` | Navigate agent list                                        |
| `s`       | Cycle sort mode (id / status / duration)                   |
| `q`       | Quit                                                       |

## How It Works

1. **Spawn** — the spawn wizard walks you through picking a base branch, creating a new branch (or selecting an existing one), and optionally naming the agent. A git worktree is created and Claude Code is launched in a new tmux window.
2. **Monitor** — pane content is polled every 2s. Stable-content hashing distinguishes active work from idle/waiting states. Pattern matching detects Claude Code UI indicators (permission prompts, input requests, working spinners) to classify agent status.
3. **Review** — when an agent finishes with uncommitted changes, its status becomes "review ready". Pressing `enter` opens lazygit in a split pane. Mastermind detects when the review is complete and whether commits were made.
4. **Dismiss** — tears down the tmux window, removes the worktree, optionally deletes the branch.

Agent state is persisted to `.worktrees/mastermind-state.json` and agents are recovered on restart. Logs are written to `.worktrees/mastermind.log`.
