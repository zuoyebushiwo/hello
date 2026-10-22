package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"frameworklab/internal/labkit"

	"github.com/cenkalti/backoff/v4"
	"github.com/panjf2000/ants/v2"
	"golang.org/x/sync/singleflight"
)

type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return "permanent: " + e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func Retry(ctx context.Context, policy backoff.BackOff, operation func(context.Context) error) (int, error) {
	attempts := 0
	for {
		attempts++
		err := operation(ctx)
		if err == nil {
			return attempts, nil
		}
		var permanent *PermanentError
		if errors.As(err, &permanent) {
			return attempts, err
		}
		delay := policy.NextBackOff()
		if delay == backoff.Stop {
			return attempts, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return attempts, ctx.Err()
		case <-timer.C:
		}
	}
}

type Loader struct {
	group singleflight.Group
	calls atomic.Int64
}

func (l *Loader) Load(ctx context.Context, key string, fn func(context.Context) (string, error)) (string, error) {
	value, err, _ := l.group.Do(key, func() (any, error) {
		l.calls.Add(1)
		return fn(ctx)
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

func (l *Loader) Calls() int64 { return l.calls.Load() }

func RunBounded(ctx context.Context, workers, tasks int, task func(context.Context)) (int64, error) {
	pool, err := ants.NewPool(workers, ants.WithPreAlloc(true))
	if err != nil {
		return 0, err
	}
	defer pool.Release()

	var active, peak atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < tasks; i++ {
		wg.Add(1)
		if err := pool.Submit(func() {
			defer wg.Done()
			n := active.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			task(ctx)
			active.Add(-1)
		}); err != nil {
			wg.Done()
			return peak.Load(), err
		}
	}
	wg.Wait()
	return peak.Load(), nil
}

func main() {
	lab := labkit.New("Day 22", "弹性组合：重试、请求合并与有界 goroutine")
	ctx := context.Background()

	lab.Section("场景 A：只重试临时错误")
	attempts := 0
	count, err := Retry(ctx, backoff.NewConstantBackOff(time.Millisecond), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary network error")
		}
		return nil
	})
	lab.Infof("attempts=%d err=%v", count, err)
	lab.Check(err == nil && count == 3, "临时错误按策略重试，第三次成功")

	count, err = Retry(ctx, backoff.NewConstantBackOff(time.Millisecond), func(context.Context) error {
		return &PermanentError{Err: errors.New("invalid credentials")}
	})
	lab.Infof("permanent attempts=%d err=%v", count, err)
	lab.Check(count == 1 && err != nil, "永久错误只执行一次，避免放大故障")

	lab.Section("场景 B：singleflight 合并缓存击穿")
	loader := &Loader{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, loadErr := loader.Load(ctx, "user:42", func(context.Context) (string, error) {
				time.Sleep(20 * time.Millisecond)
				return "alice", nil
			})
			if loadErr != nil || value != "alice" {
				lab.Check(false, "singleflight 返回值异常: %q %v", value, loadErr)
			}
		}()
	}
	wg.Wait()
	lab.Infof("concurrent=20 backend_calls=%d", loader.Calls())
	lab.Check(loader.Calls() == 1, "20 个并发请求只回源一次")

	lab.Section("场景 C：ants 限制 goroutine 并发")
	peak, err := RunBounded(ctx, 3, 20, func(ctx context.Context) {
		time.Sleep(10 * time.Millisecond)
	})
	lab.Infof("peak=%d err=%v", peak, err)
	lab.Check(err == nil && peak <= 3, "并发峰值不超过池容量")
	lab.Check(peak >= 2, "池确实并行执行，而不是串行")

	lab.Section("组合顺序")
	lab.Infof("缓存击穿：singleflight 合并回源；跨实例仍需要分布式锁/租约")
	lab.Infof("重试：只重试可重试错误，设置总预算，加 jitter，避免重试风暴")
	lab.Infof("资源边界：ants 限制 goroutine 数；不要用重试绕过容量问题")
	lab.Done()
}
