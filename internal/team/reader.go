package team

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// TeamReader reads agent team data from the Claude Code teams directory.
type TeamReader interface {
	FindTeamForSession(sessionID string) (*TeamInfo, error)
}

// RealTeamReader scans ~/.claude/teams/ and ~/.claude/tasks/ on disk.
type RealTeamReader struct {
	// teamsDir overrides the default ~/.claude/teams/ path (for testing).
	teamsDir string
	// tasksDir overrides the default ~/.claude/tasks/ path (for testing).
	tasksDir string
}

// NewReader creates a RealTeamReader using the default Claude data directories.
func NewReader() *RealTeamReader {
	home, _ := os.UserHomeDir()
	return &RealTeamReader{
		teamsDir: filepath.Join(home, ".claude", "teams"),
		tasksDir: filepath.Join(home, ".claude", "tasks"),
	}
}

// NewReaderWithDirs creates a RealTeamReader with custom directories (for testing).
func NewReaderWithDirs(teamsDir, tasksDir string) *RealTeamReader {
	return &RealTeamReader{
		teamsDir: teamsDir,
		tasksDir: tasksDir,
	}
}

// FindTeamForSession scans all teams to find one where a "lead" member's
// agent_id matches the given session ID. Returns nil (not an error) if
// no matching team is found.
func (r *RealTeamReader) FindTeamForSession(sessionID string) (*TeamInfo, error) {
	entries, err := os.ReadDir(r.teamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		teamName := entry.Name()
		configPath := filepath.Join(r.teamsDir, teamName, "config.json")

		data, err := os.ReadFile(configPath)
		if err != nil {
			slog.Debug("team config read error", "team", teamName, "error", err)
			continue
		}

		var cfg TeamConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			slog.Debug("team config parse error", "team", teamName, "error", err)
			continue
		}

		if !hasLeadWithSession(cfg.Members, sessionID) {
			continue
		}

		// Found matching team — read tasks
		tasks := r.readTasks(teamName)

		info := &TeamInfo{
			TeamName:    teamName,
			MemberCount: len(cfg.Members),
			Members:     cfg.Members,
			Tasks:       tasks,
			TotalTasks:  len(tasks),
		}
		for _, t := range tasks {
			switch t.Status {
			case TaskCompleted:
				info.CompletedTasks++
			case TaskInProgress:
				info.InProgressTasks++
			case TaskPending:
				info.PendingTasks++
			}
		}

		return info, nil
	}

	return nil, nil
}

func hasLeadWithSession(members []Member, sessionID string) bool {
	for _, m := range members {
		if m.AgentType == "lead" && m.AgentID == sessionID {
			return true
		}
	}
	return false
}

func (r *RealTeamReader) readTasks(teamName string) []Task {
	tasksDir := filepath.Join(r.tasksDir, teamName)
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("tasks dir read error", "team", teamName, "error", err)
		}
		return nil
	}

	var tasks []Task
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(tasksDir, entry.Name()))
		if err != nil {
			slog.Debug("task file read error", "file", entry.Name(), "error", err)
			continue
		}

		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			slog.Debug("task file parse error", "file", entry.Name(), "error", err)
			continue
		}
		tasks = append(tasks, t)
	}

	return tasks
}
