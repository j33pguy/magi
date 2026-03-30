package cache

import (
	"container/list"
	"crypto/sha256"
	"fmt"
	"sync"
)

// EmbeddingCache is an LRU cache for embedding vectors keyed by content hash.
type EmbeddingCache struct {
	mu       sync.RWMutex
	maxSize  int
	items    map[string]*list.Element
	eviction *list.List
}

type embeddingCacheEntry struct {
	key       string
	embedding []float32
}

// NewEmbeddingCache creates an LRU embedding cache with the given max size.
func NewEmbeddingCache(maxSize int) *EmbeddingCache {
	return &EmbeddingCache{
		maxSize:  maxSize,
		items:    make(map[string]*list.Element, maxSize),
		eviction: list.New(),
	}
}

// Get retrieves a cached embedding for the given content text.
// Returns nil if not cached.
func (ec *EmbeddingCache) Get(content string) []float32 {
	key := contentKey(content)
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if el, ok := ec.items[key]; ok {
		ec.eviction.MoveToFront(el)
		return el.Value.(*embeddingCacheEntry).embedding
	}
	return nil
}

// Set stores an embedding for the given content text.
func (ec *EmbeddingCache) Set(content string, embedding []float32) {
	key := contentKey(content)
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if el, ok := ec.items[key]; ok {
		ec.eviction.MoveToFront(el)
		el.Value.(*embeddingCacheEntry).embedding = embedding
		return
	}

	if ec.eviction.Len() >= ec.maxSize {
		oldest := ec.eviction.Back()
		if oldest != nil {
			ec.eviction.Remove(oldest)
			delete(ec.items, oldest.Value.(*embeddingCacheEntry).key)
		}
	}

	el := ec.eviction.PushFront(&embeddingCacheEntry{key: key, embedding: embedding})
	ec.items[key] = el
}

// Len returns the number of cached embeddings.
func (ec *EmbeddingCache) Len() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.eviction.Len()
}

func contentKey(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}
