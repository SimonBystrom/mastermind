package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadStatus(t *testing.T) {
	dir := t.TempDir()

	t.Run("file does not exist", func(t *testing.T) {
		sf, err := ReadStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sf != nil {
			t.Fatal("expected nil status for missing file")
		}
	})

	t.Run("valid status file", func(t *testing.T) {
		ts := time.Now().Unix()
		data, _ := json.Marshal(StatusFile{Status: StatusRunning, Timestamp: ts})
		if err := os.WriteFile(filepath.Join(dir, statusFileName), data, 0o644); err != nil {
			t.Fatal(err)
		}

		sf, err := ReadStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sf == nil {
			t.Fatal("expected non-nil status")
		}
		if sf.Status != StatusRunning {
			t.Errorf("got status %q, want %q", sf.Status, StatusRunning)
		}
		if sf.Timestamp != ts {
			t.Errorf("got ts %d, want %d", sf.Timestamp, ts)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, statusFileName), []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		sf, err := ReadStatus(dir)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if sf != nil {
			t.Fatal("expected nil status for invalid JSON")
		}
	})
}

func TestStatusFile_IsStale(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		sf := &StatusFile{Status: StatusRunning, Timestamp: time.Now().Unix()}
		if sf.IsStale() {
			t.Error("expected fresh status to not be stale")
		}
	})

	t.Run("stale", func(t *testing.T) {
		sf := &StatusFile{Status: StatusRunning, Timestamp: time.Now().Add(-60 * time.Second).Unix()}
		if !sf.IsStale() {
			t.Error("expected old status to be stale")
		}
	})
}
