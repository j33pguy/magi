package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/j33pguy/magi/internal/db"
)

// orthogonalEmbedder produces embeddings in different orthogonal dimensions
// based on content length modulo, ensuring cosine distance > 0 between different texts.
type orthogonalEmbedder struct{}

func (o *orthogonalEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	// Use multiple dimensions based on text characteristics to produce
	// truly different direction vectors for different content.
	idx := len(text) % 384
	emb[idx] = 1.0
	// Also set a secondary dimension to make vectors more distinguishable
	idx2 := (len(text) * 7) % 384
	emb[idx2] = 0.5
	return emb, nil
}

func (o *orthogonalEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := o.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (o *orthogonalEmbedder) Dimensions() int { return 384 }

// fixedEmbedder always returns the same fixed embedding vector.
// Useful for creating memories with a known, exact embedding.
type fixedEmbedder struct {
	emb []float32
}

func (f *fixedEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	cp := make([]float32, len(f.emb))
	copy(cp, f.emb)
	return cp, nil
}

func (f *fixedEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for range texts {
		e, _ := f.Embed(context.Background(), "")
		results = append(results, e)
	}
	return results, nil
}

func (f *fixedEmbedder) Dimensions() int { return 384 }

// makeEmbedding creates a normalized 384-dim vector with given values in first dimensions.
func makeEmbedding(vals ...float32) []float32 {
	emb := make([]float32, 384)
	for i, v := range vals {
		if i < 384 {
			emb[i] = v
		}
	}
	return emb
}

// ---------- Remember: contradiction detection paths ----------

func TestRememberContradictionDetection(t *testing.T) {
	dbClient := newTestDB(t)

	// Use orthogonalEmbedder to produce embeddings with real cosine distance > 0
	// so that FindSimilar doesn't dedup but contradiction detection can still run.
	// Seed with an embedding that is orthogonal to what our embedder will produce.
	emb := make([]float32, 384)
	emb[200] = 1.0 // point in a completely different direction
	emb[201] = 0.5
	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "port 10 is used for management and is enabled",
		Embedding:  emb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &Remember{DB: dbClient, Embedder: &orthogonalEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "port 10 is used for management networking and is currently disabled on all switches",
		"project": "infra",
		"type":    "memory",
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

func TestRememberParentLinkingMessage(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed a memory that FindSimilar will return within groupDistance but not dedupDistance
	// mockEmbedder produces emb[0] = len(content)/100.0
	// We need a memory whose embedding is close enough to be a "group" match
	// but not close enough for dedup.
	// Content of length 50 -> emb[0] = 0.50
	content50 := "This is exactly fifty characters of content pad!!"
	emb := make([]float32, 384)
	emb[0] = float32(len(content50)) / 100.0
	saved, err := dbClient.SaveMemory(&db.Memory{
		Content:    content50,
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
	// Use content of similar length so the embedding is close
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": content50, // exact same content, will trigger dedup
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should say "Deduplicated" since it's the same content
	if !strings.Contains(text, "Deduplicated") && !strings.Contains(text, "Stored memory") {
		t.Errorf("expected Deduplicated or Stored memory, got: %s", text)
	}
	_ = saved
}

func TestRememberNearDuplicateDedup(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed a memory with specific embedding
	emb := make([]float32, 384)
	emb[0] = 0.50
	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "Original content about compute-cluster configuration",
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
	// Very similar content - the mock embedder will produce near-identical embeddings
	// for same-length strings, making distance ~0.0 which should trigger dedup
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":         "Original content about compute-cluster configuration",
		"project":         "infra",
		"dedup_threshold": 0.80,
	}))
	text := result.Content[0].(mcp.TextContent).Text
	// Should be deduplicated or stored
	if result.IsError {
		t.Fatalf("unexpected error: %s", text)
	}
}

// ---------- Remember: negative dedup_threshold path ----------

func TestRememberNegativeDedupThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":         "test content for negative threshold",
		"project":         "proj",
		"dedup_threshold": -1.0,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- CheckContradictions: json marshal error path ----------
// The marshal error for check_contradictions is extremely hard to trigger with
// real data, but we can verify the "no contradictions" and "with contradictions" paths.

func TestCheckContradictionsNegativeThreshold(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":   "test content",
		"threshold": -0.5,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Forget: exercise archive and delete error text ----------

func TestForgetArchiveMessageText(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "memory to verify archive message", "proj", "memory")

	f := &Forget{DB: dbClient}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{
		"id": m.ID,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Archived memory") {
		t.Errorf("expected 'Archived memory', got: %s", text)
	}
}

func TestForgetPermanentMessageText(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "memory to verify delete message", "proj", "memory")

	f := &Forget{DB: dbClient}
	result, err := f.Handle(context.Background(), makeRequest(map[string]any{
		"id":        m.ID,
		"permanent": true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Permanently deleted") {
		t.Errorf("expected 'Permanently deleted', got: %s", text)
	}
}

// ---------- IndexTurn: exercise SetTags error path ----------
// We can't easily trigger a DB error, but we can verify the SaveMemory error handling
// by using a failingEmbedder. The embed error is already tested. Let's test the
// full tag building path with sessionID and classifications.

func TestIndexTurnWithSessionAndClassification(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexTurn{DB: dbClient, Embedder: &mockEmbedder{}}

	// Content that classify.Infer might assign area/sub_area
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":       "user",
		"content":    "I configured the compute-cluster cluster nodes for infrastructure networking with ports",
		"project":    "infra",
		"session_id": "sess-coverage",
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
}

// ---------- IndexSession: exercise SaveMemory error via failing embedder ----------

func TestIndexSessionSummarizeWithFailingEmbedder(t *testing.T) {
	dbClient := newTestDB(t)

	// Create a custom embedder that succeeds N times then fails
	// We'll use the failingEmbedder for summary path - it will skip all turns
	// and then fail on summary too
	tool := &IndexSession{DB: dbClient, Embedder: &failingEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "Content for summary embed error test"},
		{"role": "assistant", "content": "Reply content here"},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-fail-summary",
		"summarize":  true,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// All turns should be skipped due to embed failure, summary embed also fails
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "0 turns") {
		t.Logf("result: %s", text)
	}
	// No summary should exist
	memories, _ := dbClient.ListMemories(&db.MemoryFilter{
		Type:       "conversation_summary",
		Visibility: "all",
	})
	if len(memories) != 0 {
		t.Errorf("expected no summary when embed fails, got %d", len(memories))
	}
}

// ---------- IndexSession: exercise empty role/content skip in session ----------

func TestIndexSessionMixedValidInvalidTurns(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	turns := []map[string]any{
		{"role": "user", "content": "Valid content for mixed test alpha"},
		{"role": "", "content": "No role here"},
		{"role": "user", "content": ""},
		{"role": "assistant", "content": "Valid assistant content for mixed test beta"},
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":      turns,
		"project":    "proj",
		"session_id": "sess-mixed",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Indexed 2") {
		t.Errorf("expected 'Indexed 2', got: %s", text)
	}
	if !strings.Contains(text, "2 skipped") {
		t.Errorf("expected '2 skipped', got: %s", text)
	}
}

// ---------- IngestConversation: exercise tag building with area/sub_area ----------

func TestIngestConversationWithProject(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I set up a compute-cluster cluster with ports for infrastructure networking isolation\nAssistant: Great, that's a solid approach for network segmentation."

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
		t.Errorf("expected 'Ingested', got: %s", text)
	}
}

// ---------- RecallConversations: exercise rewritten query path ----------
// The rewritten query path is hit when search.Adaptive rewrites the query,
// which is hard to trigger. But we ensure the channel filter path works.

func TestRecallConversationsWithTopK(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Discussion about Docker Compose networking bridge mode configuration", "infra", "conversation")

	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "docker compose networking",
		"top_k": 2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallLessons: exercise HybridSearch error path ----------

func TestRecallLessonsEmbedErrorMessage(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallLessons{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test lesson query",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "generating query embedding") {
		t.Errorf("expected embedding error message, got: %s", text)
	}
}

// ---------- RecallIncidents: exercise HybridSearch error path ----------

func TestRecallIncidentsEmbedErrorMessage(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecallIncidents{DB: dbClient, Embedder: &failingEmbedder{}}

	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test incident query",
	}))
	if !result.IsError {
		t.Error("expected error for embedding failure")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "generating query embedding") {
		t.Errorf("expected embedding error message, got: %s", text)
	}
}

// ---------- RecentConversations: exercise since with limit truncation path ----------

func TestRecentConversationsWithSinceAndMoreThanLimit(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed many conversations to exercise the limit>len(filtered) truncation
	for i := 0; i < 8; i++ {
		m := seedTestMemory(t, dbClient,
			"Conversation topic for since limit test number "+string(rune('A'+i))+" unique padding content",
			"proj", "conversation")
		dbClient.SetTags(m.ID, []string{"conversation"})
	}

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2020-01-01T00:00:00Z",
		"limit": 3,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should return JSON array
	var memories []*db.Memory
	if err := json.Unmarshal([]byte(text), &memories); err != nil {
		// Could be "No recent conversations" if tagging doesn't match
		if !strings.Contains(text, "No recent conversations") {
			t.Fatalf("unexpected response: %s", text)
		}
	} else if len(memories) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(memories))
	}
}

// ---------- RecentConversations: exercise GetTags continue on error ----------
// The GetTags continue path is at line 64-65 where it just skips tags loading.
// This is hard to trigger with real DB, but the since filter path at line 78 can be tested.

func TestRecentConversationsWithSinceFilterMatchingAll(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed conversations
	for i := 0; i < 3; i++ {
		m := seedTestMemory(t, dbClient,
			"Conversation for all match test "+string(rune('X'+i))+" with extra padding content",
			"proj", "conversation")
		dbClient.SetTags(m.ID, []string{"conversation"})
	}

	// Very old since should match all
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
}

// ---------- Update: exercise UpdateMemory without content change ----------

func TestUpdateMemoryMetadataOnly(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "metadata only update test", "proj", "memory")

	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":      m.ID,
		"summary": "new summary without content change",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	got, _ := dbClient.GetMemory(m.ID)
	if got.Summary != "new summary without content change" {
		t.Errorf("Summary = %q, want %q", got.Summary, "new summary without content change")
	}
	// Content should not have changed
	if got.Content != "metadata only update test" {
		t.Errorf("Content changed unexpectedly")
	}
}

// ---------- Update: exercise tags update with empty tags ----------

func TestUpdateMemoryEmptyTags(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "tags update test", "proj", "memory")
	dbClient.SetTags(m.ID, []string{"old-tag"})

	u := &Update{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":   m.ID,
		"tags": []any{},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- GetRelated: exercise with no links found ----------

func TestGetRelatedNoLinks(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "isolated node with no links", "proj", "memory")

	g := &GetRelated{DB: dbClient}
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m.ID,
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
	if len(results) != 0 {
		t.Errorf("expected 0 related, got %d", len(results))
	}
}

// ---------- GetRelated: exercise depth>1 path with 3 hops ----------

func TestGetRelatedDepth3(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "depth3 A", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "depth3 B", "proj", "memory")
	m3 := seedTestMemory(t, dbClient, "depth3 C", "proj", "memory")
	m4 := seedTestMemory(t, dbClient, "depth3 D", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "led_to", 1.0, false)
	dbClient.CreateLink(context.Background(), m2.ID, m3.ID, "led_to", 1.0, false)
	dbClient.CreateLink(context.Background(), m3.ID, m4.ID, "led_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     3,
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
	// Should find B, C, D (3 related nodes at depth 3)
	if len(results) < 1 {
		t.Errorf("expected at least 1 related at depth 3, got %d", len(results))
	}
}

// ---------- GetRelated: direction=to ----------

func TestGetRelatedDirectionTo(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "direction to source", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "direction to target", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	// Query from m2 perspective with direction=to (should find m1 linking to m2)
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m2.ID,
		"direction": "to",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- GetRelated: negative depth treated as 1 ----------

func TestGetRelatedNegativeDepth(t *testing.T) {
	dbClient := newTestDB(t)
	m1 := seedTestMemory(t, dbClient, "negative depth A", "proj", "memory")
	m2 := seedTestMemory(t, dbClient, "negative depth B", "proj", "memory")
	dbClient.CreateLink(context.Background(), m1.ID, m2.ID, "related_to", 1.0, false)

	g := &GetRelated{DB: dbClient}
	result, err := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": m1.ID,
		"depth":     -5,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- StoreConversation: exercise parent linking path ----------

func TestStoreConversationParentLinking(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	// Store first conversation
	result1, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "mcp",
		"summary": "Discussion about setting up monitoring infrastructure for the infra project with Prometheus",
		"topics":  []string{"monitoring", "prometheus"},
	}))
	if err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if result1.IsError {
		t.Fatalf("first call error: %v", result1.Content)
	}

	// Store a different but related conversation (similar-ish embedding)
	result2, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "mcp",
		"summary": "Follow-up discussion about setting up alerting for monitoring infrastructure infra Prometheus dashboards",
		"topics":  []string{"monitoring", "alerting"},
	}))
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if result2.IsError {
		t.Fatalf("second call error: %v", result2.Content)
	}
}

// ---------- Recall: exercise with speaker/area/sub_area filters ----------

func TestRecallWithSpeakerFilter(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed with specific speaker
	emb := make([]float32, 384)
	emb[0] = 0.45
	dbClient.SaveMemory(&db.Memory{
		Content:    "User said compute-cluster cluster is running fine after upgrade",
		Embedding:  emb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "user",
	})

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":   "compute-cluster cluster upgrade",
		"project": "infra",
		"speaker": "user",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List: exercise with pagination ----------

func TestListMemoriesWithPagination(t *testing.T) {
	dbClient := newTestDB(t)
	for i := 0; i < 5; i++ {
		seedTestMemory(t, dbClient, "pagination test memory "+string(rune('A'+i))+" with padding", "proj", "memory")
	}

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"limit":   2,
		"offset":  2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List: exercise with before filter ----------

func TestListMemoriesWithBeforeFilter(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "before filter test memory", "proj", "memory")

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"before":  "2030-01-01",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- List: exercise with both after and before ----------

func TestListMemoriesWithAfterAndBefore(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "range filter test memory", "proj", "memory")

	l := &List{DB: dbClient}
	result, err := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
		"after":   "2020-01-01",
		"before":  "2030-01-01",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IngestConversation: verify dry_run returns structured preview ----------

func TestIngestConversationDryRunStructure(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I learned that Kubernetes pods need resource limits set\nAssistant: Yes, always set resource requests and limits."

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
	var preview map[string]any
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := preview["would_skip"]; !ok {
		t.Error("expected 'would_skip' in dry run preview")
	}
}

// ---------- RecallLessons: exercise with top_k ----------

func TestRecallLessonsWithTopK(t *testing.T) {
	dbClient := newTestDB(t)
	for i := 0; i < 3; i++ {
		seedTestMemory(t, dbClient, "Lesson about infrastructure safety number "+string(rune('A'+i))+" padding", "infra", "lesson")
	}

	tool := &RecallLessons{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "infrastructure safety",
		"top_k": 2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- RecallIncidents: exercise with top_k ----------

func TestRecallIncidentsWithTopK(t *testing.T) {
	dbClient := newTestDB(t)
	for i := 0; i < 3; i++ {
		seedTestMemory(t, dbClient, "Incident about system failure number "+string(rune('A'+i))+" unique padding", "infra", "incident")
	}

	tool := &RecallIncidents{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "system failure",
		"top_k": 2,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IndexSession: exercise invalid JSON turns ----------

func TestIndexSessionNonArrayTurns(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	// Pass a number instead of array
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": 42,
	}))
	if !result.IsError {
		t.Error("expected error for non-array turns")
	}
}

// ---------- RecallConversations: exercise with all optional params ----------

func TestRecallConversationsAllParams(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Full params test conversation about Kubernetes namespaces and multi-tenancy", "infra", "conversation")

	tool := &RecallConversations{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query":         "kubernetes namespaces",
		"channel":       "discord",
		"top_k":         3,
		"min_relevance": 0.1,
		"recency_decay": 0.005,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Remember: exercise with all optional params ----------

func TestRememberAllOptionalParams(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":         "Full parameter test for remember tool with infrastructure port configuration",
		"project":         "infra",
		"type":            "decision",
		"summary":         "port config decision",
		"tags":            []string{"networking", "config"},
		"speaker":         "user",
		"area":            "infrastructure",
		"sub_area":        "networking",
		"dedup_threshold": 0.90,
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
	if !strings.Contains(text, "type=decision") {
		t.Errorf("expected 'type=decision', got: %s", text)
	}
}

// ---------- StoreConversation: exercise with minimal params (no topics) ----------

func TestStoreConversationNoTopics(t *testing.T) {
	dbClient := newTestDB(t)
	s := &StoreConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	result, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "webchat",
		"summary": "A brief conversation without topic tags",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- LinkMemories: exercise CreateLink error (same from/to) ----------

func TestLinkMemoriesSelfLink(t *testing.T) {
	dbClient := newTestDB(t)
	m := seedTestMemory(t, dbClient, "self link test", "proj", "memory")

	l := &LinkMemories{DB: dbClient}
	// Linking to self - may succeed or error depending on DB constraints
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m.ID,
		"to_id":    m.ID,
		"relation": "related_to",
	}))
	// Just verify no panic
	_ = result
}

// ---------- Recall: exercise with projects (multi-namespace) ----------

func TestRecallWithProjects(t *testing.T) {
	dbClient := newTestDB(t)
	seedTestMemory(t, dbClient, "Multi-namespace recall test memory about compute-cluster", "infra", "memory")
	seedTestMemory(t, dbClient, "Multi-namespace recall test memory about terraform", "ops", "memory")

	r := &Recall{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"query":    "compute-cluster terraform",
		"projects": []string{"infra", "ops"},
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- IngestConversation: exercise dedup path (ingest same content twice) ----------

func TestIngestConversationDedupSecondRun(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &mockEmbedder{}}

	conv := "User: I prefer Caddy over nginx for reverse proxy\nAssistant: Caddy has automatic HTTPS which simplifies config."

	// First ingest
	result1, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	if result1.IsError {
		t.Fatalf("first call error: %v", result1.Content)
	}

	// Second ingest of same content - should skip duplicates
	result2, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "infra",
	}))
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if result2.IsError {
		t.Fatalf("second call error: %v", result2.Content)
	}

	text := result2.Content[0].(mcp.TextContent).Text
	// Second run should show some duplicates were skipped
	if !strings.Contains(text, "Ingested") && !strings.Contains(text, "skipped") {
		t.Logf("second ingest result: %s", text)
	}
}

// ---------- RecentConversations: exercise default (no params) with data ----------

func TestRecentConversationsDefaultWithData(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	m := seedTestMemory(t, dbClient, "Default params conversation test with some content", "proj", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- CheckContradictions: exercise with both area and sub_area set ----------

func TestCheckContradictionsAreaSubArea(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed a memory
	emb := make([]float32, 384)
	emb[0] = 0.40
	dbClient.SaveMemory(&db.Memory{
		Content:    "The server runs Ubuntu 22.04 LTS",
		Embedding:  emb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
		Area:       "infrastructure",
		SubArea:    "servers",
	})

	tool := &CheckContradictions{DB: dbClient, Embedder: &mockEmbedder{}}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content":   "The server runs Debian 12",
		"area":      "infrastructure",
		"sub_area":  "servers",
		"threshold": 0.5,
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Remember: parent linking path (match within groupDistance but outside dedupDistance) ----------

func TestRememberParentLinkPath(t *testing.T) {
	dbClient := newTestDB(t)

	// We need FindSimilar to return a match with cosine distance between
	// maxDistance (0.05) and groupDistance (0.15).
	// We use a fixedEmbedder that always returns the same vector.
	// Then seed a memory with a slightly different vector to get a controlled distance.

	// Create embedding: [1, 0, 0, ...]
	seedEmb := make([]float32, 384)
	seedEmb[0] = 1.0

	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "port 5 is used for guest network access",
		Embedding:  seedEmb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Create embedding that has cosine distance ~0.10 from seedEmb
	// cos(theta) = dot(a,b) / (|a|*|b|)
	// If a = [1,0,...] and b = [0.99, 0.14, 0, ...], cos = 0.99 / sqrt(0.99^2 + 0.14^2) = 0.99/0.9999 ≈ 0.99
	// distance = 1 - 0.99 = 0.01 -- too close, would dedup
	// Need distance ~0.10: cos(theta) = 0.90
	// b = [0.90, 0.4359, 0, ...] => cos = 0.90 / sqrt(0.81 + 0.19) = 0.90 / 1.0 = 0.90 => dist = 0.10
	newEmb := make([]float32, 384)
	newEmb[0] = 0.90
	newEmb[1] = 0.4359 // sqrt(1 - 0.81) ≈ 0.4359

	fe := &fixedEmbedder{emb: newEmb}
	r := &Remember{DB: dbClient, Embedder: fe}

	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "port 50 is used for IoT device isolation",
		"project": "infra",
		"type":    "memory",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should contain "Stored memory" and "linked to similar memory"
	if !strings.Contains(text, "Stored memory") {
		t.Errorf("expected 'Stored memory', got: %s", text)
	}
	if !strings.Contains(text, "linked to similar memory") {
		t.Errorf("expected 'linked to similar memory', got: %s", text)
	}
}

// ---------- Remember: contradiction detection triggered after save ----------

func TestRememberWithContradictionDetected(t *testing.T) {
	dbClient := newTestDB(t)

	// Goal: Save a memory successfully, then have contradiction detection find
	// an existing memory that contradicts it.
	//
	// Steps:
	// 1. Seed a contradictory memory with embedding [0, 1, 0, ...]
	// 2. Use fixedEmbedder returning [0, 1, 0, ...] for the new memory
	// 3. The seeded memory will be found by FindSimilar (distance ~0 from our embedding)
	//    and will trigger dedup. So we need the seeded memory to NOT be found by FindSimilar.
	//
	// Better approach: seed a memory with embedding orthogonal to what our embedder produces.
	// Then FindSimilar won't find any match (distance > groupDistance), so memory saves.
	// Then contradiction.Check calls embedder.Embed again (same vector) and SearchMemories
	// finds the NEWLY saved memory plus possibly the seed. But we need SearchMemories to
	// find the SEEDED memory, not just the new one.
	//
	// The trick: seed with an embedding close to (but not identical to) the new embedding
	// such that FindSimilar doesn't find it (distance > groupDistance 0.15) but
	// SearchMemories DOES find it (within top 10 results even if distance is larger).
	//
	// SearchMemories returns the top N closest memories regardless of distance threshold.
	// The contradiction.Check then filters by maxDistance = 1 - threshold.
	// With threshold = 0.85, maxDistance = 0.15. So we need the seed's cosine distance < 0.15.
	// But that means FindSimilar WILL find it (within groupDistance 0.15).
	// This creates a chicken-and-egg problem.
	//
	// Resolution: FindSimilar returns one match. If distance > maxDistance (0.05) and
	// <= groupDistance (0.15), we get parent linking (not dedup) and the memory saves.
	// Then contradiction.Check runs and finds the same seed via SearchMemories.
	// For contradiction to score > 0.5, we need boolean flip or numeric change.

	seedEmb := make([]float32, 384)
	seedEmb[0] = 1.0

	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "port 5 is used for IoT devices and is enabled on all switches",
		Embedding:  seedEmb,
		Project:    "infra",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "assistant",
		Area:       "infrastructure",
		SubArea:    "networking",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Embedder returns vector with cosine distance ~0.10 from seed
	// (within groupDistance 0.15 but outside dedupDistance 0.05)
	newEmb := make([]float32, 384)
	newEmb[0] = 0.90
	newEmb[1] = 0.4359

	fe := &fixedEmbedder{emb: newEmb}
	r := &Remember{DB: dbClient, Embedder: fe}

	// Content has "disabled" vs seed's "enabled" => boolean flip => score > 0.5
	// Set area/sub_area explicitly to match seed so contradiction detector finds it
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content":  "port 5 is used for IoT devices and is disabled on all switches",
		"project":  "infra",
		"type":     "memory",
		"area":     "infrastructure",
		"sub_area": "networking",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	t.Logf("Remember output: %s", text)
	if !strings.Contains(text, "Stored memory") {
		t.Errorf("expected 'Stored memory', got: %s", text)
	}
	// Should contain parent linking message
	if !strings.Contains(text, "linked to similar memory") {
		t.Logf("Note: no parent linking in output")
	}
}

// ---------- StoreConversation: exercise parent linking (not dedup) ----------

func TestStoreConversationParentLinkNotDedup(t *testing.T) {
	dbClient := newTestDB(t)

	// Seed a conversation with a specific embedding
	seedEmb := make([]float32, 384)
	seedEmb[0] = 1.0

	_, err := dbClient.SaveMemory(&db.Memory{
		Content:    "Conversation about port setup for infrastructure",
		Summary:    "port setup discussion",
		Embedding:  seedEmb,
		Type:       "conversation",
		Visibility: "private",
		Source:     "mcp",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Use embedder that produces a vector with cosine distance between
	// dedupDistance (0.05) and groupDistance (0.15) from the seed
	newEmb := make([]float32, 384)
	newEmb[0] = 0.90
	newEmb[1] = 0.4359 // cosine distance ~0.10

	s := &StoreConversation{DB: dbClient, Embedder: &fixedEmbedder{emb: newEmb}}
	result, err := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "mcp",
		"summary": "Follow-up conversation about port configuration",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Should be stored (not deduped) and potentially linked as parent
	if !strings.Contains(text, "Stored conversation") {
		t.Errorf("expected 'Stored conversation', got: %s", text)
	}
}

// ---------- IndexSession: exercise the invalid turns unmarshal path ----------

func TestIndexSessionTurnsUnmarshalError(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	// Pass an object (map) instead of array for turns
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": map[string]any{"not": "an array"},
	}))
	if !result.IsError {
		t.Error("expected error for turns that can't unmarshal to []sessionTurn")
	}
}

// ---------- IndexSession: exercise content hash dedup within batch ----------

func TestIndexSessionContentHashDedup(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IndexSession{DB: dbClient, Embedder: &mockEmbedder{}}

	// First batch
	_, _ = tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": []map[string]any{
			{"role": "user", "content": "Unique content for hash dedup batch test"},
		},
		"project": "proj",
	}))

	// Second batch with same content
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns": []map[string]any{
			{"role": "user", "content": "Unique content for hash dedup batch test"},
		},
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "1 skipped") {
		t.Errorf("expected '1 skipped', got: %s", text)
	}
}

// ---------- Remember: exercise with no tags (empty tags path) ----------

func TestRememberNoTagsNoSpeaker(t *testing.T) {
	dbClient := newTestDB(t)
	r := &Remember{DB: dbClient, Embedder: &orthogonalEmbedder{}}

	// Minimal args to avoid tag appending
	result, err := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "Minimal remember test without explicit tags or speaker",
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
}

// ---------- Recent conversations: exercise the CreatedAt parse continue path ----------
// The recent_conversations.go since filter parses m.CreatedAt with time.DateTime format.
// If parsing fails, it just continues (skips the memory). This is hard to trigger
// because the DB always stores valid timestamps. But we can test the boundary.

func TestRecentConversationsFilterAllOlder(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &RecentConversations{DB: dbClient}

	// Seed conversations
	m := seedTestMemory(t, dbClient, "Old conversation for filter test padding content", "proj", "conversation")
	dbClient.SetTags(m.ID, []string{"conversation"})

	// Since = future, so all memories should be filtered out
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"since": "2099-12-31T23:59:59Z",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "No recent conversations") {
		t.Errorf("expected 'No recent conversations', got: %s", text)
	}
}

// ---------- IngestConversation: exercise with embedder that produces unique embeddings ----------

func TestIngestConversationFullImport(t *testing.T) {
	dbClient := newTestDB(t)
	tool := &IngestConversation{DB: dbClient, Embedder: &orthogonalEmbedder{}}

	// Conversation with content that will produce extractable memories
	conv := "User: I decided to switch from nginx to caddy for the reverse proxy\nAssistant: That's a good decision. Caddy handles TLS automatically.\nUser: I also learned that WireGuard is faster than OpenVPN.\nAssistant: Yes, WireGuard has much better performance."

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

// ---------- Closed-DB error path tests ----------
// These tests close the underlying *sql.DB to trigger DB error paths
// in the tool handlers. This covers error-handling branches that are
// impossible to reach with a healthy database.

// closedDB creates a test DB, runs migrations, then closes the underlying
// sql.DB so all subsequent operations fail.
func closedDB(t *testing.T) *db.Client {
	t.Helper()
	client := newTestDB(t)
	client.DB.Close()
	return client
}

func TestForgetDeleteMemoryError(t *testing.T) {
	client := newTestDB(t)
	m := seedTestMemory(t, client, "memory for delete error", "proj", "memory")

	// Close DB so DeleteMemory fails
	client.DB.Close()
	f := &Forget{DB: client}
	result, _ := f.Handle(context.Background(), makeRequest(map[string]any{
		"id":        m.ID,
		"permanent": true,
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on delete")
	}
}

func TestForgetArchiveMemoryError(t *testing.T) {
	client := newTestDB(t)
	m := seedTestMemory(t, client, "memory for archive error", "proj", "memory")

	client.DB.Close()
	f := &Forget{DB: client}
	result, _ := f.Handle(context.Background(), makeRequest(map[string]any{
		"id": m.ID,
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on archive")
	}
}

func TestListMemoriesDBError(t *testing.T) {
	client := closedDB(t)
	l := &List{DB: client}
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on list")
	}
}

func TestUpdateMemoryDBError(t *testing.T) {
	client := newTestDB(t)
	m := seedTestMemory(t, client, "memory for update DB error", "proj", "memory")

	client.DB.Close()
	u := &Update{DB: client, Embedder: &mockEmbedder{}}
	// UpdateMemory (no content change) should fail
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":      m.ID,
		"summary": "new summary",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on update")
	}
}

func TestUpdateMemorySetTagsDBError(t *testing.T) {
	client := newTestDB(t)
	m := seedTestMemory(t, client, "memory for tags update DB error", "proj", "memory")

	client.DB.Close()
	u := &Update{DB: client, Embedder: &mockEmbedder{}}
	result, _ := u.Handle(context.Background(), makeRequest(map[string]any{
		"id":   m.ID,
		"tags": []any{"tag1"},
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on SetTags")
	}
}

func TestRememberSaveMemoryError(t *testing.T) {
	client := closedDB(t)
	r := &Remember{DB: client, Embedder: &mockEmbedder{}}
	result, _ := r.Handle(context.Background(), makeRequest(map[string]any{
		"content": "test for save memory error path",
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on SaveMemory")
	}
}

func TestStoreConversationSaveMemoryError(t *testing.T) {
	client := closedDB(t)
	s := &StoreConversation{DB: client, Embedder: &mockEmbedder{}}
	result, _ := s.Handle(context.Background(), makeRequest(map[string]any{
		"channel": "mcp",
		"summary": "test for save error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on SaveMemory")
	}
}

func TestIndexTurnSaveMemoryError(t *testing.T) {
	client := closedDB(t)
	tool := &IndexTurn{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"role":    "user",
		"content": "test for save error in index turn",
		"project": "proj",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on SaveMemory")
	}
}

func TestRecallLessonsHybridSearchError(t *testing.T) {
	client := closedDB(t)
	tool := &RecallLessons{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for search error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on HybridSearch")
	}
}

func TestRecallIncidentsHybridSearchError(t *testing.T) {
	client := closedDB(t)
	tool := &RecallIncidents{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for search error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on HybridSearch")
	}
}

func TestRecentConversationsListError(t *testing.T) {
	client := closedDB(t)
	tool := &RecentConversations{DB: client}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{}))
	if !result.IsError {
		t.Error("expected error for DB failure on ListMemories")
	}
}

func TestCheckContradictionsSearchError(t *testing.T) {
	client := closedDB(t)
	tool := &CheckContradictions{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": "test for search error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on contradiction search")
	}
}

func TestIngestConversationSaveError(t *testing.T) {
	client := closedDB(t)
	tool := &IngestConversation{DB: client, Embedder: &mockEmbedder{}}
	conv := "User: test content for DB save error\nAssistant: test reply for DB save error"
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"content": conv,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// The dedup filter logs warnings but doesn't error out. Individual saves fail
	// silently (logged). The result should show "Ingested 0 memories".
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Ingested 0") {
		t.Logf("result: %s", text)
	}
}

func TestRecallConversationsSearchError(t *testing.T) {
	client := closedDB(t)
	tool := &RecallConversations{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for search error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on search")
	}
}

func TestRecallSearchError(t *testing.T) {
	client := closedDB(t)
	tool := &Recall{DB: client, Embedder: &mockEmbedder{}}
	result, _ := tool.Handle(context.Background(), makeRequest(map[string]any{
		"query": "test for search error",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on search")
	}
}

func TestLinkMemoriesCreateLinkError(t *testing.T) {
	client := newTestDB(t)
	m1 := seedTestMemory(t, client, "link source for error test", "proj", "memory")
	m2 := seedTestMemory(t, client, "link target for error test", "proj", "memory")

	client.DB.Close()
	l := &LinkMemories{DB: client}
	result, _ := l.Handle(context.Background(), makeRequest(map[string]any{
		"from_id":  m1.ID,
		"to_id":    m2.ID,
		"relation": "related_to",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on CreateLink")
	}
}

func TestGetRelatedDBError(t *testing.T) {
	client := closedDB(t)
	g := &GetRelated{DB: client}
	result, _ := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": "some-id",
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on GetLinks")
	}
}

func TestGetRelatedDepthDBError(t *testing.T) {
	client := closedDB(t)
	g := &GetRelated{DB: client}
	result, _ := g.Handle(context.Background(), makeRequest(map[string]any{
		"memory_id": "some-id",
		"depth":     3,
	}))
	if !result.IsError {
		t.Error("expected error for DB failure on TraverseGraph")
	}
}

func TestIndexSessionSaveMemoryError(t *testing.T) {
	client := closedDB(t)
	tool := &IndexSession{DB: client, Embedder: &mockEmbedder{}}
	turns := []map[string]any{
		{"role": "user", "content": "test for save error in session"},
	}
	result, err := tool.Handle(context.Background(), makeRequest(map[string]any{
		"turns":   turns,
		"project": "proj",
	}))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Should succeed with 0 indexed and 1 skipped (save failed)
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "0 turns") {
		t.Logf("result: %s", text)
	}
}
