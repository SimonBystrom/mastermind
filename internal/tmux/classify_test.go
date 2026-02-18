package tmux

import (
	"testing"
)

func TestClassifyStablePane(t *testing.T) {
	m := NewPaneMonitor()

	tests := []struct {
		name            string
		content         string
		wantWaitingFor  string
		wantNumberedList bool
	}{
		{
			name:           "running indicator with suffix",
			content:        "\n\n\nRunning npm install…\n",
			wantWaitingFor: "",
		},
		{
			name:           "accept edits with yes/no permission",
			content:        "\n\nSome output\nDo you want to accept edits to file.go?\nYes  No\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "accept edits banner is not permission",
			content:        "\n\nBuild passes cleanly.\n✻ Brewed for 3m 0s\n❯ commit this\n>> accept edits on (shift+tab to cycle) · 5 files +50 -76\n",
			wantWaitingFor: "unknown",
		},
		{
			name:           "yes and no permission",
			content:        "\n\nWould you like to proceed?\nYes  No\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "yes alone is not permission",
			content:        "\n\nYes, this is correct\n",
			wantWaitingFor: "unknown",
		},
		{
			name:           "allow and deny permission",
			content:        "\n\nAllow this action?\nAllow  Deny\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "allow for permission",
			content:        "\n\nDo you want to allow for this?\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "always allow permission",
			content:        "\n\nAlways allow this tool?\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "chat about this permission",
			content:        "\n\nHere are the options:\nChat about this\n",
			wantWaitingFor: "permission",
		},
		{
			name:           "input prompt with shortcuts",
			content:        "\n\nType your message\nfor shortcuts\n",
			wantWaitingFor: "input",
		},
		{
			name:           "random stable content",
			content:        "\n\nSome random output that doesn't match anything\nJust sitting here\n",
			wantWaitingFor: "unknown",
		},
		{
			name:           "empty content",
			content:        "",
			wantWaitingFor: "",
		},
		{
			name:           "only whitespace",
			content:        "   \n  \n   \n",
			wantWaitingFor: "",
		},
		{
			name:            "numbered list detected",
			content:         "\n\nWhich approach?\n1. Option A\n2. Option B\n3. Option C\nChat about this\n",
			wantWaitingFor:  "permission",
			wantNumberedList: true,
		},
		{
			name:            "numbered list at input prompt",
			content:         "\n\n1. First item\n2. Second item\nfor shortcuts\n",
			wantWaitingFor:  "input",
			wantNumberedList: true,
		},
		{
			name:            "single numbered item is not a list",
			content:         "\n\n1. Only one item\nfor shortcuts\n",
			wantWaitingFor:  "input",
			wantNumberedList: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.classifyStablePane(tt.content)
			if got.waitingFor != tt.wantWaitingFor {
				t.Errorf("classifyStablePane().waitingFor = %q, want %q", got.waitingFor, tt.wantWaitingFor)
			}
			if got.hasNumberedList != tt.wantNumberedList {
				t.Errorf("classifyStablePane().hasNumberedList = %v, want %v", got.hasNumberedList, tt.wantNumberedList)
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
