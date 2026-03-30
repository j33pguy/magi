package pipeline

import (
	"log/slog"
	"testing"
	"time"

	"github.com/j33pguy/magi/internal/db"
)

func TestBatchFlushOnSize(t *testing.T) {
	store := newMockStore()
	status := NewStatusTracker()
	defer status.Close()

	bi := NewBatchInserter(store, status, 5*time.Second, 3, slog.Default())

	// Add 3 items — should trigger size-based flush
	for i := 0; i < 3; i++ {
		bi.Add(completedWrite{
			memory: &db.Memory{
				Content:   "batch size test",
				Embedding: make([]float32, 384),
				Project:   "test",
				Type:      "memory",
			},
			tags: []string{"test"},
			id:   "test-id-" + string(rune('a'+i)),
		})
	}

	// Give time for flush to complete
	time.Sleep(50 * time.Millisecond)

	saved := store.getSaved()
	if len(saved) != 3 {
		t.Errorf("expected 3 saved after size flush, got %d", len(saved))
	}

	bi.Close()
}

func TestBatchFlushOnInterval(t *testing.T) {
	store := newMockStore()
	status := NewStatusTracker()
	defer status.Close()

	bi := NewBatchInserter(store, status, 20*time.Millisecond, 100, slog.Default())

	bi.Add(completedWrite{
		memory: &db.Memory{
			Content:   "interval test",
			Embedding: make([]float32, 384),
			Project:   "test",
			Type:      "memory",
		},
		id: "interval-1",
	})

	// Wait for interval flush
	time.Sleep(100 * time.Millisecond)

	saved := store.getSaved()
	if len(saved) != 1 {
		t.Errorf("expected 1 saved after interval flush, got %d", len(saved))
	}

	bi.Close()
}

func TestBatchStatusTracking(t *testing.T) {
	store := newMockStore()
	st := NewStatusTracker()
	defer st.Close()

	bi := NewBatchInserter(store, st, 10*time.Millisecond, 1, slog.Default())

	st.Set("track-1", StatePending, "")
	bi.Add(completedWrite{
		memory: &db.Memory{
			Content:   "status track test",
			Embedding: make([]float32, 384),
			Project:   "test",
			Type:      "memory",
		},
		id: "track-1",
	})

	time.Sleep(50 * time.Millisecond)

	ws := st.Get("track-1")
	if ws == nil {
		t.Fatal("expected status for track-1")
	}
	if ws.State != StateComplete {
		t.Errorf("expected state complete, got %s", ws.State)
	}

	bi.Close()
}
