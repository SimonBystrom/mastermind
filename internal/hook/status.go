package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StatusRunning           = "running"
	StatusWaitingPermission = "waiting_permission"
	StatusWaitingInput      = "waiting_input"
	StatusIdle              = "idle"
	StatusStopped           = "stopped"

	// statusFileName is written by the hook script into the worktree root.
	statusFileName = ".mastermind-status"

	// StalenessThreshold is how old a status file can be before we consider
	// it stale and fall back to tmux polling.
	StalenessThreshold = 30 * time.Second
)

// StatusFile represents the JSON written by the hook script.
type StatusFile struct {
	Status    string `json:"status"`
	Timestamp int64  `json:"ts"`
}

// IsStale returns true if the status file timestamp is older than the threshold.
func (sf *StatusFile) IsStale() bool {
	return time.Since(time.Unix(sf.Timestamp, 0)) > StalenessThreshold
}

// ReadStatus reads and parses the .mastermind-status file from the given worktree path.
// Returns nil, nil if the file does not exist.
func ReadStatus(worktreePath string) (*StatusFile, error) {
	path := filepath.Join(worktreePath, statusFileName)
	return readStatusFile(path)
}

func readStatusFile(path string) (*StatusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read status file: %w", err)
	}

	var sf StatusFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse status file: %w", err)
	}

	return &sf, nil
}
