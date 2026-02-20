package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/simonbystrom/mastermind/internal/agent"
	"github.com/simonbystrom/mastermind/internal/config"
	"github.com/simonbystrom/mastermind/internal/orchestrator"
	"github.com/simonbystrom/mastermind/internal/tmux"
	"github.com/simonbystrom/mastermind/internal/ui"
)

var version = "dev"

func main() {
	repo := flag.String("repo", "", "path to git repository (defaults to current directory)")
	session := flag.String("session", "", "tmux session name (defaults to current session)")
	showVersion := flag.Bool("version", false, "print version and exit")
	initConfig := flag.Bool("init-config", false, "write default config file and print its path")
	flag.Parse()

	if *showVersion {
		fmt.Println("mastermind " + version)
		os.Exit(0)
	}

	if *initConfig {
		path := config.Path()
		if err := config.WriteDefault(path); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(path)
		os.Exit(0)
	}

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

	// Load user configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Write default config file if it doesn't exist
	if err := config.WriteDefault(config.Path()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write default config: %v\n", err)
	}

	// Install the statusline script for Claude Code integration
	if err := config.WriteStatuslineScript(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write statusline script: %v\n", err)
	}

	worktreeDir := filepath.Join(absRepo, ".worktrees")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating worktree directory: %v\n", err)
		os.Exit(1)
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
	orch := orchestrator.New(ctx, store, absRepo, *session, worktreeDir,
		orchestrator.WithLazygitSplit(cfg.Layout.LazygitSplit),
	)

	// Recover agents from previous session
	orch.RecoverAgents()

	// Clean up any stale preview left over from a previous session that
	// exited abnormally (e.g. SIGKILL, crash, tmux pane closed).
	orch.CleanupPreview()
	orch.ResetPreviewCleanup()

	model := ui.NewApp(cfg, orch, store, absRepo, *session)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithReportFocus())

	orch.SetProgram(p)
	go orch.StartMonitor()

	// Handle SIGTERM/SIGHUP so preview cleanup runs even when the
	// process is killed outside of the TUI (e.g. tmux session closed).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		orch.CleanupPreview()
		p.Kill()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Ensure preview branch is cleaned up on exit
	orch.CleanupPreview()

}

func validateDependencies() error {
	deps := []string{"tmux", "git", "claude", "lazygit", "jq"}
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

