package node

import (
	"context"
	"fmt"
	"sync"
)

// Pool manages a fixed-size goroutine pool that processes work items.
// Each worker pulls from a shared channel, providing natural back-pressure.
type Pool[Req any, Resp any] struct {
	name    string
	size    int
	workCh  chan workItem[Req, Resp]
	handler func(ctx context.Context, req Req) Resp
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

type workItem[Req any, Resp any] struct {
	ctx    context.Context
	req    Req
	respCh chan<- Resp
}

// NewPool creates a goroutine pool with the given size and handler function.
// Workers are not started until Start is called.
func NewPool[Req any, Resp any](name string, size int, handler func(ctx context.Context, req Req) Resp) *Pool[Req, Resp] {
	if size < 1 {
		size = 1
	}
	return &Pool[Req, Resp]{
		name:    name,
		size:    size,
		workCh:  make(chan workItem[Req, Resp], size*2), // buffered for throughput
		handler: handler,
	}
}

// Start launches the worker goroutines.
func (p *Pool[Req, Resp]) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(p.size)
	for i := range p.size {
		go p.worker(ctx, i)
	}
}

// Stop signals workers to stop and waits for them to drain.
func (p *Pool[Req, Resp]) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	close(p.workCh)
	p.wg.Wait()
}

// Submit sends a request to the pool and blocks until a response is available.
// Returns an error if the context is cancelled before the work can be submitted.
func (p *Pool[Req, Resp]) Submit(ctx context.Context, req Req) (Resp, error) {
	respCh := make(chan Resp, 1)
	item := workItem[Req, Resp]{ctx: ctx, req: req, respCh: respCh}

	select {
	case p.workCh <- item:
	case <-ctx.Done():
		var zero Resp
		return zero, fmt.Errorf("pool %s: submit cancelled: %w", p.name, ctx.Err())
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		var zero Resp
		return zero, fmt.Errorf("pool %s: response cancelled: %w", p.name, ctx.Err())
	}
}

// Size returns the number of workers in the pool.
func (p *Pool[Req, Resp]) Size() int {
	return p.size
}

// Name returns the pool's name.
func (p *Pool[Req, Resp]) Name() string {
	return p.name
}

func (p *Pool[Req, Resp]) worker(ctx context.Context, _ int) {
	defer p.wg.Done()
	for {
		select {
		case item, ok := <-p.workCh:
			if !ok {
				return
			}
			resp := p.handler(item.ctx, item.req)
			item.respCh <- resp
		case <-ctx.Done():
			return
		}
	}
}
