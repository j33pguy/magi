package tools

import (
	"testing"
)

func TestContentHash(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "deterministic",
			content: "hello world",
			want:    contentHash("hello world"),
		},
		{
			name:    "trims whitespace",
			content: "  hello world  ",
			want:    contentHash("hello world"),
		},
		{
			name:    "different content different hash",
			content: "goodbye world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentHash(tt.content)
			if len(got) != 16 {
				t.Errorf("hash length = %d, want 16", len(got))
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("hash = %q, want %q", got, tt.want)
			}
		})
	}

	// Verify different inputs produce different hashes
	h1 := contentHash("hello world")
	h2 := contentHash("goodbye world")
	if h1 == h2 {
		t.Errorf("different content produced same hash: %q", h1)
	}
}

func TestContentHashDedup(t *testing.T) {
	// Same content indexed twice should produce identical hash
	content := "I configured the compute cluster with 3 nodes"
	h1 := contentHash(content)
	h2 := contentHash(content)
	if h1 != h2 {
		t.Errorf("same content produced different hashes: %q vs %q", h1, h2)
	}
}

func TestIndexTurnToolDefinition(t *testing.T) {
	tool := (&IndexTurn{}).Tool()
	if tool.Name != "index_turn" {
		t.Errorf("tool name = %q, want %q", tool.Name, "index_turn")
	}
}

func TestIndexSessionToolDefinition(t *testing.T) {
	tool := (&IndexSession{}).Tool()
	if tool.Name != "index_session" {
		t.Errorf("tool name = %q, want %q", tool.Name, "index_session")
	}
}
