package local

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// mockStore implements db.Store for testing.
type mockStore struct {
	memories map[string]*db.Memory
	mu       sync.RWMutex
	nextID   int
}

func newMockStore() *mockStore {
	return &mockStore{memories: make(map[string]*db.Memory)}
}

func (m *mockStore) SaveMemory(mem *db.Memory) (*db.Memory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	mem.ID = fmt.Sprintf("mem-%d", m.nextID)
	m.memories[mem.ID] = mem
	return mem, nil
}

func (m *mockStore) GetMemory(id string) (*db.Memory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mem, ok := m.memories[id]; ok {
		return mem, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockStore) UpdateMemory(mem *db.Memory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.memories[mem.ID]; !ok {
		return fmt.Errorf("not found: %s", mem.ID)
	}
	m.memories[mem.ID] = mem
	return nil
}

func (m *mockStore) ArchiveMemory(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.memories[id]; !ok {
		return fmt.Errorf("not found: %s", id)
	}
	delete(m.memories, id)
	return nil
}

func (m *mockStore) DeleteMemory(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.memories, id)
	return nil
}

func (m *mockStore) ListMemories(_ *db.MemoryFilter) ([]*db.Memory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*db.Memory, 0, len(m.memories))
	for _, mem := range m.memories {
		result = append(result, mem)
	}
	return result, nil
}

func (m *mockStore) SearchMemories(_ []float32, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, nil
}

func (m *mockStore) SearchMemoriesBM25(_ string, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, nil
}

func (m *mockStore) HybridSearch(_ []float32, _ string, _ *db.MemoryFilter, _ int) ([]*db.HybridResult, error) {
	return nil, nil
}

func (m *mockStore) GetContextMemories(_ string, _ int) ([]*db.Memory, error) { return nil, nil }
func (m *mockStore) FindSimilar(_ []float32, _ float64) (*db.VectorResult, error) {
	return nil, nil
}
func (m *mockStore) ExistsWithContentHash(_ string) (string, error) { return "", nil }
func (m *mockStore) GetTags(_ string) ([]string, error)            { return nil, nil }
func (m *mockStore) SetTags(_ string, _ []string) error            { return nil }
func (m *mockStore) CreateLink(_ context.Context, _, _, _ string, _ float64, _ bool) (*db.MemoryLink, error) {
	return nil, nil
}
func (m *mockStore) GetLinks(_ context.Context, _ string, _ string) ([]*db.MemoryLink, error) {
	return nil, nil
}
func (m *mockStore) DeleteLink(_ context.Context, _ string) error                { return nil }
func (m *mockStore) TraverseGraph(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}
func (m *mockStore) GetGraphData(_ context.Context, _ int) ([]*db.Memory, []*db.MemoryLink, error) {
	return nil, nil, nil
}
func (m *mockStore) Migrate() error { return nil }
func (m *mockStore) Close() error   { return nil }

func TestWriterWrite(t *testing.T) {
	store := newMockStore()
	w := NewWriter(store)

	resp := w.Write(context.Background(), &node.WriteRequest{
		Memory: &db.Memory{Content: "hello"},
	})
	if resp.Err != nil {
		t.Fatalf("unexpected error: %v", resp.Err)
	}
	if resp.Memory.ID == "" {
		t.Error("expected memory to have an ID")
	}
}

func TestWriterUpdateArchiveDelete(t *testing.T) {
	store := newMockStore()
	w := NewWriter(store)
	ctx := context.Background()

	resp := w.Write(ctx, &node.WriteRequest{Memory: &db.Memory{Content: "test"}})
	if resp.Err != nil {
		t.Fatalf("save: %v", resp.Err)
	}
	id := resp.Memory.ID

	if err := w.Update(ctx, &db.Memory{ID: id, Content: "updated"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := w.Archive(ctx, id); err != nil {
		t.Fatalf("archive: %v", err)
	}
}

func TestReaderGet(t *testing.T) {
	store := newMockStore()
	store.SaveMemory(&db.Memory{Content: "findme"})
	r := NewReader(store)

	mem, err := r.Get(context.Background(), "mem-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if mem.Content != "findme" {
		t.Errorf("content = %q, want %q", mem.Content, "findme")
	}
}

func TestReaderList(t *testing.T) {
	store := newMockStore()
	store.SaveMemory(&db.Memory{Content: "a"})
	store.SaveMemory(&db.Memory{Content: "b"})
	r := NewReader(store)

	list, err := r.List(context.Background(), &db.MemoryFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("list length = %d, want 2", len(list))
	}
}

func TestReaderHandleReadDispatch(t *testing.T) {
	store := newMockStore()
	store.SaveMemory(&db.Memory{Content: "dispatch"})
	r := NewReader(store)

	// Test ID dispatch
	resp := r.HandleRead(context.Background(), &node.ReadRequest{ID: "mem-1"})
	if resp.Err != nil {
		t.Fatalf("handle get: %v", resp.Err)
	}
	if resp.Memory == nil || resp.Memory.Content != "dispatch" {
		t.Error("expected memory from ID dispatch")
	}

	// Test list dispatch
	resp = r.HandleRead(context.Background(), &node.ReadRequest{Filter: &db.MemoryFilter{}})
	if resp.Err != nil {
		t.Fatalf("handle list: %v", resp.Err)
	}
	if len(resp.List) != 1 {
		t.Errorf("list length = %d, want 1", len(resp.List))
	}
}

func TestCoordinatorStartStop(t *testing.T) {
	store := newMockStore()
	cfg := &node.Config{
		Mode:               node.ModeEmbedded,
		WriterPoolSize:     2,
		ReaderPoolSize:     2,
		CoordinatorEnabled: true,
	}
	coord := NewCoordinator(cfg, store, slog.Default())

	if err := coord.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	reg := coord.Registry()
	if !reg.Has(node.TypeWriter) {
		t.Error("writer not registered")
	}
	if !reg.Has(node.TypeReader) {
		t.Error("reader not registered")
	}

	if err := coord.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestCoordinatorSaveAndGet(t *testing.T) {
	store := newMockStore()
	cfg := &node.Config{
		Mode:               node.ModeEmbedded,
		WriterPoolSize:     2,
		ReaderPoolSize:     4,
		CoordinatorEnabled: true,
	}
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	ctx := context.Background()

	saved, err := coord.SaveMemory(ctx, &db.Memory{Content: "coord test"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID == "" {
		t.Error("expected ID")
	}

	got, err := coord.GetMemory(ctx, saved.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "coord test" {
		t.Errorf("content = %q, want %q", got.Content, "coord test")
	}
}

func TestCoordinatorConcurrentOps(t *testing.T) {
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
	const N = 50

	// Concurrent writes
	var wg sync.WaitGroup
	ids := make([]string, N)
	var mu sync.Mutex

	wg.Add(N)
	for i := range N {
		go func(idx int) {
			defer wg.Done()
			saved, err := coord.SaveMemory(ctx, &db.Memory{
				Content: fmt.Sprintf("memory-%d", idx),
			})
			if err != nil {
				t.Errorf("save %d: %v", idx, err)
				return
			}
			mu.Lock()
			ids[idx] = saved.ID
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Verify all readable
	list, err := coord.ListMemories(ctx, &db.MemoryFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != N {
		t.Errorf("list length = %d, want %d", len(list), N)
	}
}

func TestCoordinatedStoreImplementsInterface(t *testing.T) {
	store := newMockStore()
	cfg := node.DefaultConfig()
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	cs := NewCoordinatedStore(coord, store)

	// Compile-time check that CoordinatedStore implements db.Store.
	var _ db.Store = cs
}

func TestCoordinatedStoreSaveAndGet(t *testing.T) {
	store := newMockStore()
	cfg := node.DefaultConfig()
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	cs := NewCoordinatedStore(coord, store)

	saved, err := cs.SaveMemory(&db.Memory{Content: "via store"})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := cs.GetMemory(saved.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "via store" {
		t.Errorf("content = %q, want %q", got.Content, "via store")
	}
}

func TestCoordinatedStoreDelegatedOps(t *testing.T) {
	store := newMockStore()
	cfg := node.DefaultConfig()
	coord := NewCoordinator(cfg, store, slog.Default())
	coord.Start(context.Background())
	defer coord.Stop()

	cs := NewCoordinatedStore(coord, store)

	// These operations delegate directly to the underlying store.
	if err := cs.Migrate(); err != nil {
		t.Errorf("migrate: %v", err)
	}
	if _, err := cs.ExistsWithContentHash("abc"); err != nil {
		t.Errorf("exists: %v", err)
	}
	if _, err := cs.GetTags("x"); err != nil {
		t.Errorf("get tags: %v", err)
	}
	if err := cs.SetTags("x", []string{"a"}); err != nil {
		t.Errorf("set tags: %v", err)
	}
	if err := cs.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
