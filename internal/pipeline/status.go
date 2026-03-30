package pipeline

import (
	"sync"
	"time"
)

// State represents the current state of a write operation.
type State string

const (
	StatePending    State = "pending"
	StateProcessing State = "processing"
	StateComplete   State = "complete"
	StateFailed     State = "failed"
)

// WriteStatus tracks the progress of an async write.
type WriteStatus struct {
	State     State         `json:"state"`
	Error     string        `json:"error,omitempty"`
	StartedAt time.Time    `json:"started_at"`
	ElapsedMs int64         `json:"elapsed_ms"`
}

const statusTTL = 5 * time.Minute

type statusEntry struct {
	status    WriteStatus
	expiresAt time.Time
}

// StatusTracker tracks write operation states with automatic TTL cleanup.
type StatusTracker struct {
	mu      sync.RWMutex
	entries map[string]*statusEntry
	done    chan struct{}
}

// NewStatusTracker creates a status tracker that cleans up expired entries.
func NewStatusTracker() *StatusTracker {
	st := &StatusTracker{
		entries: make(map[string]*statusEntry),
		done:    make(chan struct{}),
	}
	go st.cleanup()
	return st
}

// Set records a write status for the given ID.
func (st *StatusTracker) Set(id string, state State, writeErr string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	e, ok := st.entries[id]
	if !ok {
		e = &statusEntry{
			status: WriteStatus{StartedAt: time.Now()},
		}
		st.entries[id] = e
	}

	e.status.State = state
	e.status.Error = writeErr
	e.status.ElapsedMs = time.Since(e.status.StartedAt).Milliseconds()
	e.expiresAt = time.Now().Add(statusTTL)
}

// Get returns the write status for a given ID, or nil if not found.
func (st *StatusTracker) Get(id string) *WriteStatus {
	st.mu.RLock()
	defer st.mu.RUnlock()

	e, ok := st.entries[id]
	if !ok {
		return nil
	}
	// Return a copy with current elapsed time
	ws := e.status
	if ws.State == StatePending || ws.State == StateProcessing {
		ws.ElapsedMs = time.Since(ws.StartedAt).Milliseconds()
	}
	return &ws
}

// Len returns the number of tracked entries.
func (st *StatusTracker) Len() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.entries)
}

// Close stops the cleanup goroutine.
func (st *StatusTracker) Close() {
	close(st.done)
}

func (st *StatusTracker) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-st.done:
			return
		case now := <-ticker.C:
			st.mu.Lock()
			for id, e := range st.entries {
				if now.After(e.expiresAt) {
					delete(st.entries, id)
				}
			}
			st.mu.Unlock()
		}
	}
}
