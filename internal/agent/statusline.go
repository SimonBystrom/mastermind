package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// StatuslineData holds parsed fields from Claude Code's statusline JSON.
type StatuslineData struct {
	Model          string
	CostUSD        float64
	ContextPct     float64
	LinesAdded     int
	LinesRemoved   int
	DurationMs     int64
	SessionID      string
}

// statuslineJSON mirrors the nested structure of Claude Code's statusline output.
type statuslineJSON struct {
	SessionID     string `json:"session_id"`
	Model         struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD      float64 `json:"total_cost_usd"`
		TotalDurationMs   int64   `json:"total_duration_ms"`
		TotalLinesAdded   int     `json:"total_lines_added"`
		TotalLinesRemoved int     `json:"total_lines_removed"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage float64 `json:"used_percentage"`
	} `json:"context_window"`
}

// ReadStatuslineFile reads and parses the .claude-status.json sidecar file
// from the given worktree path.
func ReadStatuslineFile(worktreePath string) (*StatuslineData, error) {
	data, err := os.ReadFile(filepath.Join(worktreePath, ".claude-status.json"))
	if err != nil {
		return nil, err
	}

	var raw statuslineJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return &StatuslineData{
		Model:        raw.Model.DisplayName,
		CostUSD:      raw.Cost.TotalCostUSD,
		ContextPct:   raw.ContextWindow.UsedPercentage,
		LinesAdded:   raw.Cost.TotalLinesAdded,
		LinesRemoved: raw.Cost.TotalLinesRemoved,
		DurationMs:   raw.Cost.TotalDurationMs,
		SessionID:    raw.SessionID,
	}, nil
}
