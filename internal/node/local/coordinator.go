package local

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/node"
)

// Coordinator is the embedded-mode coordinator. It creates Writer and Reader
// goroutine pools backed by a shared db.Store, routes requests through them,
// and registers capabilities in the node registry.
type Coordinator struct {
	cfg      *node.Config
	store    db.Store
	logger   *slog.Logger
	registry *node.Registry
	router   *node.Router

	writer *Writer
	reader *Reader
	index  *Index

	writerPool *node.Pool[*node.WriteRequest, *node.WriteResponse]
	readerPool *node.Pool[*node.ReadRequest, *node.ReadResponse]
}

// NewCoordinator creates an embedded coordinator wired to the given Store.
func NewCoordinator(cfg *node.Config, store db.Store, logger *slog.Logger) *Coordinator {
	writer := NewWriter(store)
	reader := NewReader(store)
	idx := NewIndex(store)

	writerPool := node.NewPool("writer", cfg.WriterPoolSize, writer.Write)
	readerPool := node.NewPool("reader", cfg.ReaderPoolSize, reader.HandleRead)

	router := node.NewRouter(writerPool, readerPool)
	registry := node.NewRegistry()

	return &Coordinator{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		registry:   registry,
		router:     router,
		writer:     writer,
		reader:     reader,
		index:      idx,
		writerPool: writerPool,
		readerPool: readerPool,
	}
}

func (c *Coordinator) Start(ctx context.Context) error {
	c.writerPool.Start(ctx)
	c.readerPool.Start(ctx)

	c.registry.Register(&node.Capability{Type: node.TypeWriter, PoolSize: c.cfg.WriterPoolSize, Mode: c.cfg.Mode})
	c.registry.Register(&node.Capability{Type: node.TypeReader, PoolSize: c.cfg.ReaderPoolSize, Mode: c.cfg.Mode})
	c.registry.Register(&node.Capability{Type: node.TypeIndex, PoolSize: 1, Mode: c.cfg.Mode})
	c.registry.Register(&node.Capability{Type: node.TypeCoordinator, PoolSize: 1, Mode: c.cfg.Mode})

	c.logger.Info("Node mesh started",
		"mode", c.cfg.Mode,
		"writers", c.cfg.WriterPoolSize,
		"readers", c.cfg.ReaderPoolSize,
	)
	return nil
}

func (c *Coordinator) Stop() error {
	c.writerPool.Stop()
	c.readerPool.Stop()
	c.logger.Info("Node mesh stopped")
	return nil
}

// Registry returns the coordinator's node registry.
func (c *Coordinator) Registry() *node.Registry {
	return c.registry
}

// Router returns the coordinator's request router.
func (c *Coordinator) Router() *node.Router {
	return c.router
}

// --- Write operations ---

func (c *Coordinator) SaveMemory(ctx context.Context, m *db.Memory) (*db.Memory, error) {
	resp, err := c.router.RouteWrite(ctx, &node.WriteRequest{Memory: m})
	if err != nil {
		return nil, fmt.Errorf("coordinator save: %w", err)
	}
	return resp.Memory, resp.Err
}

func (c *Coordinator) UpdateMemory(ctx context.Context, m *db.Memory) error {
	return c.writer.Update(ctx, m)
}

func (c *Coordinator) ArchiveMemory(ctx context.Context, id string) error {
	return c.writer.Archive(ctx, id)
}

func (c *Coordinator) DeleteMemory(ctx context.Context, id string) error {
	return c.writer.Delete(ctx, id)
}

// --- Read operations ---

func (c *Coordinator) GetMemory(ctx context.Context, id string) (*db.Memory, error) {
	resp, err := c.router.RouteRead(ctx, &node.ReadRequest{ID: id})
	if err != nil {
		return nil, fmt.Errorf("coordinator get: %w", err)
	}
	return resp.Memory, resp.Err
}

func (c *Coordinator) ListMemories(ctx context.Context, filter *db.MemoryFilter) ([]*db.Memory, error) {
	resp, err := c.router.RouteRead(ctx, &node.ReadRequest{Filter: filter})
	if err != nil {
		return nil, fmt.Errorf("coordinator list: %w", err)
	}
	return resp.List, resp.Err
}

func (c *Coordinator) SearchMemories(ctx context.Context, embedding []float32, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	resp, err := c.router.RouteRead(ctx, &node.ReadRequest{Embedding: embedding, Filter: filter, TopK: topK})
	if err != nil {
		return nil, fmt.Errorf("coordinator search: %w", err)
	}
	return resp.Vector, resp.Err
}

func (c *Coordinator) SearchMemoriesBM25(ctx context.Context, query string, filter *db.MemoryFilter, topK int) ([]*db.VectorResult, error) {
	resp, err := c.router.RouteRead(ctx, &node.ReadRequest{Query: query, Filter: filter, TopK: topK})
	if err != nil {
		return nil, fmt.Errorf("coordinator bm25: %w", err)
	}
	return resp.Vector, resp.Err
}

func (c *Coordinator) HybridSearch(ctx context.Context, embedding []float32, query string, filter *db.MemoryFilter, topK int) ([]*db.HybridResult, error) {
	resp, err := c.router.RouteRead(ctx, &node.ReadRequest{Embedding: embedding, Query: query, Filter: filter, TopK: topK})
	if err != nil {
		return nil, fmt.Errorf("coordinator hybrid: %w", err)
	}
	return resp.Hybrid, resp.Err
}
