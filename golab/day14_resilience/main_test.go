package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"golab/internal/resilience"
)

func TestBreakerAndRetry(t *testing.T) {
	now := time.Unix(0, 0)
	b := resilience.NewBreaker(2, 50*time.Millisecond, func() time.Time { return now })
	boom := errors.New("x")
	for i := 0; i < 2; i++ {
		_ = b.Call(context.Background(), func() error { return boom })
	}
	if b.State() != resilience.BreakerOpen {
		t.Fatalf("state = %s", b.State())
	}
	now = now.Add(60 * time.Millisecond)
	if err := b.Call(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if b.State() != resilience.BreakerClosed {
		t.Fatalf("state = %s", b.State())
	}
}
