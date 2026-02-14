package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
	"github.com/simonbystrom/mastermind/internal/ui"
)

func main() {
	repo := flag.String("repo", "", "path to git repository (defaults to current directory)")
	session := flag.String("session", "", "tmux session name (defaults to current session)")
	flag.Parse()

	if *repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		*repo = cwd
	}

	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error resolving repo path: %v\n", err)
		os.Exit(1)
	}

	if err := validateDependencies(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := validateGitRepo(absRepo); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Auto-detect current tmux session if not specified
	if *session == "" {
		if os.Getenv("TMUX") == "" {
			fmt.Fprintf(os.Stderr, "error: not inside a tmux session (run inside tmux or pass --session)\n")
			os.Exit(1)
		}
		detected, err := detectTmuxSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error detecting tmux session: %v\n", err)
			os.Exit(1)
		}
		*session = detected
	}

	worktreeDir := filepath.Join(absRepo, ".worktrees")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating worktree directory: %v\n", err)
		os.Exit(1)
	}

	store := agent.NewStore()
	orch := orchestrator.New(store, absRepo, *session, worktreeDir)

	model := ui.NewApp(orch, store, absRepo, *session)
	p := tea.NewProgram(model, tea.WithAltScreen())

	orch.SetProgram(p)
	go orch.StartMonitor()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func validateDependencies() error {
	deps := []string{"tmux", "git", "claude", "lazygit"}
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			return fmt.Errorf("%s not found on PATH", dep)
		}
	}
	return nil
}

func validateGitRepo(path string) error {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s is not a git repository", path)
	}
	return nil
}

func detectTmuxSession() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to detect tmux session: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("empty session name")
	}
	return name, nil
}
