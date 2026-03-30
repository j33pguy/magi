package ingest

import (
	"testing"
)

// ============================================================
// isGrokFormat — uncovered branches
// ============================================================

func TestIsGrokFormat_InvalidFirstElement(t *testing.T) {
	// Array with an element that cannot be unmarshalled into the probe struct
	// (valid JSON array but first element is a string, not object)
	if isGrokFormat([]byte(`["not an object"]`)) {
		t.Error("expected false when first element is not an object")
	}
}

func TestIsGrokFormat_MissingTurns(t *testing.T) {
	// Has conversation_id but no turns
	if isGrokFormat([]byte(`[{"conversation_id":"abc"}]`)) {
		t.Error("expected false when turns are empty")
	}
}

func TestIsGrokFormat_EmptyRole(t *testing.T) {
	// Has conversation_id and turns but turn author role is empty
	data := []byte(`[{"conversation_id":"abc","turns":[{"author":{"role":""},"content":{}}]}]`)
	if isGrokFormat(data) {
		t.Error("expected false when turn role is empty")
	}
}

func TestIsGrokFormat_MissingConversationID(t *testing.T) {
	// Has turns with role but no conversation_id
	data := []byte(`[{"turns":[{"author":{"role":"user"},"content":{}}]}]`)
	if isGrokFormat(data) {
		t.Error("expected false when conversation_id is empty")
	}
}

// ============================================================
// parseGrok — uncovered branches
// ============================================================

func TestParseGrok_InvalidJSON(t *testing.T) {
	// Data that passes Detect as grok-like but fails full unmarshal.
	// Force by calling parseGrok directly with broken JSON.
	_, err := parseGrok([]byte(`[{"conversation_id": broken`))
	if err == nil {
		t.Error("expected error for malformed Grok JSON")
	}
}

func TestParseGrok_EmptyConversations(t *testing.T) {
	conv, err := parseGrok([]byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conv.Turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(conv.Turns))
	}
}

func TestParseGrok_AllEmptyParts(t *testing.T) {
	// All turns have empty content — all should be skipped
	data := []byte(`[{
		"conversation_id": "c1",
		"title": "",
		"turns": [
			{"author": {"role": "user"}, "content": {"content_type": "text", "parts": [""]}},
			{"author": {"role": "assistant"}, "content": {"content_type": "text", "parts": ["  "]}}
		]
	}]`)
	conv, err := parseGrok(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conv.Turns) != 0 {
		t.Errorf("expected 0 turns (all empty), got %d", len(conv.Turns))
	}
}

// ============================================================
// isChatGPTFormat — uncovered branches
// ============================================================

func TestIsChatGPTFormat_InvalidFirstElement(t *testing.T) {
	// Valid JSON array but first element is not an object
	if isChatGPTFormat([]byte(`[42]`)) {
		t.Error("expected false when first element is a number")
	}
}

func TestIsChatGPTFormat_EmptyMapping(t *testing.T) {
	// Has mapping key but it's empty
	if isChatGPTFormat([]byte(`[{"mapping":{}}]`)) {
		t.Error("expected false when mapping is empty")
	}
}

func TestIsChatGPTFormat_NotAnArray(t *testing.T) {
	// JSON object instead of array
	if isChatGPTFormat([]byte(`{"mapping":{"a":{}}}`)) {
		t.Error("expected false for non-array JSON")
	}
}

// ============================================================
// parseChatGPT — uncovered branches
// ============================================================

func TestParseChatGPT_InvalidJSON(t *testing.T) {
	_, err := parseChatGPT([]byte(`[{"id": broken`))
	if err == nil {
		t.Error("expected error for malformed ChatGPT JSON")
	}
}

func TestParseChatGPT_EmptyConversations(t *testing.T) {
	conv, err := parseChatGPT([]byte(`[]`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conv.Turns) != 0 {
		t.Errorf("expected 0 turns, got %d", len(conv.Turns))
	}
}

func TestParseChatGPT_EmptyParts(t *testing.T) {
	// Message with empty parts should be skipped
	data := []byte(`[{
		"id": "c1",
		"title": "Test",
		"mapping": {
			"root": {"id": "root", "parent": "", "children": ["m1"], "message": null},
			"m1": {
				"id": "m1", "parent": "root", "children": [],
				"message": {"id": "x", "author": {"role": "user"}, "content": {"content_type": "text", "parts": ["  "]}}
			}
		}
	}]`)
	conv, err := parseChatGPT(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(conv.Turns) != 0 {
		t.Errorf("expected 0 turns (whitespace-only), got %d", len(conv.Turns))
	}
}

func TestParseChatGPT_MultipleConversations(t *testing.T) {
	data := []byte(`[
		{
			"id": "c1", "title": "First",
			"mapping": {
				"r": {"id": "r", "parent": "", "children": ["u1"], "message": null},
				"u1": {"id": "u1", "parent": "r", "children": [], "message": {"id": "m1", "author": {"role": "user"}, "content": {"content_type": "text", "parts": ["hello"]}}}
			}
		},
		{
			"id": "c2", "title": "Second",
			"mapping": {
				"r": {"id": "r", "parent": "", "children": ["u1"], "message": null},
				"u1": {"id": "u1", "parent": "r", "children": [], "message": {"id": "m2", "author": {"role": "user"}, "content": {"content_type": "text", "parts": ["world"]}}}
			}
		}
	]`)
	conv, err := parseChatGPT(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Title != "First" {
		t.Errorf("Title = %q, want %q", conv.Title, "First")
	}
	if len(conv.Turns) != 2 {
		t.Errorf("expected 2 turns, got %d", len(conv.Turns))
	}
}

// ============================================================
// flattenChatGPTMapping — uncovered branches
// ============================================================

func TestFlattenChatGPTMapping_ChildNotInMap(t *testing.T) {
	// Root references a child that doesn't exist in the mapping
	mapping := map[string]chatGPTMappingNode{
		"root": {ID: "root", Parent: "", Children: []string{"missing_child"}},
	}
	turns := flattenChatGPTMapping(mapping)
	if len(turns) != 0 {
		t.Errorf("expected 0 turns when child is missing, got %d", len(turns))
	}
}

func TestFlattenChatGPTMapping_EmptyMapping(t *testing.T) {
	turns := flattenChatGPTMapping(map[string]chatGPTMappingNode{})
	if len(turns) != 0 {
		t.Errorf("expected 0 turns for empty mapping, got %d", len(turns))
	}
}

func TestFlattenChatGPTMapping_NilMapping(t *testing.T) {
	turns := flattenChatGPTMapping(nil)
	if len(turns) != 0 {
		t.Errorf("expected 0 turns for nil mapping, got %d", len(turns))
	}
}

func TestFlattenChatGPTMapping_RootWithMessage(t *testing.T) {
	// Root node has a user message directly
	mapping := map[string]chatGPTMappingNode{
		"root": {
			ID: "root", Parent: "", Children: nil,
			Message: &chatGPTMessage{
				ID:      "m1",
				Author:  chatGPTAuthor{Role: "user"},
				Content: chatGPTMsgContent{Parts: []any{"root message"}},
			},
		},
	}
	turns := flattenChatGPTMapping(mapping)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Content != "root message" {
		t.Errorf("Content = %q, want %q", turns[0].Content, "root message")
	}
}
