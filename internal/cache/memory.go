package cache

import (
	"container/list"
	"sync"

	"github.com/j33pguy/magi/internal/db"
)

// MemoryCache is an LRU cache of frequently accessed memories.
type MemoryCache struct {
	mu       sync.RWMutex
	maxSize  int
	items    map[string]*list.Element
	eviction *list.List
}

type memoryCacheEntry struct {
	key    string
	memory *db.Memory
}

// NewMemoryCache creates an LRU memory cache with the given max size.
func NewMemoryCache(maxSize int) *MemoryCache {
	return &MemoryCache{
		maxSize:  maxSize,
		items:    make(map[string]*list.Element, maxSize),
		eviction: list.New(),
	}
}

// Get retrieves a memory by ID from the cache, promoting it to most-recent.
// Returns nil if not cached.
func (mc *MemoryCache) Get(id string) *db.Memory {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if el, ok := mc.items[id]; ok {
		mc.eviction.MoveToFront(el)
		return el.Value.(*memoryCacheEntry).memory
	}
	return nil
}

// Set adds or updates a memory in the cache, evicting the LRU entry if full.
func (mc *MemoryCache) Set(id string, m *db.Memory) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if el, ok := mc.items[id]; ok {
		mc.eviction.MoveToFront(el)
		el.Value.(*memoryCacheEntry).memory = m
		return
	}

	if mc.eviction.Len() >= mc.maxSize {
		oldest := mc.eviction.Back()
		if oldest != nil {
			mc.eviction.Remove(oldest)
			delete(mc.items, oldest.Value.(*memoryCacheEntry).key)
		}
	}

	el := mc.eviction.PushFront(&memoryCacheEntry{key: id, memory: m})
	mc.items[id] = el
}

// Delete removes a memory from the cache.
func (mc *MemoryCache) Delete(id string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if el, ok := mc.items[id]; ok {
		mc.eviction.Remove(el)
		delete(mc.items, id)
	}
}

// Len returns the number of cached memories.
func (mc *MemoryCache) Len() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.eviction.Len()
}
