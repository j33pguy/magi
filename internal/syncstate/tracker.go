package syncstate

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// Tracker tracks sync activity for a project.
type Tracker struct {
	mu       sync.Mutex
	cond     *sync.Cond
	lastSync time.Time
	syncing  bool
	lastErr  string
}

// Snapshot captures a point-in-time view of sync state.
type Snapshot struct {
	LastSync time.Time
	Syncing  bool
	LastErr  string
}

// NewTracker creates a new sync tracker.
func NewTracker() *Tracker {
	t := &Tracker{}
	t.cond = sync.NewCond(&t.mu)
	return t
}

// Start marks a sync as in progress. Returns false if a sync is already running.
func (t *Tracker) Start() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.syncing {
		return false
	}
	t.syncing = true
	return true
}

// Finish marks a sync as complete and records any error.
func (t *Tracker) Finish(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.syncing = false
	if err == nil {
		t.lastSync = time.Now().UTC()
		t.lastErr = ""
	} else {
		t.lastErr = err.Error()
	}
	t.cond.Broadcast()
}

// Wait blocks until any in-progress sync finishes.
func (t *Tracker) Wait() {
	t.mu.Lock()
	for t.syncing {
		t.cond.Wait()
	}
	t.mu.Unlock()
}

// Snapshot returns the latest sync state.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
		LastSync: t.lastSync,
		Syncing:  t.syncing,
		LastErr:  t.lastErr,
	}
}

// MaxAge returns the configured sync staleness threshold.
func MaxAge() time.Duration {
	const defaultSeconds = 300
	v := os.Getenv("MAGI_SYNC_MAX_AGE")
	if v == "" {
		return time.Duration(defaultSeconds) * time.Second
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		return time.Duration(defaultSeconds) * time.Second
	}
	return time.Duration(seconds) * time.Second
}
