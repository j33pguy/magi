package local

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// BenchmarkDirectSave measures direct Store.SaveMemory throughput.
func BenchmarkDirectSave(b *testing.B) {
	store := newMockStore()
	b.ResetTimer()
	for i := range b.N {
		store.SaveMemory(&db.Memory{Content: fmt.Sprintf("bench-%d", i)})
	}
}

// BenchmarkRoutedSave measures SaveMemory routed through the coordinator pool.
func BenchmarkRoutedSave(b *testing.B) {
	store := newMockStore()
	cfg := &node.Config{
		Mode:               node.ModeEmbedded,
		WriterPoolSize:     4,
		ReaderPoolSize:     8,
		CoordinatorEnabled: true,
	}
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	ctx := context.Background()
	b.ResetTimer()
	for i := range b.N {
		coord.SaveMemory(ctx, &db.Memory{Content: fmt.Sprintf("bench-%d", i)})
	}
}

// BenchmarkDirectGet measures direct Store.GetMemory throughput.
func BenchmarkDirectGet(b *testing.B) {
	store := newMockStore()
	store.SaveMemory(&db.Memory{Content: "target"})

	b.ResetTimer()
	for range b.N {
		store.GetMemory("mem-1")
	}
}

// BenchmarkRoutedGet measures GetMemory routed through the coordinator pool.
func BenchmarkRoutedGet(b *testing.B) {
	store := newMockStore()
	store.SaveMemory(&db.Memory{Content: "target"})

	cfg := &node.Config{
		Mode:               node.ModeEmbedded,
		WriterPoolSize:     4,
		ReaderPoolSize:     8,
		CoordinatorEnabled: true,
	}
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		coord.GetMemory(ctx, "mem-1")
	}
}

// BenchmarkCoordinatedStoreSave measures the db.Store adapter overhead.
func BenchmarkCoordinatedStoreSave(b *testing.B) {
	store := newMockStore()
	cfg := node.DefaultConfig()
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()
	cs := NewCoordinatedStore(coord, store)

	b.ResetTimer()
	for i := range b.N {
		cs.SaveMemory(&db.Memory{Content: fmt.Sprintf("bench-%d", i)})
	}
}

// BenchmarkPoolSubmit measures raw pool submit/response latency.
func BenchmarkPoolSubmit(b *testing.B) {
	handler := func(_ context.Context, n int) int { return n * 2 }
	p := node.NewPool("bench", 4, handler)
	p.Start(context.Background())
	defer p.Stop()

	ctx := context.Background()
	b.ResetTimer()
	for i := range b.N {
		p.Submit(ctx, i)
	}
}

// BenchmarkDirectList measures direct Store.ListMemories throughput.
func BenchmarkDirectList(b *testing.B) {
	store := newMockStore()
	for i := range 100 {
		store.SaveMemory(&db.Memory{Content: fmt.Sprintf("item-%d", i)})
	}

	filter := &db.MemoryFilter{}
	b.ResetTimer()
	for range b.N {
		store.ListMemories(filter)
	}
}

// BenchmarkRoutedList measures ListMemories routed through the coordinator.
func BenchmarkRoutedList(b *testing.B) {
	store := newMockStore()
	for i := range 100 {
		store.SaveMemory(&db.Memory{Content: fmt.Sprintf("item-%d", i)})
	}

	cfg := node.DefaultConfig()
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	ctx := context.Background()
	filter := &db.MemoryFilter{}
	b.ResetTimer()
	for range b.N {
		coord.ListMemories(ctx, filter)
	}
}
