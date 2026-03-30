package pipeline

import (
	"log/slog"
	"sync"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// completedWrite holds a fully-processed memory ready for DB insertion.
type completedWrite struct {
	memory *db.Memory
	tags   []string
	id     string // pre-generated ID for status tracking
}

// BatchInserter collects completed writes and flushes them in batches.
type BatchInserter struct {
	store         db.Store
	status        *StatusTracker
	flushInterval time.Duration
	maxSize       int
	logger        *slog.Logger

	mu      sync.Mutex
	pending []completedWrite
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewBatchInserter creates a batch inserter that flushes on interval or size threshold.
func NewBatchInserter(store db.Store, status *StatusTracker, flushInterval time.Duration, maxSize int, logger *slog.Logger) *BatchInserter {
	bi := &BatchInserter{
		store:         store,
		status:        status,
		flushInterval: flushInterval,
		maxSize:       maxSize,
		logger:        logger,
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
		saved, err := bi.store.SaveMemory(cw.memory)
		if err != nil {
			bi.logger.Error("batch insert failed", "id", cw.id, "error", err)
			bi.status.Set(cw.id, StateFailed, err.Error())
			continue
		}

		if len(cw.tags) > 0 {
			if err := bi.store.SetTags(saved.ID, cw.tags); err != nil {
				bi.logger.Warn("batch tag write failed (non-fatal)", "id", saved.ID, "error", err)
			}
		}

		bi.status.Set(cw.id, StateComplete, "")
		bi.logger.Debug("batch inserted memory", "id", saved.ID, "pre_id", cw.id)
	}
}
