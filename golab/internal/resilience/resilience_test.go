package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTokenBucket(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewTokenBucket(2, 3, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if b.Allow() {
		t.Fatal("fourth request should be limited")
	}
	now = now.Add(500 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("one token should refill after 500ms at 2/s")
	}
}

func TestZeroValueClockUsesSystemTime(t *testing.T) {
	bucket := NewTokenBucket(1, 1, nil)
	if !bucket.Allow() {
		t.Fatal("nil clock should fall back to time.Now")
	}
	breaker := NewBreaker(1, time.Millisecond, nil)
	if err := breaker.Call(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("nil clock should not panic: %v", err)
	}
}

func TestRetryCountAndNonRetryable(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Nanosecond,
		Sleep:       func(context.Context, time.Duration) error { return nil },
		ShouldRetry: func(error) bool { return true },
	}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}

	attempts = 0
	perm := errors.New("bad request")
	err = Do(context.Background(), RetryConfig{
		MaxAttempts: 5,
		Sleep:       func(context.Context, time.Duration) error { return nil },
		ShouldRetry: func(error) bool { return false },
	}, func(context.Context) error { attempts++; return perm })
	if !errors.Is(err, perm) || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

func TestBreakerTransition(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(3, 100*time.Millisecond, func() time.Time { return now })
	boom := errors.New("down")
	for i := 0; i < 3; i++ {
		_ = b.Call(context.Background(), func() error { return boom })
	}
	if b.State() != BreakerOpen {
		t.Fatalf("state = %s, want open", b.State())
	}
	called := false
	if err := b.Call(context.Background(), func() error { called = true; return nil }); !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("open breaker must reject without calling fn")
	}
	now = now.Add(150 * time.Millisecond)
	if err := b.Call(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if b.State() != BreakerClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}
