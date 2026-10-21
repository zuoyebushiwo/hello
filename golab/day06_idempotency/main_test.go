package main

import (
	"context"
	"sync"
	"testing"
)

func TestDuplicateDelivery(t *testing.T) {
	p := NewProcessor()
	for i := 0; i < 3; i++ {
		if _, err := p.Handle(context.Background(), Job{ID: "m1", Rows: 10}); err != nil {
			t.Fatal(err)
		}
	}
	if got := p.Effects(); got != 10 {
		t.Fatalf("effects = %d, want 10", got)
	}
}

func TestCrashAfterEffect(t *testing.T) {
	p := NewProcessor()
	p.crashOnce = true
	if _, err := p.Handle(context.Background(), Job{ID: "m2", Rows: 7}); err == nil {
		t.Fatal("expected simulated crash")
	}
	if _, err := p.Handle(context.Background(), Job{ID: "m2", Rows: 7}); err != nil {
		t.Fatal(err)
	}
	if got := p.Effects(); got != 7 {
		t.Fatalf("effects = %d, want 7", got)
	}
}

func TestConcurrentDuplicates(t *testing.T) {
	p := NewProcessor()
	var wg sync.WaitGroup
	var processed int
	var mu sync.Mutex
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _ := p.Handle(context.Background(), Job{ID: "m3", Rows: 5})
			if out == "processed" {
				mu.Lock()
				processed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if processed != 1 || p.Effects() != 5 {
		t.Fatalf("processed = %d, effects = %d", processed, p.Effects())
	}
}
