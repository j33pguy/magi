package db

import "context"

// Store is the primary storage interface for memories.
// Implementations: TursoClient (cloud), SQLiteClient (local).
type Store interface {
	// Core CRUD
	SaveMemory(m *Memory) (*Memory, error)
	GetMemory(id string) (*Memory, error)
	UpdateMemory(m *Memory) error
	ArchiveMemory(id string) error
	DeleteMemory(id string) error
	ListMemories(filter *MemoryFilter) ([]*Memory, error)

	// Search
	SearchMemories(embedding []float32, filter *MemoryFilter, topK int) ([]*VectorResult, error)
	SearchMemoriesBM25(query string, filter *MemoryFilter, topK int) ([]*VectorResult, error)
	HybridSearch(embedding []float32, query string, filter *MemoryFilter, topK int) ([]*HybridResult, error)
	GetContextMemories(project string, limit int) ([]*Memory, error)
	FindSimilar(embedding []float32, maxDistance float64) (*VectorResult, error)

	// Tags
	ExistsWithContentHash(hash string) (string, error)
	GetTags(memoryID string) ([]string, error)
	SetTags(memoryID string, tags []string) error

	// Links
	CreateLink(ctx context.Context, fromID, toID, relation string, weight float64, auto bool) (*MemoryLink, error)
	GetLinks(ctx context.Context, memoryID string, direction string) ([]*MemoryLink, error)
	DeleteLink(ctx context.Context, linkID string) error
	TraverseGraph(ctx context.Context, startID string, maxDepth int) ([]string, error)
	GetGraphData(ctx context.Context, topN int) ([]*Memory, []*MemoryLink, error)

	// Migrations
	Migrate() error

	// Lifecycle
	Close() error
}
