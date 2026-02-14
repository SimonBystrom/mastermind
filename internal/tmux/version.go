package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CheckVersion returns the tmux version string and warns if < 3.0.
func CheckVersion() (string, error) {
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		return "", fmt.Errorf("get tmux version: %w", err)
	}

	version := strings.TrimSpace(string(out))
	// tmux outputs something like "tmux 3.4"
	parts := strings.Fields(version)
	if len(parts) < 2 {
		return version, nil
	}

	numStr := parts[1]
	// Strip trailing non-numeric suffixes like "a" in "3.3a"
	numStr = strings.TrimRight(numStr, "abcdefghijklmnopqrstuvwxyz")
	major, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return version, nil
	}

	if major < 3.0 {
		return version, fmt.Errorf("tmux version %s is below 3.0; some features may not work correctly", version)
	}

	return version, nil
}

// PaneExists returns true if the given tmux pane ID still exists.
func PaneExists(paneID string) bool {
	err := exec.Command("tmux", "display-message", "-t", paneID, "-p", "").Run()
	return err == nil
}
