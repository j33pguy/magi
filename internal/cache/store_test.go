package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

type fakeStore struct {
	memories     map[string]*db.Memory
	tags         map[string][]string
	hybrid       []*db.HybridResult
	getCalls     int
	hybridCalls  int
	saveCalls    int
	updateCalls  int
	archiveCalls int
	deleteCalls  int
	closeCalls   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		memories: make(map[string]*db.Memory),
		tags:     make(map[string][]string),
	}
}

func (f *fakeStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	f.saveCalls++
	if m.ID == "" {
		m.ID = fmt.Sprintf("mem-%d", len(f.memories)+1)
	}
	cp := cloneMemory(m)
	f.memories[m.ID] = cp
	return cloneMemory(cp), nil
}

func (f *fakeStore) GetMemory(id string) (*db.Memory, error) {
	f.getCalls++
	m, ok := f.memories[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := cloneMemory(m)
	if tags, ok := f.tags[id]; ok {
		cp.Tags = cloneStrings(tags)
	}
	return cp, nil
}

func (f *fakeStore) UpdateMemory(m *db.Memory) error {
	f.updateCalls++
	f.memories[m.ID] = cloneMemory(m)
	return nil
}

func (f *fakeStore) ArchiveMemory(id string) error {
	f.archiveCalls++
	delete(f.memories, id)
	return nil
}

func (f *fakeStore) DeleteMemory(id string) error {
	f.deleteCalls++
	delete(f.memories, id)
	return nil
}

func (f *fakeStore) ListMemories(_ *db.MemoryFilter) ([]*db.Memory, error) {
	var out []*db.Memory
	for _, m := range f.memories {
		out = append(out, cloneMemory(m))
	}
	return out, nil
}

func (f *fakeStore) CountMemories(_ *db.MemoryFilter) (int, error) {
	return len(f.memories), nil
}

func (f *fakeStore) SearchMemories(_ []float32, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, nil
}

func (f *fakeStore) SearchMemoriesBM25(_ string, _ *db.MemoryFilter, _ int) ([]*db.VectorResult, error) {
	return nil, nil
}

func (f *fakeStore) HybridSearch(_ []float32, _ string, _ *db.MemoryFilter, _ int) ([]*db.HybridResult, error) {
	f.hybridCalls++
	return cloneHybridResults(f.hybrid), nil
}

func (f *fakeStore) GetContextMemories(_ string, _ int) ([]*db.Memory, error) {
	return nil, nil
}

func (f *fakeStore) FindSimilar(_ []float32, _ float64) (*db.VectorResult, error) {
	return nil, nil
}

func (f *fakeStore) ExistsWithContentHash(_ string) (string, error) {
	return "", nil
}

func (f *fakeStore) GetTags(memoryID string) ([]string, error) {
	return cloneStrings(f.tags[memoryID]), nil
}

func (f *fakeStore) SetTags(memoryID string, tags []string) error {
	f.tags[memoryID] = cloneStrings(tags)
	return nil
}

func (f *fakeStore) CreateLink(_ context.Context, _, _, _ string, _ float64, _ bool) (*db.MemoryLink, error) {
	return nil, nil
}

func (f *fakeStore) GetLinks(_ context.Context, _ string, _ string) ([]*db.MemoryLink, error) {
	return nil, nil
}

func (f *fakeStore) DeleteLink(_ context.Context, _ string) error {
	return nil
}

func (f *fakeStore) TraverseGraph(_ context.Context, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (f *fakeStore) GetGraphData(_ context.Context, _ int) ([]*db.Memory, []*db.MemoryLink, error) {
	return nil, nil, nil
}

func (f *fakeStore) Migrate() error {
	return nil
}

func (f *fakeStore) Close() error {
	f.closeCalls++
	return nil
}

func TestStoreCachesGetMemory(t *testing.T) {
	delegate := newFakeStore()
	delegate.memories["mem-1"] = &db.Memory{ID: "mem-1", Content: "hello", Tags: []string{"tag-1"}}

	store := NewStore(delegate, Config{Enabled: true, QueryTTL: time.Minute, MemorySize: 10})
	defer store.Close()

	first, err := store.GetMemory("mem-1")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	first.Content = "changed"
	first.Tags[0] = "mutated"

	second, err := store.GetMemory("mem-1")
	if err != nil {
		t.Fatalf("GetMemory second: %v", err)
	}

	if delegate.getCalls != 1 {
		t.Fatalf("expected 1 delegate get, got %d", delegate.getCalls)
	}
	if second.Content != "hello" {
		t.Fatalf("expected cached memory to remain unchanged, got %q", second.Content)
	}
	if got := second.Tags[0]; got != "tag-1" {
		t.Fatalf("expected cached tags to remain unchanged, got %q", got)
	}
}

func TestStoreCachesHybridSearchAndInvalidatesOnWrite(t *testing.T) {
	delegate := newFakeStore()
	delegate.hybrid = []*db.HybridResult{
		{Memory: &db.Memory{ID: "mem-1", Content: "hello"}, RRFScore: 0.8},
	}

	store := NewStore(delegate, Config{Enabled: true, QueryTTL: time.Minute, MemorySize: 10})
	defer store.Close()

	first, err := store.HybridSearch([]float32{1}, "hello", &db.MemoryFilter{Project: "magi"}, 5)
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	first[0].Memory.Content = "changed"

	second, err := store.HybridSearch([]float32{1}, "hello", &db.MemoryFilter{Project: "magi"}, 5)
	if err != nil {
		t.Fatalf("HybridSearch second: %v", err)
	}
	if delegate.hybridCalls != 1 {
		t.Fatalf("expected 1 delegate search call, got %d", delegate.hybridCalls)
	}
	if second[0].Memory.Content != "hello" {
		t.Fatalf("expected cached query result to remain unchanged, got %q", second[0].Memory.Content)
	}

	if _, err := store.SaveMemory(&db.Memory{Content: "new"}); err != nil {
		t.Fatalf("SaveMemory: %v", err)
	}

	if _, err := store.HybridSearch([]float32{1}, "hello", &db.MemoryFilter{Project: "magi"}, 5); err != nil {
		t.Fatalf("HybridSearch after write: %v", err)
	}
	if delegate.hybridCalls != 2 {
		t.Fatalf("expected query cache invalidation after write, got %d delegate calls", delegate.hybridCalls)
	}
}

func TestStoreSetTagsRefreshesCachedMemory(t *testing.T) {
	delegate := newFakeStore()
	delegate.memories["mem-1"] = &db.Memory{ID: "mem-1", Content: "hello"}

	store := NewStore(delegate, Config{Enabled: true, QueryTTL: time.Minute, MemorySize: 10})
	defer store.Close()

	if _, err := store.GetMemory("mem-1"); err != nil {
		t.Fatalf("GetMemory warm cache: %v", err)
	}
	if err := store.SetTags("mem-1", []string{"owner:UserA", "conversation"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	got, err := store.GetMemory("mem-1")
	if err != nil {
		t.Fatalf("GetMemory after SetTags: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "owner:UserA" {
		t.Fatalf("expected cached tags to be refreshed, got %#v", got.Tags)
	}
}
