package notify

import (
	"strings"
	"testing"
	"time"
)

func TestBuildScript(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		message string
		sound   string
		wantSub []string // substrings that must appear
	}{
		{
			name:    "basic",
			title:   "Mastermind",
			message: "Agent alpha finished",
			sound:   "Glass",
			wantSub: []string{
				"display notification",
				"Agent alpha finished",
				"Mastermind",
				"Glass",
			},
		},
		{
			name:    "special characters in message",
			title:   "Mastermind",
			message: `Agent "beta" needs permission`,
			sound:   "Ping",
			wantSub: []string{
				"display notification",
				"Ping",
			},
		},
		{
			name:    "custom sound",
			title:   "Test",
			message: "Hello",
			sound:   "Tink",
			wantSub: []string{
				`sound name "Tink"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildScript(tt.title, tt.message, tt.sound)
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("BuildScript() = %q, want substring %q", got, sub)
				}
			}
		})
	}
}

func TestNewDarwinDefaults(t *testing.T) {
	d := NewDarwin("")
	if d.sound != "Glass" {
		t.Errorf("expected default sound Glass, got %q", d.sound)
	}
	if d.cooldown != 3*time.Second {
		t.Errorf("expected 3s cooldown, got %v", d.cooldown)
	}
}

func TestNewDarwinCustomSound(t *testing.T) {
	d := NewDarwin("Ping")
	if d.sound != "Ping" {
		t.Errorf("expected sound Ping, got %q", d.sound)
	}
}

func TestDarwinNotifierCooldown(t *testing.T) {
	d := NewDarwin("Glass")
	d.cooldown = 100 * time.Millisecond

	// Manually set lastSent to now so the next call is within cooldown.
	d.mu.Lock()
	d.lastSent = time.Now()
	d.mu.Unlock()

	// This should be silently dropped (within cooldown).
	// We cannot easily verify osascript was NOT called without more
	// complex test infrastructure, but we verify it doesn't panic.
	d.Notify("Test", "should be dropped")

	// After cooldown expires, the next call should succeed.
	time.Sleep(150 * time.Millisecond)
	// The call will try to run osascript which may not exist in CI,
	// but the function should not panic.
	d.Notify("Test", "should succeed")
}

func TestNoopNotifier(t *testing.T) {
	n := NoopNotifier{}
	// Should not panic.
	n.Notify("title", "message")
}

func TestNewReturnsNoopWhenDisabled(t *testing.T) {
	n := New(false, "Glass")
	if _, ok := n.(NoopNotifier); !ok {
		t.Errorf("expected NoopNotifier when disabled, got %T", n)
	}
}

func TestNewReturnsDarwinOnDarwin(t *testing.T) {
	n := New(true, "Glass")
	// On macOS this should be *DarwinNotifier, on other platforms NoopNotifier.
	// We test the type based on what platform we're on.
	switch n.(type) {
	case *DarwinNotifier, NoopNotifier:
		// both are acceptable depending on platform
	default:
		t.Errorf("unexpected notifier type %T", n)
	}
}
