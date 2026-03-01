package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	todosFileName = ".mastermind-todos"

	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
)

// TodoItem represents a single todo entry from Claude Code's TodoWrite tool.
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending", "in_progress", "completed"
}

// ReadTodos reads and parses the .mastermind-todos file from the given worktree path.
// Returns nil, nil if the file does not exist.
func ReadTodos(worktreePath string) ([]TodoItem, error) {
	path := filepath.Join(worktreePath, todosFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read todos file: %w", err)
	}

	var todos []TodoItem
	if err := json.Unmarshal(data, &todos); err != nil {
		return nil, fmt.Errorf("parse todos file: %w", err)
	}

	return todos, nil
}
