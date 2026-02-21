package tmux

import (
	"fmt"
	"log/slog"
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
	if err := exec.Command("tmux", "set-option", "-t", paneID, "remain-on-exit", "on").Run(); err != nil {
		slog.Warn("failed to set remain-on-exit on pane", "pane", paneID, "error", err)
	}

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
	if err := exec.Command("tmux", "kill-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("kill tmux window %s: %w", target, err)
	}
	return nil
}

func SendKeys(paneID string, keys ...string) error {
	args := append([]string{"send-keys", "-t", paneID}, keys...)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		return fmt.Errorf("send keys to pane %s: %w", paneID, err)
	}
	return nil
}

func KillPane(paneID string) error {
	if err := exec.Command("tmux", "kill-pane", "-t", paneID).Run(); err != nil {
		return fmt.Errorf("kill tmux pane %s: %w", paneID, err)
	}
	return nil
}

func SelectWindow(target string) error {
	if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("select tmux window %s: %w", target, err)
	}
	return nil
}

func SelectPane(paneID string) error {
	if err := exec.Command("tmux", "select-pane", "-t", paneID).Run(); err != nil {
		return fmt.Errorf("select tmux pane %s: %w", paneID, err)
	}
	return nil
}

// PaneExistsInWindow returns true if the given pane ID exists inside the given window.
// This is more robust than checking pane/window separately since tmux reuses IDs.
func PaneExistsInWindow(paneID, windowID string) bool {
	out, err := exec.Command("tmux", "list-panes", "-t", windowID, "-F", "#{pane_id}").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == paneID {
			return true
		}
	}
	return false
}

// WindowIDForPane returns the window ID that contains the given pane.
func WindowIDForPane(paneID string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{window_id}").Output()
	if err != nil {
		return "", fmt.Errorf("get window id for pane %s: %w", paneID, err)
	}
	return strings.TrimSpace(string(out)), nil
}
