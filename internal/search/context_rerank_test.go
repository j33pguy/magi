package search

import (
	"context"
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

type contextOnlyStore struct {
	contexts map[string]db.MemoryContextLookup
}

func (s contextOnlyStore) LookupMemoryContexts(memoryIDs []string) (map[string]db.MemoryContextLookup, error) {
	out := make(map[string]db.MemoryContextLookup, len(memoryIDs))
	for _, id := range memoryIDs {
		if ctx, ok := s.contexts[id]; ok {
			out[id] = ctx
		}
	}
	return out, nil
}

func (contextOnlyStore) SaveMemory(*db.Memory) (*db.Memory, error)           { panic("unused") }
func (contextOnlyStore) GetMemory(string) (*db.Memory, error)                { panic("unused") }
func (contextOnlyStore) UpdateMemory(*db.Memory) error                       { panic("unused") }
func (contextOnlyStore) ArchiveMemory(string) error                          { panic("unused") }
func (contextOnlyStore) DeleteMemory(string) error                           { panic("unused") }
func (contextOnlyStore) ListMemories(*db.MemoryFilter) ([]*db.Memory, error) { panic("unused") }
func (contextOnlyStore) CountMemories(*db.MemoryFilter) (int, error)         { panic("unused") }
func (contextOnlyStore) SearchMemories([]float32, *db.MemoryFilter, int) ([]*db.VectorResult, error) {
	panic("unused")
}
func (contextOnlyStore) SearchMemoriesBM25(string, *db.MemoryFilter, int) ([]*db.VectorResult, error) {
	panic("unused")
}
func (contextOnlyStore) HybridSearch([]float32, string, *db.MemoryFilter, int) ([]*db.HybridResult, error) {
	panic("unused")
}
func (contextOnlyStore) GetContextMemories(string, int) ([]*db.Memory, error)     { panic("unused") }
func (contextOnlyStore) FindSimilar([]float32, float64) (*db.VectorResult, error) { panic("unused") }
func (contextOnlyStore) ExistsWithContentHash(string) (string, error)             { panic("unused") }
func (contextOnlyStore) GetTags(string) ([]string, error)                         { panic("unused") }
func (contextOnlyStore) SetTags(string, []string) error                           { panic("unused") }
func (contextOnlyStore) CreateLink(context.Context, string, string, string, float64, bool) (*db.MemoryLink, error) {
	panic("unused")
}
func (contextOnlyStore) GetLinks(context.Context, string, string) ([]*db.MemoryLink, error) {
	panic("unused")
}
func (contextOnlyStore) DeleteLink(context.Context, string) error { panic("unused") }
func (contextOnlyStore) TraverseGraph(context.Context, string, int) ([]string, error) {
	panic("unused")
}
func (contextOnlyStore) GetGraphData(context.Context, int) ([]*db.Memory, []*db.MemoryLink, error) {
	panic("unused")
}
func (contextOnlyStore) Migrate() error { panic("unused") }
func (contextOnlyStore) Close() error   { return nil }

func TestApplyContextBoostsPrefersSameRepository(t *testing.T) {
	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "other"}, RRFScore: 0.20, Score: 0.7},
		{Memory: &db.Memory{ID: "same"}, RRFScore: 0.19, Score: 0.7},
	}
	store := contextOnlyStore{contexts: map[string]db.MemoryContextLookup{
		"same":  {MemoryID: "same", RepositoryCanonical: "j33pguy/magi"},
		"other": {MemoryID: "other", RepositoryCanonical: "someone/else"},
	}}

	applyContextBoosts(store, &db.MemoryFilter{Project: "https://github.com/j33pguy/magi.git"}, results)

	if results[0].Memory.ID != "same" {
		t.Fatalf("expected same-repo result first, got %s", results[0].Memory.ID)
	}
	if results[0].RRFScore <= results[1].RRFScore {
		t.Fatalf("expected boosted score, got %#v", results)
	}
}

func TestApplyContextBoostsNoopWithoutRequestContext(t *testing.T) {
	results := []*db.HybridResult{{Memory: &db.Memory{ID: "same"}, RRFScore: 0.19, Score: 0.7}}
	store := contextOnlyStore{contexts: map[string]db.MemoryContextLookup{
		"same": {MemoryID: "same", RepositoryCanonical: "j33pguy/magi"},
	}}

	applyContextBoosts(store, &db.MemoryFilter{}, results)

	if results[0].RRFScore != 0.19 {
		t.Fatalf("expected no boost without request context, got %v", results[0].RRFScore)
	}
}
