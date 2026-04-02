package cache

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/metrics"
)

// Store wraps a db.Store with hot memory and query caches.
type Store struct {
	delegate db.Store
	memories *MemoryCache
	queries  *QueryCache
}

// NewStore wraps a db.Store with cache layers driven by Config.
func NewStore(delegate db.Store, cfg Config) *Store {
	return &Store{
		delegate: delegate,
		memories: NewMemoryCache(cfg.MemorySize),
		queries:  NewQueryCache(cfg.QueryTTL),
	}
}

func (s *Store) SaveMemory(m *db.Memory) (*db.Memory, error) {
	saved, err := s.delegate.SaveMemory(m)
	if err != nil {
		return nil, err
	}
	if saved != nil {
		s.memories.Set(saved.ID, saved)
	}
	s.invalidateQueries("", "")
	return cloneMemory(saved), nil
}

func (s *Store) GetMemory(id string) (*db.Memory, error) {
	if cached := s.memories.Get(id); cached != nil {
		metrics.CacheHits.WithLabelValues("memory").Inc()
		return cached, nil
	}
	metrics.CacheMisses.WithLabelValues("memory").Inc()

	memory, err := s.delegate.GetMemory(id)
	if err != nil {
		return nil, err
	}
	if memory != nil {
		s.memories.Set(memory.ID, memory)
	}
	return cloneMemory(memory), nil
}

func (s *Store) UpdateMemory(m *db.Memory) error {
	if err := s.delegate.UpdateMemory(m); err != nil {
		return err
	}
	if m != nil && m.ID != "" {
		s.memories.Delete(m.ID)
	}
	s.invalidateQueries("", "")
	return nil
}

func (s *Store) ArchiveMemory(id string) error {
	if err := s.delegate.ArchiveMemory(id); err != nil {
		return err
	}
	s.memories.Delete(id)
	s.invalidateQueries("", "")
	return nil
}

func (s *Store) DeleteMemory(id string) error {
	if err := s.delegate.DeleteMemory(id); err != nil {
		return err
	}
	s.memories.Delete(id)
	s.invalidateQueries("", "")
	return nil
}

func (s *Store) ListMemories(filter *db.MemoryFilter) ([]*db.Memory, error) {
	memories, err := s.delegate.ListMemories(filter)
	if err != nil {
		return nil, err
	}
	s.warmMemories(memories)
	return cloneMemories(memories), nil
}

func (s *Store) CountMemories(filter *db.MemoryFilter) (int, error) {
	return s.delegate.CountMemories(filter)
}

func (s *Store) SearchMemories(embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	results, err := s.delegate.SearchMemories(embedding, filter, topK)
	if err != nil {
		return nil, err
	}
	s.warmVectorResults(results)
	return cloneVectorResults(results), nil
}

func (s *Store) SearchMemoriesBM25(query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	results, err := s.delegate.SearchMemoriesBM25(query, filter, topK)
	if err != nil {
		return nil, err
	}
	s.warmVectorResults(results)
	return cloneVectorResults(results), nil
}

func (s *Store) HybridSearch(embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error) {
	key := Key(query, filter, topK)
	if cached := s.queries.Get(key); cached != nil {
		metrics.CacheHits.WithLabelValues("query").Inc()
		s.warmHybridResults(cached)
		return cached, nil
	}
	metrics.CacheMisses.WithLabelValues("query").Inc()

	results, err := s.delegate.HybridSearch(embedding, query, filter, topK)
	if err != nil {
		return nil, err
	}
	s.queries.Set(key, results)
	s.warmHybridResults(results)
	return cloneHybridResults(results), nil
}

func (s *Store) GetContextMemories(project string, limit int) ([]*db.Memory, error) {
	memories, err := s.delegate.GetContextMemories(project, limit)
	if err != nil {
		return nil, err
	}
	s.warmMemories(memories)
	return cloneMemories(memories), nil
}

func (s *Store) FindSimilar(embedding []float32, maxDistance float64) (*db.VectorResult, error) {
	result, err := s.delegate.FindSimilar(embedding, maxDistance)
	if err != nil {
		return nil, err
	}
	if result != nil && result.Memory != nil {
		s.memories.Set(result.Memory.ID, result.Memory)
	}
	if result == nil {
		return nil, nil
	}
	cp := *result
	cp.Memory = cloneMemory(result.Memory)
	return &cp, nil
}

func (s *Store) ExistsWithContentHash(hash string) (string, error) {
	return s.delegate.ExistsWithContentHash(hash)
}

func (s *Store) GetTags(memoryID string) ([]string, error) {
	return s.delegate.GetTags(memoryID)
}

func (s *Store) SetTags(memoryID string, tags []string) error {
	if err := s.delegate.SetTags(memoryID, tags); err != nil {
		return err
	}
	if cached := s.memories.Get(memoryID); cached != nil {
		cached.Tags = cloneStrings(tags)
		s.memories.Set(memoryID, cached)
	}
	s.invalidateQueries("", "")
	return nil
}

func (s *Store) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*db.MemoryLink, error) {
	return s.delegate.CreateLink(ctx, fromID, toID, relation, weight, auto)
}

func (s *Store) GetLinks(ctx context.Context, memoryID string, direction string) ([]*db.MemoryLink, error) {
	return s.delegate.GetLinks(ctx, memoryID, direction)
}

func (s *Store) DeleteLink(ctx context.Context, linkID string) error {
	return s.delegate.DeleteLink(ctx, linkID)
}

func (s *Store) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
	return s.delegate.TraverseGraph(ctx, startID, maxDepth)
}

func (s *Store) GetGraphData(ctx context.Context, topN int) ([]*db.Memory, []*db.MemoryLink, error) {
	memories, links, err := s.delegate.GetGraphData(ctx, topN)
	if err != nil {
		return nil, nil, err
	}
	s.warmMemories(memories)
	return cloneMemories(memories), links, nil
}

func (s *Store) Migrate() error {
	return s.delegate.Migrate()
}

func (s *Store) Close() error {
	if s.queries != nil {
		s.queries.Close()
	}
	return s.delegate.Close()
}

func (s *Store) invalidateQueries(project, area string) {
	if s.queries != nil {
		s.queries.InvalidateForProject(project, area)
	}
}

func (s *Store) warmMemories(memories []*db.Memory) {
	for _, memory := range memories {
		if memory == nil || memory.ID == "" {
			continue
		}
		s.memories.Set(memory.ID, memory)
	}
}

func (s *Store) warmHybridResults(results []*db.HybridResult) {
	for _, result := range results {
		if result == nil || result.Memory == nil || result.Memory.ID == "" {
			continue
		}
		s.memories.Set(result.Memory.ID, result.Memory)
	}
}

func (s *Store) warmVectorResults(results []*db.VectorResult) {
	for _, result := range results {
		if result == nil || result.Memory == nil || result.Memory.ID == "" {
			continue
		}
		s.memories.Set(result.Memory.ID, result.Memory)
	}
}
