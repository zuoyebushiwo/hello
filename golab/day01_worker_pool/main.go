package main

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"golab/internal/labkit"
	"golab/internal/workerpool"
)

func main() {
	lab := labkit.New("Day 01", "并发原语：ExecutorService 的 Go 版本")

	lab.Section("场景 A：24 个同步任务，最多 6 个并发")
	var completed atomic.Int64
	pool := workerpool.New(context.Background(), 6, 32)
	start := time.Now()
	for i := 1; i <= 24; i++ {
		id := i
		err := pool.Submit(func(ctx context.Context) {
			d := time.Duration(15+(id*7)%35) * time.Millisecond
			if !labkit.Sleep(ctx, d) {
				return
			}
			completed.Add(1)
			if id == 13 {
				panic("simulated poison task")
			}
		})
		if err != nil {
			lab.Failf("submit task %d: %v", id, err)
		}
	}
	pool.Close()
	stats := pool.Stats()
	lab.Infof("completed=%d max_concurrency=%d panics=%d elapsed=%s", completed.Load(), stats.MaxRunning, stats.Panics, time.Since(start).Round(time.Millisecond))
	lab.Check(completed.Load() == 24, "所有任务都被执行（包括 panic 前完成的那一个）")
	lab.Check(stats.MaxRunning <= 6, "并发度始终不超过 6，实际峰值 %d", stats.MaxRunning)
	lab.Check(stats.Panics == 1, "panic 被 worker 隔离并计数，服务未崩溃")

	lab.Section("场景 B：对照组，裸起 24 个 goroutine")
	var naiveActive, naiveMax atomic.Int64
	var wgDone atomic.Int64
	for i := 0; i < 24; i++ {
		go func() {
			n := naiveActive.Add(1)
			for {
				old := naiveMax.Load()
				if n <= old || naiveMax.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			naiveActive.Add(-1)
			wgDone.Add(1)
		}()
	}
	time.Sleep(60 * time.Millisecond)
	lab.Infof("naive max_concurrency=%d completed=%d", naiveMax.Load(), wgDone.Load())
	lab.Check(naiveMax.Load() > 6, "没有池化时并发度不可控")

	lab.Section("场景 C：队列满时，提交方必须能被超时唤醒")
	small := workerpool.New(context.Background(), 1, 0)
	_ = small.Submit(func(ctx context.Context) {
		_ = labkit.Sleep(ctx, 180*time.Millisecond)
	})
	time.Sleep(10 * time.Millisecond)
	submitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := small.SubmitContext(submitCtx, func(context.Context) {})
	lab.Infof("submit error = %v", err)
	lab.Check(errors.Is(err, context.DeadlineExceeded), "等待槽位超时返回 DeadlineExceeded，而不是永久阻塞")
	small.Close()

	lab.Section("场景 D：有返回值任务，Future.Get 带超时")
	futurePool := workerpool.New(context.Background(), 4, 16)
	future, err := workerpool.SubmitFuture(futurePool, context.Background(), func(ctx context.Context) (int, error) {
		if !labkit.Sleep(ctx, 20*time.Millisecond) {
			return 0, ctx.Err()
		}
		return 7 * 7, nil
	})
	if err != nil {
		lab.Failf("submit future: %v", err)
	} else {
		getCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		value, getErr := future.Get(getCtx)
		cancel()
		lab.Infof("future value=%d err=%v done=%v", value, getErr, future.IsDone())
		lab.Check(getErr == nil && value == 49, "Future.Get 拿到 Callable 的返回值")
	}

	slow, err := workerpool.SubmitFuture(futurePool, context.Background(), func(ctx context.Context) (string, error) {
		if !labkit.Sleep(ctx, 120*time.Millisecond) {
			return "", ctx.Err()
		}
		return "late result", nil
	})
	if err != nil {
		lab.Failf("submit slow future: %v", err)
	} else {
		shortCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		_, getErr := slow.Get(shortCtx)
		cancel()
		lab.Infof("short Get err=%v done=%v", getErr, slow.IsDone())
		lab.Check(errors.Is(getErr, context.DeadlineExceeded), "Get 超时只取消等待，不取消任务")

		longCtx, cancelLong := context.WithTimeout(context.Background(), time.Second)
		value, getErr := slow.Get(longCtx)
		cancelLong()
		lab.Check(getErr == nil && value == "late result", "超时后任务仍执行完成，可再次 Get")
	}

	panicFuture, err := workerpool.SubmitFuture(futurePool, context.Background(), func(context.Context) (int, error) {
		panic("simulated callable panic")
	})
	if err != nil {
		lab.Failf("submit panic future: %v", err)
	} else {
		_, getErr := panicFuture.Get(context.Background())
		lab.Infof("panic future err=%v", getErr)
		lab.Check(errors.Is(getErr, workerpool.ErrTaskPanic), "任务 panic 通过 Future 返回错误，同时 worker 仍可复用")
	}
	futurePool.Close()

	lab.Section("Java → Go 对照")
	lab.Infof("ExecutorService.newFixedThreadPool(6)  ≈  N 个 goroutine + 带缓冲 channel")
	lab.Infof("submit(Callable<T>)                    ≈  SubmitFuture(pool, ctx, fn) 返回 Future[T]")
	lab.Infof("Future.get(timeout)                    ≈  future.Get(context.WithTimeout(...))")
	lab.Infof("get 超时不会取消任务                    ≈  同样只取消等待，任务继续完成")
	lab.Infof("ThreadPoolExecutor 拒绝策略             ≈  SubmitContext 返回错误，由调用方决定降级")
	lab.Done()
}
