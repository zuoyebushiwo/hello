package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golab/internal/fsm"
	"golab/internal/graph"
	"golab/internal/labkit"
	"golab/internal/workerpool"
)

type NodeSpec struct {
	ID           string
	Upstream     []string
	Work         time.Duration
	FailAttempts int
}

func specs() []NodeSpec {
	return []NodeSpec{
		{ID: "start", Work: 5 * time.Millisecond},
		{ID: "extract", Upstream: []string{"start"}, Work: 15 * time.Millisecond},
		{ID: "clean_a", Upstream: []string{"extract"}, Work: 20 * time.Millisecond},
		{ID: "clean_b", Upstream: []string{"extract"}, Work: 25 * time.Millisecond, FailAttempts: 1},
		{ID: "join", Upstream: []string{"clean_a", "clean_b"}, Work: 15 * time.Millisecond},
		{ID: "publish", Upstream: []string{"join"}, Work: 10 * time.Millisecond},
	}
}

type Engine struct {
	specs  map[string]NodeSpec
	graph  *graph.Graph
	flow   *fsm.Flow
	queue  *labkit.Queue
	kv     *labkit.KV
	pool   *workerpool.Pool
	reg    *labkit.Registry
	logger *slog.Logger

	pending  atomic.Int64
	inflight sync.WaitGroup

	mu        sync.Mutex
	processed int
	skipped   int
	attempts  map[string]int
	triggers  map[string]int
}

func NewEngine(ctx context.Context, list []NodeSpec, logger *slog.Logger, reg *labkit.Registry) *Engine {
	e := &Engine{
		specs:    make(map[string]NodeSpec),
		graph:    graph.New(),
		flow:     fsm.New(),
		queue:    labkit.NewQueue(),
		kv:       labkit.NewKV(),
		pool:     workerpool.New(ctx, 3, 64),
		reg:      reg,
		logger:   logger,
		attempts: make(map[string]int),
		triggers: make(map[string]int),
	}
	for _, s := range list {
		e.specs[s.ID] = s
		e.graph.AddNode(s.ID)
		maxAttempts := 1
		if s.FailAttempts > 0 {
			maxAttempts = s.FailAttempts + 1
		}
		_ = e.flow.AddNode(s.ID, maxAttempts)
	}
	for _, s := range list {
		for _, up := range s.Upstream {
			_ = e.graph.AddEdge(up, s.ID)
			_ = e.flow.AddEdge(up, s.ID)
		}
	}
	e.flow.Init()
	// Duplicate delivery of the root message: the broker is at-least-once.
	e.publish("start", 1)
	e.publish("start", 1)
	return e
}

func (e *Engine) publish(node string, attempt int) {
	e.pending.Add(1)
	e.queue.Publish(node, fmt.Sprintf("%s|%d", node, attempt))
}

func parseMessage(payload string) (string, int) {
	parts := strings.SplitN(payload, "|", 2)
	attempt, _ := strconv.Atoi(parts[1])
	return parts[0], attempt
}

func (e *Engine) Run(ctx context.Context) error {
	for e.pending.Load() > 0 {
		msg, ok := e.queue.Receive()
		if !ok {
			if !labkit.Sleep(ctx, time.Millisecond) {
				return ctx.Err()
			}
			continue
		}
		e.inflight.Add(1)
		err := e.pool.Submit(func(ctx context.Context) {
			defer e.inflight.Done()
			defer e.pending.Add(-1)
			e.handle(ctx, msg)
		})
		if err != nil {
			e.inflight.Done()
			e.pending.Add(-1)
			return err
		}
	}
	e.inflight.Wait()
	e.pool.Close()
	return nil
}

func (e *Engine) handle(ctx context.Context, msg *labkit.Message) {
	node, attempt := parseMessage(msg.Payload)
	spec := e.specs[node]

	// Message-level idempotency: the same (node, attempt) is processed once.
	if !e.kv.SetNX(fmt.Sprintf("exec:%s:%d", node, attempt), "claimed", time.Hour) {
		e.skip("duplicate-message")
		return
	}
	if e.flow.State(node) != fsm.Ready {
		e.skip("not-ready:" + string(e.flow.State(node)))
		return
	}
	if err := e.flow.Start(node); err != nil {
		e.skip("start-error:" + err.Error())
		return
	}
	e.mu.Lock()
	e.attempts[node]++
	e.mu.Unlock()
	e.logger.Info("node started", "node", node, "attempt", attempt, "state", e.flow.State(node))

	started := time.Now()
	if !labkit.Sleep(ctx, spec.Work) {
		e.skip("cancelled")
		return
	}

	if attempt <= spec.FailAttempts {
		retry, _ := e.flow.Fail(node)
		e.reg.Inc("capstone_node_total", map[string]string{"node": node, "status": "failed"})
		e.logger.Warn("node failed", "node", node, "attempt", attempt, "retrying", retry)
		if retry {
			e.publish(node, attempt+1)
		}
		return
	}

	if err := e.flow.Succeed(node); err != nil {
		e.skip("succeed-error:" + err.Error())
		return
	}
	e.reg.Inc("capstone_node_total", map[string]string{"node": node, "status": "success"})
	e.reg.Observe("capstone_node_duration_seconds", time.Since(started).Seconds(), map[string]string{"node": node})
	e.mu.Lock()
	e.processed++
	e.mu.Unlock()
	e.logger.Info("node succeeded", "node", node, "attempt", attempt, "duration_ms", time.Since(started).Milliseconds())

	for _, down := range e.graph.Downstreams(node) {
		if e.flow.State(down) != fsm.Ready {
			continue
		}
		// Lock + recheck + SetNX is the same pattern as Day 07. The fsm already
		// serializes state, but the enqueue guard also protects the broker side.
		if !e.kv.SetNX("enqueue:"+down, "1", time.Hour) {
			continue
		}
		e.mu.Lock()
		e.triggers[down]++
		e.mu.Unlock()
		e.publish(down, 1)
	}
}

func (e *Engine) skip(reason string) {
	e.mu.Lock()
	e.skipped++
	e.mu.Unlock()
	e.reg.Inc("capstone_node_total", map[string]string{"status": "skipped", "reason": reason})
}

type Report struct {
	Processed int
	Skipped   int
	Final     fsm.State
	Attempts  map[string]int
	Triggers  map[string]int
}

func (e *Engine) Report() Report {
	e.mu.Lock()
	defer e.mu.Unlock()
	attempts := make(map[string]int, len(e.attempts))
	for k, v := range e.attempts {
		attempts[k] = v
	}
	triggers := make(map[string]int, len(e.triggers))
	for k, v := range e.triggers {
		triggers[k] = v
	}
	return Report{Processed: e.processed, Skipped: e.skipped, Final: e.flow.Final(), Attempts: attempts, Triggers: triggers}
}

func main() {
	lab := labkit.New("Day 15", "综合演练：一条完整的数据流")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(labkit.NewLockedWriter(&logBuf), &slog.HandlerOptions{Level: slog.LevelInfo}))
	reg := labkit.NewRegistry()
	engine := NewEngine(ctx, specs(), logger, reg)

	lab.Section("场景：start 重复投递，clean_b 首次失败，其余节点正常")
	if err := engine.Run(ctx); err != nil {
		lab.Failf("run: %v", err)
		lab.Done()
		return
	}
	report := engine.Report()
	lab.Infof("processed=%d skipped=%d final=%s", report.Processed, report.Skipped, report.Final)
	lab.Infof("attempts = %v", report.Attempts)
	lab.Infof("triggers = %v", report.Triggers)

	lab.Section("最终状态")
	for _, n := range engine.flow.Snapshot() {
		lab.Infof("%-8s state=%-8s attempts=%d/%d", n.ID, n.State, n.Attempts, n.MaxAttempts)
	}
	lab.Check(report.Final == fsm.Success, "流程终态 success")
	lab.Check(report.Processed == 6, "六个节点各成功一次")
	lab.Check(report.Attempts["clean_b"] == 2, "clean_b 恰好执行两次（失败 + 重试）")
	lab.Check(report.Triggers["join"] == 1, "两个上游都成功后，join 只被触发一次")
	lab.Check(report.Skipped >= 1, "重复的 start 消息被幂等键拦截")

	lab.Section("Prometheus 指标（节选）")
	metrics := reg.Render()
	shown := 0
	for _, line := range strings.Split(strings.TrimSpace(metrics), "\n") {
		if strings.HasPrefix(line, "capstone_node_total") && shown < 8 {
			lab.Infof("%s", line)
			shown++
		}
	}
	lab.Check(strings.Contains(metrics, `capstone_node_total{node="clean_b",status="failed"} 1`), "失败的执行被单独计数")
	lab.Check(strings.Contains(metrics, `capstone_node_total{node="join",status="success"} 1`), "join 成功计数为 1")

	lab.Section("结构化日志（节选）")
	logLines := strings.Split(strings.TrimSpace(logBuf.String()), "\n")
	for i, line := range logLines {
		if i >= 3 {
			break
		}
		lab.Infof("%s", line)
	}
	lab.Check(strings.Contains(logBuf.String(), `"node":"clean_b"`), "日志带 node 字段，可定位到具体节点")

	lab.Section("这 15 天串起来的知识点")
	lab.Infof("Day 01-02  有界池 + context 预算")
	lab.Infof("Day 05-07  错误分类、幂等、锁与 CAS")
	lab.Infof("Day 08-09  图算法、状态机、重试与下游跳过")
	lab.Infof("Day 12-13  指标、结构化日志、显式装配与关闭")
	lab.Infof("Day 14     生产环境还要加限流、退避和熔断")
	lab.Done()
}
