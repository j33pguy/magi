package db

import (
	"context"
	"testing"
	"time"
)

// ---------- BM25 Search ----------

func TestSQLiteBM25Search(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, err := c.SaveMemory(&Memory{
		Content:    "kubernetes pod crash loop backoff error",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "incident",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	_, err = c.SaveMemory(&Memory{
		Content:    "deployed new feature to production successfully",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	results, err := c.SearchMemoriesBM25("kubernetes crash", &MemoryFilter{Project: "proj", Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("SearchMemoriesBM25: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 BM25 result")
	}
	if results[0].Memory.Content != "kubernetes pod crash loop backoff error" {
		t.Errorf("top result = %q, want kubernetes crash memory", results[0].Memory.Content)
	}
}

func TestSQLiteBM25SearchNoResults(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, err := c.SaveMemory(&Memory{
		Content:    "hello world",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	results, err := c.SearchMemoriesBM25("nonexistent terms zzzzz", &MemoryFilter{Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("SearchMemoriesBM25: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// ---------- Hybrid Search ----------

func TestSQLiteHybridSearch(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb1 := make([]float32, 384)
	emb1[0] = 1.0
	emb2 := make([]float32, 384)
	emb2[1] = 1.0

	_, err := c.SaveMemory(&Memory{
		Content:    "proxmox cluster backup strategy",
		Embedding:  emb1,
		Project:    "homelab",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory 1: %v", err)
	}
	_, err = c.SaveMemory(&Memory{
		Content:    "vault cluster unsealer automation",
		Embedding:  emb2,
		Project:    "homelab",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory 2: %v", err)
	}

	results, err := c.HybridSearch(emb1, "proxmox backup", &MemoryFilter{Project: "homelab", Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 hybrid result")
	}
	// The proxmox memory should rank higher (matches both vector and BM25)
	if results[0].Memory.Content != "proxmox cluster backup strategy" {
		t.Errorf("top result = %q, want proxmox memory", results[0].Memory.Content)
	}
	if results[0].RRFScore <= 0 {
		t.Errorf("RRFScore should be > 0, got %f", results[0].RRFScore)
	}
}

func TestSQLiteHybridSearchDiagnosticBoost(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb := zeroEmbedding()
	emb[0] = 0.5

	_, err := c.SaveMemory(&Memory{
		Content:    "normal memory about deployment",
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}
	_, err = c.SaveMemory(&Memory{
		Content:    "incident memory about deployment failure",
		Embedding:  emb,
		Project:    "proj",
		Type:       "incident",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Using a diagnostic keyword should boost incident/lesson types
	results, err := c.HybridSearch(emb, "why did deployment fail error", &MemoryFilter{Project: "proj", Visibility: "all"}, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// Incident should be boosted to top
	if results[0].Memory.Type != "incident" {
		t.Errorf("expected incident type at top with diagnostic keywords, got %q", results[0].Memory.Type)
	}
}

func TestHybridSearchDefaultTopK(t *testing.T) {
	c := newTestSQLiteClient(t)

	for i := 0; i < 15; i++ {
		emb := make([]float32, 384)
		emb[0] = float32(i+1) / 100.0 // distinct embeddings to avoid NULL distance
		_, err := c.SaveMemory(&Memory{
			Content:    "memory number " + string(rune('A'+i)),
			Embedding:  emb,
			Project:    "proj",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			t.Fatalf("SaveMemory[%d]: %v", i, err)
		}
	}

	queryEmb := make([]float32, 384)
	queryEmb[0] = 0.5
	// topK=0 should default to 10
	results, err := c.HybridSearch(queryEmb, "memory", &MemoryFilter{Visibility: "all"}, 0)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) > 10 {
		t.Errorf("default topK should cap at 10, got %d", len(results))
	}
}

// ---------- FindSimilar ----------

func TestSQLiteFindSimilar(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb := make([]float32, 384)
	emb[0] = 1.0

	saved, err := c.SaveMemory(&Memory{
		Content:    "find me by similarity",
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	result, err := c.FindSimilar(emb, 0.5)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if result == nil {
		t.Fatal("expected a result, got nil")
	}
	if result.Memory.ID != saved.ID {
		t.Errorf("ID = %q, want %q", result.Memory.ID, saved.ID)
	}
}

func TestSQLiteFindSimilarNoMatch(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb1 := make([]float32, 384)
	emb1[0] = 1.0

	_, err := c.SaveMemory(&Memory{
		Content:    "far away memory",
		Embedding:  emb1,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	// Query with very different embedding, very low maxDistance
	emb2 := make([]float32, 384)
	emb2[383] = 1.0

	result, err := c.FindSimilar(emb2, 0.001)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for distant vector, got distance=%f", result.Distance)
	}
}

func TestSQLiteFindSimilarEmpty(t *testing.T) {
	c := newTestSQLiteClient(t)

	result, err := c.FindSimilar(zeroEmbedding(), 1.0)
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if result != nil {
		t.Error("expected nil result on empty DB")
	}
}

// ---------- ExistsWithContentHash ----------

func TestSQLiteExistsWithContentHash(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, err := c.SaveMemory(&Memory{
		Content:    "hashed content",
		Embedding:  zeroEmbedding(),
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if err := c.SetTags(saved.ID, []string{"hash:abc123"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	id, err := c.ExistsWithContentHash("abc123")
	if err != nil {
		t.Fatalf("ExistsWithContentHash: %v", err)
	}
	if id != saved.ID {
		t.Errorf("ID = %q, want %q", id, saved.ID)
	}
}

func TestSQLiteExistsWithContentHashNotFound(t *testing.T) {
	c := newTestSQLiteClient(t)

	id, err := c.ExistsWithContentHash("nonexistent")
	if err != nil {
		t.Fatalf("ExistsWithContentHash: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty ID, got %q", id)
	}
}

// ---------- Visibility Filtering ----------

func TestSQLiteVisibilityFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, err := c.SaveMemory(&Memory{Content: "private memory", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "private"})
	if err != nil {
		t.Fatalf("SaveMemory private: %v", err)
	}
	_, err = c.SaveMemory(&Memory{Content: "internal memory", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	if err != nil {
		t.Fatalf("SaveMemory internal: %v", err)
	}
	_, err = c.SaveMemory(&Memory{Content: "public memory", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "public"})
	if err != nil {
		t.Fatalf("SaveMemory public: %v", err)
	}

	// Default: exclude private
	list, err := c.ListMemories(&MemoryFilter{Project: "proj"})
	if err != nil {
		t.Fatalf("ListMemories default: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("default visibility: got %d, want 2 (exclude private)", len(list))
	}

	// Visibility=all: include everything
	list, err = c.ListMemories(&MemoryFilter{Project: "proj", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories all: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("visibility=all: got %d, want 3", len(list))
	}

	// Visibility=public: only public
	list, err = c.ListMemories(&MemoryFilter{Project: "proj", Visibility: "public"})
	if err != nil {
		t.Fatalf("ListMemories public: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("visibility=public: got %d, want 1", len(list))
	}
}

// ---------- Default Visibility ----------

func TestSQLiteDefaultVisibility(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, err := c.SaveMemory(&Memory{
		Content:   "no visibility set",
		Embedding: zeroEmbedding(),
		Project:   "proj",
		Type:      "memory",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	got, err := c.GetMemory(saved.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Visibility != "internal" {
		t.Errorf("default visibility = %q, want %q", got.Visibility, "internal")
	}
}

// ---------- Project Filtering ----------

func TestSQLiteMultiProjectFilter(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "a1", Embedding: zeroEmbedding(), Project: "agent:gilfoyle", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "a2", Embedding: zeroEmbedding(), Project: "crew:shared", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "a3", Embedding: zeroEmbedding(), Project: "agent:dinesh", Type: "memory", Visibility: "internal"})

	list, err := c.ListMemories(&MemoryFilter{
		Projects:   []string{"agent:gilfoyle", "crew:shared"},
		Visibility: "all",
	})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("multi-project filter: got %d, want 2", len(list))
	}
}

// ---------- Tag Filtering ----------

func TestSQLiteTagFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)

	m1, _ := c.SaveMemory(&Memory{Content: "tagged one", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "tagged two", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "untagged", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})

	_ = c.SetTags(m1.ID, []string{"important", "homelab"})
	_ = c.SetTags(m2.ID, []string{"important", "work"})

	list, err := c.ListMemories(&MemoryFilter{
		Project:    "proj",
		Tags:       []string{"important"},
		Visibility: "all",
	})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("tag filter: got %d, want 2", len(list))
	}
}

// ---------- Tag Replacement ----------

func TestSQLiteTagReplacement(t *testing.T) {
	c := newTestSQLiteClient(t)

	saved, _ := c.SaveMemory(&Memory{Content: "tag test", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})

	_ = c.SetTags(saved.ID, []string{"alpha", "beta"})
	tags, _ := c.GetTags(saved.ID)
	if len(tags) != 2 {
		t.Fatalf("initial tags: got %d, want 2", len(tags))
	}

	// Replace tags
	_ = c.SetTags(saved.ID, []string{"gamma"})
	tags, _ = c.GetTags(saved.ID)
	if len(tags) != 1 {
		t.Fatalf("replaced tags: got %d, want 1", len(tags))
	}
	if tags[0] != "gamma" {
		t.Errorf("tag = %q, want %q", tags[0], "gamma")
	}

	// Clear tags
	_ = c.SetTags(saved.ID, nil)
	tags, _ = c.GetTags(saved.ID)
	if len(tags) != 0 {
		t.Errorf("cleared tags: got %d, want 0", len(tags))
	}
}

// ---------- Time Filtering ----------

func TestSQLiteTimeFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "recent", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})

	// Note: created_at is stored as time.DateTime ("2006-01-02 15:04:05")
	// but appendTimeConditions formats AfterTime/BeforeTime as RFC3339.
	// SQLite does string comparison, so this only works reliably when the
	// time difference is large enough that format differences don't matter.

	// After a time far in the future should return nothing
	future := time.Now().Add(24 * time.Hour)
	list, err := c.ListMemories(&MemoryFilter{
		Project:    "proj",
		AfterTime:  &future,
		Visibility: "all",
	})
	if err != nil {
		t.Fatalf("ListMemories after future: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("after future: got %d, want 0", len(list))
	}

	// Before a time far in the future should return the memory
	list, err = c.ListMemories(&MemoryFilter{
		Project:    "proj",
		BeforeTime: &future,
		Visibility: "all",
	})
	if err != nil {
		t.Fatalf("ListMemories before future: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("before future: got %d, want 1", len(list))
	}
}

// ---------- TraverseGraph ----------

func TestSQLiteTraverseGraph(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "node A", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "node B", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	m3, _ := c.SaveMemory(&Memory{Content: "node C", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})

	// A -> B -> C
	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "led_to", 1.0, false)
	_, _ = c.CreateLink(ctx, m2.ID, m3.ID, "led_to", 1.0, false)

	// Depth 1: from A should find B
	ids, err := c.TraverseGraph(ctx, m1.ID, 1)
	if err != nil {
		t.Fatalf("TraverseGraph depth 1: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("depth 1: got %d nodes, want 1", len(ids))
	}

	// Depth 2: from A should find B and C
	ids, err = c.TraverseGraph(ctx, m1.ID, 2)
	if err != nil {
		t.Fatalf("TraverseGraph depth 2: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("depth 2: got %d nodes, want 2", len(ids))
	}

	// Default depth (0 -> 1)
	ids, err = c.TraverseGraph(ctx, m1.ID, 0)
	if err != nil {
		t.Fatalf("TraverseGraph depth 0: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("default depth: got %d nodes, want 1", len(ids))
	}
}

// ---------- GetGraphData ----------

func TestSQLiteGetGraphData(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "graph node 1", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "graph node 2", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "related_to", 0.8, true)

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("memories: got %d, want 2", len(memories))
	}
	if len(links) != 1 {
		t.Errorf("links: got %d, want 1", len(links))
	}
	if links[0].Auto != true {
		t.Error("expected auto=true on link")
	}
}

func TestSQLiteGetGraphDataEmpty(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	memories, links, err := c.GetGraphData(ctx, 10)
	if err != nil {
		t.Fatalf("GetGraphData: %v", err)
	}
	if len(memories) != 0 {
		t.Errorf("expected 0 memories, got %d", len(memories))
	}
	if links != nil {
		t.Errorf("expected nil links, got %d", len(links))
	}
}

// ---------- Link Directions ----------

func TestSQLiteLinkDirections(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	m1, _ := c.SaveMemory(&Memory{Content: "from node", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	m2, _ := c.SaveMemory(&Memory{Content: "to node", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	_, _ = c.CreateLink(ctx, m1.ID, m2.ID, "caused_by", 1.0, false)

	// "from" direction: m1 has outbound links
	links, _ := c.GetLinks(ctx, m1.ID, "from")
	if len(links) != 1 {
		t.Errorf("from m1: got %d, want 1", len(links))
	}

	// "to" direction: m1 has no inbound links
	links, _ = c.GetLinks(ctx, m1.ID, "to")
	if len(links) != 0 {
		t.Errorf("to m1: got %d, want 0", len(links))
	}

	// "to" direction: m2 has inbound links
	links, _ = c.GetLinks(ctx, m2.ID, "to")
	if len(links) != 1 {
		t.Errorf("to m2: got %d, want 1", len(links))
	}

	// "both" direction: m1
	links, _ = c.GetLinks(ctx, m1.ID, "both")
	if len(links) != 1 {
		t.Errorf("both m1: got %d, want 1", len(links))
	}
}

// ---------- DeleteLink not found ----------

func TestSQLiteDeleteLinkNotFound(t *testing.T) {
	c := newTestSQLiteClient(t)
	ctx := context.Background()

	err := c.DeleteLink(ctx, "nonexistent-link-id")
	if err == nil {
		t.Error("expected error deleting non-existent link")
	}
}

// ---------- UpdateMemory with embedding ----------

func TestSQLiteUpdateMemoryWithEmbedding(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb1 := zeroEmbedding()
	saved, err := c.SaveMemory(&Memory{
		Content:    "original content",
		Summary:    "original summary",
		Embedding:  emb1,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
	})
	if err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	emb2 := make([]float32, 384)
	emb2[0] = 1.0
	saved.Content = "updated content"
	saved.Summary = "updated summary"
	saved.Embedding = emb2
	saved.Type = "lesson"
	saved.TokenCount = 42

	if err := c.UpdateMemory(saved); err != nil {
		t.Fatalf("UpdateMemory: %v", err)
	}

	got, err := c.GetMemory(saved.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "updated content" {
		t.Errorf("Content = %q, want %q", got.Content, "updated content")
	}
	if got.Summary != "updated summary" {
		t.Errorf("Summary = %q, want %q", got.Summary, "updated summary")
	}
	if got.Type != "lesson" {
		t.Errorf("Type = %q, want %q", got.Type, "lesson")
	}
}

// ---------- ListMemories pagination ----------

func TestSQLiteListMemoriesPagination(t *testing.T) {
	c := newTestSQLiteClient(t)

	for i := 0; i < 5; i++ {
		_, _ = c.SaveMemory(&Memory{
			Content:    "paginated " + string(rune('A'+i)),
			Embedding:  zeroEmbedding(),
			Project:    "proj",
			Type:       "memory",
			Visibility: "internal",
		})
	}

	// Limit 2
	list, err := c.ListMemories(&MemoryFilter{Project: "proj", Limit: 2, Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories limit 2: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("limit 2: got %d, want 2", len(list))
	}

	// Offset 3
	list, err = c.ListMemories(&MemoryFilter{Project: "proj", Limit: 10, Offset: 3, Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories offset 3: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("offset 3: got %d, want 2", len(list))
	}
}

// ---------- Type Filtering ----------

func TestSQLiteTypeFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "a memory", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "an incident", Embedding: zeroEmbedding(), Project: "proj", Type: "incident", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "a lesson", Embedding: zeroEmbedding(), Project: "proj", Type: "lesson", Visibility: "internal"})

	list, err := c.ListMemories(&MemoryFilter{Project: "proj", Type: "incident", Visibility: "all"})
	if err != nil {
		t.Fatalf("ListMemories type=incident: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("type=incident: got %d, want 1", len(list))
	}
	if list[0].Content != "an incident" {
		t.Errorf("Content = %q, want %q", list[0].Content, "an incident")
	}
}

// ---------- Speaker/Area/SubArea Filtering ----------

func TestSQLiteTaxonomyFiltering(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "j33p work", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal", Speaker: "j33p", Area: "work", SubArea: "magi"})
	_, _ = c.SaveMemory(&Memory{Content: "gilfoyle homelab", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal", Speaker: "gilfoyle", Area: "homelab", SubArea: "proxmox"})

	// Filter by speaker
	list, _ := c.ListMemories(&MemoryFilter{Project: "proj", Speaker: "j33p", Visibility: "all"})
	if len(list) != 1 {
		t.Errorf("speaker=j33p: got %d, want 1", len(list))
	}

	// Filter by area
	list, _ = c.ListMemories(&MemoryFilter{Project: "proj", Area: "homelab", Visibility: "all"})
	if len(list) != 1 {
		t.Errorf("area=homelab: got %d, want 1", len(list))
	}

	// Filter by sub_area
	list, _ = c.ListMemories(&MemoryFilter{Project: "proj", SubArea: "proxmox", Visibility: "all"})
	if len(list) != 1 {
		t.Errorf("sub_area=proxmox: got %d, want 1", len(list))
	}
}

// ---------- hasDiagnosticKeywords ----------

func TestHasDiagnosticKeywords(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"why is the server down", true},
		{"fix the deployment", true},
		{"error in kubernetes", true},
		{"crashed pod", true},
		{"what is kubernetes", false},
		{"deploy new version", false},
		{"update configuration", false},
		{"", false},
		{"DEBUG mode enabled", true},
		{"troubleshoot network", true},
	}
	for _, tt := range tests {
		got := hasDiagnosticKeywords(tt.query)
		if got != tt.want {
			t.Errorf("hasDiagnosticKeywords(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

// ---------- float32sToBytes ----------

func TestFloat32sToBytes(t *testing.T) {
	// nil input
	if b := float32sToBytes(nil); b != nil {
		t.Error("expected nil for nil input")
	}

	// empty slice
	b := float32sToBytes([]float32{})
	if len(b) != 0 {
		t.Errorf("expected empty bytes, got %d", len(b))
	}

	// known values
	b = float32sToBytes([]float32{1.0, 0.0})
	if len(b) != 8 {
		t.Errorf("expected 8 bytes, got %d", len(b))
	}
}

// ---------- GetContextMemories with project filter ----------

func TestSQLiteGetContextMemoriesNoProject(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "no project context", Embedding: zeroEmbedding(), Project: "proj1", Type: "memory", Visibility: "internal"})
	_, _ = c.SaveMemory(&Memory{Content: "other project context", Embedding: zeroEmbedding(), Project: "proj2", Type: "memory", Visibility: "internal"})

	// Empty project = all projects
	memories, err := c.GetContextMemories("", 10)
	if err != nil {
		t.Fatalf("GetContextMemories: %v", err)
	}
	if len(memories) != 2 {
		t.Errorf("expected 2, got %d", len(memories))
	}
}

// ---------- GetContextMemories excludes private ----------

func TestSQLiteGetContextMemoriesExcludesPrivate(t *testing.T) {
	c := newTestSQLiteClient(t)

	_, _ = c.SaveMemory(&Memory{Content: "private context", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "private"})
	_, _ = c.SaveMemory(&Memory{Content: "internal context", Embedding: zeroEmbedding(), Project: "proj", Type: "memory", Visibility: "internal"})

	memories, err := c.GetContextMemories("proj", 10)
	if err != nil {
		t.Fatalf("GetContextMemories: %v", err)
	}
	if len(memories) != 1 {
		t.Errorf("expected 1 (exclude private), got %d", len(memories))
	}
}

// ---------- SearchMemories with filters ----------

func TestSQLiteSearchMemoriesWithFilters(t *testing.T) {
	c := newTestSQLiteClient(t)

	emb := make([]float32, 384)
	emb[0] = 1.0

	_, _ = c.SaveMemory(&Memory{Content: "internal search", Embedding: emb, Project: "proj", Type: "memory", Visibility: "internal", Speaker: "j33p"})
	_, _ = c.SaveMemory(&Memory{Content: "private search", Embedding: emb, Project: "proj", Type: "memory", Visibility: "private", Speaker: "j33p"})

	// Default: exclude private
	results, err := c.SearchMemories(emb, &MemoryFilter{Project: "proj"}, 10)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("default visibility: got %d, want 1", len(results))
	}

	// All: include private
	results, err = c.SearchMemories(emb, &MemoryFilter{Project: "proj", Visibility: "all"}, 10)
	if err != nil {
		t.Fatalf("SearchMemories: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("visibility=all: got %d, want 2", len(results))
	}
}
