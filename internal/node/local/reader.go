package local

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// Reader wraps a db.Store to satisfy the node.Reader interface.
type Reader struct {
	store db.Store
}

// NewReader creates a local Reader backed by the given Store.
func NewReader(store db.Store) *Reader {
	return &Reader{store: store}
}

func (r *Reader) Get(_ context.Context, id string) (*db.Memory, error) {
	return r.store.GetMemory(id)
}

func (r *Reader) List(_ context.Context, filter *db.MemoryFilter) ([]*db.Memory, error) {
	return r.store.ListMemories(filter)
}

func (r *Reader) Search(_ context.Context, embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	return r.store.SearchMemories(embedding, filter, topK)
}

func (r *Reader) SearchBM25(_ context.Context, query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	return r.store.SearchMemoriesBM25(query, filter, topK)
}

func (r *Reader) HybridSearch(_ context.Context, embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error) {
	return r.store.HybridSearch(embedding, query, filter, topK)
}

// HandleRead is the pool handler function for read requests.
// It dispatches to the appropriate Reader method based on which fields are set.
func (r *Reader) HandleRead(ctx context.Context, req *node.ReadRequest) *node.ReadResponse {
	resp := &node.ReadResponse{}

	switch {
	case req.ID != "":
		resp.Memory, resp.Err = r.Get(ctx, req.ID)
	case req.Embedding != nil && req.Query != "":
		resp.Hybrid, resp.Err = r.HybridSearch(ctx, req.Embedding, req.Query, req.Filter, req.TopK)
	case req.Embedding != nil:
		resp.Vector, resp.Err = r.Search(ctx, req.Embedding, req.Filter, req.TopK)
	case req.Query != "":
		resp.Vector, resp.Err = r.SearchBM25(ctx, req.Query, req.Filter, req.TopK)
	case req.Filter != nil:
		resp.List, resp.Err = r.List(ctx, req.Filter)
	}

	return resp
}
