package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golab/internal/labkit"
)

type requestIDKey struct{}

func call(ctx context.Context, name string, latency time.Duration) error {
	start := time.Now()
	if !labkit.Sleep(ctx, latency) {
		return fmt.Errorf("%s cancelled after %s: %w", name, time.Since(start).Round(time.Millisecond), ctx.Err())
	}
	return nil
}

func main() {
	lab := labkit.New("Day 02", "context：超时、取消与请求级数据")

	lab.Section("场景 A：父预算 400ms，三个下游各自耗时")
	root, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	root = context.WithValue(root, requestIDKey{}, "req-7f3a")

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	calls := []struct {
		name    string
		latency time.Duration
	}{
		{"mysql", 120 * time.Millisecond},
		{"internal-api", 260 * time.Millisecond},
		{"cold-cache", 600 * time.Millisecond},
	}
	for _, c := range calls {
		wg.Add(1)
		go func(name string, latency time.Duration) {
			defer wg.Done()
			results <- result{name: name, err: call(root, name, latency)}
		}(c.name, c.latency)
	}
	wg.Wait()
	close(results)

	var cacheTimedOut bool
	for r := range results {
		if r.err == nil {
			lab.Infof("%-12s success", r.name)
		} else {
			lab.Infof("%-12s error: %v", r.name, r.err)
			if r.name == "cold-cache" && errors.Is(r.err, context.DeadlineExceeded) {
				cacheTimedOut = true
			}
		}
	}
	lab.Check(cacheTimedOut, "慢调用继承父 context 的 deadline 并返回 DeadlineExceeded")
	lab.Check(root.Value(requestIDKey{}) == "req-7f3a", "request_id 通过 context 传递，日志链路可关联")

	lab.Section("场景 B：忽略 context 的 goroutine 会泄漏")
	var leakyDone, goodDone atomic.Int64
	ctx, stop := context.WithCancel(context.Background())
	for i := 0; i < 10; i++ {
		go func() {
			<-make(chan struct{}) // 永远不返回，也不看 ctx
			leakyDone.Add(1)
		}()
		go func() {
			<-ctx.Done()
			goodDone.Add(1)
		}()
	}
	stop()
	time.Sleep(30 * time.Millisecond)
	lab.Infof("cancelled-aware goroutines finished=%d, leaky goroutines finished=%d", goodDone.Load(), leakyDone.Load())
	lab.Check(goodDone.Load() == 10 && leakyDone.Load() == 0, "只有监听 ctx.Done 的 goroutine 能退出")

	lab.Section("场景 C：子预算比父预算更短时，以更短者为准")
	parent, pcancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer pcancel()
	child, ccancel := context.WithTimeout(parent, 80*time.Millisecond)
	defer ccancel()
	err := call(child, "short-budget", 300*time.Millisecond)
	lab.Infof("child error = %v", err)
	lab.Check(errors.Is(err, context.DeadlineExceeded), "子 context 的 80ms 预算生效")

	lab.Section("Java → Go 对照")
	lab.Infof("Future.cancel(true) / Thread.interrupt  ≈  cancel() + 每个阻塞点 select ctx.Done()")
	lab.Infof("ThreadLocal + MDC                       ≈  context.Value 只放 request_id/trace_id 等横切元数据")
	lab.Infof("业务参数（project/flow/node）           ≈  显式结构体字段，不要塞进 context")
	lab.Done()
}
