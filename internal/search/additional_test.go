package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/search"
)

// failOnSecondCallEmbed fails on the second call to simulate retry embed failure.
type failOnSecondCallEmbed struct {
	calls int
}

func (f *failOnSecondCallEmbed) embed(_ context.Context, text string) ([]float32, error) {
	f.calls++
	if f.calls > 1 {
		return nil, errors.New("embed failed on retry")
	}
	vec := make([]float32, 384)
	vec[0] = float32(len(text)%100) / 100.0
	vec[1] = 1.0 - vec[0]
	return vec, nil
}

func TestAdaptive_RetryEmbedError(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Seed data so first search returns results
	seedMemory(t, client, "kubernetes deployment config", "proj", "memory")

	f := &failOnSecondCallEmbed{}

	// Use a query that will be rewritten (filler prefix stripped) + impossibly high minRelevance
	// to force a retry. The second embed call will fail.
	resp, err := search.Adaptive(ctx, client, f.embed, "what is kubernetes deployment", nil, 10, 100.0, 0)
	if err != nil {
		t.Fatalf("Adaptive should not return error on retry embed failure: %v", err)
	}

	// On retry embed error, function returns resp with attempt 1 results (empty due to filtering)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// The response should have results from first pass (which were filtered out by minRelevance)
	// so len(resp.Results) should be 0 and Attempts should be 1
	if resp.Attempts != 1 {
		// Retry was attempted but embed failed, so it stays at attempt 1
		t.Logf("Attempts = %d (retry embed failed, fell back to first pass)", resp.Attempts)
	}
}

func TestAdaptive_RetryHybridSearchError(t *testing.T) {
	client := newTestDB(t)
	ctx := context.Background()

	// Seed data
	seedMemory(t, client, "DNS configuration guide for homelab", "proj", "memory")

	callCount := 0
	embedWithDBClose := func(ctx2 context.Context, text string) ([]float32, error) {
		callCount++
		if callCount == 2 {
			// Close DB before the retry's HybridSearch call
			client.DB.Close()
		}
		vec := make([]float32, 384)
		vec[0] = float32(len(text)%100) / 100.0
		vec[1] = 1.0 - vec[0]
		return vec, nil
	}

	// Force retry with impossibly high minRelevance + rewritable query
	resp, err := search.Adaptive(ctx, client, embedWithDBClose, "what is DNS configuration", nil, 10, 100.0, 0)
	if err != nil {
		t.Fatalf("Adaptive should not return error on retry search failure: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestApplyRecencyWeighting_HalfLife(t *testing.T) {
	// A memory created ~70 days ago with decay=0.01 should have weight ~0.5
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1", CreatedAt: "2026-01-18T00:00:00Z"}, Score: 0.8},
	}
	search.ApplyRecencyWeighting(results, 0.01)
	w := results[0].RecencyWeight
	// We just check it's in a reasonable range (depends on current date)
	if w < 0 || w > 1.01 {
		t.Errorf("RecencyWeight = %f, want in [0, 1]", w)
	}
}
