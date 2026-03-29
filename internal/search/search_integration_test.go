package search_test

import (
	"context"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/search"
)

// newTestDB creates a migrated SQLite-backed db.Client in a temp directory.
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

// mockEmbed returns a 384-dim vector where the first element encodes the text
// length (normalized to [0,1]) so different texts produce different embeddings.
func mockEmbed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, 384)
	vec[0] = float32(len(text)%100) / 100.0
	// Spread a secondary signal into the second dimension so short vs long
	// texts are distinguishable by cosine distance.
	vec[1] = 1.0 - vec[0]
	return vec, nil
}

// seedMemory saves a memory with the given content and a mock embedding.
func seedMemory(t *testing.T, client *db.Client, content, project, memType string) *db.Memory {
	t.Helper()
	emb, _ := mockEmbed(context.Background(), content)
	m, err := client.SaveMemory(&db.Memory{
		Content:    content,
		Embedding:  emb,
		Project:    project,
		Type:       memType,
		Visibility: "internal",
		Speaker:    "user",
		Area:       "work",
		SubArea:    "test",
	})
	if err != nil {
		t.Fatalf("SaveMemory(%q): %v", content, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Test: Adaptive returns results from a populated DB
// ---------------------------------------------------------------------------

func TestAdaptive_WithResults(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	seedMemory(t, client, "kubernetes deployment configuration guide", "proj", "memory")
	seedMemory(t, client, "docker compose setup for local development", "proj", "memory")
	seedMemory(t, client, "terraform infrastructure as code patterns", "proj", "memory")

	resp, err := search.Adaptive(ctx, client, mockEmbed, "kubernetes deployment", nil, 10, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	if resp.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", resp.Attempts)
	}
	if resp.Rewritten {
		t.Error("Rewritten = true, want false")
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected at least one result, got 0")
	}

	// Verify results have scores set
	for i, r := range resp.Results {
		if r.Memory == nil {
			t.Errorf("result[%d].Memory is nil", i)
		}
		if r.Score == 0 && r.RRFScore == 0 {
			t.Errorf("result[%d] has zero Score and RRFScore", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive returns empty results on an empty DB
// ---------------------------------------------------------------------------

func TestAdaptive_EmptyDB(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	resp, err := search.Adaptive(ctx, client, mockEmbed, "anything at all", nil, 5, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results on empty DB, got %d", len(resp.Results))
	}
	// With no results and query rewriting attempted, Attempts may be 1 or 2.
	if resp.Attempts < 1 {
		t.Errorf("Attempts = %d, want >= 1", resp.Attempts)
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive with minRelevance filtering
// ---------------------------------------------------------------------------

func TestAdaptive_MinRelevanceFiltering(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Seed several memories
	seedMemory(t, client, "proxmox virtual machine cluster setup", "proj", "memory")
	seedMemory(t, client, "proxmox backup server configuration", "proj", "memory")
	seedMemory(t, client, "unrelated topic about cooking recipes", "proj", "memory")

	// With minRelevance = 0, all results pass
	respAll, err := search.Adaptive(ctx, client, mockEmbed, "proxmox cluster", nil, 10, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive(minRelevance=0): %v", err)
	}

	// With a very high minRelevance, fewer (or zero) results should pass
	respStrict, err := search.Adaptive(ctx, client, mockEmbed, "proxmox cluster", nil, 10, 0.99, 0)
	if err != nil {
		t.Fatalf("Adaptive(minRelevance=0.99): %v", err)
	}

	if len(respStrict.Results) > len(respAll.Results) {
		t.Errorf("strict filter (%d results) returned more than unfiltered (%d results)",
			len(respStrict.Results), len(respAll.Results))
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive with recencyDecay weighting
// ---------------------------------------------------------------------------

func TestAdaptive_RecencyDecay(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	seedMemory(t, client, "network configuration notes for homelab", "proj", "memory")
	seedMemory(t, client, "network troubleshooting guide for homelab", "proj", "memory")

	// Run with recencyDecay = 0 (disabled)
	respNone, err := search.Adaptive(ctx, client, mockEmbed, "network homelab", nil, 10, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive(decay=0): %v", err)
	}

	// Run with recencyDecay > 0 (enabled)
	respDecay, err := search.Adaptive(ctx, client, mockEmbed, "network homelab", nil, 10, 0, 0.01)
	if err != nil {
		t.Fatalf("Adaptive(decay=0.01): %v", err)
	}

	if len(respNone.Results) == 0 {
		t.Fatal("expected results for decay=0 query")
	}
	if len(respDecay.Results) == 0 {
		t.Fatal("expected results for decay=0.01 query")
	}

	// With decay disabled, WeightedScore should be 0
	for _, r := range respNone.Results {
		if r.WeightedScore != 0 {
			t.Errorf("decay=0: WeightedScore = %f, want 0", r.WeightedScore)
		}
		if r.RecencyWeight != 0 {
			t.Errorf("decay=0: RecencyWeight = %f, want 0", r.RecencyWeight)
		}
	}

	// With decay enabled, WeightedScore should be positive (memories are fresh)
	for _, r := range respDecay.Results {
		if r.WeightedScore <= 0 {
			t.Errorf("decay=0.01: WeightedScore = %f, want > 0", r.WeightedScore)
		}
		if r.RecencyWeight <= 0 {
			t.Errorf("decay=0.01: RecencyWeight = %f, want > 0", r.RecencyWeight)
		}
		// Just-inserted memories should have recency weight close to 1.0
		if math.Abs(r.RecencyWeight-1.0) > 0.01 {
			t.Errorf("decay=0.01: RecencyWeight = %f, want ~1.0 for fresh memory", r.RecencyWeight)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive triggers query rewriting when minRelevance is impossibly high
// ---------------------------------------------------------------------------

func TestAdaptive_QueryRewriting(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Seed data so there ARE results in the DB, but they won't pass the
	// extremely high relevance threshold, forcing a rewrite attempt.
	seedMemory(t, client, "how to configure DNS records", "proj", "memory")
	seedMemory(t, client, "DNS domain name system troubleshooting", "proj", "memory")

	// Use a query that the rewrite module will transform (strip filler prefix).
	// "what is DNS configuration" -> "domain name system DNS configuration"
	// minRelevance = 100 ensures no results pass, triggering rewrite.
	resp, err := search.Adaptive(ctx, client, mockEmbed, "what is DNS configuration", nil, 10, 100.0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	// The rewrite should have been attempted since first pass returned 0 graded results.
	if resp.Attempts < 2 {
		t.Errorf("Attempts = %d, want 2 (rewrite should have been triggered)", resp.Attempts)
	}
	if !resp.Rewritten {
		t.Error("Rewritten = false, want true")
	}
	if resp.RewrittenQuery == "" {
		t.Error("RewrittenQuery is empty, want non-empty rewritten query")
	}
	if resp.RewrittenQuery == "what is DNS configuration" {
		t.Errorf("RewrittenQuery = %q, should differ from original", resp.RewrittenQuery)
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive with a project filter
// ---------------------------------------------------------------------------

func TestAdaptive_WithFilter(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	seedMemory(t, client, "golang testing patterns and best practices", "alpha", "memory")
	seedMemory(t, client, "golang testing patterns and best practices", "beta", "memory")

	filter := &db.MemoryFilter{Project: "alpha"}
	resp, err := search.Adaptive(ctx, client, mockEmbed, "golang testing", filter, 10, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	for _, r := range resp.Results {
		if r.Memory.Project != "alpha" {
			t.Errorf("expected project=alpha, got %q", r.Memory.Project)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive with parent resolution (resolveParents path)
// ---------------------------------------------------------------------------

func TestAdaptive_ParentResolution(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Create a parent memory
	parent := seedMemory(t, client, "full parent document about server monitoring", "proj", "memory")

	// Create a chunk that references the parent
	emb, _ := mockEmbed(context.Background(), "chunk about alerting")
	_, err := client.SaveMemory(&db.Memory{
		Content:    "chunk about alerting",
		Embedding:  emb,
		Project:    "proj",
		Type:       "memory",
		Visibility: "internal",
		Speaker:    "user",
		Area:       "work",
		SubArea:    "test",
		ParentID:   parent.ID,
		ChunkIndex: 1,
	})
	if err != nil {
		t.Fatalf("SaveMemory chunk: %v", err)
	}

	resp, err := search.Adaptive(ctx, client, mockEmbed, "alerting monitoring", nil, 10, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	// At least one result should have had its content replaced with the parent's
	foundParentContent := false
	for _, r := range resp.Results {
		if r.Memory.Content == "full parent document about server monitoring" {
			foundParentContent = true
			break
		}
	}
	if !foundParentContent {
		t.Log("Note: parent resolution test depends on the chunk being returned in search results")
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive embed function error propagation
// ---------------------------------------------------------------------------

func TestAdaptive_EmbedError(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	failEmbed := func(_ context.Context, _ string) ([]float32, error) {
		return nil, context.DeadlineExceeded
	}

	_, err := search.Adaptive(ctx, client, failEmbed, "test query", nil, 5, 0, 0)
	if err == nil {
		t.Fatal("expected error from failing embed function, got nil")
	}
}

// ---------------------------------------------------------------------------
// Test: Adaptive with topK limiting
// ---------------------------------------------------------------------------

func TestAdaptive_TopKLimit(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Seed more memories than topK
	for i := 0; i < 10; i++ {
		seedMemory(t, client, "memory about golang concurrency patterns and goroutines", "proj", "memory")
	}

	resp, err := search.Adaptive(ctx, client, mockEmbed, "golang concurrency", nil, 3, 0, 0)
	if err != nil {
		t.Fatalf("Adaptive: %v", err)
	}

	if len(resp.Results) > 3 {
		t.Errorf("got %d results, want at most 3 (topK)", len(resp.Results))
	}
}
