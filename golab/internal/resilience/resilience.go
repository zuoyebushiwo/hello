// Package resilience contains three primitives every network client needs:
// token bucket, retry with backoff/jitter, and a circuit breaker.
package resilience

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

type Clock func() time.Time

func (c Clock) now() time.Time {
	if c == nil {
		return time.Now()
	}
	return c()
}

type TokenBucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
	now    Clock
}

func NewTokenBucket(rate, burst float64, now Clock) *TokenBucket {
	if now == nil {
		now = time.Now
	}
	return &TokenBucket{rate: rate, burst: burst, tokens: burst, last: now.now(), now: now}
}

func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if now.After(b.last) {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens = math.Min(b.burst, b.tokens+elapsed*b.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (b *TokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Factor      float64
	Jitter      bool
	ShouldRetry func(error) bool
	Sleep       func(context.Context, time.Duration) error
	Rand        func() float64
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func Do(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 2 * time.Second
	}
	if cfg.Factor <= 1 {
		cfg.Factor = 2
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleep
	}
	if cfg.Rand == nil {
		cfg.Rand = func() float64 { return 0.5 }
	}
	var last error
	delay := cfg.BaseDelay
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(last, err)
		}
		last = fn(ctx)
		if last == nil {
			return nil
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		if cfg.ShouldRetry != nil && !cfg.ShouldRetry(last) {
			return last
		}
		wait := delay
		if cfg.Jitter {
			wait = time.Duration(float64(wait) * (0.5 + cfg.Rand()))
		}
		if err := cfg.Sleep(ctx, wait); err != nil {
			return errors.Join(last, err)
		}
		next := time.Duration(float64(delay) * cfg.Factor)
		if next > cfg.MaxDelay {
			next = cfg.MaxDelay
		}
		delay = next
	}
	return fmt.Errorf("failed after %d attempts: %w", cfg.MaxAttempts, last)
}

type BreakerState string

const (
	BreakerClosed   BreakerState = "closed"
	BreakerOpen     BreakerState = "open"
	BreakerHalfOpen BreakerState = "half-open"
)

var ErrOpen = errors.New("circuit breaker is open")

type Breaker struct {
	mu        sync.Mutex
	threshold int
	openFor   time.Duration
	now       Clock
	state     BreakerState
	failures  int
	openedAt  time.Time
	probing   bool
}

func NewBreaker(threshold int, openFor time.Duration, now Clock) *Breaker {
	if threshold < 1 {
		threshold = 1
	}
	if now == nil {
		now = time.Now
	}
	return &Breaker{threshold: threshold, openFor: openFor, now: now, state: BreakerClosed}
}

func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker) Call(ctx context.Context, fn func() error) error {
	b.mu.Lock()
	now := b.now.now()
	switch b.state {
	case BreakerOpen:
		if now.Sub(b.openedAt) < b.openFor {
			b.mu.Unlock()
			return ErrOpen
		}
		b.state = BreakerHalfOpen
	case BreakerHalfOpen:
		if b.probing {
			b.mu.Unlock()
			return ErrOpen
		}
	}
	half := b.state == BreakerHalfOpen
	if half {
		b.probing = true
	}
	b.mu.Unlock()

	if err := ctx.Err(); err != nil {
		b.finish(half, err)
		return err
	}
	err := fn()
	b.finish(half, err)
	return err
}

func (b *Breaker) finish(half bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if half {
		b.probing = false
		if err != nil {
			b.state = BreakerOpen
			b.openedAt = b.now.now()
			return
		}
		b.state = BreakerClosed
		b.failures = 0
		return
	}
	if err != nil {
		b.failures++
		if b.failures >= b.threshold {
			b.state = BreakerOpen
			b.openedAt = b.now.now()
		}
		return
	}
	b.failures = 0
}
