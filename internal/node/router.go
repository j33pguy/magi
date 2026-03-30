package node

import (
	"context"
	"sync"
	"sync/atomic"
)

// Router directs requests to node pools with session affinity for
// read-your-writes consistency. After a write, subsequent reads in the
// same session are routed to ensure the write is visible.
type Router struct {
	writerPool *Pool[*WriteRequest, *WriteResponse]
	readerPool *Pool[*ReadRequest, *ReadResponse]

	// Session affinity: tracks the latest write sequence per session.
	// After a write completes, the session's sequence is bumped so readers
	// know they must see data at least as fresh as that sequence.
	mu       sync.RWMutex
	sessions map[string]*sessionState

	// Global monotonic write counter for ordering.
	writeSeq atomic.Int64
}

type sessionState struct {
	// lastWriteSeq is the sequence number of the last write in this session.
	lastWriteSeq int64
}

type sessionKeyType struct{}

// SessionKey is the context key for session affinity.
var SessionKey = sessionKeyType{}

// WithSession returns a context annotated with the given session ID.
func WithSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionKey, sessionID)
}

// SessionFrom extracts the session ID from a context, or "" if none.
func SessionFrom(ctx context.Context) string {
	if v, ok := ctx.Value(SessionKey).(string); ok {
		return v
	}
	return ""
}

// NewRouter creates a router that dispatches to the given pools.
func NewRouter(writerPool *Pool[*WriteRequest, *WriteResponse], readerPool *Pool[*ReadRequest, *ReadResponse]) *Router {
	return &Router{
		writerPool: writerPool,
		readerPool: readerPool,
		sessions:   make(map[string]*sessionState),
	}
}

// RouteWrite submits a write request and updates session affinity state.
func (r *Router) RouteWrite(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
	resp, err := r.writerPool.Submit(ctx, req)
	if err != nil {
		return nil, err
	}

	// Bump session write sequence on success.
	if resp.Err == nil {
		seq := r.writeSeq.Add(1)
		if sid := SessionFrom(ctx); sid != "" {
			r.mu.Lock()
			s, ok := r.sessions[sid]
			if !ok {
				s = &sessionState{}
				r.sessions[sid] = s
			}
			s.lastWriteSeq = seq
			r.mu.Unlock()
		}
	}

	return resp, nil
}

// RouteRead submits a read request. Session affinity metadata is available
// for future use when nodes are distributed (Phase 2+). In embedded mode,
// reads always see the latest writes because they share the same Store.
func (r *Router) RouteRead(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
	return r.readerPool.Submit(ctx, req)
}

// LastWriteSeq returns the latest write sequence for a session, or 0 if none.
func (r *Router) LastWriteSeq(sessionID string) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.sessions[sessionID]; ok {
		return s.lastWriteSeq
	}
	return 0
}

// GlobalWriteSeq returns the current global write sequence counter.
func (r *Router) GlobalWriteSeq() int64 {
	return r.writeSeq.Load()
}
