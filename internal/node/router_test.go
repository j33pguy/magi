package node

import (
	"context"
	"testing"
)

func TestRouterWriteUpdatesSession(t *testing.T) {
	handler := func(_ context.Context, req *WriteRequest) *WriteResponse {
		return &WriteResponse{Memory: req.Memory}
	}
	wp := NewPool("writer", 2, handler)
	rp := NewPool[*ReadRequest, *ReadResponse]("reader", 2, func(_ context.Context, _ *ReadRequest) *ReadResponse {
		return &ReadResponse{}
	})
	wp.Start(context.Background())
	rp.Start(context.Background())
	defer wp.Stop()
	defer rp.Stop()

	router := NewRouter(wp, rp)

	ctx := WithSession(context.Background(), "session-1")
	_, err := router.RouteWrite(ctx, &WriteRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if seq := router.LastWriteSeq("session-1"); seq != 1 {
		t.Errorf("session write seq = %d, want 1", seq)
	}
	if seq := router.GlobalWriteSeq(); seq != 1 {
		t.Errorf("global write seq = %d, want 1", seq)
	}
}

func TestRouterReadRoutes(t *testing.T) {
	rp := NewPool("reader", 2, func(_ context.Context, req *ReadRequest) *ReadResponse {
		return &ReadResponse{Memory: nil}
	})
	wp := NewPool[*WriteRequest, *WriteResponse]("writer", 1, nil)
	rp.Start(context.Background())
	defer rp.Stop()

	router := NewRouter(wp, rp)

	resp, err := router.RouteRead(context.Background(), &ReadRequest{ID: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
}

func TestSessionContext(t *testing.T) {
	ctx := WithSession(context.Background(), "abc")
	if got := SessionFrom(ctx); got != "abc" {
		t.Errorf("session = %q, want %q", got, "abc")
	}
	if got := SessionFrom(context.Background()); got != "" {
		t.Errorf("empty context session = %q, want empty", got)
	}
}

func TestRouterNoSessionOnWrite(t *testing.T) {
	handler := func(_ context.Context, req *WriteRequest) *WriteResponse {
		return &WriteResponse{Memory: req.Memory}
	}
	wp := NewPool("writer", 1, handler)
	rp := NewPool[*ReadRequest, *ReadResponse]("reader", 1, nil)
	wp.Start(context.Background())
	defer wp.Stop()

	router := NewRouter(wp, rp)

	// Write without session — should still succeed, just no session tracking.
	_, err := router.RouteWrite(context.Background(), &WriteRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seq := router.GlobalWriteSeq(); seq != 1 {
		t.Errorf("global seq = %d, want 1", seq)
	}
}
