// Package node defines the internal node abstraction for distributed MAGI.
//
// Phase 1 runs all node types (Writer, Reader, Index, Coordinator) as goroutine
// pools inside the same binary, communicating via Go channels with zero
// serialization overhead.
package node

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
)

// NodeType identifies the role a node plays in the mesh.
type NodeType string

const (
	TypeWriter      NodeType = "writer"
	TypeReader      NodeType = "reader"
	TypeIndex       NodeType = "index"
	TypeCoordinator NodeType = "coordinator"
)

// WriteRequest is a request to persist a memory.
type WriteRequest struct {
	Memory *db.Memory
}

// WriteResponse is the result of a write operation.
type WriteResponse struct {
	Memory *db.Memory
	Err    error
}

// ReadRequest is a request to retrieve or search memories.
type ReadRequest struct {
	// Exactly one of these should be set.
	ID        string           // GetMemory by ID
	Filter    *db.MemoryFilter // ListMemories
	Embedding []float32        // vector search
	Query     string           // BM25 / hybrid text query
	TopK      int              // max results for search
}

// ReadResponse is the result of a read operation.
type ReadResponse struct {
	Memory  *db.Memory         // single-get result
	List    []*db.Memory       // list result
	Hybrid  []*db.HybridResult // hybrid search result
	Vector  []*db.VectorResult // vector search result
	Err     error
}

// IndexRequest asks the index node to update search indices for a memory.
type IndexRequest struct {
	MemoryID  string
	Embedding []float32
	Content   string
	Tags      []string
}

// IndexResponse is the result of an indexing operation.
type IndexResponse struct {
	Err error
}

// Writer persists memories to storage.
type Writer interface {
	Write(ctx context.Context, req *WriteRequest) *WriteResponse
	Update(ctx context.Context, m *db.Memory) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// Reader retrieves and searches memories.
type Reader interface {
	Get(ctx context.Context, id string) (*db.Memory, error)
	List(ctx context.Context, filter *db.MemoryFilter) ([]*db.Memory, error)
	Search(ctx context.Context, embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error)
	SearchBM25(ctx context.Context, query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error)
	HybridSearch(ctx context.Context, embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error)
}

// Index manages search index updates.
type Index interface {
	Reindex(ctx context.Context, req *IndexRequest) *IndexResponse
}

// Coordinator routes requests to the appropriate node pools.
type Coordinator interface {
	// Store operations — delegated to Writer pool.
	SaveMemory(ctx context.Context, m *db.Memory) (*db.Memory, error)
	UpdateMemory(ctx context.Context, m *db.Memory) error
	ArchiveMemory(ctx context.Context, id string) error
	DeleteMemory(ctx context.Context, id string) error

	// Retrieval operations — delegated to Reader pool.
	GetMemory(ctx context.Context, id string) (*db.Memory, error)
	ListMemories(ctx context.Context, filter *db.MemoryFilter) ([]*db.Memory, error)
	SearchMemories(ctx context.Context, embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error)
	SearchMemoriesBM25(ctx context.Context, query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error)
	HybridSearch(ctx context.Context, embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error)

	// Lifecycle
	Start(ctx context.Context) error
	Stop() error
}
