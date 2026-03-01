package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTodos(t *testing.T) {
	dir := t.TempDir()

	t.Run("file does not exist", func(t *testing.T) {
		todos, err := ReadTodos(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if todos != nil {
			t.Fatal("expected nil todos for missing file")
		}
	})

	t.Run("valid todos file", func(t *testing.T) {
		items := []TodoItem{
			{ID: "1", Content: "Write tests", Status: TodoPending},
			{ID: "2", Content: "Fix bug", Status: TodoCompleted},
			{ID: "3", Content: "Refactor code", Status: TodoInProgress},
		}
		data, _ := json.Marshal(items)
		if err := os.WriteFile(filepath.Join(dir, todosFileName), data, 0o644); err != nil {
			t.Fatal(err)
		}

		todos, err := ReadTodos(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 3 {
			t.Fatalf("expected 3 todos, got %d", len(todos))
		}
		if todos[0].ID != "1" || todos[0].Content != "Write tests" || todos[0].Status != TodoPending {
			t.Errorf("todo[0] = %+v, unexpected", todos[0])
		}
		if todos[1].Status != TodoCompleted {
			t.Errorf("todo[1].Status = %q, want %q", todos[1].Status, TodoCompleted)
		}
		if todos[2].Status != TodoInProgress {
			t.Errorf("todo[2].Status = %q, want %q", todos[2].Status, TodoInProgress)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, todosFileName), []byte("[]"), 0o644); err != nil {
			t.Fatal(err)
		}

		todos, err := ReadTodos(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(todos) != 0 {
			t.Fatalf("expected 0 todos, got %d", len(todos))
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, todosFileName), []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		todos, err := ReadTodos(dir)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if todos != nil {
			t.Fatal("expected nil todos for invalid JSON")
		}
	})
}
