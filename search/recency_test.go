package search

import (
	"math"
	"testing"
	"time"

	"github.com/j33pguy/magi/db"
)

func TestApplyRecencyWeighting_Disabled(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{CreatedAt: time.Now().UTC().Format(time.RFC3339)}, Score: 0.9},
		{Memory: &db.Memory{CreatedAt: time.Now().Add(-72 * 24 * time.Hour).UTC().Format(time.RFC3339)}, Score: 0.8},
	}

	ApplyRecencyWeighting(results, 0.0)

	if results[0].RecencyWeight != 0 || results[0].WeightedScore != 0 {
		t.Error("expected no weighting when decay=0")
	}
}

func TestApplyRecencyWeighting_RecentWins(t *testing.T) {
	now := time.Now().UTC()
	results := []*db.HybridResult{
		{Memory: &db.Memory{CreatedAt: now.Add(-60 * 24 * time.Hour).Format(time.RFC3339)}, Score: 0.9},
		{Memory: &db.Memory{CreatedAt: now.Add(-1 * 24 * time.Hour).Format(time.RFC3339)}, Score: 0.85},
	}

	ApplyRecencyWeighting(results, 0.01)

	// The recent memory (0.85 score, 1 day old) should now rank higher
	// than the old memory (0.9 score, 60 days old) after weighting.
	if results[0].Score != 0.85 {
		t.Errorf("expected recent memory first, got score=%f", results[0].Score)
	}
	if results[0].WeightedScore <= results[1].WeightedScore {
		t.Errorf("expected recent memory to have higher weighted score: %f vs %f",
			results[0].WeightedScore, results[1].WeightedScore)
	}
}

func TestApplyRecencyWeighting_HalfLife(t *testing.T) {
	now := time.Now().UTC()
	results := []*db.HybridResult{
		{Memory: &db.Memory{CreatedAt: now.Add(-70 * 24 * time.Hour).Format(time.RFC3339)}, Score: 1.0},
	}

	ApplyRecencyWeighting(results, 0.01)

	// At ~70 days with decay=0.01, weight should be ~0.5 (half-life).
	if math.Abs(results[0].RecencyWeight-0.5) > 0.05 {
		t.Errorf("expected recency weight ~0.5 at 70 days, got %f", results[0].RecencyWeight)
	}
}

func TestApplyRecencyWeighting_EmptySlice(t *testing.T) {
	// Should not panic.
	ApplyRecencyWeighting(nil, 0.01)
	ApplyRecencyWeighting([]*db.HybridResult{}, 0.01)
}

func TestDaysSince(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name      string
		createdAt string
		wantDays  float64
		tolerance float64
	}{
		{"RFC3339 now", now.Format(time.RFC3339), 0, 0.01},
		{"DateTime now", now.Format(time.DateTime), 0, 0.01},
		{"10 days ago", now.Add(-10 * 24 * time.Hour).Format(time.RFC3339), 10, 0.01},
		{"unparseable", "garbage", 0, 0},
		{"future", now.Add(24 * time.Hour).Format(time.RFC3339), 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daysSince(tt.createdAt, now)
			if math.Abs(got-tt.wantDays) > tt.tolerance {
				t.Errorf("daysSince(%q) = %f, want %f (±%f)", tt.createdAt, got, tt.wantDays, tt.tolerance)
			}
		})
	}
}
