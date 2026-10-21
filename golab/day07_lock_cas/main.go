package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golab/internal/labkit"
)

type Scenario struct {
	kv        *labkit.KV
	triggered atomic.Int64

	arrive  chan struct{}
	goAhead chan struct{}
}

func newScenario(upstreams []string) *Scenario {
	s := &Scenario{
		kv:      labkit.NewKV(),
		arrive:  make(chan struct{}, 4),
		goAhead: make(chan struct{}),
	}
	for _, id := range upstreams {
		s.kv.Set("up:"+id, "success", 0)
	}
	return s
}

func (s *Scenario) allSuccess(upstreams []string) bool {
	for _, id := range upstreams {
		v, ok := s.kv.Get("up:" + id)
		if !ok || v != "success" {
			return false
		}
	}
	return true
}

func (s *Scenario) naive(upstreams []string) bool {
	if !s.allSuccess(upstreams) {
		return false
	}
	if state, ok := s.kv.Get("downstream"); ok && (state == "running" || state == "success") {
		return false
	}
	// Widen the race window on purpose: this is where "check then act" breaks.
	s.arrive <- struct{}{}
	<-s.goAhead
	time.Sleep(5 * time.Millisecond)
	s.kv.Set("downstream", "running", 0)
	s.triggered.Add(1)
	return true
}

func (s *Scenario) safe(upstreams []string) bool {
	token := fmt.Sprintf("token-%d", time.Now().UnixNano())
	lock, ok := s.kv.AcquireLock("lock:downstream", token, 2*time.Second, false)
	if !ok {
		return false
	}
	defer lock.Release()

	// Re-read under the lock. The state you checked before the lock is stale.
	if !s.allSuccess(upstreams) {
		return false
	}
	if state, ok := s.kv.Get("downstream"); ok && (state == "running" || state == "success") {
		return false
	}
	// Final guard: CAS/SetNX makes double execution impossible even if the lock
	// expires or is lost.
	if !s.kv.SetNX("downstream", "running", 0) {
		return false
	}
	s.triggered.Add(1)
	return true
}

func runNaiveRound(upstreams []string, workers int) int {
	s := newScenario(upstreams)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.naive(upstreams)
		}()
	}
	for i := 0; i < workers; i++ {
		<-s.arrive
	}
	close(s.goAhead)
	wg.Wait()
	return int(s.triggered.Load())
}

func runSafeRound(upstreams []string, workers int) int {
	s := newScenario(upstreams)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s.safe(upstreams)
		}()
	}
	close(start)
	wg.Wait()
	return int(s.triggered.Load())
}

func main() {
	lab := labkit.New("Day 07", "锁与 CAS：两个上游同时完成，下游只能执行一次")
	upstreams := []string{"extract", "clean"}

	lab.Section("场景 A：先查后写，没有锁")
	naiveTotal := 0
	for i := 0; i < 10; i++ {
		naiveTotal += runNaiveRound(upstreams, 2)
	}
	lab.Infof("10 rounds triggered=%d (expected 20)", naiveTotal)
	lab.Check(naiveTotal == 20, "每一轮两个 goroutine 都通过了检查，下游被触发两次")

	lab.Section("场景 B：锁 + 锁内重查 + SetNX 最终守卫")
	safeTotal := 0
	for i := 0; i < 30; i++ {
		safeTotal += runSafeRound(upstreams, 4)
	}
	lab.Infof("30 rounds triggered=%d (expected 30)", safeTotal)
	lab.Check(safeTotal == 30, "每一轮恰好一个 goroutine 获得触发权")

	lab.Section("场景 C：租约过期后，老持有者不能删除新锁")
	kv := labkit.NewKV()
	old, ok := kv.AcquireLock("lease", "t1", 50*time.Millisecond, false)
	lab.Check(ok, "t1 获得锁")
	_, ok = kv.AcquireLock("lease", "t2", 50*time.Millisecond, false)
	lab.Check(!ok, "锁未过期时 t2 获取失败")
	time.Sleep(70 * time.Millisecond)
	fresh, ok := kv.AcquireLock("lease", "t3", 50*time.Millisecond, false)
	lab.Check(ok, "租约过期后 t3 可以获得锁")
	released := old.Release()
	token, _ := kv.Get("lease")
	lab.Infof("old.Release()=%v current token=%q", released, token)
	lab.Check(!released && token == "t3", "token 校验阻止 t1 误删 t3 的锁")
	fresh.Release()

	lab.Section("场景 D：续期让长任务保住租约")
	kv2 := labkit.NewKV()
	long, ok := kv2.AcquireLock("renew", "r1", 60*time.Millisecond, true)
	lab.Check(ok, "r1 获得可续期锁")
	time.Sleep(160 * time.Millisecond)
	_, ok = kv2.AcquireLock("renew", "r2", 60*time.Millisecond, false)
	lab.Check(!ok, "r1 仍在续期，r2 无法抢锁")
	long.Release()
	_, ok = kv2.AcquireLock("renew", "r3", 60*time.Millisecond, false)
	lab.Check(ok, "释放后 r3 可以获取")

	lab.Section("Java → Go 对照")
	lab.Infof("synchronized / ReentrantLock  ≈  仅单进程有效；跨 Pod 必须用 Redis/DB 锁")
	lab.Infof("lock.lock()                   ≈  SetNX + TTL + token；defer Release")
	lab.Infof("check-then-act                ≈  锁内重查 + CAS/SetNX 双保险")
	lab.Infof("锁超时                         ≈  租约 + 续期 + fencing token，不能假设锁永久有效")
	lab.Done()
}
