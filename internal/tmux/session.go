package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

func CurrentSession() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current session: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func SessionExists(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", name).Run()
	return err == nil
}

func CreateSession(name string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name)
	return cmd.Run()
}

func AttachSession(name string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func RenameWindow(target, name string) error {
	return exec.Command("tmux", "rename-window", "-t", target, name).Run()
}
