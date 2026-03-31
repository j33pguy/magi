package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// mockEmbedder implements embeddings.Provider for tests.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if len(text) > 0 {
		emb[0] = float32(len(text)) / 100.0
	}
	return emb, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := m.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int { return 384 }

// newTestDB creates a SQLite test client with migrations applied.
func newTestDB(t *testing.T) *db.Client {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "test.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return client.TursoClient
}

// seedTestMemory inserts a test memory.
func seedTestMemory(t *testing.T, dbClient *db.Client, content, project, memType string) *db.Memory {
	t.Helper()
	emb := make([]float32, 384)
	emb[0] = float32(len(content)) / 100.0
	m, err := dbClient.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  emb,
		Project:    project,
		Type:       memType,
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seedTestMemory: %v", err)
	}
	return m
}

// makeRequest creates an MCP CallToolRequest with the given args.
func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

// ---------- Forget ----------

func TestForgetToolDefinition(t *testing.T) {
	f := &Forget{DB: nil}
	tool := f.Tool()
	if tool.Name != "forget" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "forget")
	}
}

func TestForgetArchive(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "to be forgotten", "proj", "memory")

	f := &Forget{DB: dbClient}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{"id": m.ID}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Verify archived (not in listing)
	list, _ := dbClient.ListMemories(&db.MemoryFilter{Project: "proj", Visibility: "all"})
	if len(list) != 0 {
		t.Errorf("expected 0 memories after archive, got %d", len(list))
	}

	// But still retrievable via GetMemory
	got, err := dbClient.GetMemory(m.ID)
	if err != nil {
		t.Fatalf("GetMemory after archive: %v", err)
	}
	if got.ArchivedAt == "" {
		t.Error("expected non-empty ArchivedAt")
	}
}

func TestForgetPermanent(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "to be deleted", "proj", "memory")

	f := &Forget{DB: dbClient}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{
		"id":        m.ID,
		"permanent": true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Verify permanently deleted
	_, err = dbClient.GetMemory(m.ID)
	if err == nil {
		t.Error("expected error after permanent delete")
	}
}

func TestForgetMissingID(t *testing.T) {
	dbClient := newTestDB(t)
	f := &Forget{DB: dbClient}
	result, _ := f.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing ID")
	}
}

func TestForgetNotFound(t *testing.T) {
	dbClient := newTestDB(t)
	f := &Forget{DB: dbClient}
	result, _ := f.Handle(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
	if !result.IsError {
		t.Error("expected error for non-existent memory")
	}
}

// ---------- List ----------

func TestListToolDefinition(t *testing.T) {
	l := &List{DB: nil}
	tool := l.Tool()
	if tool.Name != "list_memories" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "list_memories")
	}
}

func TestListMemories(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "list test 1", "proj", "memory")
	seedTestMemory(t, dbClient, "list test 2", "proj", "memory")

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var memories []db.Memory
	if err := json.Unmarshal([]byte(text), &memories); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("got %d memories, want 2", len(memories))
	}
}

func TestListMemoriesEmpty(t *testing.T) {
	dbClient := newTestDB(t)

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No memories found") {
		t.Errorf("expected 'No memories found', got: %s", text)
	}
}

func TestListMemoriesWithFilters(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "list filtered", "proj", "incident")
	seedTestMemory(t, dbClient, "list other", "proj", "memory")

	l := &List{DB: dbClient}
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"type":    "incident",
	}))
	text := result.Content[0].(mcp.TextContent).Text
	var memories []db.Memory
	json.Unmarshal([]byte(text), &memories)
	if len(memories) != 1 {
		t.Errorf("type filter: got %d, want 1", len(memories))
	}
}

// ---------- LinkMemories ----------

func TestLinkMemoriesToolDefinition(t *testing.T) {
	l := &LinkMemories{DB: nil}
	tool := l.Tool()
	if tool.Name != "link_memories" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "link_memories")
	}
}

func TestLinkMemories(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "from memory", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "to memory", "proj", "memory")

	l := &LinkMemories{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m1.ID,
		"to_id":    m2.ID,
		"relation": "related_to",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	// Verify link created
	links, _ := dbClient.GetLinks(context.Background(), m1.ID, "from")
	if len(links) != 1 {
		t.Errorf("got %d links, want 1", len(links))
	}
}

func TestLinkMemoriesMissingFields(t *testing.T) {
	dbClient := newTestDB(t)
	l := &LinkMemories{DB: dbClient}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing from_id", map[string]any{"to_id": "x", "relation": "related_to"}},
		{"missing to_id", map[string]any{"from_id": "x", "relation": "related_to"}},
		{"missing relation", map[string]any{"from_id": "x", "to_id": "y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := l.Handle(context.Background(), makeRequest(tt.args))
			if !result.IsError {
				t.Error("expected error")
			}
		})
	}
}

func TestLinkMemoriesNotFound(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "exists", "proj", "memory")

	l := &LinkMemories{DB: dbClient}
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m1.ID,
		"to_id":    "nonexistent",
		"relation": "related_to",
	}))
	if !result.IsError {
		t.Error("expected error for non-existent target")
	}
}

// ---------- UnlinkMemories ----------

func TestUnlinkMemories(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "from", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "to", "proj", "memory")

	link, _ := dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	u := &UnlinkMemories{DB: dbClient}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{"link_id": link.ID}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestUnlinkMemoriesMissingID(t *testing.T) {
	dbClient := newTestDB(t)
	u := &UnlinkMemories{DB: dbClient}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing link_id")
	}
}

// ---------- GetRelated ----------

func TestGetRelated(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "center node", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "related node", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
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
		t.Errorf("got %d related, want 1", len(results))
	}
}

func TestGetRelatedDeepTraversal(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "A", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "B", "proj", "memory")
	m3 := seedTestMemory(t, dbClient, "C", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "led_to", 1.0, false)
	dbClient.CreateLink(context.Background(), m2.ID, m3.ID, "led_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	result, _ := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     2,
	}))

	text := result.Content[0].(mcp.TextContent).Text
	var results []relatedResult
	json.Unmarshal([]byte(text), &results)
	if len(results) != 2 {
		t.Errorf("depth 2: got %d related, want 2", len(results))
	}
}

// ---------- Recall ----------

func TestRecallToolDefinition(t *testing.T) {
	r := &Recall{DB: nil, Embedder: nil}
	tool := r.Tool()
	if tool.Name != "recall" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "recall")
	}
}

func TestRecallMissingQuery(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing query")
	}
}

func TestRecallWithResults(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "kubernetes cluster backup strategy", "proj", "memory")

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "kubernetes backup",
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

func TestRecallNoResults(t *testing.T) {
	dbClient := newTestDB(t)

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "something totally unique",
		"project": "nonexistent",
	}))

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No matching memories") {
		t.Errorf("expected 'No matching memories', got: %s", text)
	}
}

// ---------- Remember ----------

func TestRememberToolDefinition(t *testing.T) {
	r := &Remember{DB: nil, Embedder: nil}
	tool := r.Tool()
	if tool.Name != "remember" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "remember")
	}
}

func TestRememberMissingContent(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for missing content")
	}
}

func TestRememberMissingProject(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "test",
	}))
	if !result.IsError {
		t.Error("expected error for missing project")
	}
}

func TestRememberSuccess(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "new memory about compute cluster setup",
		"project": "infra",
		"type":    "memory",
		"tags":    []string{"compute", "infra"},
		"speaker": "user",
		"area":    "infrastructure",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Stored memory") {
		t.Errorf("expected 'Stored memory', got: %s", text)
	}
}

func TestRememberSecretDetection(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "api_key=sk-1234567890abcdef1234567890abcdef",
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for secret content")
	}
}

// ---------- StoreConversation ----------

func TestStoreConversationToolDefinition(t *testing.T) {
	s := &StoreConversation{DB: nil, Embedder: nil}
	tool := s.Tool()
	if tool.Name != "store_conversation" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "store_conversation")
	}
}

func TestStoreConversation(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "discord",
		"summary": "Discussed infrastructure changes",
		"topics":  []string{"infrastructure", "networking"},
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

func TestStoreConversationMissingChannel(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := s.Handle(context.Background(), makeRequest(map[string]any{
		"summary": "test",
	}))
	if !result.IsError {
		t.Error("expected error for missing channel")
	}
}

func TestStoreConversationMissingSummary(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "discord",
	}))
	if !result.IsError {
		t.Error("expected error for missing summary")
	}
}

// ---------- Update ----------

func TestUpdateToolDefinition(t *testing.T) {
	u := &Update{DB: nil, Embedder: nil}
	tool := u.Tool()
	if tool.Name != "update_memory" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "update_memory")
	}
}

func TestUpdateMemory(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "original content", "proj", "memory")

	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":      m.ID,
		"content": "updated content",
		"summary": "new summary",
		"type":    "lesson",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	got, _ := dbClient.GetMemory(m.ID)
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want %q", got.Content, "updated content")
	}
	if got.Type != "lesson" {
		t.Errorf("Type = %q, want %q", got.Type, "lesson")
	}
}

func TestUpdateMemoryMissingID(t *testing.T) {
	dbClient := newTestDB(t)
	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for missing id")
	}
}

func TestUpdateMemoryNotFound(t *testing.T) {
	dbClient := newTestDB(t)
	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
	if !result.IsError {
		t.Error("expected error for non-existent memory")
	}
}

func TestUpdateMemoryTags(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "tag update test", "proj", "memory")

	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	_, _ = u.Handle(context.Background(), makeRequest(map[string]any{
		"id":   m.ID,
		"tags": []any{"new-tag-1", "new-tag-2"},
	}))

	tags, _ := dbClient.GetTags(m.ID)
	if len(tags) != 2 {
		t.Errorf("got %d tags, want 2", len(tags))
	}
}

// ---------- detectSecrets ----------

func TestDetectSecrets(t *testing.T) {
	tests := []struct {
		content string
		hasWarn bool
	}{
		{"api_key=abc123", true},
		{"password: mysecret", true},
		{"Bearer eyJhbGciOiJIUzI1NiJ9", true},
		{"ghp_abcdefghijklmnopqrstuvwxyz1234567890", true},
		{"sk-abcdefghijklmnopqrstuvwxyz123456", true},
		{"-----BEGIN RSA PRIVATE KEY-----", true},
		{"normal memory content", false},
		{"the api is working fine", false},
		{"password reset instructions for the user portal", false},
	}

	for _, tt := range tests {
		result := detectSecrets(tt.content)
		if tt.hasWarn && result == "" {
			t.Errorf("expected warning for %q, got none", tt.content)
		}
		if !tt.hasWarn && result != "" {
			t.Errorf("unexpected warning for %q: %s", tt.content, result)
		}
	}
}

// ---------- formatConversation ----------

func TestFormatConversation(t *testing.T) {
	result := formatConversation("discord", "sess1", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", 5,
		"Test summary", []string{"topic1"}, []string{"decision1"}, []string{"action1"})

	checks := []string{
		"Conversation on discord",
		"(session: sess1)",
		"Time: 2026-01-01T00:00:00Z to 2026-01-01T01:00:00Z",
		"Turns: 5",
		"Topics: topic1",
		"Test summary",
		"Decisions:",
		"- decision1",
		"Action Items:",
		"- action1",
	}
	for _, want := range checks {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in:\n%s", want, result)
		}
	}
}

func TestFormatConversationMinimal(t *testing.T) {
	result := formatConversation("mcp", "", "", "", 0, "Just a summary", nil, nil, nil)
	if !strings.Contains(result, "Conversation on mcp") {
		t.Error("missing channel header")
	}
	if !strings.Contains(result, "Just a summary") {
		t.Error("missing summary")
	}
	if strings.Contains(result, "session:") {
		t.Error("should not include session when empty")
	}
}
