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
		{
			name:            "completion summary is not a prompt",
			content:         "\n\n1. Fixed the authentication bug\n2. Updated the test cases\n3. Refactored error handling\nfor shortcuts\n",
			wantWaitingFor:  "input",
			wantNumberedList: false,
		},
		{
			name:            "completion summary with various verbs",
			content:         "\n\n1. Added new validation logic\n2. Removed deprecated imports\n3. Cleaned up unused variables\n4. Implemented retry mechanism\nfor shortcuts\n",
			wantWaitingFor:  "input",
			wantNumberedList: false,
		},
		{
			name:            "mixed list with mostly summary verbs",
			content:         "\n\n1. Fixed the bug\n2. Updated tests\n3. Check the output\nfor shortcuts\n",
			wantWaitingFor:  "input",
			wantNumberedList: false,
		},
		{
			name:            "actual prompt options not filtered",
			content:         "\n\nWhich approach?\n1. Use Redis caching\n2. Use in-memory cache\n3. Use file-based cache\nChat about this\n",
			wantWaitingFor:  "permission",
			wantNumberedList: true,
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


func TestParseStatuslineFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    *StatuslineFromPane
	}{
		{
			name:    "full statusline with git branch",
			content: "Some output\n➜  myproject git:(main) [ctx: 46%] +10 -5 $0.7300  claude-sonnet-4-6\n",
			want: &StatuslineFromPane{
				Model:        "claude-sonnet-4-6",
				ContextPct:   46,
				CostUSD:      0.73,
				LinesAdded:   10,
				LinesRemoved: 5,
			},
		},
		{
			name:    "statusline without diff stats",
			content: "Output\n➜  myproject git:(feature) [ctx: 12%] $1.5000  claude-opus-4-6\n",
			want: &StatuslineFromPane{
				Model:        "claude-opus-4-6",
				ContextPct:   12,
				CostUSD:      1.50,
				LinesAdded:   0,
				LinesRemoved: 0,
			},
		},
		{
			name:    "statusline without git branch",
			content: "Output\n➜  myproject [ctx: 30%] +5 -2 $0.2500  claude-opus-4-6\n",
			want: &StatuslineFromPane{
				Model:        "claude-opus-4-6",
				ContextPct:   30,
				CostUSD:      0.25,
				LinesAdded:   5,
				LinesRemoved: 2,
			},
		},
		{
			name:    "no statusline",
			content: "Regular output\nNo statusline here\n",
			want:    nil,
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
		{
			name:    "statusline among other content",
			content: "Building project...\nCompiling files...\n➜  app git:(dev) [ctx: 88%] +100 -50 $0.0500  claude-haiku-4-5\nfor shortcuts\n",
			want: &StatuslineFromPane{
				Model:        "claude-haiku-4-5",
				ContextPct:   88,
				CostUSD:      0.05,
				LinesAdded:   100,
				LinesRemoved: 50,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStatuslineFromContent(tt.content)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.want.Model)
			}
			if got.ContextPct != tt.want.ContextPct {
				t.Errorf("ContextPct = %v, want %v", got.ContextPct, tt.want.ContextPct)
			}
			if got.CostUSD != tt.want.CostUSD {
				t.Errorf("CostUSD = %v, want %v", got.CostUSD, tt.want.CostUSD)
			}
			if got.LinesAdded != tt.want.LinesAdded {
				t.Errorf("LinesAdded = %d, want %d", got.LinesAdded, tt.want.LinesAdded)
			}
			if got.LinesRemoved != tt.want.LinesRemoved {
				t.Errorf("LinesRemoved = %d, want %d", got.LinesRemoved, tt.want.LinesRemoved)
			}
		})
	}
}

func TestPaneMonitor_Remove(t *testing.T) {
	m := NewPaneMonitor()

	// Simulate some internal state
	m.mu.Lock()
	m.lastContent["%0"] = []byte("abc")
	m.stableCount["%0"] = 3
	m.mu.Unlock()

	m.Remove("%0")

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.lastContent["%0"]; ok {
		t.Error("Remove should clear lastContent entry")
	}
	if _, ok := m.stableCount["%0"]; ok {
		t.Error("Remove should clear stableCount entry")
	}
}
