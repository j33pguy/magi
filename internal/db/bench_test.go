package db

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"testing"
)

// --- float32sToBytes ---

func BenchmarkFloat32sToBytes384(b *testing.B) {
	v := make([]float32, 384)
	for i := range v {
		v[i] = rand.Float32()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = float32sToBytes(v)
	}
}

// --- SaveMemory ---

func BenchmarkSaveMemory(b *testing.B) {
	c := newBenchSQLiteClient(b)
	emb := randomEmbedding()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.SaveMemory(&Memory{
			Content:    fmt.Sprintf("benchmark memory %d", i),
			Embedding:  emb,
			Project:    "bench",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSaveMemoryBatch10(b *testing.B) {
	c := newBenchSQLiteClient(b)
	emb := randomEmbedding()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 10; j++ {
			_, err := c.SaveMemory(&Memory{
				Content:    fmt.Sprintf("benchmark memory %d-%d", i, j),
				Embedding:  emb,
				Project:    "bench",
				Type:       "memory",
				Visibility: "internal",
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// --- SearchMemories (vector) ---

func BenchmarkSearchMemoriesVector(b *testing.B) {
	c := newBenchSQLiteClient(b)
	seedMemories(b, c, 100)
	query := randomEmbedding()
	filter := &MemoryFilter{Project: "bench", Visibility: "all"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.SearchMemories(query, filter, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- SearchMemoriesBM25 (FTS) ---

func BenchmarkSearchMemoriesBM25(b *testing.B) {
	c := newBenchSQLiteClient(b)
	seedMemories(b, c, 100)
	filter := &MemoryFilter{Project: "bench", Visibility: "all"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.SearchMemoriesBM25("benchmark memory", filter, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- HybridSearch (concurrent vec + BM25) ---

func BenchmarkHybridSearch(b *testing.B) {
	c := newBenchSQLiteClient(b)
	seedMemories(b, c, 100)
	query := randomEmbedding()
	filter := &MemoryFilter{Project: "bench", Visibility: "all"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := c.HybridSearch(query, "benchmark memory", filter, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// --- SetTags ---

func BenchmarkSetTags(b *testing.B) {
	c := newBenchSQLiteClient(b)
	saved, err := c.SaveMemory(&Memory{
		Content: "tag bench", Embedding: randomEmbedding(),
		Project: "bench", Type: "memory", Visibility: "internal",
	})
	if err != nil {
		b.Fatal(err)
	}
	tags := []string{"alpha", "beta", "gamma", "delta"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := c.SetTags(saved.ID, tags); err != nil {
			b.Fatal(err)
		}
	}
}

// --- hasDiagnosticKeywords ---

func BenchmarkHasDiagnosticKeywords_Hit(b *testing.B) {
	q := "why did the server crash last night"
	for i := 0; i < b.N; i++ {
		hasDiagnosticKeywords(q)
	}
}

func BenchmarkHasDiagnosticKeywords_Miss(b *testing.B) {
	q := "what is the capital of france"
	for i := 0; i < b.N; i++ {
		hasDiagnosticKeywords(q)
	}
}

// --- helpers ---

func newBenchSQLiteClient(b *testing.B) *SQLiteClient {
	b.Helper()
	tmp := b.TempDir()

	c, err := newBenchClient(tmp)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { c.Close() })
	return c
}

func newBenchClient(dir string) (*SQLiteClient, error) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	c, err := NewSQLiteClient(dir+"/bench.db", logger)
	if err != nil {
		return nil, err
	}
	if err := c.Migrate(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func randomEmbedding() []float32 {
	emb := make([]float32, 384)
	for i := range emb {
		emb[i] = rand.Float32()*2 - 1
	}
	// L2 normalize
	var norm float32
	for _, v := range emb {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range emb {
		emb[i] /= norm
	}
	return emb
}

func seedMemories(b *testing.B, c *SQLiteClient, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		_, err := c.SaveMemory(&Memory{
			Content:    fmt.Sprintf("benchmark memory number %d about topic %d", i, i%10),
			Embedding:  randomEmbedding(),
			Project:    "bench",
			Type:       "memory",
			Visibility: "internal",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
