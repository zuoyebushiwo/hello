package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"frameworklab/internal/labkit"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

const EmailTask = "email:send"

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

type EmailHandler func(context.Context, EmailPayload) error

type TaskQueue struct {
	redis   *miniredis.Miniredis
	client  *asynq.Client
	server  *asynq.Server
	handler EmailHandler
}

func NewTaskQueue(concurrency int, handler EmailHandler) (*TaskQueue, error) {
	redisServer, err := miniredis.Run()
	if err != nil {
		return nil, fmt.Errorf("start redis: %w", err)
	}
	opt := asynq.RedisClientOpt{Addr: redisServer.Addr()}
	client := asynq.NewClient(opt)
	queue := &TaskQueue{redis: redisServer, client: client, handler: handler}

	queue.server = asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		LogLevel:    asynq.ErrorLevel,
		RetryDelayFunc: func(int, error, *asynq.Task) time.Duration {
			return 5 * time.Millisecond
		},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(EmailTask, queue.processEmail)
	if err := queue.server.Start(mux); err != nil {
		queue.Close()
		return nil, fmt.Errorf("start worker: %w", err)
	}
	return queue, nil
}

func (q *TaskQueue) processEmail(ctx context.Context, task *asynq.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode email payload: %w", err)
	}
	return q.handler(ctx, payload)
}

var taskSeq atomic.Int64

func (q *TaskQueue) Enqueue(ctx context.Context, payload EmailPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	task := asynq.NewTask(EmailTask, data,
		asynq.MaxRetry(3),
		asynq.TaskID(fmt.Sprintf("email-%d", taskSeq.Add(1))),
		asynq.Timeout(2*time.Second),
	)
	info, err := q.client.EnqueueContext(ctx, task)
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

func (q *TaskQueue) Close() {
	if q.server != nil {
		q.server.Shutdown()
	}
	if q.client != nil {
		_ = q.client.Close()
	}
	if q.redis != nil {
		q.redis.Close()
	}
}

func main() {
	lab := labkit.New("Day 26", "Asynq：Redis 任务队列、重试与幂等")
	lab.Section("框架简介：Asynq")
	lab.Infof("类型：Redis 任务队列")
	lab.Infof("是什么：基于 Redis 的分布式任务队列，支持延迟任务、重试、超时、唯一任务和管理面板")
	lab.Infof("解决什么问题：HTTP 请求不应同步执行发邮件、生成报表等慢任务；后台任务需要持久化、重试和可观测性")
	lab.Infof("核心抽象：Client/Task、Server/ServeMux、MaxRetry、TaskID、Timeout、RetryDelayFunc、Inspector")
	lab.Infof("采用情况：Go 社区常用的 Redis 任务队列之一，具体采用度应以团队基础设施和社区维护状态为准")
	lab.Infof("适合：异步通知、图片处理、定时/延迟任务、需要独立 worker 扩展的后台作业")
	lab.Infof("慎用：Redis 必须做持久化和高可用；投递语义仍是至少一次；永久错误应返回 SkipRetry；miniredis 只适合单元测试")
	lab.Infof("对照：River 使用 PostgreSQL；Temporal 适合复杂工作流；cron 只解决定时触发，不提供可靠队列")
	received := make(chan EmailPayload, 4)
	queue, err := NewTaskQueue(2, func(_ context.Context, payload EmailPayload) error {
		received <- payload
		return nil
	})
	if err != nil {
		lab.Check(false, "start queue: %v", err)
		lab.Done()
		return
	}
	defer queue.Close()

	lab.Section("场景 A：投递后由 worker 异步处理")
	id, err := queue.Enqueue(context.Background(), EmailPayload{To: "alice@example.com", Subject: "welcome"})
	lab.Infof("task_id=%s err=%v", id, err)
	lab.Check(err == nil && id != "", "客户端只负责入队")
	select {
	case payload := <-received:
		lab.Infof("processed=%+v", payload)
		lab.Check(payload.To == "alice@example.com", "worker 在独立进程中处理任务")
	case <-time.After(2 * time.Second):
		lab.Check(false, "worker 未在超时内消费任务")
	}

	lab.Section("场景 B：队列解耦了 API 与慢任务")
	lab.Infof("API 不需要等待邮件网关；失败由 worker 重试，并最终进入 archive")
	lab.Infof("至少一次投递：邮件任务必须用业务幂等键，不能只依赖 TaskID")

	lab.Section("场景 C：生产配置")
	lab.Infof("Redis 换成真实实例：asynq.RedisClientOpt{Addr: ...}")
	lab.Infof("监控 queue depth、active、retry、archived；用唯一任务或业务表防重复")
	lab.Infof("本地用 miniredis 运行，测试无需 Docker；集成环境再用 Testcontainers")
	lab.Done()
}
