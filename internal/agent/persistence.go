package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// PersistedAgent is the JSON-serializable representation of an Agent.
type PersistedAgent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Branch       string    `json:"branch"`
	BaseBranch   string    `json:"base_branch"`
	WorktreePath string    `json:"worktree_path"`
	TmuxWindow   string    `json:"tmux_window"`
	TmuxPaneID   string    `json:"tmux_pane_id"`
	Status       Status    `json:"status"`
	WaitingFor   string    `json:"waiting_for"`
	EverActive   bool      `json:"ever_active"`
	ExitCode     int       `json:"exit_code"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
}

// SaveState atomically writes agent state to a JSON file.
func SaveState(path string, agents []*Agent) error {
	persisted := make([]PersistedAgent, len(agents))
	for i, a := range agents {
		persisted[i] = PersistedAgent{
			ID:           a.ID,
			Name:         a.Name,
			Branch:       a.Branch,
			BaseBranch:   a.BaseBranch,
			WorktreePath: a.WorktreePath,
			TmuxWindow:   a.TmuxWindow,
			TmuxPaneID:   a.TmuxPaneID,
			Status:       a.GetStatus(),
			WaitingFor:   a.GetWaitingFor(),
			EverActive:   a.GetEverActive(),
			ExitCode:     a.GetExitCode(),
			StartedAt:    a.StartedAt,
			FinishedAt:   a.GetFinishedAt(),
		}
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write state temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename state file: %w", err)
	}

	return nil
}

// LoadState reads persisted agent state from a JSON file.
// Returns nil, nil if the file does not exist.
func LoadState(path string) ([]PersistedAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var agents []PersistedAgent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return agents, nil
}
