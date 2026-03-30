package cache

import (
	"testing"

	"github.com/j33pguy/magi/internal/db"
)

func TestMemoryCacheSetGet(t *testing.T) {
	mc := NewMemoryCache(10)

	m := &db.Memory{ID: "mem-1", Content: "hello"}
	mc.Set("mem-1", m)

	got := mc.Get("mem-1")
	if got == nil {
		t.Fatal("expected cached memory")
	}
	if got.Content != "hello" {
		t.Errorf("unexpected content: %s", got.Content)
	}
}

func TestMemoryCacheMiss(t *testing.T) {
	mc := NewMemoryCache(10)

	if got := mc.Get("nonexistent"); got != nil {
		t.Error("expected nil for cache miss")
	}
}

func TestMemoryCacheLRUEviction(t *testing.T) {
	mc := NewMemoryCache(3)

	mc.Set("a", &db.Memory{ID: "a", Content: "alpha"})
	mc.Set("b", &db.Memory{ID: "b", Content: "beta"})
	mc.Set("c", &db.Memory{ID: "c", Content: "gamma"})

	// Access "a" to make it recently used
	mc.Get("a")

	// Insert "d" — should evict "b" (least recently used)
	mc.Set("d", &db.Memory{ID: "d", Content: "delta"})

	if mc.Get("b") != nil {
		t.Error("expected 'b' to be evicted")
	}
	if mc.Get("a") == nil {
		t.Error("expected 'a' to still be cached")
	}
	if mc.Get("d") == nil {
		t.Error("expected 'd' to be cached")
	}
	if mc.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", mc.Len())
	}
}

func TestMemoryCacheDelete(t *testing.T) {
	mc := NewMemoryCache(10)

	mc.Set("del-1", &db.Memory{ID: "del-1", Content: "to delete"})
	mc.Delete("del-1")

	if mc.Get("del-1") != nil {
		t.Error("expected nil after delete")
	}
	if mc.Len() != 0 {
		t.Errorf("expected 0 entries, got %d", mc.Len())
	}
}

func TestMemoryCacheUpdate(t *testing.T) {
	mc := NewMemoryCache(10)

	mc.Set("upd-1", &db.Memory{ID: "upd-1", Content: "original"})
	mc.Set("upd-1", &db.Memory{ID: "upd-1", Content: "updated"})

	got := mc.Get("upd-1")
	if got == nil {
		t.Fatal("expected cached memory")
	}
	if got.Content != "updated" {
		t.Errorf("expected updated content, got: %s", got.Content)
	}
	if mc.Len() != 1 {
		t.Errorf("expected 1 entry after update, got %d", mc.Len())
	}
}
