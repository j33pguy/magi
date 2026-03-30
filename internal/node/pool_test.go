package node

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPoolSubmitAndResponse(t *testing.T) {
	handler := func(_ context.Context, n int) int { return n * 2 }
	p := NewPool("test", 2, handler)
	p.Start(context.Background())
	defer p.Stop()

	resp, err := p.Submit(context.Background(), 21)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != 42 {
		t.Errorf("got %d, want 42", resp)
	}
}

func TestPoolConcurrentSubmits(t *testing.T) {
	var processed atomic.Int64
	handler := func(_ context.Context, n int) int {
		processed.Add(1)
		return n
	}

	p := NewPool("concurrent", 4, handler)
	p.Start(context.Background())
	defer p.Stop()

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := range N {
		go func(v int) {
			defer wg.Done()
			resp, err := p.Submit(context.Background(), v)
			if err != nil {
				t.Errorf("submit %d: %v", v, err)
				return
			}
			if resp != v {
				t.Errorf("got %d, want %d", resp, v)
			}
		}(i)
	}
	wg.Wait()

	if got := processed.Load(); got != N {
		t.Errorf("processed %d items, want %d", got, N)
	}
}

func TestPoolContextCancellation(t *testing.T) {
	// Submit with an already-cancelled context — should fail immediately.
	handler := func(_ context.Context, n int) int { return n }

	p := NewPool("cancel", 1, handler)
	p.Start(context.Background())
	defer p.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before submit

	_, err := p.Submit(ctx, 1)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestPoolSizeAndName(t *testing.T) {
	p := NewPool[int, int]("mypool", 7, nil)
	if p.Name() != "mypool" {
		t.Errorf("name = %q, want %q", p.Name(), "mypool")
	}
	if p.Size() != 7 {
		t.Errorf("size = %d, want 7", p.Size())
	}
}

func TestPoolMinSize(t *testing.T) {
	p := NewPool[int, int]("min", 0, nil)
	if p.Size() != 1 {
		t.Errorf("size = %d, want 1 (minimum)", p.Size())
	}
}
