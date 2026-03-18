// Package notify provides OS-level notifications for agent attention events.
package notify

import (
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// Notifier sends OS-level notifications when agents need attention.
type Notifier interface {
	Notify(title, message string)
}

// DarwinNotifier sends macOS notifications via osascript with an audible sound.
// It deduplicates rapid-fire notifications within a cooldown window.
type DarwinNotifier struct {
	sound    string
	cooldown time.Duration

	mu       sync.Mutex
	lastSent time.Time
}

// NewDarwin creates a DarwinNotifier with the given system sound name.
// Valid sounds: Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse,
// Ping, Pop, Purr, Sosumi, Submarine, Tink.
func NewDarwin(sound string) *DarwinNotifier {
	if sound == "" {
		sound = "Glass"
	}
	return &DarwinNotifier{
		sound:    sound,
		cooldown: 3 * time.Second,
	}
}

// Notify sends a macOS notification via osascript. It is safe to call from
// any goroutine. Notifications that arrive within the cooldown window after
// the previous notification are silently dropped to avoid spam.
func (d *DarwinNotifier) Notify(title, message string) {
	d.mu.Lock()
	if time.Since(d.lastSent) < d.cooldown {
		d.mu.Unlock()
		return
	}
	d.lastSent = time.Now()
	d.mu.Unlock()

	script := fmt.Sprintf(
		`display notification %q with title %q sound name %q`,
		message, title, d.sound,
	)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Start(); err != nil {
		slog.Error("notification: osascript failed to start", "error", err)
		return
	}
	// Fire-and-forget: reap the process in the background.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Error("notification: osascript failed", "error", err)
		}
	}()
}

// BuildScript returns the osascript AppleScript string that would be executed
// for a given title, message, and sound. Exported for testing.
func BuildScript(title, message, sound string) string {
	return fmt.Sprintf(
		`display notification %q with title %q sound name %q`,
		message, title, sound,
	)
}

// NoopNotifier discards all notifications. Used when notifications are
// disabled or on unsupported platforms.
type NoopNotifier struct{}

// Notify is a no-op.
func (NoopNotifier) Notify(string, string) {}

// New returns a platform-appropriate Notifier. On macOS it returns a
// DarwinNotifier with the given sound; on other platforms it returns a
// NoopNotifier.
func New(enabled bool, sound string) Notifier {
	if !enabled {
		return NoopNotifier{}
	}
	if runtime.GOOS == "darwin" {
		return NewDarwin(sound)
	}
	return NoopNotifier{}
}
