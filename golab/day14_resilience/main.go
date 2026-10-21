package main

import (
	"context"
	"errors"
	"time"

	"golab/internal/labkit"
	"golab/internal/resilience"
)

func main() {
	lab := labkit.New("Day 14", "弹性：限流、退避重试与熔断")

	lab.Section("场景 A：令牌桶，burst=3，rate=5/s")
	bucket := resilience.NewTokenBucket(5, 3, nil)
	allowed := 0
	for i := 0; i < 10; i++ {
		if bucket.Allow() {
			allowed++
		}
	}
	lab.Infof("10 immediate calls allowed=%d tokens=%.2f", allowed, bucket.Tokens())
	lab.Check(allowed == 3, "桶容量为 3，超出立即拒绝")
	time.Sleep(250 * time.Millisecond)
	refilled := 0
	for i := 0; i < 5; i++ {
		if bucket.Allow() {
			refilled++
		}
	}
	lab.Infof("after 250ms allowed=%d", refilled)
	lab.Check(refilled >= 1, "时间推进后令牌被补充")

	lab.Section("场景 B：只对可重试错误做指数退避")
	attempts := 0
	err := resilience.Do(context.Background(), resilience.RetryConfig{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
		MaxDelay:    4 * time.Millisecond,
		Factor:      2,
		Jitter:      true,
		ShouldRetry: func(err error) bool { return !errors.Is(err, context.Canceled) },
	}, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary connection reset")
		}
		return nil
	})
	lab.Infof("attempts=%d err=%v", attempts, err)
	lab.Check(err == nil && attempts == 3, "前两次失败后第三次成功")

	attempts = 0
	perm := errors.New("invalid SQL")
	err = resilience.Do(context.Background(), resilience.RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		ShouldRetry: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
	}, func(context.Context) error {
		attempts++
		return perm
	})
	lab.Infof("permanent attempts=%d err=%v", attempts, err)
	lab.Check(attempts == 1 && errors.Is(err, perm), "永久错误立即返回，不浪费重试")

	lab.Section("场景 C：熔断器 closed -> open -> half-open -> closed")
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	breaker := resilience.NewBreaker(3, 100*time.Millisecond, clock)
	boom := errors.New("upstream down")
	for i := 0; i < 3; i++ {
		_ = breaker.Call(context.Background(), func() error { return boom })
		lab.Infof("failure %d -> state=%s", i+1, breaker.State())
	}
	lab.Check(breaker.State() == resilience.BreakerOpen, "连续 3 次失败后熔断")
	called := false
	err = breaker.Call(context.Background(), func() error { called = true; return nil })
	lab.Infof("while open: called=%v err=%v", called, err)
	lab.Check(errors.Is(err, resilience.ErrOpen) && !called, "open 状态快速失败，不压垮上游")
	now = now.Add(150 * time.Millisecond)
	err = breaker.Call(context.Background(), func() error { return nil })
	lab.Infof("probe after cooldown: err=%v state=%s", err, breaker.State())
	lab.Check(err == nil && breaker.State() == resilience.BreakerClosed, "half-open 探测成功后恢复 closed")

	lab.Section("Java → Go 对照")
	lab.Infof("RateLimiter / Sentinel           ≈  TokenBucket（本地）或 Redis 令牌桶（分布式）")
	lab.Infof("Resilience4j Retry               ≈  Do + ShouldRetry + backoff + jitter")
	lab.Infof("Resilience4j CircuitBreaker      ≈  Breaker 状态机 + 快速失败")
	lab.Infof("Hystrix 线程隔离                  ≈  Go 里用有界池 + context 预算，不要无限起 goroutine")
	lab.Done()
}
