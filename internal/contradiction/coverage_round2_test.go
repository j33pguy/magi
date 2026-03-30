package contradiction

import (
	"context"
	"fmt"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// ---------- Check: embed error (line 43-45) ----------

type failingEmbedder struct{}

var _ embeddings.Provider = (*failingEmbedder)(nil)

func (f *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embedding service unavailable")
}

func (f *failingEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embedding service unavailable")
}

func (f *failingEmbedder) Dimensions() int { return 384 }

func TestCheck_EmbedError(t *testing.T) {
	client := newTestDB(t)
	emb := &failingEmbedder{}

	d := &Detector{}
	_, err := d.Check(context.Background(), client, emb, "some content", "homelab", "proxmox")
	if err == nil {
		t.Fatal("expected error for embed failure")
	}
	if got := err.Error(); got != "embedding new content: embedding service unavailable" {
		t.Errorf("unexpected error message: %s", got)
	}
}

// ---------- Check: search error (line 62-64) ----------

type brokenSearchStore struct {
	db.Store
}

func (b *brokenSearchStore) SearchMemories(_ []float32, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, fmt.Errorf("search index corrupted")
}

// Implement remaining Store interface methods needed for the test to compile.
// We only need SearchMemories to fail.
func (b *brokenSearchStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestCheck_SearchError(t *testing.T) {
	store := &brokenSearchStore{}
	emb := &mockEmbedder{dims: 384}

	d := &Detector{}
	_, err := d.Check(context.Background(), store, emb, "some content", "homelab", "proxmox")
	if err == nil {
		t.Fatal("expected error for search failure")
	}
	if got := err.Error(); got != "searching similar memories: search index corrupted" {
		t.Errorf("unexpected error message: %s", got)
	}
}

// ---------- Check: high distance skips result (line 69-70) ----------

// distantEmbedder produces embeddings that are distant from what's stored.
type distantEmbedder struct {
	dims int
}

func (d *distantEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, d.dims)
	// Produce a completely different direction from unitVec for the new content
	v[100] = 1.0
	return v, nil
}

func (d *distantEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for range texts {
		e, _ := d.Embed(context.Background(), "")
		results = append(results, e)
	}
	return results, nil
}

func (d *distantEmbedder) Dimensions() int { return d.dims }

func TestCheck_HighDistanceSkipped(t *testing.T) {
	client := newTestDB(t)
	vec := unitVec()

	// Save a memory with unitVec (dimension 0 = 1.0)
	saveTestMemory(t, client, "the firewall is enabled", "homelab", "proxmox", vec)

	// Use an embedder that produces a vector in a very different direction
	// so the cosine distance will be high (> 1-0.85 = 0.15) and the result is skipped.
	emb := &distantEmbedder{dims: 384}
	d := &Detector{Threshold: 0.99} // very strict threshold
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled", "homelab", "proxmox")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// The result should be skipped due to high distance
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for high-distance result, got %d", len(candidates))
	}
}

// ---------- Check: default threshold when <= 0 ----------

func TestCheck_DefaultThresholdWhenZero(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the firewall is enabled", "homelab", "", vec)

	// Threshold 0 should default to 0.85
	d := &Detector{Threshold: 0}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled", "homelab", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// unitVec -> distance 0 -> passes 0.85 threshold -> boolean flip detected
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate with default threshold")
	}
}

// ---------- Check: negative threshold ----------

func TestCheck_NegativeThreshold(t *testing.T) {
	client := newTestDB(t)
	emb := &mockEmbedder{dims: 384}
	vec := unitVec()

	saveTestMemory(t, client, "the firewall is enabled", "homelab", "", vec)

	// Negative threshold should default to 0.85
	d := &Detector{Threshold: -0.5}
	candidates, err := d.Check(context.Background(), client, emb, "the firewall is disabled", "homelab", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected at least 1 candidate with negative threshold (defaults to 0.85)")
	}
}
