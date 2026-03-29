package ingest

import (
	"context"
	"strings"
	"testing"
)

// --- Mock DB client for dedup tests ---

type mockDBClient struct {
	hashes map[string]string // content hash -> existing memory ID
	err    error
}

func (m *mockDBClient) ExistsWithContentHash(hash string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.hashes[hash], nil
}

// --- Mock embedder ---

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	vec := make([]float32, m.dim)
	for i := range vec {
		vec[i] = 0.01 * float32(i%10)
	}
	return vec, nil
}

// ============================================================
// Deduplicator.Filter tests
// ============================================================

func TestDeduplicator_Filter_NoDuplicates(t *testing.T) {
	d := &Deduplicator{
		DB:       &mockDBClient{hashes: map[string]string{}},
		Embedder: &mockEmbedder{dim: 384},
	}

	candidates := []ExtractedMemory{
		{Content: "first memory", Type: "decision"},
		{Content: "second memory", Type: "lesson"},
		{Content: "third memory", Type: "preference"},
	}

	kept, skipped, err := d.Filter(context.Background(), candidates)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	if len(kept) != 3 {
		t.Errorf("expected 3 kept, got %d", len(kept))
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
}

func TestDeduplicator_Filter_WithDuplicates(t *testing.T) {
	// Pre-compute the hash of "duplicate content"
	hash := contentHash("duplicate content")

	d := &Deduplicator{
		DB: &mockDBClient{hashes: map[string]string{
			hash: "existing-id-123",
		}},
		Embedder: &mockEmbedder{dim: 384},
	}

	candidates := []ExtractedMemory{
		{Content: "unique content", Type: "decision"},
		{Content: "duplicate content", Type: "lesson"},
		{Content: "another unique", Type: "preference"},
	}

	kept, skipped, err := d.Filter(context.Background(), candidates)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	if len(kept) != 2 {
		t.Errorf("expected 2 kept, got %d", len(kept))
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", skipped)
	}
}

func TestDeduplicator_Filter_DBError_KeepsCandidate(t *testing.T) {
	d := &Deduplicator{
		DB:       &mockDBClient{err: context.DeadlineExceeded},
		Embedder: &mockEmbedder{dim: 384},
	}

	candidates := []ExtractedMemory{
		{Content: "content", Type: "decision"},
	}

	kept, skipped, err := d.Filter(context.Background(), candidates)
	if err != nil {
		t.Fatalf("Filter should not return error on DB failure: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("expected 1 kept (DB error = keep), got %d", len(kept))
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped on DB error, got %d", skipped)
	}
}

func TestDeduplicator_Filter_Empty(t *testing.T) {
	d := &Deduplicator{
		DB:       &mockDBClient{hashes: map[string]string{}},
		Embedder: &mockEmbedder{dim: 384},
	}

	kept, skipped, err := d.Filter(context.Background(), nil)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	if len(kept) != 0 || skipped != 0 {
		t.Errorf("expected 0/0 for nil input, got kept=%d skipped=%d", len(kept), skipped)
	}
}

func TestDeduplicator_Filter_AllDuplicates(t *testing.T) {
	h1 := contentHash("content A")
	h2 := contentHash("content B")

	d := &Deduplicator{
		DB: &mockDBClient{hashes: map[string]string{
			h1: "id-1",
			h2: "id-2",
		}},
		Embedder: &mockEmbedder{dim: 384},
	}

	candidates := []ExtractedMemory{
		{Content: "content A", Type: "decision"},
		{Content: "content B", Type: "lesson"},
	}

	kept, skipped, err := d.Filter(context.Background(), candidates)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	if len(kept) != 0 {
		t.Errorf("expected 0 kept, got %d", len(kept))
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}
}

// ============================================================
// contentHash tests
// ============================================================

func TestContentHash_Deterministic(t *testing.T) {
	h1 := contentHash("hello world")
	h2 := contentHash("hello world")
	if h1 != h2 {
		t.Errorf("contentHash not deterministic: %q != %q", h1, h2)
	}
}

func TestContentHash_TrimSpace(t *testing.T) {
	h1 := contentHash("hello world")
	h2 := contentHash("  hello world  ")
	if h1 != h2 {
		t.Errorf("contentHash should trim whitespace: %q != %q", h1, h2)
	}
}

func TestContentHash_DifferentContent(t *testing.T) {
	h1 := contentHash("hello")
	h2 := contentHash("world")
	if h1 == h2 {
		t.Error("contentHash should differ for different content")
	}
}

func TestContentHash_Length(t *testing.T) {
	h := contentHash("test")
	if len(h) != 16 {
		t.Errorf("contentHash length = %d, want 16", len(h))
	}
}

// ============================================================
// parseMarkdown tests
// ============================================================

func TestParseMarkdown(t *testing.T) {
	data := []byte("# My Notes\n\nSome content here about a project")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse markdown error: %v", err)
	}
	if conv.Format != FormatMarkdown {
		t.Errorf("Format = %q, want %q", conv.Format, FormatMarkdown)
	}
	if conv.Source != "markdown" {
		t.Errorf("Source = %q, want %q", conv.Source, "markdown")
	}
	if len(conv.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(conv.Turns))
	}
	if conv.Turns[0].Role != "user" {
		t.Errorf("Turn[0].Role = %q, want %q", conv.Turns[0].Role, "user")
	}
	if !strings.Contains(conv.Turns[0].Content, "My Notes") {
		t.Errorf("Turn content should contain markdown text")
	}
}

func TestParseMarkdown_EmptyContent(t *testing.T) {
	data := []byte("#    ")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse markdown error: %v", err)
	}
	if conv.Format != FormatMarkdown {
		t.Errorf("Format = %q, want %q", conv.Format, FormatMarkdown)
	}
	// "#    " trimmed is "#" which is non-empty
	if len(conv.Turns) != 1 {
		t.Errorf("expected 1 turn, got %d", len(conv.Turns))
	}
}

// ============================================================
// normalizeRole tests
// ============================================================

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user", "user"},
		{"User", "user"},
		{"USER", "user"},
		{"human", "user"},
		{"Human", "user"},
		{"assistant", "assistant"},
		{"Assistant", "assistant"},
		{"model", "assistant"},
		{"bot", "assistant"},
		{"grok", "assistant"},
		{"chatgpt", "assistant"},
		{"system", "system"},
		{"System", "system"},
		{"unknown_role", "unknown_role"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeRole(tt.input)
		if got != tt.want {
			t.Errorf("normalizeRole(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ============================================================
// mapSpeaker tests
// ============================================================

func TestMapSpeaker(t *testing.T) {
	tests := []struct {
		role   string
		source string
		want   string
	}{
		{"user", "grok", "user"},
		{"user", "chatgpt", "user"},
		{"user", "plaintext", "user"},
		{"assistant", "grok", "grok"},
		{"assistant", "chatgpt", "chatgpt"},
		{"assistant", "plaintext", "assistant"},
		{"assistant", "markdown", "assistant"},
		{"assistant", "", "assistant"},
	}
	for _, tt := range tests {
		got := mapSpeaker(tt.role, tt.source)
		if got != tt.want {
			t.Errorf("mapSpeaker(%q, %q) = %q, want %q", tt.role, tt.source, got, tt.want)
		}
	}
}

// ============================================================
// summarize tests
// ============================================================

func TestSummarize_Short(t *testing.T) {
	got := summarize("short text")
	if got != "short text" {
		t.Errorf("summarize short = %q, want %q", got, "short text")
	}
}

func TestSummarize_Long(t *testing.T) {
	// Generate >100 chars of content
	long := strings.Repeat("word ", 30)
	got := summarize(long)
	if len(got) > 100 {
		t.Errorf("summarize should truncate to 100 chars, got %d", len(got))
	}
}

func TestSummarize_CollapsesWhitespace(t *testing.T) {
	got := summarize("  hello   world  \n\t foo  ")
	if got != "hello world foo" {
		t.Errorf("summarize whitespace = %q, want %q", got, "hello world foo")
	}
}

// ============================================================
// ExtractMemories edge cases
// ============================================================

func TestExtractMemories_SkipsEmptyTurns(t *testing.T) {
	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
		Turns: []Turn{
			{Role: "user", Content: "   "},     // whitespace only
			{Role: "user", Content: ""},         // empty
			{Role: "user", Content: "real content that is a decision: we decided to go with gRPC"},
		},
	}

	memories := ExtractMemories(conv)
	if len(memories) != 1 {
		t.Errorf("expected 1 memory (skipping empty turns), got %d", len(memories))
	}
}

func TestExtractMemories_ConversationType(t *testing.T) {
	// Turn at index >= 3 with > 200 chars and no decision/lesson/preference keywords
	longContent := strings.Repeat("This is a general discussion point about architecture and implementation details. ", 5)

	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
		Turns: []Turn{
			{Role: "user", Content: "context turn 0"},
			{Role: "assistant", Content: "context turn 1"},
			{Role: "user", Content: "context turn 2"},
			{Role: "assistant", Content: longContent}, // index 3, > 200 chars
		},
	}

	memories := ExtractMemories(conv)
	found := false
	for _, m := range memories {
		if m.Type == "conversation" {
			found = true
		}
	}
	if !found {
		t.Error("expected a 'conversation' type memory for long turn at index >= 3")
	}
}

func TestExtractMemories_ShortTurnAfterThreshold(t *testing.T) {
	// Turn at index >= 3, < 200 chars, not a decision/lesson/preference => skipped
	conv := &ParsedConversation{
		Format: FormatPlainText,
		Source: "plaintext",
		Turns: []Turn{
			{Role: "user", Content: "context"},
			{Role: "assistant", Content: "context"},
			{Role: "user", Content: "context"},
			{Role: "user", Content: "short unimportant note"}, // index 3, < 200 chars
		},
	}

	memories := ExtractMemories(conv)
	for _, m := range memories {
		if m.Type == "conversation" && strings.Contains(m.Content, "short unimportant") {
			t.Error("should not extract short content as conversation type")
		}
	}
}

// ============================================================
// buildTags test
// ============================================================

func TestBuildTags(t *testing.T) {
	tags := buildTags("grok", "decision")
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}

	expected := map[string]bool{
		"source:grok":    true,
		"ingested:true":  true,
		"type:decision":  true,
	}
	for _, tag := range tags {
		if !expected[tag] {
			t.Errorf("unexpected tag: %q", tag)
		}
	}
}

// ============================================================
// parsePlainText edge cases
// ============================================================

func TestParsePlainText_MultilineContent(t *testing.T) {
	data := []byte("Human: First line\nSecond line of same turn\nThird line\nAssistant: Response here")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(conv.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(conv.Turns))
	}
	if !strings.Contains(conv.Turns[0].Content, "Second line") {
		t.Errorf("expected multiline content in first turn, got %q", conv.Turns[0].Content)
	}
}

func TestParsePlainText_UserPrefix(t *testing.T) {
	data := []byte("User: Hello there\nAssistant: Hi!")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(conv.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(conv.Turns))
	}
	if conv.Turns[0].Role != "user" {
		t.Errorf("Turn[0].Role = %q, want %q", conv.Turns[0].Role, "user")
	}
}

func TestParsePlainText_NoRoleLines(t *testing.T) {
	// Lines before any role marker are ignored
	data := []byte("some preamble text\nmore preamble\nHuman: actual content")
	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(conv.Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(conv.Turns))
	}
	if conv.Turns[0].Content != "actual content" {
		t.Errorf("Content = %q, want %q", conv.Turns[0].Content, "actual content")
	}
}

func TestPlainTextRoleMatch(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"Human: hello", "user"},
		{"User: hello", "user"},
		{"Assistant: hello", "assistant"},
		{"random line", ""},
		{"", ""},
		{"  Human: indented", "user"},
	}
	for _, tt := range tests {
		got := plainTextRoleMatch(tt.line)
		if got != tt.want {
			t.Errorf("plainTextRoleMatch(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

// ============================================================
// Grok parsing edge cases
// ============================================================

func TestParseGrok_EmptyParts(t *testing.T) {
	data := []byte(`[{
		"conversation_id": "abc",
		"title": "Test",
		"create_time": "2024-01-01",
		"turns": [
			{"author": {"role": "user"}, "content": {"content_type": "text", "parts": [""]}},
			{"author": {"role": "assistant"}, "content": {"content_type": "text", "parts": ["response"]}}
		]
	}]`)

	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Empty part should be skipped
	if len(conv.Turns) != 1 {
		t.Errorf("expected 1 turn (empty skipped), got %d", len(conv.Turns))
	}
}

func TestParseGrok_MultipleConversations(t *testing.T) {
	data := []byte(`[
		{"conversation_id": "c1", "title": "First", "turns": [
			{"author": {"role": "user"}, "content": {"content_type": "text", "parts": ["hello"]}}
		]},
		{"conversation_id": "c2", "title": "Second", "turns": [
			{"author": {"role": "user"}, "content": {"content_type": "text", "parts": ["world"]}}
		]}
	]`)

	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if conv.Title != "First" {
		t.Errorf("Title should be from first conversation: %q", conv.Title)
	}
	if len(conv.Turns) != 2 {
		t.Errorf("expected 2 turns from 2 conversations, got %d", len(conv.Turns))
	}
}

// ============================================================
// isGrokFormat/isChatGPTFormat edge cases
// ============================================================

func TestIsGrokFormat_InvalidJSON(t *testing.T) {
	if isGrokFormat([]byte(`not json`)) {
		t.Error("expected false for invalid JSON")
	}
}

func TestIsGrokFormat_EmptyArray(t *testing.T) {
	if isGrokFormat([]byte(`[]`)) {
		t.Error("expected false for empty array")
	}
}

func TestIsGrokFormat_NoConversationID(t *testing.T) {
	if isGrokFormat([]byte(`[{"title":"test"}]`)) {
		t.Error("expected false when no conversation_id")
	}
}

func TestIsChatGPTFormat_InvalidJSON(t *testing.T) {
	if isChatGPTFormat([]byte(`not json`)) {
		t.Error("expected false for invalid JSON")
	}
}

func TestIsChatGPTFormat_EmptyArray(t *testing.T) {
	if isChatGPTFormat([]byte(`[]`)) {
		t.Error("expected false for empty array")
	}
}

func TestIsChatGPTFormat_NoMapping(t *testing.T) {
	if isChatGPTFormat([]byte(`[{"id":"test"}]`)) {
		t.Error("expected false when no mapping")
	}
}

// ============================================================
// ChatGPT parsing edge cases
// ============================================================

func TestParseChatGPT_SystemRole(t *testing.T) {
	data := []byte(`[{
		"id": "conv1",
		"title": "Test",
		"mapping": {
			"root": {"id": "root", "parent": "", "children": ["sys"], "message": null},
			"sys": {
				"id": "sys",
				"parent": "root",
				"children": ["u1"],
				"message": {"id": "s1", "author": {"role": "system"}, "content": {"content_type": "text", "parts": ["You are helpful"]}}
			},
			"u1": {
				"id": "u1",
				"parent": "sys",
				"children": [],
				"message": {"id": "m1", "author": {"role": "user"}, "content": {"content_type": "text", "parts": ["hello"]}}
			}
		}
	}]`)

	conv, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// System messages should be excluded from turns
	if len(conv.Turns) != 1 {
		t.Errorf("expected 1 turn (system excluded), got %d", len(conv.Turns))
	}
	if conv.Turns[0].Role != "user" {
		t.Errorf("Turn[0].Role = %q, want %q", conv.Turns[0].Role, "user")
	}
}

func TestFlattenChatGPTMapping_NoRoot(t *testing.T) {
	// All nodes have parents, so no root found
	mapping := map[string]chatGPTMappingNode{
		"a": {ID: "a", Parent: "b", Children: nil},
		"b": {ID: "b", Parent: "a", Children: nil},
	}
	turns := flattenChatGPTMapping(mapping)
	if len(turns) != 0 {
		t.Errorf("expected 0 turns when no root, got %d", len(turns))
	}
}

func TestExtractChatGPTParts_MixedTypes(t *testing.T) {
	parts := []any{
		"text part",
		map[string]any{"type": "image", "url": "http://example.com"},
		"another text",
	}
	got := extractChatGPTParts(parts)
	if got != "text part\nanother text" {
		t.Errorf("extractChatGPTParts = %q, want %q", got, "text part\nanother text")
	}
}

// ============================================================
// Detect edge cases
// ============================================================

func TestDetect_JSONButNotGrokOrChatGPT(t *testing.T) {
	data := []byte(`[{"some": "random json"}]`)
	got := Detect(data)
	if got != FormatUnknown {
		t.Errorf("Detect for random JSON = %q, want %q", got, FormatUnknown)
	}
}

func TestDetect_JSONObject(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	got := Detect(data)
	if got != FormatUnknown {
		t.Errorf("Detect for JSON object = %q, want %q", got, FormatUnknown)
	}
}

// ============================================================
// matchesDecision / matchesLesson / matchesPreference
// ============================================================

func TestMatchesDecision(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"we decided to use gRPC", true},
		{"will use Docker for this", true},
		{"going with the new approach", true},
		{"just a normal sentence", false},
	}
	for _, tt := range tests {
		got := matchesDecision(tt.input)
		if got != tt.want {
			t.Errorf("matchesDecision(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchesLesson(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"I learned that caching helps", true},
		{"figured out the root cause", true},
		{"the issue was a race condition", true},
		{"just normal text", false},
	}
	for _, tt := range tests {
		got := matchesLesson(tt.input)
		if got != tt.want {
			t.Errorf("matchesLesson(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMatchesPreference(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"I prefer Go for CLI tools", true},
		{"I always use vim", true},
		{"don't use that library", true},
		{"just normal text", false},
	}
	for _, tt := range tests {
		got := matchesPreference(tt.input)
		if got != tt.want {
			t.Errorf("matchesPreference(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
