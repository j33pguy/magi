package search

import (
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// ---------- gradeResults ----------

func TestGradeResultsNoFilter(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1"}, Score: 0.5},
		{Memory: &db.Memory{ID: "2"}, Score: 0.3},
	}
	filtered := gradeResults(results, 0)
	if len(filtered) != 2 {
		t.Errorf("minRelevance=0: got %d, want 2", len(filtered))
	}
}

func TestGradeResultsWithThreshold(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1"}, Score: 0.9},
		{Memory: &db.Memory{ID: "2"}, Score: 0.5},
		{Memory: &db.Memory{ID: "3"}, Score: 0.3},
	}
	filtered := gradeResults(results, 0.6)
	if len(filtered) != 1 {
		t.Errorf("minRelevance=0.6: got %d, want 1", len(filtered))
	}
	if filtered[0].Memory.ID != "1" {
		t.Errorf("expected ID=1, got %s", filtered[0].Memory.ID)
	}
}

func TestGradeResultsAllFiltered(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1"}, Score: 0.3},
	}
	filtered := gradeResults(results, 0.9)
	if len(filtered) != 0 {
		t.Errorf("expected 0 results, got %d", len(filtered))
	}
}

func TestGradeResultsNegativeThreshold(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1"}, Score: 0.1},
	}
	filtered := gradeResults(results, -1.0)
	if len(filtered) != 1 {
		t.Errorf("negative threshold should pass all, got %d", len(filtered))
	}
}

func TestGradeResultsEmpty(t *testing.T) {
	filtered := gradeResults(nil, 0.5)
	if len(filtered) != 0 {
		t.Errorf("expected 0, got %d", len(filtered))
	}
}

// daysSince is already tested in recency_test.go

// ---------- ApplyRecencyWeighting ----------

func TestApplyRecencyWeightingDisabled(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1", CreatedAt: "2026-03-01T00:00:00Z"}, Score: 0.8},
	}
	ApplyRecencyWeighting(results, 0)
	if results[0].RecencyWeight != 0 {
		t.Errorf("disabled decay: RecencyWeight = %f, want 0", results[0].RecencyWeight)
	}
}

func TestApplyRecencyWeightingNegative(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1", CreatedAt: "2026-03-01T00:00:00Z"}, Score: 0.8},
	}
	ApplyRecencyWeighting(results, -0.5)
	if results[0].RecencyWeight != 0 {
		t.Errorf("negative decay: RecencyWeight = %f, want 0", results[0].RecencyWeight)
	}
}

func TestApplyRecencyWeightingEmpty(t *testing.T) {
	// Should not panic on empty input
	ApplyRecencyWeighting(nil, 0.01)
	ApplyRecencyWeighting([]*db.HybridResult{}, 0.01)
}

func TestApplyRecencyWeightingSorting(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)

	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "old", CreatedAt: old}, Score: 0.9},
		{Memory: &db.Memory{ID: "recent", CreatedAt: recent}, Score: 0.8},
	}

	ApplyRecencyWeighting(results, 0.01)

	// Recent should be first after weighting despite lower raw score
	if results[0].Memory.ID != "recent" {
		t.Errorf("expected recent first, got %q", results[0].Memory.ID)
	}
	if results[0].WeightedScore <= 0 {
		t.Errorf("expected positive WeightedScore, got %f", results[0].WeightedScore)
	}
}

func TestApplyRecencyWeightingRecent(t *testing.T) {
	now := time.Now().UTC()
	justCreated := now.Add(-1 * time.Minute).Format(time.RFC3339)

	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1", CreatedAt: justCreated}, Score: 0.8},
	}

	ApplyRecencyWeighting(results, 0.01)
	// Just-created memory should have weight close to 1.0
	if results[0].RecencyWeight < 0.99 {
		t.Errorf("recent memory RecencyWeight = %f, want ~1.0", results[0].RecencyWeight)
	}
}

// ---------- resolveParents ----------

func TestResolveParentsNoParent(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "1", Content: "original", ParentID: ""}},
	}
	// Should not panic or modify content when no parent
	resolveParents(nil, results) // client is nil, but ParentID is empty so it won't be called
	if results[0].Memory.Content != "original" {
		t.Errorf("Content changed unexpectedly to %q", results[0].Memory.Content)
	}
}
