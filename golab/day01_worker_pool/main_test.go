package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"golab/internal/workerpool"
)

// Submit accepts func(context.Context), which has no return value. A test
// cannot inspect a "result"; it must assert the observable side effect.
func TestVoidTaskIsVerifiedBySideEffect(t *testing.T) {
	const tasks = 24
	p := workerpool.New(context.Background(), 6, 32)
	var completed atomic.Int64

	for i := 0; i < tasks; i++ {
		if err := p.Submit(func(context.Context) {
			completed.Add(1)
		}); err != nil {
			t.Fatal(err)
		}
	}
	p.Close()

	if got := completed.Load(); got != tasks {
		t.Fatalf("completed = %d, want %d", got, tasks)
	}
	if got := p.Stats().MaxRunning; got > 6 {
		t.Fatalf("max concurrency = %d, want <= 6", got)
	}
}

// A void task cannot return a business error to Submit's caller. If the caller
// needs the outcome, it must pass an explicit result/error channel or use an
// API whose task signature returns a result.
func TestVoidTaskErrorNeedsExplicitChannel(t *testing.T) {
	p := workerpool.New(context.Background(), 1, 1)
	errCh := make(chan error, 1)

	if err := p.Submit(func(context.Context) {
		errCh <- errors.New("downstream write failed")
	}); err != nil {
		t.Fatal(err)
	}
	p.Close()

	if err := <-errCh; err == nil || err.Error() != "downstream write failed" {
		t.Fatalf("err = %v, want the task error", err)
	}
}
