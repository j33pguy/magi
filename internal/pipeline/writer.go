package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/embeddings"
	"github.com/j33pguy/magi/internal/remember"
	"github.com/j33pguy/magi/internal/secretstore"
)

// WriteRequest is a memory write submitted to the async pipeline.
type WriteRequest struct {
	Memory         *db.Memory
	Input          *remember.Input
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
	queue         chan writeJob
	embedder      embeddings.Provider
	store         db.Store
	batch         *BatchInserter
	status        *StatusTracker
	logger        *slog.Logger
	cfg           Config
	secretManager secretstore.Manager

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
	w := &Writer{
		queue:    make(chan writeJob, cfg.QueueSize),
		embedder: embedder,
		store:    store,
		status:   status,
		logger:   logger,
		cfg:      cfg,
		done:     make(chan struct{}),
	}
	w.batch = NewBatchInserter(store, status, cfg.FlushInterval, cfg.BatchMaxSize, logger.WithGroup("batch"), &w.completed, &w.failed)

	for i := 0; i < cfg.Workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	logger.Info("Async write pipeline started", "workers", cfg.Workers, "queue_size", cfg.QueueSize)
	return w
}

// SetSecretManager enables secret externalization for async remember writes.
func (w *Writer) SetSecretManager(manager secretstore.Manager) {
	w.secretManager = manager
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

	logger.Info("async write processing started", "id", job.id, "project", mem.Project, "type", mem.Type, "content_len", len(mem.Content))

	var dedupThreshold *float64
	if job.req.DedupThreshold > 0 {
		dedupThreshold = &job.req.DedupThreshold
	}
	input := remember.Input{
		Content:    mem.Content,
		Summary:    mem.Summary,
		Project:    mem.Project,
		Type:       mem.Type,
		Visibility: mem.Visibility,
		Source:     mem.Source,
		SourceFile: mem.SourceFile,
		Speaker:    mem.Speaker,
		Area:       mem.Area,
		SubArea:    mem.SubArea,
		Tags:       job.req.Tags,
	}
	if job.req.Input != nil {
		input = *job.req.Input
		if len(input.Tags) == 0 && len(job.req.Tags) > 0 {
			input.Tags = append([]string(nil), job.req.Tags...)
		}
	}

	prepared, err := remember.Prepare(ctx, w.store, w.embedder, input, remember.Options{
		DedupThreshold: dedupThreshold,
		TagMode:        remember.TagModeWarn,
		Logger:         logger,
		SecretManager:  w.secretManager,
	})
	if err != nil {
		logger.Error("async write prepare failed", "id", job.id, "project", mem.Project, "error", err)
		w.status.Set(job.id, StateFailed, err.Error())
		w.failed.Add(1)
		return
	}
	if prepared.Deduplicated {
		logger.Info("deduplicated async write", "id", job.id, "existing_id", prepared.Match.Memory.ID, "project", mem.Project)
		w.status.Set(job.id, StateComplete, "")
		w.completed.Add(1)
		return
	}

	logger.Info("queueing async write for batch insert", "id", job.id, "project", prepared.Memory.Project, "parent_id", prepared.Memory.ParentID, "tag_count", len(prepared.Tags))
	w.batch.Add(completedWrite{prepared: prepared, id: job.id})
}

// generateID produces a 32-char hex string matching SQLite's lower(hex(randomblob(16))).
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
