package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// errStore wraps a real db.Store, injecting errors on selected methods.
// This lets us exercise error branches that require GetMemory to succeed
// while a subsequent operation (e.g. UpdateMemory, SetTags) fails.
type errStore struct {
	db.Store
	archiveErr    error
	deleteErr     error
	updateErr     error
	setTagsErr    error
	getTagsErr    error
	createLinkErr error
	listErr       error
}

func (e *errStore) ArchiveMemory(id string) error {
	if e.archiveErr != nil {
		return e.archiveErr
	}
	return e.Store.ArchiveMemory(id)
}

func (e *errStore) DeleteMemory(id string) error {
	if e.deleteErr != nil {
		return e.deleteErr
	}
	return e.Store.DeleteMemory(id)
}

func (e *errStore) UpdateMemory(m *db.Memory) error {
	if e.updateErr != nil {
		return e.updateErr
	}
	return e.Store.UpdateMemory(m)
}

func (e *errStore) SetTags(memoryID string, tags []string) error {
	if e.setTagsErr != nil {
		return e.setTagsErr
	}
	return e.Store.SetTags(memoryID, tags)
}

func (e *errStore) GetTags(memoryID string) ([]string, error) {
	if e.getTagsErr != nil {
		return nil, e.getTagsErr
	}
	return e.Store.GetTags(memoryID)
}

func (e *errStore) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*db.MemoryLink, error) {
	if e.createLinkErr != nil {
		return nil, e.createLinkErr
	}
	return e.Store.CreateLink(ctx, fromID, toID, relation, weight, auto)
}

func (e *errStore) ListMemories(filter *db.MemoryFilter) ([]*db.Memory, error) {
	if e.listErr != nil {
		return nil, e.listErr
	}
	return e.Store.ListMemories(filter)
}

// ---------- Forget: ArchiveMemory error with GetMemory succeeding (line 46-48) ----------

func TestForgetArchiveErrorViaErrStore(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "will fail to archive via errStore", "proj", "memory")

	store := &errStore{Store: dbClient, archiveErr: fmt.Errorf("disk full")}
	f := &Forget{DB: store}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{"id": m.ID}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for archive failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "archiving memory") {
		t.Errorf("expected 'archiving memory' in error, got: %s", text)
	}
}

// ---------- Forget: DeleteMemory error with GetMemory succeeding (line 40-42) ----------

func TestForgetPermanentDeleteErrorViaErrStore(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "will fail to delete via errStore", "proj", "memory")

	store := &errStore{Store: dbClient, deleteErr: fmt.Errorf("constraint violation")}
	f := &Forget{DB: store}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{
		"id":        m.ID,
		"permanent": true,
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for delete failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "deleting memory") {
		t.Errorf("expected 'deleting memory' in error, got: %s", text)
	}
}

// ---------- List: GetTags error path (line 79-81) ----------

func TestListGetTagsErrorPath(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "list tags error path test", "proj", "memory")

	store := &errStore{Store: dbClient, getTagsErr: fmt.Errorf("tags table locked")}
	l := &List{DB: store}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for GetTags failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "getting tags") {
		t.Errorf("expected 'getting tags' in error, got: %s", text)
	}
}

// ---------- List: invalid 'after' time param (line 46-48) ----------

func TestListInvalidAfterTimeParam(t *testing.T) {
	dbClient := newTestDB(t)
	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"after": "not-a-time",
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for invalid after")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "invalid 'after'") {
		t.Errorf("expected \"invalid 'after'\" in error, got: %s", text)
	}
}

// ---------- List: invalid 'before' time param (line 49-52) ----------

func TestListInvalidBeforeTimeParam(t *testing.T) {
	dbClient := newTestDB(t)
	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"before": "garbage",
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for invalid before")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "invalid 'before'") {
		t.Errorf("expected \"invalid 'before'\" in error, got: %s", text)
	}
}

// ---------- Update: UpdateMemory error with GetMemory succeeding (line 68-70) ----------

func TestUpdateMemoryUpdateError(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "update will fail via errStore", "proj", "memory")

	store := &errStore{Store: dbClient, updateErr: fmt.Errorf("readonly database")}
	u := &Update{DB: store, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":      m.ID,
		"summary": "new summary",
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for UpdateMemory failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "updating memory") {
		t.Errorf("expected 'updating memory' in error, got: %s", text)
	}
}

// ---------- Update: SetTags error with UpdateMemory succeeding (line 79-81) ----------

func TestUpdateMemorySetTagsErrorPath(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "tags will fail via errStore", "proj", "memory")

	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags table broken")}
	u := &Update{DB: store, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":   m.ID,
		"tags": []any{"new-tag"},
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for SetTags failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "updating tags") {
		t.Errorf("expected 'updating tags' in error, got: %s", text)
	}
}

// ---------- RecentConversations: GetTags error continue path (line 64-65) ----------

func TestRecentConversationsGetTagsContinue(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "conversation with broken tags path", "proj", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	store := &errStore{Store: dbClient, getTagsErr: fmt.Errorf("tags corrupted")}
	tool := &RecentConversations{DB: store}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	// GetTags error triggers continue, not a fatal error
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecentConversations: since filter truncation path (line 83-85) ----------

func TestRecentConversationsSinceTruncation(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed enough conversations that after since-filtering, len > limit
	for i := 0; i < 6; i++ {
		mm := seedTestMemory(t, dbClient,
			fmt.Sprintf("conversation for truncation test number %d with unique padding", i),
			"proj", "conversation")
		dbClient.SetTags(mm.ID, []string{"conversation"})
	}

	tool := &RecentConversations{DB: dbClient}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2000-01-01T00:00:00Z",
		"limit": 2,
	}))
	if err != nil {
		t.Fatalf("Handle returned Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// The truncation at line 83-85 should be exercised since 6 > 2
}

// ---------- Recall: invalid after param ----------

func TestRecallInvalidAfterTimeParam(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test",
		"after": "xyz",
	}))
	if !result.IsError {
		t.Fatal("expected error for invalid after")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "invalid 'after'") {
		t.Errorf("expected \"invalid 'after'\" error, got: %s", text)
	}
}

// ---------- Recall: invalid before param ----------

func TestRecallInvalidBeforeTimeParam(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":  "test",
		"before": "xyz",
	}))
	if !result.IsError {
		t.Fatal("expected error for invalid before")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "invalid 'before'") {
		t.Errorf("expected \"invalid 'before'\" error, got: %s", text)
	}
}

// ---------- List: with speaker, area, sub_area combined filters ----------

func TestListWithAllTaxonomyFilters(t *testing.T) {
	dbClient := newTestDB(t)
	emb := make([]float32, 384)
	emb[0] = 0.42
	dbClient.SaveMemory(&db.Memory{
		Content:    "taxonomy filter test memory with enough padding for embedding",
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "user",
		Area:       "homelab",
		SubArea:    "proxmox",
	})

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project":  "proj",
		"speaker":  "user",
		"area":     "homelab",
		"sub_area": "proxmox",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List: with tags filter ----------

func TestListWithTagsFilterPath(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "tagged memory for list tags filter path test", "proj", "memory")
	dbClient.SetTags(m.ID, []string{"infra", "networking"})

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"tags":    []string{"infra"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List: with relative time after filter ----------

func TestListWithRelativeAfterTime(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "relative time filter test memory content", "proj", "memory")

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"after":   "30d",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Recall: with projects multi-namespace filter ----------

func TestRecallWithMultipleProjects(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "multi-project recall test memory content about DNS config", "proj-a", "memory")
	seedTestMemory(t, dbClient, "another namespace DNS memory content test", "proj-b", "memory")

	tool := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":    "DNS configuration",
		"projects": []string{"proj-a", "proj-b"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}
