package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

func NewWindow(session, name, dir string, command []string) (string, error) {
	args := []string{
		"new-window",
		"-t", session + ":",
		"-n", name,
		"-c", dir,
		"-e", "CLAUDECODE=",
		"-e", "CLAUDE_CODE_ENTRYPOINT=",
		"-P", "-F", "#{pane_id}",
	}
	args = append(args, command...)

	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create tmux window: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	paneID := strings.TrimSpace(string(out))

	// Set remain-on-exit so we can detect when the process exits
	exec.Command("tmux", "set-option", "-t", paneID, "remain-on-exit", "on").Run()

	return paneID, nil
}

func SplitWindow(paneID, dir string, horizontal bool, sizePercent int, command []string) (string, error) {
	args := []string{
		"split-window",
		"-t", paneID,
		"-c", dir,
		"-P", "-F", "#{pane_id}",
	}
	if horizontal {
		args = append(args, "-h")
	}
	if sizePercent > 0 {
		args = append(args, "-l", fmt.Sprintf("%d%%", sizePercent))
	}
	args = append(args, command...)

	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to split pane: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func KillWindow(target string) error {
	return exec.Command("tmux", "kill-window", "-t", target).Run()
}

func KillPane(paneID string) error {
	return exec.Command("tmux", "kill-pane", "-t", paneID).Run()
}

func SelectWindow(target string) error {
	return exec.Command("tmux", "select-window", "-t", target).Run()
}

func SelectPane(paneID string) error {
	return exec.Command("tmux", "select-pane", "-t", paneID).Run()
}

// WindowIDForPane returns the window ID that contains the given pane.
func WindowIDForPane(paneID string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{window_id}").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get window id for pane: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
