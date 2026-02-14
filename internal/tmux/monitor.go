package tmux

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type PaneStatus struct {
	Dead       bool
	ExitCode   int
	WaitingFor string // "permission", "input", or "" (working)
}

// PaneMonitor tracks pane content over time to detect when Claude is waiting.
// If the visible pane content is changing between polls, Claude is working.
// If it's stable, we classify what it's waiting for.
type PaneMonitor struct {
	mu          sync.Mutex
	lastHash    map[string]string // paneID → sha256 of last capture
	stableCount map[string]int    // paneID → number of consecutive polls with same content
}

func NewPaneMonitor() *PaneMonitor {
	return &PaneMonitor{
		lastHash:    make(map[string]string),
		stableCount: make(map[string]int),
	}
}

func (m *PaneMonitor) Remove(paneID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.lastHash, paneID)
	delete(m.stableCount, paneID)
}

func (m *PaneMonitor) GetPaneStatus(paneID string) (PaneStatus, error) {
	out, err := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{pane_dead}|#{pane_dead_status}").Output()
	if err != nil {
		return PaneStatus{}, fmt.Errorf("failed to get pane status for %s: %w", paneID, err)
	}

	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) != 2 {
		return PaneStatus{}, fmt.Errorf("unexpected pane status format: %s", string(out))
	}

	dead := parts[0] == "1"
	exitCode := 0
	if dead && parts[1] != "" {
		exitCode, _ = strconv.Atoi(parts[1])
	}

	status := PaneStatus{Dead: dead, ExitCode: exitCode}

	if !dead {
		status.WaitingFor = m.detectWaiting(paneID)
	}

	return status, nil
}

func (m *PaneMonitor) detectWaiting(paneID string) string {
	content := capturePane(paneID)
	if content == "" {
		return ""
	}

	// Hash the content and compare with previous capture
	hash := hashContent(content)

	m.mu.Lock()
	prev, hasPrev := m.lastHash[paneID]
	m.lastHash[paneID] = hash
	if hasPrev && prev == hash {
		m.stableCount[paneID]++
	} else {
		m.stableCount[paneID] = 0
	}
	stable := m.stableCount[paneID]
	m.mu.Unlock()

	// Content is still changing — Claude is actively working
	// Require 2 consecutive stable polls (~4 seconds) before declaring waiting
	if stable < 2 {
		return ""
	}

	// Content is stable — classify what Claude is waiting for
	return classifyStablePane(content)
}

// classifyStablePane looks at a stable (non-changing) pane and determines
// what kind of waiting state Claude is in.
func classifyStablePane(content string) string {
	lines := strings.Split(content, "\n")

	// Collect non-empty lines from the bottom (status area)
	var bottomLines []string
	for i := len(lines) - 1; i >= 0 && len(bottomLines) < 20; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			bottomLines = append(bottomLines, trimmed)
		}
	}

	if len(bottomLines) == 0 {
		return ""
	}

	bottom := strings.Join(bottomLines, "\n")

	// --- Still working even though content is stable ---

	// Tool execution that takes a while (e.g. long Bash command)
	// Shows "Running…" in the tool output area
	for _, line := range bottomLines {
		if strings.HasSuffix(line, "…") && strings.Contains(line, "Running") {
			return ""
		}
	}

	// --- Permission prompts ---

	// Edit acceptance: "accept edits on (shift+tab to cycle)"
	if strings.Contains(bottom, "accept edits") {
		return "permission"
	}

	// Tool permission: Yes/No selector
	if strings.Contains(bottom, "Yes") && strings.Contains(bottom, "No") {
		return "permission"
	}

	// Tool permission: Allow/Deny selector
	if strings.Contains(bottom, "Allow") && strings.Contains(bottom, "Deny") {
		return "permission"
	}

	// "always allow" / "allow for this project" type prompts
	if strings.Contains(bottom, "allow for") || strings.Contains(bottom, "Always allow") {
		return "permission"
	}

	// --- Idle at input prompt ---

	// Claude is at the "❯" prompt waiting for user's next message
	if strings.Contains(bottom, "for shortcuts") {
		return "input"
	}

	// Fallback: content is stable and no working indicators found.
	// This catches any permission/question UI we haven't explicitly matched.
	return "input"
}

func capturePane(paneID string) string {
	out, err := exec.Command("tmux", "capture-pane", "-t", paneID, "-p").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func hashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}
