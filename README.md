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

Orchestrate multiple [Claude Code](https://docs.anthropic.com/en/docs/claude-code) agents in parallel using tmux and git worktrees, with a Bubble Tea TUI dashboard.

Each agent runs in its own tmux window on a separate git worktree/branch, so multiple Claude instances can work on different tasks simultaneously without conflicting.

## Quick Install

**Homebrew** (macOS):

```bash
brew install simonbystrom/tap/mastermind
```

**Install script** (macOS / Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/simonbystrom/mastermind/main/install.sh | sh
```

**Build from source:**

```bash
git clone https://github.com/simonbystrom/mastermind.git
cd mastermind
make install
```

## Prerequisites

| Dependency | Install |
|---|---|
| **tmux** 3.0+ | `brew install tmux` |
| **git** | `brew install git` (or via Xcode CLT) |
| **claude** | [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) |
| **lazygit** | `brew install lazygit` |

## Usage

Run inside a tmux session from any git repository:

```bash
cd /path/to/your/repo
mastermind
```

Mastermind creates a `.worktrees/` directory in your repo for worktrees, state, and logs.

### Flags

| Flag | Description |
|---|---|
| `--repo <path>` | Path to git repository (defaults to current directory) |
| `--session <name>` | tmux session name (defaults to current session) |
| `--version` | Print version and exit |
| `--init-config` | Write default config file and print its path |

## Configuration

Mastermind reads its config from `$XDG_CONFIG_HOME/mastermind/mastermind.conf` (defaults to `~/.config/mastermind/mastermind.conf`). A default config file with all values commented out is created on first run or via `mastermind --init-config`.

The config uses TOML format:

```toml
[colors]
# Values can be hex (#rrggbb) or xterm-256 codes (0-255).
# Defaults use the Catppuccin Mocha palette.
# title          = "#cba6f7"  # Mauve
# header         = "#89b4fa"  # Blue
# selected_bg    = "#313244"  # Surface 0
# selected_fg    = "#cdd6f4"  # Text
# running        = "#89b4fa"  # Blue
# review_ready   = "#94e2d5"  # Teal
# done           = "#7f849c"  # Overlay 1
# waiting        = "#f9e2af"  # Yellow
# permission     = "#fab387"  # Peach
# reviewing      = "#b4befe"  # Lavender
# reviewed       = "#a6e3a1"  # Green
# conflicts      = "#f38ba8"  # Red
# notification   = "#a6adc8"  # Subtext 0
# help           = "#7f849c"  # Overlay 1
# border         = "#585b70"  # Surface 2
# separator      = "#585b70"  # Surface 2
# wizard_title   = "#cba6f7"  # Mauve
# wizard_active  = "#cba6f7"  # Mauve
# wizard_dim     = "#7f849c"  # Overlay 1
# error          = "#f38ba8"  # Red
# attention      = "#fab387"  # Peach
# logo           = "#cba6f7"  # Mauve
# previewing     = "#f5c2e7"  # Pink
# preview_banner = "#f5c2e7"  # Pink

[layout]
# dashboard_width = 55   # percentage of terminal width for left panel
# lazygit_split   = 80   # percentage for lazygit pane size
```

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

## Keybindings

| Key | Action |
|---|---|
| `n` | Open spawn wizard to create a new agent |
| `enter` | Focus agent window / open lazygit for review-ready, reviewed, or conflicting agents |
| `m` | Merge agent branch into base branch (review-ready or reviewed, with confirmation) |
| `d` | Dismiss finished agent (keep branch) |
| `D` | Dismiss finished agent + delete branch (with confirmation) |
| `c` | Clean up dead agents |
| `j` / `k` / `↓` / `↑` | Navigate agent list |
| `s` | Cycle sort mode (id / status / duration) |
| `q` / `ctrl+c` | Quit |

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

| Status | Description |
|---|---|
| **running** | Claude Code is actively working |
| **waiting** | Agent needs permission approval or user input |
| **attention?** | Agent may need attention (stable but unrecognized state) |
| **review ready** | Agent finished with uncommitted changes |
| **reviewing** | LazyGit is open for review |
| **reviewed** | Review completed, new commits were made |
| **conflicts** | Merge conflicts detected, needs resolution |
| **done** | Agent finished with no pending changes |

## How It Works

1. **Spawn** — the spawn wizard walks you through picking a base branch, creating a new branch (or selecting an existing one), and optionally naming the agent. A git worktree is created and Claude Code is launched in a new tmux window.
2. **Monitor** — pane content is polled every 2s. SHA256 content hashing detects when output stabilizes (~4s). Pattern matching classifies the stable state: active work (running spinners), permission prompts, input prompts, or idle.
3. **Review** — when an agent finishes with uncommitted changes, its status becomes "review ready". Pressing `enter` opens lazygit in a split pane. Mastermind tracks the pre-review commit hash and detects whether new commits were made during review.
4. **Merge** — after review, press `m` to merge the agent branch into its base. Fast-forward is used when possible. If merge conflicts occur, lazygit reopens for manual resolution; mastermind monitors and completes the merge once conflicts are resolved.
5. **Dismiss** — tears down the tmux window, removes the worktree, optionally deletes the branch.

Agent state is persisted to `.worktrees/mastermind-state.json` and agents are recovered on restart. Logs are written to `.worktrees/mastermind.log`.

## Uninstall

**Homebrew:**

```bash
brew uninstall mastermind
```

**Install script / manual:**

```bash
sudo rm /usr/local/bin/mastermind
rm -rf ~/.config/mastermind
```

**Build from source:**

```bash
make uninstall
```

## License

[MIT](LICENSE)
