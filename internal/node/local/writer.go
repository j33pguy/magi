// Package local provides in-process node implementations that communicate
// via Go channels with zero serialization overhead.
package local

import (
	"context"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// Writer wraps a db.Store to satisfy the node.Writer interface.
type Writer struct {
	store db.Store
}

// NewWriter creates a local Writer backed by the given Store.
func NewWriter(store db.Store) *Writer {
	return &Writer{store: store}
}

func (w *Writer) Write(_ context.Context, req *node.WriteRequest) *node.WriteResponse {
	saved, err := w.store.SaveMemory(req.Memory)
	return &node.WriteResponse{Memory: saved, Err: err}
}

func (w *Writer) Update(_ context.Context, m *db.Memory) error {
	return w.store.UpdateMemory(m)
}

func (w *Writer) Archive(_ context.Context, id string) error {
	return w.store.ArchiveMemory(id)
}

func (w *Writer) Delete(_ context.Context, id string) error {
	return w.store.DeleteMemory(id)
}
