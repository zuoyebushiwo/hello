// Package workerpool is a small bounded worker pool. The two lessons are:
//  1. channel + WaitGroup can replace ExecutorService for simple pipelines.
//  2. Close must stop admission before closing the task channel, otherwise a
//     concurrent Submit panics with "send on closed channel".
package workerpool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var ErrClosed = errors.New("worker pool is closed")

type Stats struct {
	Running    int64
	MaxRunning int64
	Panics     int64
}

type Pool struct {
	ctx    context.Context
	cancel context.CancelFunc
	tasks  chan func(context.Context)
	wg     sync.WaitGroup

	mu     sync.RWMutex
	closed bool

	running    atomic.Int64
	maxRunning atomic.Int64
	panics     atomic.Int64
}

func New(parent context.Context, workers, queue int) *Pool {
	if workers < 1 {
		workers = 1
	}
	if queue < 0 {
		queue = 0
	}
	ctx, cancel := context.WithCancel(parent)
	p := &Pool{ctx: ctx, cancel: cancel, tasks: make(chan func(context.Context), queue)}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	return p
}

// Submit uses the pool lifetime as its cancellation boundary.
func (p *Pool) Submit(task func(context.Context)) error {
	return p.SubmitContext(p.ctx, task)
}

// SubmitContext lets the caller bound how long it is willing to wait for a free
// slot. This is how a request handler avoids piling up goroutines.
func (p *Pool) SubmitContext(ctx context.Context, task func(context.Context)) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrClosed
	}
	select {
	case p.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for task := range p.tasks {
		p.run(id, task)
	}
}

func (p *Pool) run(id int, task func(context.Context)) {
	n := p.running.Add(1)
	for {
		old := p.maxRunning.Load()
		if n <= old || p.maxRunning.CompareAndSwap(old, n) {
			break
		}
	}
	defer p.running.Add(-1)
	defer func() {
		if r := recover(); r != nil {
			p.panics.Add(1)
			fmt.Printf("  [worker %d] recovered panic: %v\n", id, r)
		}
	}()
	task(p.ctx)
}

// Close stops admission, drains queued tasks, then cancels the context.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.tasks)
	p.mu.Unlock()
	p.wg.Wait()
	p.cancel()
}

func (p *Pool) Stats() Stats {
	return Stats{Running: p.running.Load(), MaxRunning: p.maxRunning.Load(), Panics: p.panics.Load()}
}
