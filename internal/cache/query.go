package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

// QueryCache caches search results keyed by query+filters hash.
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]*queryEntry
	ttl     time.Duration
	done    chan struct{}
}

type queryEntry struct {
	results   []*db.HybridResult
	expiresAt time.Time
}

// NewQueryCache creates a query cache with the given TTL.
func NewQueryCache(ttl time.Duration) *QueryCache {
	qc := &QueryCache{
		entries: make(map[string]*queryEntry),
		ttl:     ttl,
		done:    make(chan struct{}),
	}
	go qc.cleanup()
	return qc
}

// Get retrieves cached results for the given key, or nil if not cached/expired.
func (qc *QueryCache) Get(key string) []*db.HybridResult {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	e, ok := qc.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil
	}
	return cloneHybridResults(e.results)
}

// Set stores search results under the given key.
func (qc *QueryCache) Set(key string, results []*db.HybridResult) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	qc.entries[key] = &queryEntry{
		results:   cloneHybridResults(results),
		expiresAt: time.Now().Add(qc.ttl),
	}
}

// InvalidateForProject removes all cached entries that match a project or area.
// Called on writes to ensure stale results are not served.
func (qc *QueryCache) InvalidateForProject(project, area string) {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	// Simple approach: clear everything on write.
	// More targeted invalidation would require storing project/area with each entry.
	// For a 60s TTL this is fine — writes are less frequent than reads.
	qc.entries = make(map[string]*queryEntry)
}

// Len returns the number of cached entries.
func (qc *QueryCache) Len() int {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	return len(qc.entries)
}

// Close stops the cleanup goroutine.
func (qc *QueryCache) Close() {
	close(qc.done)
}

// Key generates a cache key from query string and filter.
func Key(query string, filter *db.MemoryFilter, topK int) string {
	h := sha256.New()
	h.Write([]byte(query))
	if filter != nil {
		b, _ := json.Marshal(filter)
		h.Write(b)
	}
	h.Write([]byte(fmt.Sprintf("topK=%d", topK)))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (qc *QueryCache) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-qc.done:
			return
		case now := <-ticker.C:
			qc.mu.Lock()
			for k, e := range qc.entries {
				if now.After(e.expiresAt) {
					delete(qc.entries, k)
				}
			}
			qc.mu.Unlock()
		}
	}
}
