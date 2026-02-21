package tmux

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var numberedListRegex = regexp.MustCompile(`^\d+\.\s`)

// completionVerbRegex matches numbered list items that start with a past-tense
// verb (e.g. "1. Fixed ...", "2. Updated ..."), indicating a summary of
// completed work rather than an interactive prompt asking for user input.
var completionVerbRegex = regexp.MustCompile(`(?i)^\d+\.\s+(fixed|added|updated|created|removed|refactored|implemented|changed|moved|renamed|deleted|resolved|configured|installed|upgraded|cleaned|improved|converted|enabled|disabled|replaced|merged|extracted|simplified|optimized|reorganized|wrapped|adjusted|corrected|patched|migrated|set up|handled|ensured|introduced|rewrote|modified|integrated|applied|addressed|extended|standardized|consolidated|split|separated|normalized|aligned|documented)\b`)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-t", paneID, "-p", "#{pane_dead}|#{pane_dead_status}").Output()
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
// option list (at least 2 items like "1. …", "2. …") that looks like an
// interactive prompt. Returns false if the items appear to be a completion
// summary (starting with past-tense verbs like "Fixed", "Updated", etc.).
func detectNumberedList(bottomLines []string) bool {
	var numbered, summaryVerbs int
	for _, line := range bottomLines {
		if numberedListRegex.MatchString(line) {
			numbered++
			if completionVerbRegex.MatchString(line) {
				summaryVerbs++
			}
		}
	}
	if numbered < 2 {
		return false
	}
	// If most items start with past-tense verbs, it's a summary, not a prompt.
	return summaryVerbs < numbered/2
}

// ExtractTeammateName captures the pane content and looks for a @teammate-name
// label rendered by Claude Code. Returns the extracted name or empty string.
func (m *PaneMonitor) ExtractTeammateName(paneID string) string {
	content := capturePane(paneID)
	return ExtractTeammateNameFromContent(content)
}

// ExtractTeammateNameFromContent extracts a @teammate-name label from raw pane
// content text. Returns the name (without @) or empty string if not found.
func ExtractTeammateNameFromContent(content string) string {
	if content == "" {
		return ""
	}
	match := TeammateNamePattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// ParseStatuslineFromContent extracts Claude Code statusline data from raw pane
// content. The statusline format is: [Model Name] XX% ctx | $X.XX | +N -N
func ParseStatuslineFromContent(content string) *StatuslineFromPane {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		match := statuslineRegex.FindStringSubmatch(line)
		if len(match) < 5 {
			continue
		}
		model := match[1]
		ctxPct, _ := strconv.ParseFloat(match[2], 64)
		cost, _ := strconv.ParseFloat(match[3], 64)

		// Parse lines changed: "+N -N"
		linesChanged := match[4]
		var linesAdded, linesRemoved int
		parts := strings.Fields(linesChanged)
		for _, p := range parts {
			if strings.HasPrefix(p, "+") {
				linesAdded, _ = strconv.Atoi(p[1:])
			} else if strings.HasPrefix(p, "-") {
				linesRemoved, _ = strconv.Atoi(p[1:])
			}
		}

		return &StatuslineFromPane{
			Model:        model,
			ContextPct:   ctxPct,
			CostUSD:      cost,
			LinesAdded:   linesAdded,
			LinesRemoved: linesRemoved,
		}
	}
	return nil
}

// StatuslineFromPane holds statusline data parsed from pane content.
type StatuslineFromPane struct {
	Model        string
	ContextPct   float64
	CostUSD      float64
	LinesAdded   int
	LinesRemoved int
}

// statuslineRegex matches Claude Code's statusline: [Model] XX% ctx | $X.XX | +N -N
var statuslineRegex = regexp.MustCompile(`\[([^\]]+)\]\s+(\d+)%\s+ctx\s+\|\s+\$([0-9.]+)\s+\|\s+(\+\d+\s+-\d+)`)

func capturePane(paneID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", paneID, "-p").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func hashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}
