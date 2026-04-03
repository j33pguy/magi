package syncstate

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	tr := NewTracker()
	if tr == nil {
		t.Fatal("NewTracker returned nil")
	}
	snap := tr.Snapshot()
	if snap.Syncing {
		t.Error("new tracker should not be syncing")
	}
	if snap.LastErr != "" {
		t.Error("new tracker should have no error")
	}
	if !snap.LastSync.IsZero() {
		t.Error("new tracker should have zero LastSync")
	}
}

func TestStart_Basic(t *testing.T) {
	tr := NewTracker()
	if !tr.Start() {
		t.Error("first Start should succeed")
	}
	snap := tr.Snapshot()
	if !snap.Syncing {
		t.Error("should be syncing after Start")
	}
}

func TestStart_RejectsConcurrent(t *testing.T) {
	tr := NewTracker()
	if !tr.Start() {
		t.Fatal("first Start should succeed")
	}
	if tr.Start() {
		t.Error("second Start should fail while syncing")
	}
}

func TestFinish_Success(t *testing.T) {
	tr := NewTracker()
	tr.Start()

	before := time.Now().UTC()
	tr.Finish(nil)
	after := time.Now().UTC()

	snap := tr.Snapshot()
	if snap.Syncing {
		t.Error("should not be syncing after Finish")
	}
	if snap.LastErr != "" {
		t.Errorf("expected no error, got %q", snap.LastErr)
	}
	if snap.LastSync.Before(before) || snap.LastSync.After(after) {
		t.Errorf("LastSync %v not between %v and %v", snap.LastSync, before, after)
	}
}

func TestFinish_Error(t *testing.T) {
	tr := NewTracker()
	tr.Start()
	tr.Finish(errors.New("sync failed"))

	snap := tr.Snapshot()
	if snap.Syncing {
		t.Error("should not be syncing after Finish")
	}
	if snap.LastErr != "sync failed" {
		t.Errorf("expected error 'sync failed', got %q", snap.LastErr)
	}
	if !snap.LastSync.IsZero() {
		t.Error("LastSync should remain zero after error")
	}
}

func TestFinish_ClearsErrorOnSuccess(t *testing.T) {
	tr := NewTracker()

	tr.Start()
	tr.Finish(errors.New("first error"))
	if tr.Snapshot().LastErr != "first error" {
		t.Fatal("error should be recorded")
	}

	tr.Start()
	tr.Finish(nil)
	snap := tr.Snapshot()
	if snap.LastErr != "" {
		t.Errorf("error should be cleared, got %q", snap.LastErr)
	}
	if snap.LastSync.IsZero() {
		t.Error("LastSync should be set after successful sync")
	}
}

func TestStart_AfterFinish(t *testing.T) {
	tr := NewTracker()
	tr.Start()
	tr.Finish(nil)

	if !tr.Start() {
		t.Error("should be able to Start again after Finish")
	}
}

func TestWait_NotSyncing(t *testing.T) {
	tr := NewTracker()
	// Should return immediately when not syncing
	done := make(chan struct{})
	go func() {
		tr.Wait()
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("Wait should return immediately when not syncing")
	}
}

func TestWait_BlocksUntilFinish(t *testing.T) {
	tr := NewTracker()
	tr.Start()

	done := make(chan struct{})
	go func() {
		tr.Wait()
		close(done)
	}()

	// Give Wait time to block
	time.Sleep(10 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Wait should block while syncing")
	default:
	}

	tr.Finish(nil)

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("Wait should unblock after Finish")
	}
}

func TestConcurrentStartFinish(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tr.Start() {
				time.Sleep(time.Microsecond)
				tr.Finish(nil)
			}
		}()
	}

	wg.Wait()
	snap := tr.Snapshot()
	if snap.Syncing {
		t.Error("should not be syncing after all goroutines complete")
	}
}

func TestMaxAge_Default(t *testing.T) {
	t.Setenv("MAGI_SYNC_MAX_AGE", "")
	got := MaxAge()
	want := 300 * time.Second
	if got != want {
		t.Errorf("default MaxAge = %v, want %v", got, want)
	}
}

func TestMaxAge_CustomValue(t *testing.T) {
	t.Setenv("MAGI_SYNC_MAX_AGE", "60")
	got := MaxAge()
	want := 60 * time.Second
	if got != want {
		t.Errorf("MaxAge = %v, want %v", got, want)
	}
}

func TestMaxAge_InvalidValue(t *testing.T) {
	t.Setenv("MAGI_SYNC_MAX_AGE", "not-a-number")
	got := MaxAge()
	want := 300 * time.Second
	if got != want {
		t.Errorf("MaxAge with invalid value = %v, want default %v", got, want)
	}
}

func TestMaxAge_NegativeValue(t *testing.T) {
	t.Setenv("MAGI_SYNC_MAX_AGE", "-10")
	got := MaxAge()
	want := 300 * time.Second
	if got != want {
		t.Errorf("MaxAge with negative value = %v, want default %v", got, want)
	}
}

func TestMaxAge_ZeroValue(t *testing.T) {
	t.Setenv("MAGI_SYNC_MAX_AGE", "0")
	got := MaxAge()
	want := 300 * time.Second
	if got != want {
		t.Errorf("MaxAge with zero = %v, want default %v", got, want)
	}
}

func TestSnapshot_ThreadSafe(t *testing.T) {
	tr := NewTracker()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tr.Snapshot()
		}()
	}

	wg.Wait()
}
