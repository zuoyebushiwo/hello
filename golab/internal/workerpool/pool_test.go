package workerpool

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolCapsConcurrency(t *testing.T) {
	var active, max atomic.Int64
	p := New(context.Background(), 4, 16)
	for i := 0; i < 40; i++ {
		if err := p.Submit(func(ctx context.Context) {
			n := active.Add(1)
			for {
				old := max.Load()
				if n <= old || max.CompareAndSwap(old, n) {
					break
				}
			}
			defer active.Add(-1)
			time.Sleep(2 * time.Millisecond)
		}); err != nil {
			t.Fatal(err)
		}
	}
	p.Close()
	if got := p.Stats().MaxRunning; got > 4 {
		t.Fatalf("max concurrency = %d, want <= 4", got)
	}
	if got := p.Stats().MaxRunning; got < 2 {
		t.Fatalf("expected real parallelism, max = %d", got)
	}
}

func TestPoolRecoversPanic(t *testing.T) {
	p := New(context.Background(), 1, 1)
	_ = p.Submit(func(context.Context) { panic("boom") })
	_ = p.Submit(func(context.Context) {})
	p.Close()
	if got := p.Stats().Panics; got != 1 {
		t.Fatalf("panics = %d, want 1", got)
	}
}

func TestSubmitAfterClose(t *testing.T) {
	p := New(context.Background(), 1, 1)
	p.Close()
	if err := p.Submit(func(context.Context) {}); err != ErrClosed {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestPoolDrains(t *testing.T) {
	var n atomic.Int64
	p := New(context.Background(), 3, 10)
	for i := 0; i < 20; i++ {
		_ = p.Submit(func(context.Context) { n.Add(1) })
	}
	p.Close()
	if got := n.Load(); got != 20 {
		t.Fatalf("completed = %d, want 20", got)
	}
	_ = fmt.Sprint(n.Load())
}
