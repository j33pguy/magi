package local

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
)

// CoordinatedStore wraps a Coordinator to implement db.Store.
// This allows the node mesh to be used as a drop-in replacement for direct
// Store access — all existing API/gRPC/MCP endpoints remain unchanged.
type CoordinatedStore struct {
	coord    *Coordinator
	delegate db.Store // underlying store for operations not yet routed through pools
}

// NewCoordinatedStore creates a Store that routes through the coordinator's pools.
func NewCoordinatedStore(coord *Coordinator, delegate db.Store) *CoordinatedStore {
	return &CoordinatedStore{coord: coord, delegate: delegate}
}

func (s *CoordinatedStore) SaveMemory(m *db.Memory) (*db.Memory, error) {
	return s.coord.SaveMemory(context.Background(), m)
}

func (s *CoordinatedStore) GetMemory(id string) (*db.Memory, error) {
	return s.coord.GetMemory(context.Background(), id)
}

func (s *CoordinatedStore) UpdateMemory(m *db.Memory) error {
	return s.coord.UpdateMemory(context.Background(), m)
}

func (s *CoordinatedStore) ArchiveMemory(id string) error {
	return s.coord.ArchiveMemory(context.Background(), id)
}

func (s *CoordinatedStore) DeleteMemory(id string) error {
	return s.coord.DeleteMemory(context.Background(), id)
}

func (s *CoordinatedStore) ListMemories(filter *db.MemoryFilter) ([]*db.Memory, error) {
	return s.coord.ListMemories(context.Background(), filter)
}

func (s *CoordinatedStore) SearchMemories(embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	return s.coord.SearchMemories(context.Background(), embedding, filter, topK)
}

func (s *CoordinatedStore) SearchMemoriesBM25(query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	return s.coord.SearchMemoriesBM25(context.Background(), query, filter, topK)
}

func (s *CoordinatedStore) HybridSearch(embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error) {
	return s.coord.HybridSearch(context.Background(), embedding, query, filter, topK)
}

func (s *CoordinatedStore) GetContextMemories(project string, limit int) ([]*db.Memory, error) {
	return s.delegate.GetContextMemories(project, limit)
}

func (s *CoordinatedStore) FindSimilar(embedding []float32, maxDistance float64) (*db.VectorResult, error) {
	return s.delegate.FindSimilar(embedding, maxDistance)
}

func (s *CoordinatedStore) ExistsWithContentHash(hash string) (string, error) {
	return s.delegate.ExistsWithContentHash(hash)
}

func (s *CoordinatedStore) GetTags(memoryID string) ([]string, error) {
	return s.delegate.GetTags(memoryID)
}

func (s *CoordinatedStore) SetTags(memoryID string, tags []string) error {
	return s.delegate.SetTags(memoryID, tags)
}

func (s *CoordinatedStore) CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*db.MemoryLink, error) {
	return s.delegate.CreateLink(ctx, fromID, toID, relation, weight, auto)
}

func (s *CoordinatedStore) GetLinks(ctx context.Context, memoryID string, direction string) ([]*db.MemoryLink, error) {
	return s.delegate.GetLinks(ctx, memoryID, direction)
}

func (s *CoordinatedStore) DeleteLink(ctx context.Context, linkID string) error {
	return s.delegate.DeleteLink(ctx, linkID)
}

func (s *CoordinatedStore) TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error) {
	return s.delegate.TraverseGraph(ctx, startID, maxDepth)
}

func (s *CoordinatedStore) GetGraphData(ctx context.Context, topN int) ([]*db.Memory, []*db.MemoryLink, error) {
	return s.delegate.GetGraphData(ctx, topN)
}

func (s *CoordinatedStore) Migrate() error {
	return s.delegate.Migrate()
}

func (s *CoordinatedStore) Close() error {
	return s.delegate.Close()
}
