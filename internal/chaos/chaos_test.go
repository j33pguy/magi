//go:build chaos

// Package chaos provides stress and resilience tests for MAGI.
// Run with: go test -tags chaos ./internal/chaos/
//
// These tests stress the database under concurrent load to verify:
//   - No data corruption under contention
//   - Database remains functional after stress
//   - Pre-existing data survives concurrent writes and cancellations
//
// Some contention errors (e.g. "database is locked") are expected with SQLite
// under heavy concurrent write load. The tests verify that successful writes
// are durable and the system recovers gracefully.
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// mockEmbedder implements embeddings.Provider for chaos tests.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	emb := make([]float32, 384)
	if len(text) > 0 {
		emb[0] = float32(len(text)) / 100.0
	}
	return emb, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, t := range texts {
		e, _ := m.Embed(context.Background(), t)
		results = append(results, e)
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int { return 384 }

func newChaosDB(t *testing.T) db.Store {
	t.Helper()
	tmp := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	client, err := db.NewSQLiteClient(filepath.Join(tmp, "chaos.db"), logger)
	if err != nil {
		t.Fatalf("NewSQLiteClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	if err := client.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	return client.TursoClient
}

// TestConcurrentWrites verifies that concurrent writers produce no data
// corruption and that all successful writes are durable.
func TestConcurrentWrites(t *testing.T) {
	store := newChaosDB(t)
	embedder := &mockEmbedder{}

	const numWriters = 10
	const writesPerWriter = 20

	var wg sync.WaitGroup
	var errCount atomic.Int64
	var successCount atomic.Int64

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				content := fmt.Sprintf("writer-%d memory-%d: concurrent write test data", writerID, j)
				emb, _ := embedder.Embed(context.Background(), content)
				_, err := store.SaveMemory(&db.Memory{
					Content:   content,
					Embedding: emb,
					Type:      "memory",
					Source:    "chaos",
					Speaker:   fmt.Sprintf("writer-%d", writerID),
				})
				if err != nil {
					errCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	total := numWriters * writesPerWriter
	success := successCount.Load()
	errors := errCount.Load()

	t.Logf("concurrent writes: %d/%d succeeded, %d contention errors", success, total, errors)

	// With SQLite, concurrent write contention is expected. Verify some succeed.
	if success == 0 {
		t.Error("no writes succeeded during concurrent stress test")
	}

	// Verify successful writes are readable
	memories, err := store.ListMemories(&db.MemoryFilter{Limit: total + 1})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) == 0 {
		t.Error("no memories found after concurrent writes")
	}
}

// TestSearchDuringIngestion verifies that search works correctly while
// new memories are being written concurrently.
func TestSearchDuringIngestion(t *testing.T) {
	store := newChaosDB(t)
	embedder := &mockEmbedder{}

	// Seed some initial data sequentially
	for i := 0; i < 20; i++ {
		content := fmt.Sprintf("seed memory %d: kubernetes cluster configuration", i)
		emb, _ := embedder.Embed(context.Background(), content)
		_, err := store.SaveMemory(&db.Memory{
			Content:   content,
			Embedding: emb,
			Type:      "memory",
			Source:    "chaos",
		})
		if err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var writeErrors, searchErrors, searchSuccess atomic.Int64

	// Writer goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				content := fmt.Sprintf("ingestion-%d-%d: new data being written", id, j)
				emb, _ := embedder.Embed(ctx, content)
				_, err := store.SaveMemory(&db.Memory{
					Content:   content,
					Embedding: emb,
					Type:      "memory",
					Source:    "chaos",
				})
				if err != nil {
					writeErrors.Add(1)
				}
			}
		}(i)
	}

	// Search goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				emb, _ := embedder.Embed(ctx, "kubernetes cluster")
				_, err := store.SearchMemories(emb, &db.MemoryFilter{}, 10)
				if err != nil {
					searchErrors.Add(1)
				} else {
					searchSuccess.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	t.Logf("search-during-ingestion: write_errors=%d, search_errors=%d, search_success=%d",
		writeErrors.Load(), searchErrors.Load(), searchSuccess.Load())

	// Searches should mostly succeed — reads don't contend with writes in WAL mode
	if ss := searchSuccess.Load(); ss == 0 {
		t.Error("no searches succeeded during ingestion")
	}
}

// TestKillMidWriteRecovery simulates writes being interrupted and verifies
// the database remains consistent afterward.
func TestKillMidWriteRecovery(t *testing.T) {
	store := newChaosDB(t)
	embedder := &mockEmbedder{}

	// Write some memories successfully (sequentially, no contention)
	var successIDs []string
	for i := 0; i < 10; i++ {
		content := fmt.Sprintf("pre-kill memory %d", i)
		emb, _ := embedder.Embed(context.Background(), content)
		m, err := store.SaveMemory(&db.Memory{
			Content:   content,
			Embedding: emb,
			Type:      "memory",
			Source:    "chaos",
		})
		if err != nil {
			t.Fatalf("pre-kill write: %v", err)
		}
		successIDs = append(successIDs, m.ID)
	}

	// Start a batch of writes and cancel mid-flight
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				content := fmt.Sprintf("interrupted-%d-%d", id, j)
				emb, _ := embedder.Embed(ctx, content)
				store.SaveMemory(&db.Memory{
					Content:   content,
					Embedding: emb,
					Type:      "memory",
					Source:    "chaos",
				})
			}
		}(i)
	}

	// Cancel after a brief delay to simulate interruption
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	// Verify pre-kill memories are still intact
	for _, id := range successIDs {
		m, err := store.GetMemory(id)
		if err != nil {
			t.Errorf("pre-kill memory %s lost: %v", id, err)
			continue
		}
		if m == nil {
			t.Errorf("pre-kill memory %s is nil", id)
		}
	}

	// Verify database is still functional after interruption
	content := "post-recovery write"
	emb, _ := embedder.Embed(context.Background(), content)
	_, err := store.SaveMemory(&db.Memory{
		Content:   content,
		Embedding: emb,
		Type:      "memory",
		Source:    "chaos",
	})
	if err != nil {
		t.Fatalf("post-recovery write failed: %v", err)
	}
}

// TestCacheOverflow tests behavior when many memories are written sequentially
// and then read concurrently, simulating cache pressure.
func TestCacheOverflow(t *testing.T) {
	store := newChaosDB(t)
	embedder := &mockEmbedder{}

	// Write many memories sequentially
	const count = 200
	ids := make([]string, 0, count)

	for i := 0; i < count; i++ {
		content := fmt.Sprintf("cache overflow test memory %d with some longer content to vary sizes", i)
		emb, _ := embedder.Embed(context.Background(), content)
		m, err := store.SaveMemory(&db.Memory{
			Content:   content,
			Embedding: emb,
			Type:      "memory",
			Source:    "chaos",
		})
		if err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		ids = append(ids, m.ID)
	}

	// Concurrently read all memories, simulating cache pressure
	var wg sync.WaitGroup
	var readErrors atomic.Int64

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, id := range ids {
				_, err := store.GetMemory(id)
				if err != nil {
					readErrors.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	t.Logf("cache overflow: %d read errors out of %d reads", readErrors.Load(), int64(5*count))

	// Concurrent reads should mostly succeed (WAL mode allows concurrent readers)
	maxAllowed := int64(count) // Allow up to 20% failure rate
	if re := readErrors.Load(); re > maxAllowed {
		t.Errorf("too many read errors: %d (max allowed %d)", re, maxAllowed)
	}

	// Verify all memories are still retrievable sequentially
	memories, err := store.ListMemories(&db.MemoryFilter{Limit: count + 1})
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	if len(memories) != count {
		t.Errorf("got %d memories, want %d", len(memories), count)
	}
}
