package pipeline

import (
	"log/slog"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func BenchmarkAsyncWriteSubmit(b *testing.B) {
	store := newMockStore()
	embedder := &mockEmbedder{}
	cfg := Config{
		Enabled:       true,
		Workers:       4,
		QueueSize:     100000,
		FlushInterval: 1 * time.Millisecond,
		BatchMaxSize:  100,
	}

	w := NewWriter(store, embedder, cfg, slog.Default())
	defer w.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = w.Submit(WriteRequest{
			Memory: &db.Memory{
				Content: "benchmark memory content for async pipeline testing",
				Project: "bench",
				Type:    "memory",
				Speaker: "assistant",
			},
			Tags: []string{"bench"},
		})
	}
}

func BenchmarkSyncWritePath(b *testing.B) {
	store := newMockStore()
	embedder := &mockEmbedder{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		emb, _ := embedder.Embed(nil, "benchmark memory content for sync pipeline testing")
		mem := &db.Memory{
			Content:   "benchmark memory content for sync pipeline testing",
			Embedding: emb,
			Project:   "bench",
			Type:      "memory",
			Speaker:   "assistant",
		}
		_, err := store.SaveMemory(mem)
		if err != nil {
			b.Fatalf("save failed: %v", err)
		}
	}
}

func BenchmarkGenerateID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := generateID()
		if err != nil {
			b.Fatal(err)
		}
	}
}
