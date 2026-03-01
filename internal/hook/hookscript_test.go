package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteHookFiles(t *testing.T) {
	dir := t.TempDir()

	if err := WriteHookFiles(dir); err != nil {
		t.Fatalf("WriteHookFiles() error: %v", err)
	}

	t.Run("creates status hook script", func(t *testing.T) {
		path := filepath.Join(dir, ".claude", "hooks", "mastermind-status.sh")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("status script not found: %v", err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Error("status script should be executable")
		}
		data, _ := os.ReadFile(path)
		if string(data) != hookScript {
			t.Error("status script content mismatch")
		}
	})

	t.Run("creates todos hook script", func(t *testing.T) {
		path := filepath.Join(dir, ".claude", "hooks", "mastermind-todos.sh")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("todos script not found: %v", err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Error("todos script should be executable")
		}
		data, _ := os.ReadFile(path)
		if string(data) != todosHookScript {
			t.Error("todos script content mismatch")
		}
	})

	t.Run("creates settings.local.json with TodoWrite matcher", func(t *testing.T) {
		path := filepath.Join(dir, ".claude", "settings.local.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("settings.local.json not found: %v", err)
		}

		var settings map[string]interface{}
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatalf("invalid JSON in settings: %v", err)
		}

		// Verify hooks key exists
		hooks, ok := settings["hooks"].(map[string]interface{})
		if !ok {
			t.Fatal("settings missing 'hooks' key")
		}

		// Verify PostToolUse has TodoWrite matcher
		postToolUse, ok := hooks["PostToolUse"].([]interface{})
		if !ok {
			t.Fatal("settings missing 'PostToolUse' hook")
		}

		foundTodoWrite := false
		for _, entry := range postToolUse {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if m["matcher"] == "TodoWrite" {
				foundTodoWrite = true
				break
			}
		}
		if !foundTodoWrite {
			t.Error("PostToolUse missing TodoWrite matcher entry")
		}
	})
}
