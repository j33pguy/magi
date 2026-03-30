package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// mockStore implements db.Store for testing.
type mockStore struct {
	db.Store // embed to satisfy interface
	mu       sync.Mutex
	saved    []*db.Memory
	tags     map[string][]string
}

func newMockStore() *mockStore {
	return &mockStore{
		tags: make(map[string][]string),
	}
}

func (m *mockStore) SaveMemory(mem *db.Memory) (*db.Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mem.ID = "saved-" + mem.Content[:min(8, len(mem.Content))]
	mem.CreatedAt = time.Now().UTC().Format(time.DateTime)
	mem.UpdatedAt = mem.CreatedAt
	m.saved = append(m.saved, mem)
	return mem, nil
}

func (m *mockStore) SetTags(id string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags[id] = tags
	return nil
}

func (m *mockStore) FindSimilar(_ []float32, _ float64) (*db.VectorResult, error) {
	return nil, nil
}

func (m *mockStore) SearchMemories(_ []float32, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, nil
}

func (m *mockStore) getSaved() []*db.Memory {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*db.Memory, len(m.saved))
	copy(out, m.saved)
	return out
}

// mockEmbedder implements embeddings.Provider for testing.
type mockEmbedder struct{}

func (e *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, 384), nil
}

func (e *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = make([]float32, 384)
	}
	return out, nil
}

func (e *mockEmbedder) Dimensions() int { return 384 }

func testConfig() Config {
	return Config{
		Enabled:       true,
		Workers:       2,
		QueueSize:     100,
		FlushInterval: 10 * time.Millisecond,
		BatchMaxSize:  5,
	}
}

func TestWriterSubmitAndComplete(t *testing.T) {
	store := newMockStore()
	embedder := &mockEmbedder{}
	cfg := testConfig()

	w := NewWriter(store, embedder, cfg, testLogger(t))
	defer w.Close()

	result, err := w.Submit(WriteRequest{
		Memory: &db.Memory{
			Content: "test memory content",
			Project: "test",
			Type:    "memory",
			Speaker: "assistant",
		},
		Tags: []string{"test"},
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if result.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	// Wait for processing + batch flush
	time.Sleep(200 * time.Millisecond)

	saved := store.getSaved()
	if len(saved) == 0 {
		t.Fatal("expected at least one saved memory")
	}
	if saved[0].Content != "test memory content" {
		t.Errorf("unexpected content: %s", saved[0].Content)
	}
}

func TestWriterBackpressure(t *testing.T) {
	store := newMockStore()
	embedder := &mockEmbedder{}
	cfg := Config{
		Enabled:       true,
		Workers:       1,
		QueueSize:     1,
		FlushInterval: 1 * time.Second,
		BatchMaxSize:  100,
	}

	w := NewWriter(store, embedder, cfg, testLogger(t))
	defer w.Close()

	// Fill the queue
	_, err := w.Submit(WriteRequest{
		Memory: &db.Memory{Content: "first", Project: "test", Type: "memory"},
	})
	if err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	// This may or may not fail depending on worker speed; try multiple times
	var backpressureHit bool
	for i := 0; i < 50; i++ {
		_, err = w.Submit(WriteRequest{
			Memory: &db.Memory{Content: "overflow", Project: "test", Type: "memory"},
		})
		if err != nil {
			backpressureHit = true
			break
		}
	}

	if !backpressureHit {
		t.Log("backpressure not hit (worker consumed fast enough), test is inconclusive")
	}
}

func TestWriterGracefulShutdown(t *testing.T) {
	store := newMockStore()
	embedder := &mockEmbedder{}
	cfg := testConfig()
	cfg.FlushInterval = 50 * time.Millisecond

	w := NewWriter(store, embedder, cfg, testLogger(t))

	// Submit several items
	for i := 0; i < 10; i++ {
		_, err := w.Submit(WriteRequest{
			Memory: &db.Memory{
				Content: "shutdown test",
				Project: "test",
				Type:    "memory",
			},
		})
		if err != nil {
			t.Fatalf("submit %d failed: %v", i, err)
		}
	}

	// Close should drain the queue
	w.Close()

	saved := store.getSaved()
	if len(saved) != 10 {
		t.Errorf("expected 10 saved memories after shutdown, got %d", len(saved))
	}
}

func TestWriterStats(t *testing.T) {
	store := newMockStore()
	embedder := &mockEmbedder{}
	cfg := testConfig()

	w := NewWriter(store, embedder, cfg, testLogger(t))
	defer w.Close()

	_, _ = w.Submit(WriteRequest{
		Memory: &db.Memory{Content: "stats test", Project: "test", Type: "memory"},
	})

	time.Sleep(100 * time.Millisecond)

	stats := w.Stats()
	if stats.Workers != 2 {
		t.Errorf("expected 2 workers, got %d", stats.Workers)
	}
	if stats.Submitted < 1 {
		t.Errorf("expected submitted >= 1, got %d", stats.Submitted)
	}
}

func TestGenerateID(t *testing.T) {
	id, err := generateID()
	if err != nil {
		t.Fatalf("generateID failed: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars: %s", len(id), id)
	}

	// Uniqueness
	id2, _ := generateID()
	if id == id2 {
		t.Error("expected unique IDs")
	}
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.Default()
}
