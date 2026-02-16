package tmux

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var numberedListRegex = regexp.MustCompile(`^\d+\.\s`)

// PaneMonitor tracks pane content over time to detect when Claude is waiting.
// If the visible pane content is changing between polls, Claude is working.
// If it's stable, we classify what it's waiting for.
type PaneMonitor struct {
	mu          sync.Mutex
	lastHash    map[string]string // paneID → sha256 of last capture
	stableCount map[string]int    // paneID → number of consecutive polls with same content
	Patterns    MonitorPatterns
}

func NewPaneMonitor() *PaneMonitor {
	return &PaneMonitor{
		lastHash:    make(map[string]string),
		stableCount: make(map[string]int),
		Patterns:    DefaultPatterns,
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
		result := m.detectWaiting(paneID)
		status.WaitingFor = result.waitingFor
		status.HasNumberedList = result.hasNumberedList
	}

	return status, nil
}

// classifyInfo holds the result of pane content classification.
type classifyInfo struct {
	waitingFor      string
	hasNumberedList bool
}

func (m *PaneMonitor) detectWaiting(paneID string) classifyInfo {
	content := capturePane(paneID)
	if content == "" {
		return classifyInfo{}
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

	// Check for high-confidence permission patterns even before content
	// stabilizes — some prompts have subtle animation (cursor, spinner)
	// that prevents the hash from settling.
	if waiting := m.classifyUnstablePane(content); waiting != "" {
		return classifyInfo{waitingFor: waiting}
	}

	// Content is still changing — Claude is actively working
	// Require 2 consecutive stable polls (~4 seconds) before declaring waiting
	if stable < 2 {
		return classifyInfo{}
	}

	// Content is stable — classify what Claude is waiting for
	return m.classifyStablePane(content)
}

// classifyUnstablePane checks for high-confidence patterns that indicate
// a permission prompt even when pane content hasn't stabilized (e.g. due
// to cursor animation). Only returns non-empty for patterns that are
// unambiguous enough to trust without stability confirmation.
func (m *PaneMonitor) classifyUnstablePane(content string) string {
	for _, pattern := range m.Patterns.EarlyPermissionPatterns {
		if strings.Contains(content, pattern) {
			return "permission"
		}
	}
	return ""
}

// classifyStablePane looks at a stable (non-changing) pane and determines
// what kind of waiting state Claude is in.
func (m *PaneMonitor) classifyStablePane(content string) classifyInfo {
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
		return classifyInfo{}
	}

	bottom := strings.Join(bottomLines, "\n")

	// Detect numbered option lists (e.g. AskUserQuestion prompts)
	hasNumberedList := detectNumberedList(bottomLines)

	// --- Still working even though content is stable ---
	for _, indicator := range m.Patterns.WorkingIndicators {
		for _, line := range bottomLines {
			match := true
			if indicator.Contains != "" && !strings.Contains(line, indicator.Contains) {
				match = false
			}
			if indicator.Suffix != "" && !strings.HasSuffix(line, indicator.Suffix) {
				match = false
			}
			if match {
				return classifyInfo{}
			}
		}
	}

	// --- Permission prompts ---
	for _, pattern := range m.Patterns.PermissionPatterns {
		if !strings.Contains(bottom, pattern.Contains) {
			continue
		}
		if pattern.RequiresAlso != "" && !strings.Contains(bottom, pattern.RequiresAlso) {
			continue
		}
		return classifyInfo{waitingFor: "permission", hasNumberedList: hasNumberedList}
	}

	// --- Idle at input prompt ---
	for _, pattern := range m.Patterns.InputPatterns {
		if strings.Contains(bottom, pattern.Contains) {
			return classifyInfo{waitingFor: "input", hasNumberedList: hasNumberedList}
		}
	}

	// Fallback: content is stable and we can't identify the state.
	return classifyInfo{waitingFor: "unknown", hasNumberedList: hasNumberedList}
}

// detectNumberedList checks whether the bottom lines contain a numbered
// option list (at least 2 items like "1. …", "2. …"). This is used to
// distinguish AskUserQuestion prompts from idle input prompts.
func detectNumberedList(bottomLines []string) bool {
	count := 0
	for _, line := range bottomLines {
		if numberedListRegex.MatchString(line) {
			count++
		}
	}
	return count >= 2
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
