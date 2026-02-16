package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
	"github.com/simonbystrom/mastermind/internal/tmux"
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

	// Ensure .worktrees/ is excluded from git tracking via .git/info/exclude
	if err := ensureGitExclude(absRepo, ".worktrees/"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update .git/info/exclude: %v\n", err)
	}

	// Set up persistent logging
	logPath := filepath.Join(worktreeDir, "mastermind.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})))

	// Log startup info
	tmuxVersion, _ := tmux.CheckVersion()
	slog.Info("mastermind starting", "repo", absRepo, "session", *session, "tmuxVersion", tmuxVersion)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := agent.NewStore()
	orch := orchestrator.New(ctx, store, absRepo, *session, worktreeDir)

	// Recover agents from previous session
	orch.RecoverAgents()

	model := ui.NewApp(orch, store, absRepo, *session)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())

	orch.SetProgram(p)
	go orch.StartMonitor()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Ensure preview branch is cleaned up on exit
	orch.CleanupPreview()

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

func getCurrentPaneID() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current pane id: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("empty pane id")
	}
	return id, nil
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

func ensureGitExclude(repoPath, pattern string) error {
	excludePath := filepath.Join(repoPath, ".git", "info", "exclude")

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	// Read existing content
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if pattern is already present
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}

	// Append the pattern
	f, err := os.OpenFile(excludePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline before pattern if file doesn't end with one
	prefix := ""
	if len(content) > 0 && content[len(content)-1] != '\n' {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(f, "%s%s\n", prefix, pattern)
	return err
}
