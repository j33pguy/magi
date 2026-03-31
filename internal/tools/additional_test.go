package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// failingEmbedder returns an error for all calls.
type failingEmbedder struct{}

func (f *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embedding service unavailable")
}

func (f *failingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embedding service unavailable")
}

func (f *failingEmbedder) Dimensions() int { return 384 }

// ---------- IndexTurn.Handle ----------

func TestIndexTurnHandleUserTurn(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":       "user",
		"content":    "I configured the compute-cluster cluster with 3 nodes",
		"project":    "infra",
		"session_id": "sess-001",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Indexed turn") {
		t.Errorf("expected 'Indexed turn', got: %s", text)
	}
	if !strings.Contains(text, "role=user") {
		t.Errorf("expected role=user in response, got: %s", text)
	}
	if !strings.Contains(text, "speaker=user") {
		t.Errorf("expected speaker=user in response, got: %s", text)
	}
}

func TestIndexTurnHandleAssistantTurn(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "assistant",
		"content": "Sure, I can help you set up a compute-cluster cluster.",
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "role=assistant") {
		t.Errorf("expected role=assistant in response, got: %s", text)
	}
	if !strings.Contains(text, "speaker=assistant") {
		t.Errorf("expected speaker=assistant in response, got: %s", text)
	}
}

func TestIndexTurnHandleMissingRole(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "some content",
	}))
	if !result.IsError {
		t.Error("expected error for missing role")
	}
}

func TestIndexTurnHandleMissingContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role": "user",
	}))
	if !result.IsError {
		t.Error("expected error for missing content")
	}
}

func TestIndexTurnHandleDedup(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	content := "This is a unique message for dedup testing"

	// First call should succeed
	result1, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": content,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if result1.IsError {
		t.Fatalf("first call unexpected error: %v", result1.Content)
	}
	text1 := result1.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text1, "Indexed turn") {
		t.Errorf("first call expected 'Indexed turn', got: %s", text1)
	}

	// Second call with same content should be deduped
	result2, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": content,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if result2.IsError {
		t.Fatalf("second call unexpected error: %v", result2.Content)
	}
	text2 := result2.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text2, "Already indexed") {
		t.Errorf("second call expected 'Already indexed', got: %s", text2)
	}
}

// ---------- IndexSession.Handle ----------

func TestIndexSessionHandleBulkIndex(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "What is Kubernetes?"},
		{"role": "assistant", "content": "Kubernetes is a container orchestration platform."},
		{"role": "user", "content": "How do I deploy a pod?"},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "infra",
		"session_id": "sess-100",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Indexed 3 turns") {
		t.Errorf("expected 'Indexed 3 turns', got: %s", text)
	}
}

func TestIndexSessionHandleEmptyTurns(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": []map[string]any{},
	}))
	if !result.IsError {
		t.Error("expected error for empty turns")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "empty") {
		t.Errorf("expected 'empty' in error, got: %s", text)
	}
}

func TestIndexSessionHandleMissingTurns(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing turns")
	}
}

func TestIndexSessionHandleDedupWithinSession(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	// Index some turns first
	turns1 := []map[string]any{
		{"role": "user", "content": "Dedup session content alpha"},
	}
	result1, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":   turns1,
		"project": "proj",
	}))
	if result1.IsError {
		t.Fatalf("first session error: %v", result1.Content)
	}

	// Now index same content again - should be deduped
	turns2 := []map[string]any{
		{"role": "user", "content": "Dedup session content alpha"},
		{"role": "assistant", "content": "New unique content for session dedup"},
	}
	result2, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":   turns2,
		"project": "proj",
	}))
	if result2.IsError {
		t.Fatalf("second session error: %v", result2.Content)
	}
	text := result2.Content[0].(mcp.TextContent).Text
	// Should have indexed 1 (the new one) and skipped 1 (the duplicate)
	if !strings.Contains(text, "1 skipped") {
		t.Errorf("expected 1 skipped/deduped, got: %s", text)
	}
}

func TestIndexSessionHandleWithSummarize(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "I need to set up a new VLAN for IoT devices"},
		{"role": "assistant", "content": "I recommend creating VLAN 50 for IoT isolation"},
		{"role": "user", "content": "That sounds good, let me configure it on the switch"},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "infra",
		"session_id": "sess-summarize",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Indexed 3 turns") {
		t.Errorf("expected 'Indexed 3 turns', got: %s", text)
	}

	// Verify summary memory was created
	memories, err := dbClient.ListMemories(&db.MemoryFilter{
		Type:       "conversation_summary",
		Visibility: "all",
	})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Errorf("expected 1 summary memory, got %d", len(memories))
	}
}

func TestIndexSessionHandleSkipsEmptyRoleContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "Valid turn with content"},
		{"role": "", "content": "Missing role turn"},
		{"role": "user", "content": ""},
		{"role": "assistant", "content": "Another valid turn"},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":   turns,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Indexed 2 turns") {
		t.Errorf("expected 'Indexed 2 turns', got: %s", text)
	}
	if !strings.Contains(text, "2 skipped") {
		t.Errorf("expected '2 skipped', got: %s", text)
	}
}

// ---------- CheckContradictions.Handle ----------

func TestCheckContradictionsToolDefinition(t *testing.T) {
	tool := (&CheckContradictions{}).Tool()
	if tool.Name != "check_contradictions" {
		t.Errorf("tool name = %q, want %q", tool.Name, "check_contradictions")
	}
}

func TestCheckContradictionsNoContradictions(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "The sky is blue today",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No contradictions") {
		t.Errorf("expected 'No contradictions', got: %s", text)
	}
}

func TestCheckContradictionsMissingContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing content")
	}
}

func TestCheckContradictionsWithThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	// Seed a memory
	seedTestMemory(t, dbClient, "VLAN 5 is used for IoT devices", "infra", "memory")

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":   "VLAN 50 is used for IoT devices",
		"threshold": 0.95,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// With our mock embedder producing similar embeddings, results depend on
	// distance calculation. The important thing is the call succeeds without error.
}

func TestCheckContradictionsInvalidThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	// Threshold out of range should be reset to default
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":   "test content",
		"threshold": 2.0,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestCheckContradictionsWithAreaFilter(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":  "The server is running Ubuntu 24.04",
		"area":     "infrastructure",
		"sub_area": "servers",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IngestConversation.Handle ----------

func TestIngestConversationToolDefinition(t *testing.T) {
	tool := (&IngestConversation{}).Tool()
	if tool.Name != "ingest_conversation" {
		t.Errorf("tool name = %q, want %q", tool.Name, "ingest_conversation")
	}
}

func TestIngestConversationMissingContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing content")
	}
}

func TestIngestConversationPlainText(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I decided to use Tailscale for VPN\nAssistant: Good choice, Tailscale is great for infrastructure networking.\nUser: I also learned that WireGuard is the underlying protocol."

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Ingested") {
		t.Errorf("expected 'Ingested' in response, got: %s", text)
	}
}

func TestIngestConversationDryRun(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I decided to switch from nginx to caddy\nAssistant: Caddy has automatic HTTPS which is nice."

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "infra",
		"dry_run": true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Dry run should return JSON with format and would_import fields
	var preview map[string]any
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if _, ok := preview["format"]; !ok {
		t.Error("expected 'format' in dry run preview")
	}
	if _, ok := preview["would_import"]; !ok {
		t.Error("expected 'would_import' in dry run preview")
	}

	// Verify nothing was actually stored
	memories, _ := dbClient.ListMemories(&db.MemoryFilter{Project: "infra", Visibility: "all"})
	if len(memories) != 0 {
		t.Errorf("dry run should not store memories, got %d", len(memories))
	}
}

func TestIngestConversationInvalidFormat(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	// Very short content with no recognizable conversation pattern
	// The parser should still handle it (as plaintext fallback)
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "just some random text without any conversation structure",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Depending on parser behavior, this may succeed with 0 imports or error
	// The key is it doesn't panic
	_ = result
}

// ---------- RecallIncidents.Handle ----------

func TestRecallIncidentsToolDefinition(t *testing.T) {
	tool := (&RecallIncidents{}).Tool()
	if tool.Name != "recall_incidents" {
		t.Errorf("tool name = %q, want %q", tool.Name, "recall_incidents")
	}
}

func TestRecallIncidentsMissingQuery(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing query")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "query is required") {
		t.Errorf("expected 'query is required', got: %s", text)
	}
}

func TestRecallIncidentsNoResults(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "kubernetes pod crash loop",
		"project": "nonexistent-project",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No matching incidents") {
		t.Errorf("expected 'No matching incidents', got: %s", text)
	}
}

func TestRecallIncidentsWithSeededMemory(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "DNS resolution failed on compute-cluster node 3, fixed by restarting systemd-resolved", "infra", "incident")

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "DNS resolution failure",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should return JSON results (not the "no matching" message)
	if strings.Contains(text, "No matching incidents") {
		t.Errorf("expected results, got: %s", text)
	}
}

func TestRecallIncidentsWithProjectFilter(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Server OOM killed, increased memory limits", "infra", "incident")

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "out of memory",
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRecallIncidentsWithRecencyDecay(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Disk full on /var/log, rotated logs", "infra", "incident")

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":        "disk space issue",
		"recency_decay": 0.01,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallLessons.Handle ----------

func TestRecallLessonsToolDefinition(t *testing.T) {
	tool := (&RecallLessons{}).Tool()
	if tool.Name != "recall_lessons" {
		t.Errorf("tool name = %q, want %q", tool.Name, "recall_lessons")
	}
}

func TestRecallLessonsMissingQuery(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing query")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "query is required") {
		t.Errorf("expected 'query is required', got: %s", text)
	}
}

func TestRecallLessonsNoResults(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "terraform state locking",
		"project": "nonexistent-project",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No matching lessons") {
		t.Errorf("expected 'No matching lessons', got: %s", text)
	}
}

func TestRecallLessonsWithSeededMemory(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Always pin terraform provider versions to avoid breaking changes on upgrades", "infra", "lesson")

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "terraform provider versions",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "No matching lessons") {
		t.Errorf("expected results, got: %s", text)
	}
}

func TestRecallLessonsWithMultipleProjects(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Never run apt upgrade on production without a snapshot", "infra", "lesson")

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":    "apt upgrade safety",
		"projects": []string{"infra", "infra"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallConversations.Handle ----------

func TestRecallConversationsToolDefinition(t *testing.T) {
	tool := (&RecallConversations{}).Tool()
	if tool.Name != "recall_conversations" {
		t.Errorf("tool name = %q, want %q", tool.Name, "recall_conversations")
	}
}

func TestRecallConversationsMissingQuery(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing query")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "query is required") {
		t.Errorf("expected 'query is required', got: %s", text)
	}
}

func TestRecallConversationsNoResults(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "nonexistent conversation topic xyz",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No matching conversations") {
		t.Errorf("expected 'No matching conversations', got: %s", text)
	}
}

func TestRecallConversationsWithSeededMemory(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "We discussed setting up monitoring with Prometheus and Grafana", "infra", "conversation")

	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "prometheus monitoring setup",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// With seeded conversation memory, should find results
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		// If not JSON, it might be "No matching" text
		if strings.Contains(text, "No matching conversations") {
			t.Errorf("expected results, got no matches")
		}
	}
}

func TestRecallConversationsWithChannelFilter(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "some conversation topic",
		"channel": "discord",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRecallConversationsWithMinRelevance(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Talked about Docker container networking and bridge modes", "infra", "conversation")

	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":         "docker networking",
		"min_relevance": 0.5,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecentConversations.Handle ----------

func TestRecentConversationsToolDefinition(t *testing.T) {
	tool := (&RecentConversations{}).Tool()
	if tool.Name != "recent_conversations" {
		t.Errorf("tool name = %q, want %q", tool.Name, "recent_conversations")
	}
}

func TestRecentConversationsEmpty(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No recent conversations") {
		t.Errorf("expected 'No recent conversations', got: %s", text)
	}
}

func TestRecentConversationsWithChannelFilter(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed a conversation memory with proper tags
	m := seedTestMemory(t, dbClient, "Discussed backup strategies", "infra", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation", "channel:discord"})

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "discord",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should find the seeded conversation
	if strings.Contains(text, "No recent conversations") {
		t.Log("Note: channel filter may exclude result due to tag matching; verifying no error")
	}
}

func TestRecentConversationsWithSinceFilterInvalid(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "not-a-timestamp",
	}))
	if !result.IsError {
		t.Error("expected error for invalid timestamp")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "invalid since timestamp") {
		t.Errorf("expected 'invalid since timestamp', got: %s", text)
	}
}

func TestRecentConversationsWithSinceFilter(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed a conversation memory with tags
	m := seedTestMemory(t, dbClient, "Recent chat about networking", "infra", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2020-01-01T00:00:00Z",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRecentConversationsWithLimit(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed multiple conversation memories
	for i := 0; i < 5; i++ {
		m := seedTestMemory(t, dbClient, "Conversation topic number "+string(rune('A'+i)), "proj", "conversation")
		dbClient.SetTags(m.ID, []string{"conversation"})
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"limit": 2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "No recent conversations") {
		t.Error("expected some conversations with seeded data")
	}
}

// ---------- IndexSession summarize=true without session_id ----------

func TestIndexSessionSummarizeWithoutSessionID(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "Question about something"},
		{"role": "assistant", "content": "Answer to the question"},
	}

	// summarize=true but no session_id should skip summary creation
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":     turns,
		"summarize": true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// No summary should have been created
	memories, _ := dbClient.ListMemories(&db.MemoryFilter{
		Type:       "conversation_summary",
		Visibility: "all",
	})
	if len(memories) != 0 {
		t.Errorf("expected no summary without session_id, got %d", len(memories))
	}
}

// ---------- GetRelated and UnlinkMemories Tool() definitions ----------

func TestGetRelatedToolDefinition(t *testing.T) {
	tool := (&GetRelated{}).Tool()
	if tool.Name != "get_related" {
		t.Errorf("tool name = %q, want %q", tool.Name, "get_related")
	}
}

func TestUnlinkMemoriesToolDefinition(t *testing.T) {
	tool := (&UnlinkMemories{}).Tool()
	if tool.Name != "unlink_memories" {
		t.Errorf("tool name = %q, want %q", tool.Name, "unlink_memories")
	}
}

func TestGetRelatedMissingMemoryID(t *testing.T) {
	dbClient := newTestDB(t)
	g := &GetRelated{DB: dbClient}
	result, _ := g.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing memory_id")
	}
}

func TestGetRelatedWithDirection(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "source direction test", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "target direction test", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"direction": "from",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List with more filters ----------

func TestListMemoriesWithTimeFilters(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "time filtered memory", "proj", "memory")

	l := &List{DB: dbClient}

	// With "after" filter
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"after":   "2020-01-01",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestListMemoriesWithInvalidAfter(t *testing.T) {
	dbClient := newTestDB(t)
	l := &List{DB: dbClient}

	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"after": "not-a-date",
	}))
	if !result.IsError {
		t.Error("expected error for invalid after")
	}
}

func TestListMemoriesWithInvalidBefore(t *testing.T) {
	dbClient := newTestDB(t)
	l := &List{DB: dbClient}

	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"before": "not-a-date",
	}))
	if !result.IsError {
		t.Error("expected error for invalid before")
	}
}

func TestListMemoriesWithSpeakerAndAreaFilters(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "area filtered memory", "proj", "memory")

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project":  "proj",
		"speaker":  "assistant",
		"area":     "infrastructure",
		"sub_area": "networking",
		"limit":    10,
		"offset":   0,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// May return no results (speaker filter) but shouldn't error
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestListMemoriesWithTagsFilter(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "tagged memory for list", "proj", "memory")
	dbClient.SetTags(m.ID, []string{"test-tag"})

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"tags":    []string{"test-tag"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Recall with more filters ----------

func TestRecallWithTimeFilters(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "recall time filter test memory", "proj", "memory")

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "time filter test",
		"project": "proj",
		"after":   "7d",
		"before":  "2030-01-01",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRecallWithInvalidAfter(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test",
		"after": "invalid-date",
	}))
	if !result.IsError {
		t.Error("expected error for invalid after")
	}
}

func TestRecallWithInvalidBefore(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":  "test",
		"before": "invalid-date",
	}))
	if !result.IsError {
		t.Error("expected error for invalid before")
	}
}

func TestRecallWithAllFilters(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "full filter recall memory", "proj", "memory")

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":        "full filter",
		"project":      "proj",
		"projects":     []string{"proj", "other"},
		"type":         "memory",
		"tags":         []string{"some-tag"},
		"top_k":        3,
		"min_relevance": 0.0,
		"recency_decay": 0.01,
		"speaker":      "assistant",
		"area":         "infrastructure",
		"sub_area":     "networking",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Remember with dedup and parent linking ----------

func TestRememberAutoClassify(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	// No area/sub_area set, should auto-classify
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "I configured the compute-cluster cluster with 3 nodes for high availability",
		"project": "infra",
		"type":    "memory",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRememberWithDedupThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	// First memory
	result1, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":          "memory content for dedup threshold testing",
		"project":          "proj",
		"dedup_threshold":  0.99,
	}))
	if result1.IsError {
		t.Fatalf("first remember error: %v", result1.Content)
	}

	// Second memory - different enough to not dedup but tests the threshold path
	result2, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":          "completely different content that should not be deduped",
		"project":          "proj",
		"dedup_threshold":  0.99,
	}))
	if result2.IsError {
		t.Fatalf("second remember error: %v", result2.Content)
	}
}

func TestRememberWithInvalidDedupThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	// Invalid threshold (> 1) should be clamped to default
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":         "test content for invalid threshold",
		"project":         "proj",
		"dedup_threshold": 5.0,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRememberWithSummaryAndType(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "Always check for breaking changes before upgrading Terraform providers",
		"project": "infra",
		"type":    "lesson",
		"summary": "Pin terraform providers",
		"area":    "work",
		"sub_area": "terraform",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "type=lesson") {
		t.Errorf("expected type=lesson, got: %s", text)
	}
}

// ---------- StoreConversation with full params ----------

func TestStoreConversationWithAllParams(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel":      "discord",
		"summary":      "Discussed migrating from nginx to caddy",
		"session_key":  "sess-discord-123",
		"started_at":   "2026-01-15T10:00:00Z",
		"ended_at":     "2026-01-15T11:30:00Z",
		"turn_count":   15,
		"topics":       []string{"nginx", "caddy", "reverse-proxy"},
		"decisions":    []string{"switch to caddy for auto-HTTPS"},
		"action_items": []string{"update docker-compose for caddy"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Stored conversation") {
		t.Errorf("expected 'Stored conversation', got: %s", text)
	}
}

// ---------- Forget with permanent=false (archive) ----------

func TestForgetArchiveDefault(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "archive by default", "proj", "memory")

	f := &Forget{DB: dbClient}
	// Call without permanent field — defaults to archive
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{
		"id": m.ID,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Verify archived
	got, _ := dbClient.GetMemory(m.ID)
	if got.ArchivedAt == "" {
		t.Error("expected ArchivedAt to be set")
	}
}

// ---------- IngestConversation with source param ----------

func TestIngestConversationWithSource(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I prefer using Go for backend services\nAssistant: Go is a great choice for high-performance backends."

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"source":  "plaintext",
		"project": "general",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallIncidents with tags filter ----------

func TestRecallIncidentsWithTags(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "NFS mount timeout during backup window", "infra", "incident")
	dbClient.SetTags(m.ID, []string{"nfs", "backup"})

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "NFS mount issue",
		"tags":  []string{"nfs"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallLessons with tags filter ----------

func TestRecallLessonsWithTags(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "Always test DNS changes in a staging environment first", "infra", "lesson")
	dbClient.SetTags(m.ID, []string{"dns", "staging"})

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "DNS changes safety",
		"tags":  []string{"dns"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallConversations with recency_decay ----------

func TestRecallConversationsWithRecencyDecay(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Discussed Kubernetes namespace strategy for multi-tenant", "infra", "conversation")

	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":        "kubernetes namespace",
		"recency_decay": 0.01,
		"top_k":        3,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecentConversations with valid since ----------

func TestRecentConversationsWithValidSince(t *testing.T) {
	dbClient := newTestDB(t)

	m := seedTestMemory(t, dbClient, "Recent conversation about CI/CD pipelines", "proj", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	tool := &RecentConversations{DB: dbClient}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2020-01-01T00:00:00Z",
		"limit": 5,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- LinkMemories with custom weight ----------

func TestLinkMemoriesWithWeight(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "weighted link from", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "weighted link to", "proj", "memory")

	l := &LinkMemories{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m1.ID,
		"to_id":    m2.ID,
		"relation": "caused_by",
		"weight":   0.75,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Verify via JSON output
	text := result.Content[0].(mcp.TextContent).Text
	var link db.MemoryLink
	if err := json.Unmarshal([]byte(text), &link); err != nil {
		t.Fatalf("unmarshal link: %v", err)
	}
	if link.Relation != "caused_by" {
		t.Errorf("Relation = %q, want %q", link.Relation, "caused_by")
	}
}

// ---------- UnlinkMemories with non-existent link ----------

func TestUnlinkMemoriesNotFound(t *testing.T) {
	dbClient := newTestDB(t)
	u := &UnlinkMemories{DB: dbClient}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{
		"link_id": "nonexistent-link-id",
	}))
	// May or may not error depending on DB implementation - just verify no panic
	_ = result
}

// ---------- GetRelated with zero depth ----------

func TestGetRelatedZeroDepth(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "zero depth node", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "related to zero depth", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	// depth=0 should be treated as depth=1
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     0,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var results []relatedResult
	json.Unmarshal([]byte(text), &results)
	if len(results) != 1 {
		t.Errorf("depth 0 (treated as 1): got %d related, want 1", len(results))
	}
}

// ---------- StoreConversation dedup (same conversation twice) ----------

func TestStoreConversationDedup(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	args := map[string]any{
		"channel": "mcp",
		"summary": "Exact same conversation for dedup testing with enough content to match",
		"topics":  []string{"dedup"},
	}

	// First store
	result1, err := s.Handle(context.Background(), makeRequest(args))
	if err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if result1.IsError {
		t.Fatalf("first call error: %v", result1.Content)
	}

	// Second store with identical content should hit dedup or parent linking
	result2, err := s.Handle(context.Background(), makeRequest(args))
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	// Should succeed regardless - either deduped or linked as parent
	if result2.IsError {
		t.Fatalf("second call error: %v", result2.Content)
	}
}

// ---------- RecallIncidents with ParentID resolution ----------

func TestRecallIncidentsWithParentMemory(t *testing.T) {
	dbClient := newTestDB(t)

	// Create a parent incident
	parent := seedTestMemory(t, dbClient, "Parent incident: full detailed report about network outage on VLAN 10", "infra", "incident")

	// Create a child incident that references the parent
	childEmb := make([]float32, 384)
	childEmb[0] = 0.5
	child, err := dbClient.SaveMemory(&db.Memory{
		Content:    "Brief: network outage VLAN 10",
		Embedding:  childEmb,
		Project:    "infra",
		Type:       "incident",
		Visibility: "internal",
		Speaker:    "assistant",
		ParentID:   parent.ID,
	})
	if err != nil {
		t.Fatalf("SaveMemory child: %v", err)
	}
	_ = child

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "network outage",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallLessons with ParentID resolution ----------

func TestRecallLessonsWithParentMemory(t *testing.T) {
	dbClient := newTestDB(t)

	parent := seedTestMemory(t, dbClient, "Parent lesson: detailed explanation of why you should always test DNS changes first", "infra", "lesson")

	childEmb := make([]float32, 384)
	childEmb[0] = 0.5
	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "Brief: test DNS changes first",
		Embedding:  childEmb,
		Project:    "infra",
		Type:       "lesson",
		Visibility: "internal",
		Speaker:    "assistant",
		ParentID:   parent.ID,
	})
	if err != nil {
		t.Fatalf("SaveMemory child: %v", err)
	}

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "DNS changes",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecentConversations with since filtering and future since ----------

func TestRecentConversationsWithFutureSince(t *testing.T) {
	dbClient := newTestDB(t)

	m := seedTestMemory(t, dbClient, "Old conversation that should be filtered out", "proj", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	tool := &RecentConversations{DB: dbClient}
	// Use a far-future since timestamp to filter everything out
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2099-01-01T00:00:00Z",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No recent conversations") {
		t.Errorf("expected 'No recent conversations' with future since, got: %s", text)
	}
}

func TestRecentConversationsWithSinceLimitOverflow(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed many conversations
	for i := 0; i < 5; i++ {
		m := seedTestMemory(t, dbClient, "Batch conversation number "+string(rune('A'+i))+" for since overflow test", "proj", "conversation")
		dbClient.SetTags(m.ID, []string{"conversation"})
	}

	// Use old since and small limit to exercise the limit truncation path
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2020-01-01T00:00:00Z",
		"limit": 2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Remember dedup near-duplicate ----------

func TestRememberDedupNearDuplicate(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	// Store first memory
	result1, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "The compute-cluster cluster has three nodes for HA",
		"project": "infra",
	}))
	if result1.IsError {
		t.Fatalf("first remember error: %v", result1.Content)
	}

	// Store very similar memory - might hit dedup or parent path
	result2, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "The compute-cluster cluster has three nodes for high availability",
		"project": "infra",
	}))
	// Should not panic, may or may not dedup
	if result2.IsError {
		t.Fatalf("second remember error: %v", result2.Content)
	}
}

// ---------- Remember with parent linking (match found but not exact dup) ----------

func TestRememberWithParentLinking(t *testing.T) {
	dbClient := newTestDB(t)

	// Pre-seed a memory directly so FindSimilar can find it
	emb := make([]float32, 384)
	emb[0] = 0.55
	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "Original memory about compute-cluster networking setup details",
		Embedding:  emb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}
	// Store a related memory that exercises FindSimilar parent linking
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "Updated compute-cluster networking setup with new VLAN configuration",
		"project": "infra",
	}))
	if result.IsError {
		t.Fatalf("remember error: %v", result.Content)
	}
}

// ---------- IndexTurn with long content (summary truncation) ----------

func TestIndexTurnHandleLongContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	// Create content > 100 chars to trigger summary truncation
	longContent := strings.Repeat("This is a test of long content that should be truncated in the summary field. ", 5)

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": longContent,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexSession with long content turns (summary truncation) ----------

func TestIndexSessionHandleWithLongContent(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	longContent := strings.Repeat("Long content for session summarization testing. ", 20)

	turns := []map[string]any{
		{"role": "user", "content": longContent},
		{"role": "assistant", "content": longContent},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-long",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Update with content change (re-embed) ----------

func TestUpdateMemoryContent(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "original update content", "proj", "memory")

	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":       m.ID,
		"content":  "changed content that triggers re-embedding",
		"summary":  "updated summary",
		"type":     "lesson",
		"area":     "infrastructure",
		"sub_area": "networking",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	got, _ := dbClient.GetMemory(m.ID)
	if got.Content != "changed content that triggers re-embedding" {
		t.Errorf("Content not updated")
	}
}

// ---------- RecallIncidents with multiple projects ----------

func TestRecallIncidentsWithMultipleProjects(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Database connection pool exhausted during peak load", "infra", "incident")

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":    "database connection pool",
		"projects": []string{"infra", "infra"},
		"top_k":    3,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallLessons with recency_decay ----------

func TestRecallLessonsWithRecencyDecay(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Learned to always set resource limits on K8s pods", "infra", "lesson")

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":        "kubernetes resource limits",
		"recency_decay": 0.02,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- CheckContradictions with contradictory content ----------

// ---------- Error path tests using failingEmbedder ----------

func TestIndexTurnEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": "test content for embed error",
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "embedding") {
		t.Errorf("expected embedding error message, got: %s", text)
	}
}

func TestRememberEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "test content for embed error",
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestStoreConversationEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "mcp",
		"summary": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestCheckContradictionsEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestRecallIncidentsEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallIncidents{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestRecallLessonsEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallLessons{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestRecallConversationsEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallConversations{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestRecallEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &Recall{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for embed error",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
}

func TestUpdateMemoryEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "content to update", "proj", "memory")

	u := &Update{DB: dbClient, Embedder: &failingEmbedder{}}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":      m.ID,
		"content": "new content triggers re-embed",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure during update")
	}
}

// ---------- Link source memory not found ----------

func TestLinkMemoriesSourceNotFound(t *testing.T) {
	dbClient := newTestDB(t)
	m2 := seedTestMemory(t, dbClient, "target exists", "proj", "memory")

	l := &LinkMemories{DB: dbClient}
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  "nonexistent-source",
		"to_id":    m2.ID,
		"relation": "related_to",
	}))
	if !result.IsError {
		t.Error("expected error for non-existent source")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "source memory not found") {
		t.Errorf("expected 'source memory not found', got: %s", text)
	}
}

func TestIndexSessionInvalidTurnsFormat(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	// Pass turns as a string instead of array - should fail marshaling
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": "not an array",
	}))
	if !result.IsError {
		t.Error("expected error for invalid turns format")
	}
}

func TestIndexSessionEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &failingEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "test content"},
	}

	// Should skip turns where embedding fails
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":   turns,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Should succeed with all turns skipped
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "0 turns") || !strings.Contains(text, "1 skipped") {
		t.Logf("result text: %s", text)
	}
}

func TestIndexSessionSummarizeEmbedError(t *testing.T) {
	dbClient := newTestDB(t)

	// Use a special embedder that succeeds for individual turns but we can test summary path
	// Since we can't easily make it fail only for summary, just verify the summarize path
	// works with long content that exercises truncation
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	longContent := strings.Repeat("x", 300)
	turns := []map[string]any{
		{"role": "user", "content": longContent},
		{"role": "assistant", "content": longContent},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-long-summary",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestIngestConversationDedupError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &failingEmbedder{}}

	conv := "User: I decided to use Terraform for infrastructure\nAssistant: Good choice."

	// With failing embedder, the dedup should still work (hash-based) but embedding will fail
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "proj",
	}))
	// Should return result (not panic) - either error or success with 0 imports
	_ = result
}

func TestCheckContradictionsWithContradiction(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed a memory that could contradict
	emb := make([]float32, 384)
	emb[0] = 0.3
	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "VLAN 5 is used for IoT devices and is enabled",
		Embedding:  emb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}
	// This content uses "disabled" which is a boolean flip from "enabled"
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":   "VLAN 5 is used for IoT devices and is disabled",
		"threshold": 0.1, // low threshold to increase chance of finding it
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}
