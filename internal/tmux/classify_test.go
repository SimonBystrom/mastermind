package tmux

import (
	"testing"
)

func TestClassifyStablePane(t *testing.T) {
	m := NewPaneMonitor()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "running indicator with suffix",
			content: "\n\n\nRunning npm install…\n",
			want:    "",
		},
		{
			name:    "accept edits permission",
			content: "\n\nSome output\nDo you want to accept edits to file.go?\nYes  No\n",
			want:    "permission",
		},
		{
			name:    "yes and no permission",
			content: "\n\nWould you like to proceed?\nYes  No\n",
			want:    "permission",
		},
		{
			name:    "yes alone is not permission",
			content: "\n\nYes, this is correct\n",
			want:    "unknown",
		},
		{
			name:    "allow and deny permission",
			content: "\n\nAllow this action?\nAllow  Deny\n",
			want:    "permission",
		},
		{
			name:    "allow for permission",
			content: "\n\nDo you want to allow for this?\n",
			want:    "permission",
		},
		{
			name:    "always allow permission",
			content: "\n\nAlways allow this tool?\n",
			want:    "permission",
		},
		{
			name:    "chat about this permission",
			content: "\n\nHere are the options:\nChat about this\n",
			want:    "permission",
		},
		{
			name:    "input prompt with shortcuts",
			content: "\n\nType your message\nfor shortcuts\n",
			want:    "input",
		},
		{
			name:    "random stable content",
			content: "\n\nSome random output that doesn't match anything\nJust sitting here\n",
			want:    "unknown",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "only whitespace",
			content: "   \n  \n   \n",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.classifyStablePane(tt.content)
			if got != tt.want {
				t.Errorf("classifyStablePane() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyUnstablePane(t *testing.T) {
	m := NewPaneMonitor()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "do you want to proceed",
			content: "Some output\nDo you want to proceed?\nMore output\n",
			want:    "permission",
		},
		{
			name:    "esc to cancel",
			content: "Some output\nPress Esc to cancel\n",
			want:    "permission",
		},
		{
			name:    "normal output",
			content: "Building project...\nCompiling files...\n",
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.classifyUnstablePane(tt.content)
			if got != tt.want {
				t.Errorf("classifyUnstablePane() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHashContent(t *testing.T) {
	h1 := hashContent("hello world")
	h2 := hashContent("hello world")
	h3 := hashContent("different content")

	if h1 != h2 {
		t.Errorf("same input produced different hashes: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Error("different inputs produced same hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestPaneMonitor_Remove(t *testing.T) {
	m := NewPaneMonitor()

	// Simulate some internal state
	m.mu.Lock()
	m.lastHash["%0"] = "abc"
	m.stableCount["%0"] = 3
	m.mu.Unlock()

	m.Remove("%0")

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.lastHash["%0"]; ok {
		t.Error("Remove should clear lastHash entry")
	}
	if _, ok := m.stableCount["%0"]; ok {
		t.Error("Remove should clear stableCount entry")
	}
}
