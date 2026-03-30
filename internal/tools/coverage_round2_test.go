package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// ---------- IndexSession: SetTags error for turn tags (line 135-137) ----------

func TestR2_IndexSessionSetTagsErrorOnTurn(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags broken")}
	tool := &IndexSession{DB: store, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "a single conversation turn for tags error test"},
	}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-tags-err",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// SetTags error on turns is logged but not fatal
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexSession: summary SaveMemory error (line 183-185) ----------

type r2SaveMemoryCountStore struct {
	db.Store
	callCount     int
	failAfter     int
	setTagsResult error
}

func (s *r2SaveMemoryCountStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	s.callCount++
	if s.callCount > s.failAfter {
		return nil, fmt.Errorf("save failed after %d calls", s.failAfter)
	}
	return s.Store.SaveMemory(m)
}

func (s *r2SaveMemoryCountStore) SetTags(memoryID string, tags []string) error {
	if s.setTagsResult != nil {
		return s.setTagsResult
	}
	return s.Store.SetTags(memoryID, tags)
}

func TestR2_IndexSessionSummarySaveError(t *testing.T) {
	dbClient := newTestDB(t)
	// Fail SaveMemory on the 2nd call (the summary), let the 1st (turn) succeed
	store := &r2SaveMemoryCountStore{Store: dbClient, failAfter: 1}
	tool := &IndexSession{DB: store, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "conversation for summary save error test"},
	}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-summary-err",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Summary save error is non-fatal
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexSession: summary SetTags error (line 190-192) ----------

func TestR2_IndexSessionSummarySetTagsError(t *testing.T) {
	dbClient := newTestDB(t)
	// Let everything succeed except SetTags
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("summary tags broken")}
	tool := &IndexSession{DB: store, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "conversation for summary tags error test"},
	}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-sumtags-err",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexTurn: SetTags error (line 119-121) ----------

func TestR2_IndexTurnSetTagsError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags broken")}
	tool := &IndexTurn{DB: store, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": "content that will fail on SetTags in index_turn",
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for SetTags failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "setting tags") {
		t.Errorf("expected 'setting tags' in error, got: %s", text)
	}
}

// ---------- Ingest: dedup Filter error (line 53-55) ----------
// The dedup.Filter method never returns an error (it logs and continues),
// so ingest.go:53-55 is unreachable with the current Deduplicator implementation.

// ---------- Ingest: SetTags error (line 122-124) ----------

func TestR2_IngestSetTagsError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags broken")}
	tool := &IngestConversation{DB: store, Embedder: &mockEmbedder{}}

	content := `User: How do I configure proxmox?
Assistant: You need to set up the network bridges first.`

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": content,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// SetTags error is logged but doesn't stop ingest
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- LinkMemories: CreateLink error (line 55-57) ----------

func TestR2_LinkMemoriesCreateLinkError(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "link source memory for r2", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "link target memory for r2", "proj", "memory")

	store := &errStore{Store: dbClient, createLinkErr: fmt.Errorf("link table locked")}
	tool := &LinkMemories{DB: store}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m1.ID,
		"to_id":    m2.ID,
		"relation": "related_to",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for CreateLink failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "creating link") {
		t.Errorf("expected 'creating link' in error, got: %s", text)
	}
}

// ---------- GetRelated: GetLinks error at depth>1 (line 127-129) ----------

type r2GetLinksAfterTraverseFailStore struct {
	db.Store
	traverseDone bool
}

func (g *r2GetLinksAfterTraverseFailStore) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
	g.traverseDone = true
	return g.Store.TraverseGraph(ctx, startID, maxDepth)
}

func (g *r2GetLinksAfterTraverseFailStore) GetLinks(ctx context.Context, memoryID string, direction string) ([]*db.MemoryLink, error) {
	if g.traverseDone {
		return nil, fmt.Errorf("get links failed after traverse")
	}
	return g.Store.GetLinks(ctx, memoryID, direction)
}

func TestR2_GetRelatedDepthGreaterThanOneGetLinksError(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "root memory for traversal r2", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "neighbor memory for traversal r2", "proj", "memory")

	_, err := dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	store := &r2GetLinksAfterTraverseFailStore{Store: dbClient}
	tool := &GetRelated{DB: store}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for GetLinks failure at depth>1")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "getting links") {
		t.Errorf("expected 'getting links' in error, got: %s", text)
	}
}

// ---------- GetRelated: GetMemory error / skip deleted (line 137-138) ----------
// The memory_links table has ON DELETE CASCADE, so deleting a memory also deletes
// its links. We need to use a mock store that returns links pointing to a non-existent
// memory to exercise the continue on GetMemory error.

type r2GetMemoryFailStore struct {
	db.Store
	failID string
}

func (g *r2GetMemoryFailStore) GetMemory(id string) (*db.Memory, error) {
	if id == g.failID {
		return nil, fmt.Errorf("memory not found (simulated)")
	}
	return g.Store.GetMemory(id)
}

func TestR2_GetRelatedSkipDeletedMemory(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "root memory r2", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "neighbor to fail GetMemory r2", "proj", "memory")

	_, err := dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Use a wrapper that fails GetMemory for m2 (simulating a deleted memory
	// whose link still exists somehow)
	store := &r2GetMemoryFailStore{Store: dbClient, failID: m2.ID}
	tool := &GetRelated{DB: store}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     1,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Remember: SetTags error (line 154-156) ----------

func TestR2_RememberSetTagsError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags fail")}
	tool := &Remember{DB: store, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "memory that will fail on tags r2",
		"project": "proj",
		"tags":    []string{"infra"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for SetTags failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "setting tags") {
		t.Errorf("expected 'setting tags' in error, got: %s", text)
	}
}

// ---------- Remember: contradiction detection error (line 167-169) ----------

type r2SearchFailStore struct {
	db.Store
}

func (s *r2SearchFailStore) SearchMemories(_ []float32, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, fmt.Errorf("search broken")
}

func TestR2_RememberContradictionDetectionError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &r2SearchFailStore{Store: dbClient}
	tool := &Remember{DB: store, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "content that triggers broken contradiction check r2",
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Contradiction error is non-fatal, should still succeed
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Stored memory") {
		t.Errorf("expected 'Stored memory' in result, got: %s", text)
	}
}

// ---------- StoreConversation: SetTags error (line 102-105) ----------

func TestR2_StoreConversationSetTagsError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, setTagsErr: fmt.Errorf("tags broken")}
	tool := &StoreConversation{DB: store, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "discord",
		"summary": "test conversation r2",
		"topics":  []string{"homelab"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "warning") {
		t.Errorf("expected 'warning' in result text, got: %s", text)
	}
}

// ---------- RecentConversations: ListMemories error ----------

func TestR2_RecentConversationsListError(t *testing.T) {
	dbClient := newTestDB(t)
	store := &errStore{Store: dbClient, listErr: fmt.Errorf("db offline")}
	tool := &RecentConversations{DB: store}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for ListMemories failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "listing conversations") {
		t.Errorf("expected 'listing conversations' in error, got: %s", text)
	}
}

// ---------- RecentConversations: since filter date parsing edge case (line 78) ----------
// When the since filter is applied, conversations are filtered by parsing
// their CreatedAt field. If parsing fails, the memory is skipped (continue).
// With real SQLite, CreatedAt is always parseable. We use a mock store to
// inject a memory with a bad CreatedAt.

type r2BadCreatedAtStore struct {
	db.Store
	real *db.Client
}

func (b *r2BadCreatedAtStore) ListMemories(filter *db.MemoryFilter) ([]*db.Memory, error) {
	memories, err := b.real.ListMemories(filter)
	if err != nil {
		return nil, err
	}
	// Corrupt the CreatedAt of the first memory
	if len(memories) > 0 {
		memories[0].CreatedAt = "not-a-valid-datetime"
	}
	return memories, nil
}

func (b *r2BadCreatedAtStore) GetTags(memoryID string) ([]string, error) {
	return b.real.GetTags(memoryID)
}

func TestR2_RecentConversationsSinceFilterBadCreatedAt(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed conversations
	for i := 0; i < 3; i++ {
		mm := seedTestMemory(t, dbClient,
			fmt.Sprintf("r2 conversation since bad created_at %d with padding content", i),
			"proj", "conversation")
		dbClient.SetTags(mm.ID, []string{"conversation"})
	}

	store := &r2BadCreatedAtStore{Store: dbClient, real: dbClient}
	tool := &RecentConversations{DB: store}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2000-01-01T00:00:00Z",
		"limit": 10,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	// One memory should be skipped due to bad CreatedAt, rest should be returned
}

// Also test the truncation path: many conversations after since > limit

func TestR2_RecentConversationsSinceFilterTruncation(t *testing.T) {
	dbClient := newTestDB(t)

	for i := 0; i < 8; i++ {
		mm := seedTestMemory(t, dbClient,
			fmt.Sprintf("r2 conversation since trunc %d with unique padding content", i),
			"proj", "conversation")
		dbClient.SetTags(mm.ID, []string{"conversation"})
	}

	tool := &RecentConversations{DB: dbClient}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2000-01-01T00:00:00Z",
		"limit": 3,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexSession: summary embed error (line 166-168) ----------

type r2CountingEmbedder struct {
	callCount int
	failAfter int
}

func (c *r2CountingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	c.callCount++
	if c.callCount > c.failAfter {
		return nil, fmt.Errorf("embed failed after %d calls", c.failAfter)
	}
	emb := make([]float32, 384)
	emb[0] = float32(c.callCount) / 100.0
	return emb, nil
}

func (c *r2CountingEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for range texts {
		e, err := c.Embed(context.Background(), "")
		if err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, nil
}

func (c *r2CountingEmbedder) Dimensions() int { return 384 }

func TestR2_IndexSessionSummaryEmbedError(t *testing.T) {
	dbClient := newTestDB(t)
	// Fail embed on the 2nd call (the summary), let the 1st (turn) succeed
	emb := &r2CountingEmbedder{failAfter: 1}
	tool := &IndexSession{DB: dbClient, Embedder: emb}

	turns := []map[string]any{
		{"role": "user", "content": "conversation for summary embed error test r2"},
	}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-emb-err-r2",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Summary embed error is non-fatal
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}
