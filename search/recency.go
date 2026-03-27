package search

import (
	"math"
	"time"

	"github.com/j33pguy/claude-memory/db"
)

// ApplyRecencyWeighting multiplies each result's Score by an exponential decay
// factor based on the memory's age, then re-sorts by WeightedScore descending.
//
// recencyWeight = exp(-decayRate * daysSinceCreated)
// weightedScore = score * recencyWeight
//
// A decayRate of 0.01 gives a half-life of ~70 days.
// If decayRate is <= 0, this is a no-op (backward compat).
func ApplyRecencyWeighting(results []*db.HybridResult, decayRate float64) {
	if decayRate <= 0 || len(results) == 0 {
		return
	}

	now := time.Now().UTC()

	for _, r := range results {
		days := daysSince(r.Memory.CreatedAt, now)
		r.RecencyWeight = math.Exp(-decayRate * days)
		r.WeightedScore = r.Score * r.RecencyWeight
	}

	// Re-sort by WeightedScore descending (insertion sort — small slices).
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].WeightedScore > results[j-1].WeightedScore; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

// daysSince returns fractional days between the given RFC3339/DateTime string and now.
// Returns 0 on parse failure (treat unparseable timestamps as "just created").
func daysSince(createdAt string, now time.Time) float64 {
	// Try RFC3339 first, then time.DateTime (used by SQLite datetime('now')).
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		t, err = time.Parse(time.DateTime, createdAt)
		if err != nil {
			return 0
		}
	}
	d := now.Sub(t)
	if d < 0 {
		return 0
	}
	return d.Hours() / 24.0
}
