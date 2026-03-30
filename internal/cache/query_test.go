package cache

import (
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func TestQueryCacheSetGet(t *testing.T) {
	qc := NewQueryCache(1 * time.Second)
	defer qc.Close()

	results := []*db.HybridResult{
		{Memory: &db.Memory{ID: "test-1", Content: "hello"}, RRFScore: 0.5},
	}

	key := Key("hello world", nil, 5)
	qc.Set(key, results)

	got := qc.Get(key)
	if got == nil {
		t.Fatal("expected cached results")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Memory.ID != "test-1" {
		t.Errorf("unexpected ID: %s", got[0].Memory.ID)
	}
}

func TestQueryCacheTTL(t *testing.T) {
	qc := NewQueryCache(20 * time.Millisecond)
	defer qc.Close()

	key := Key("expiring", nil, 5)
	qc.Set(key, []*db.HybridResult{{Memory: &db.Memory{ID: "exp-1"}}})

	if got := qc.Get(key); got == nil {
		t.Fatal("expected cached results before TTL")
	}

	time.Sleep(50 * time.Millisecond)

	if got := qc.Get(key); got != nil {
		t.Error("expected nil after TTL expiry")
	}
}

func TestQueryCacheInvalidation(t *testing.T) {
	qc := NewQueryCache(10 * time.Second)
	defer qc.Close()

	key := Key("test", nil, 5)
	qc.Set(key, []*db.HybridResult{{Memory: &db.Memory{ID: "inv-1"}}})

	if qc.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", qc.Len())
	}

	qc.InvalidateForProject("test-project", "work")

	if qc.Len() != 0 {
		t.Error("expected 0 entries after invalidation")
	}
}

func TestQueryCacheKey(t *testing.T) {
	k1 := Key("hello", nil, 5)
	k2 := Key("hello", nil, 5)
	k3 := Key("world", nil, 5)
	k4 := Key("hello", &db.MemoryFilter{Project: "test"}, 5)

	if k1 != k2 {
		t.Error("same inputs should produce same key")
	}
	if k1 == k3 {
		t.Error("different queries should produce different keys")
	}
	if k1 == k4 {
		t.Error("different filters should produce different keys")
	}
}
