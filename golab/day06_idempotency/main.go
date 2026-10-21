package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"golab/internal/labkit"
)

type Job struct {
	ID   string
	Rows int
}

type Processor struct {
	kv      *labkit.KV
	mu      sync.Mutex
	effects int

	crashOnce bool
}

func NewProcessor() *Processor { return &Processor{kv: labkit.NewKV()} }

func (p *Processor) Effects() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.effects
}

// recordEffect is the business-side idempotency guard. Message-level dedupe is
// not enough: a process can crash after the side effect but before it writes the
// "done" marker.
func (p *Processor) recordEffect(job Job) {
	if p.kv.SetNX("effect:"+job.ID, "1", 24*time.Hour) {
		p.mu.Lock()
		p.effects += job.Rows
		p.mu.Unlock()
	}
}

func (p *Processor) Handle(ctx context.Context, job Job) (string, error) {
	key := "job:" + job.ID
	if !p.kv.SetNX(key, "processing", 500*time.Millisecond) {
		state, _ := p.kv.Get(key)
		if state == "done" {
			return "skipped:done", nil
		}
		return "skipped:in-progress", nil
	}

	if !labkit.Sleep(ctx, 10*time.Millisecond) {
		p.kv.Delete(key)
		return "", ctx.Err()
	}
	p.recordEffect(job)

	if p.crashOnce {
		p.crashOnce = false
		p.kv.Delete(key) // a lease would expire; we release it to keep the demo fast
		return "", errors.New("simulated crash after side effect")
	}

	p.kv.Set(key, "done", time.Hour)
	return "processed", nil
}

func main() {
	lab := labkit.New("Day 06", "幂等：至少一次投递下的副作用只发生一次")
	ctx := context.Background()

	lab.Section("场景 A：同一条消息被 broker 投递 3 次")
	q := labkit.NewQueue()
	for i := 0; i < 3; i++ {
		q.Publish("msg-1001", `{"job":"orders","rows":10}`)
	}
	p := NewProcessor()
	for {
		msg, ok := q.Receive()
		if !ok {
			break
		}
		outcome, err := p.Handle(ctx, Job{ID: msg.ID, Rows: 10})
		lab.Infof("delivery attempt=%d outcome=%s err=%v", msg.Attempt, outcome, err)
		q.Ack(msg.ID)
	}
	lab.Check(p.Effects() == 10, "三次投递只产生一次业务副作用，总行数=10")

	lab.Section("场景 B：副作用后进程崩溃，再投递一次")
	p2 := NewProcessor()
	p2.crashOnce = true
	outcome, err := p2.Handle(ctx, Job{ID: "msg-2002", Rows: 7})
	lab.Infof("first attempt: outcome=%s err=%v effects=%d", outcome, err, p2.Effects())
	outcome, err = p2.Handle(ctx, Job{ID: "msg-2002", Rows: 7})
	lab.Infof("retry attempt: outcome=%s err=%v effects=%d", outcome, err, p2.Effects())
	lab.Check(p2.Effects() == 7, "崩溃重放不会重复写业务数据，effect key 挡住了第二次")

	lab.Section("场景 C：并发重复消息，SetNX 只有一个赢家")
	p3 := NewProcessor()
	var wg sync.WaitGroup
	outcomes := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _ := p3.Handle(ctx, Job{ID: "msg-3003", Rows: 5})
			outcomes <- out
		}()
	}
	wg.Wait()
	close(outcomes)
	processed := 0
	for out := range outcomes {
		if out == "processed" {
			processed++
		}
	}
	lab.Infof("processed=%d effects=%d", processed, p3.Effects())
	lab.Check(processed == 1 && p3.Effects() == 5, "并发下只有一个 goroutine 获得处理权")

	lab.Section("场景 D：幂等键 TTL 的两难")
	lab.Infof("TTL 太短：重复消息漏过；TTL 太长：存储膨胀，且业务真的允许再次执行时被误挡")
	lab.Infof("生产做法：幂等键 TTL > 最大重试窗口，且业务副作用本身也带唯一键")

	lab.Section("Java → Go 对照")
	lab.Infof("数据库唯一索引 / 幂等表        ≈  这里的效果键 SetNX")
	lab.Infof("@Transactional + 先查后写      ≈  不可靠；要原子 SetNX / INSERT ... ON CONFLICT")
	lab.Infof("消息去重表                      ≈  处理前 claim，处理后 mark done，崩溃后按 lease 重试")
	lab.Done()
}
