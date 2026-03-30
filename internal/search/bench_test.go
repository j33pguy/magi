package search

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// --- ApplyRecencyWeighting ---

func BenchmarkApplyRecencyWeighting10(b *testing.B) {
	benchRecency(b, 10)
}

func BenchmarkApplyRecencyWeighting100(b *testing.B) {
	benchRecency(b, 100)
}

func benchRecency(b *testing.B, n int) {
	b.Helper()
	now := time.Now().UTC()
	base := make([]*db.HybridResult, n)
	for i := range base {
		base[i] = &db.HybridResult{
			Memory: &db.Memory{
				ID:        "id",
				CreatedAt: now.Add(-time.Duration(rand.Intn(180)) * 24 * time.Hour).Format(time.RFC3339),
			},
			Score: rand.Float64(),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Copy so each iteration starts fresh (weighting mutates in place).
		results := make([]*db.HybridResult, n)
		for j := range base {
			cp := *base[j]
			results[j] = &cp
		}
		ApplyRecencyWeighting(results, 0.01)
	}
}

// --- gradeResults ---

func BenchmarkGradeResults(b *testing.B) {
	results := make([]*db.HybridResult, 30)
	for i := range results {
		results[i] = &db.HybridResult{
			Memory: &db.Memory{ID: "id"},
			Score:  rand.Float64(),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gradeResults(results, 0.5)
	}
}

// --- daysSince ---

func BenchmarkDaysSince(b *testing.B) {
	now := time.Now().UTC()
	ts := now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = daysSince(ts, now)
	}
}

// --- RRF fusion (simulates HybridSearch scoring) ---

func BenchmarkRRFFusion(b *testing.B) {
	const k = 60.0
	const n = 30
	vecResults := make([]*db.VectorResult, n)
	bm25Results := make([]*db.VectorResult, n)
	for i := 0; i < n; i++ {
		vecResults[i] = &db.VectorResult{
			Memory:   &db.Memory{ID: randomID()},
			Distance: rand.Float64(),
		}
		bm25Results[i] = &db.VectorResult{
			Memory:   &db.Memory{ID: randomID()},
			Distance: rand.Float64(),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scored := make(map[string]float64, n*2)
		for rank, r := range vecResults {
			scored[r.Memory.ID] += 1.0 / (k + float64(rank+1))
		}
		for rank, r := range bm25Results {
			scored[r.Memory.ID] += 1.0 / (k + float64(rank+1))
		}
	}
}

func randomID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = byte(rand.Intn(26) + 'a')
	}
	return string(b)
}

// Ensure math import is used
var _ = math.Sqrt
