package pipeline

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/j33pguy/magi/internal/db"
	"github.com/j33pguy/magi/internal/remember"
)

// completedWrite holds a fully-processed memory ready for DB insertion.
type completedWrite struct {
	prepared *remember.PreparedWrite
	id       string // pre-generated ID for status tracking
}

// BatchInserter collects completed writes and flushes them in batches.
type BatchInserter struct {
	store         db.Store
	status        *StatusTracker
	flushInterval time.Duration
	maxSize       int
	logger        *slog.Logger
	completed     *atomic.Int64
	failed        *atomic.Int64

	mu      sync.Mutex
	pending []completedWrite
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewBatchInserter creates a batch inserter that flushes on interval or size threshold.
func NewBatchInserter(store db.Store, status *StatusTracker, flushInterval time.Duration, maxSize int, logger *slog.Logger, completed *atomic.Int64, failed *atomic.Int64) *BatchInserter {
	bi := &BatchInserter{
		store:         store,
		status:        status,
		flushInterval: flushInterval,
		maxSize:       maxSize,
		logger:        logger,
		completed:     completed,
		failed:        failed,
		pending:       make([]completedWrite, 0, maxSize),
		done:          make(chan struct{}),
	}
	bi.wg.Add(1)
	go bi.run()
	return bi
}

// Add queues a completed write for batch insertion.
// If adding this item reaches maxSize, triggers an immediate flush.
func (bi *BatchInserter) Add(cw completedWrite) {
	bi.mu.Lock()
	bi.pending = append(bi.pending, cw)
	shouldFlush := len(bi.pending) >= bi.maxSize
	bi.mu.Unlock()

	if shouldFlush {
		bi.flush()
	}
}

// Close drains remaining items and stops the batch inserter.
func (bi *BatchInserter) Close() {
	close(bi.done)
	bi.wg.Wait()
	// Final flush for any remaining items
	bi.flush()
}

// QueueDepth returns the number of items waiting to be flushed.
func (bi *BatchInserter) QueueDepth() int {
	bi.mu.Lock()
	defer bi.mu.Unlock()
	return len(bi.pending)
}

func (bi *BatchInserter) run() {
	defer bi.wg.Done()
	ticker := time.NewTicker(bi.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bi.done:
			return
		case <-ticker.C:
			bi.flush()
		}
	}
}

func (bi *BatchInserter) flush() {
	bi.mu.Lock()
	if len(bi.pending) == 0 {
		bi.mu.Unlock()
		return
	}
	batch := bi.pending
	bi.pending = make([]completedWrite, 0, bi.maxSize)
	bi.mu.Unlock()

	for _, cw := range batch {
		if cw.prepared == nil || cw.prepared.Memory == nil {
			bi.logger.Error("batch insert missing prepared write", "id", cw.id)
			bi.status.Set(cw.id, StateFailed, "prepared write missing")
			if bi.failed != nil {
				bi.failed.Add(1)
			}
			continue
		}
		bi.logger.Info("batch insert starting", "id", cw.id, "project", cw.prepared.Memory.Project, "parent_id", cw.prepared.Memory.ParentID, "tag_count", len(cw.prepared.Tags))
		result, err := remember.Persist(bi.store, cw.prepared, remember.Options{TagMode: remember.TagModeWarn, Logger: bi.logger})
		if err != nil {
			bi.logger.Error("batch insert failed", "id", cw.id, "project", cw.prepared.Memory.Project, "error", err)
			bi.status.Set(cw.id, StateFailed, err.Error())
			if bi.failed != nil {
				bi.failed.Add(1)
			}
			continue
		}

		bi.status.Set(cw.id, StateComplete, "")
		if bi.completed != nil {
			bi.completed.Add(1)
		}
		bi.logger.Info("batch insert complete", "id", cw.id, "saved_id", result.Saved.ID, "project", result.Saved.Project)
	}
}
