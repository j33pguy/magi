package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/j33pguy/magi/internal/classify"
	"github.com/j33pguy/magi/internal/contradiction"
	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
)

// WriteRequest is a memory write submitted to the async pipeline.
type WriteRequest struct {
	Memory         *db.Memory
	Tags           []string
	DedupThreshold float64
}

// WriteResult is returned immediately to the caller with the pre-generated ID.
type WriteResult struct {
	ID string
}

// Stats holds pipeline throughput statistics.
type Stats struct {
	QueueDepth   int   `json:"queue_depth"`
	BatchPending int   `json:"batch_pending"`
	Workers      int   `json:"workers"`
	Submitted    int64 `json:"submitted"`
	Completed    int64 `json:"completed"`
	Failed       int64 `json:"failed"`
}

// Writer is the async write pipeline. Callers submit writes and get back
// an ID immediately; workers handle embedding, classification, and DB insert.
type Writer struct {
	queue    chan writeJob
	embedder embeddings.Provider
	store    db.Store
	batch    *BatchInserter
	status   *StatusTracker
	logger   *slog.Logger
	cfg      Config

	submitted atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64

	wg   sync.WaitGroup
	done chan struct{}
}

type writeJob struct {
	req WriteRequest
	id  string
}

// NewWriter creates and starts the async write pipeline.
func NewWriter(store db.Store, embedder embeddings.Provider, cfg Config, logger *slog.Logger) *Writer {
	status := NewStatusTracker()
	batch := NewBatchInserter(store, status, cfg.FlushInterval, cfg.BatchMaxSize, logger.WithGroup("batch"))

	w := &Writer{
		queue:    make(chan writeJob, cfg.QueueSize),
		embedder: embedder,
		store:    store,
		batch:    batch,
		status:   status,
		logger:   logger,
		cfg:      cfg,
		done:     make(chan struct{}),
	}

	for i := 0; i < cfg.Workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	logger.Info("Async write pipeline started", "workers", cfg.Workers, "queue_size", cfg.QueueSize)
	return w
}

// Submit enqueues a write request and returns the pre-generated memory ID.
// Returns an error only if the queue is full (backpressure).
func (w *Writer) Submit(req WriteRequest) (*WriteResult, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generating ID: %w", err)
	}

	w.status.Set(id, StatePending, "")

	select {
	case w.queue <- writeJob{req: req, id: id}:
		w.submitted.Add(1)
		return &WriteResult{ID: id}, nil
	default:
		w.status.Set(id, StateFailed, "queue full")
		w.failed.Add(1)
		return nil, fmt.Errorf("write queue full (size=%d), apply backpressure", w.cfg.QueueSize)
	}
}

// Status returns the write status for a given memory ID.
func (w *Writer) Status(id string) *WriteStatus {
	return w.status.Get(id)
}

// Stats returns current pipeline statistics.
func (w *Writer) Stats() Stats {
	return Stats{
		QueueDepth:   len(w.queue),
		BatchPending: w.batch.QueueDepth(),
		Workers:      w.cfg.Workers,
		Submitted:    w.submitted.Load(),
		Completed:    w.completed.Load(),
		Failed:       w.failed.Load(),
	}
}

// Close gracefully shuts down the pipeline: stops accepting new writes,
// drains the queue, and flushes remaining batches.
func (w *Writer) Close() {
	close(w.done)
	// Drain remaining queue items
	close(w.queue)
	w.wg.Wait()
	w.batch.Close()
	w.status.Close()
	w.logger.Info("Async write pipeline stopped",
		"submitted", w.submitted.Load(),
		"completed", w.completed.Load(),
		"failed", w.failed.Load(),
	)
}

func (w *Writer) worker(id int) {
	defer w.wg.Done()
	logger := w.logger.With("worker", id)

	for job := range w.queue {
		w.processJob(logger, job)
	}
}

func (w *Writer) processJob(logger *slog.Logger, job writeJob) {
	w.status.Set(job.id, StateProcessing, "")
	mem := job.req.Memory
	ctx := context.Background()

	// 1. Generate embedding if not already present
	if mem.Embedding == nil {
		emb, err := w.embedder.Embed(ctx, mem.Content)
		if err != nil {
			logger.Error("embedding failed", "id", job.id, "error", err)
			w.status.Set(job.id, StateFailed, fmt.Sprintf("embedding: %v", err))
			w.failed.Add(1)
			return
		}
		mem.Embedding = emb
	}

	// 2. Auto-classify if area/subarea not set
	if mem.Area == "" || mem.SubArea == "" {
		c := classify.Infer(mem.Content)
		if mem.Area == "" {
			mem.Area = c.Area
		}
		if mem.SubArea == "" {
			mem.SubArea = c.SubArea
		}
	}

	// 3. Deduplication check
	dedupThreshold := job.req.DedupThreshold
	if dedupThreshold <= 0 || dedupThreshold > 1 {
		dedupThreshold = 0.95
	}
	maxDistance := 1.0 - dedupThreshold
	groupDistance := 0.15 // 1.0 - 0.85

	match, err := w.store.FindSimilar(mem.Embedding, groupDistance)
	if err != nil {
		logger.Warn("dedup check failed, proceeding", "id", job.id, "error", err)
	} else if match != nil && match.Distance <= maxDistance {
		logger.Info("deduplicated async write", "id", job.id, "existing_id", match.Memory.ID)
		w.status.Set(job.id, StateComplete, "")
		w.completed.Add(1)
		return
	}

	// Soft-group: link to similar parent
	if match != nil {
		mem.ParentID = match.Memory.ID
	}

	// 4. Build tags
	tags := append([]string{}, job.req.Tags...)
	if mem.Speaker != "" {
		tags = append(tags, "speaker:"+mem.Speaker)
	}
	if mem.Area != "" {
		tags = append(tags, "area:"+mem.Area)
	}
	if mem.SubArea != "" {
		tags = append(tags, "sub_area:"+mem.SubArea)
	}

	// 5. Contradiction detection (best-effort)
	detector := &contradiction.Detector{Threshold: 0.85}
	if _, cErr := detector.Check(ctx, w.store, w.embedder, mem.Content, mem.Area, mem.SubArea); cErr != nil {
		logger.Warn("contradiction detection failed", "id", job.id, "error", cErr)
	}

	// 6. Submit to batch inserter
	w.batch.Add(completedWrite{
		memory: mem,
		tags:   tags,
		id:     job.id,
	})

	w.completed.Add(1)
}

// generateID produces a 32-char hex string matching SQLite's lower(hex(randomblob(16))).
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
