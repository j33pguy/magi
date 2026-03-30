package local

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// Index wraps a db.Store to satisfy the node.Index interface.
// In embedded mode, indexing happens inline during writes (the DB handles FTS
// triggers and vector storage), so this is a thin pass-through for tag updates.
type Index struct {
	store db.Store
}

// NewIndex creates a local Index backed by the given Store.
func NewIndex(store db.Store) *Index {
	return &Index{store: store}
}

func (idx *Index) Reindex(_ context.Context, req *node.IndexRequest) *node.IndexResponse {
	if len(req.Tags) > 0 {
		if err := idx.store.SetTags(req.MemoryID, req.Tags); err != nil {
			return &node.IndexResponse{Err: err}
		}
	}
	return &node.IndexResponse{}
}
