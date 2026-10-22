package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"frameworklab/internal/labkit"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

const OrderCreatedTopic = "orders.created"

type OrderCreated struct {
	OrderID    string `json:"order_id"`
	CustomerID string `json:"customer_id"`
	Amount     int64  `json:"amount"`
}

type Bus struct {
	router  *message.Router
	pubsub  *gochannel.GoChannel
	Audit   <-chan OrderCreated
	Notify  <-chan OrderCreated
	cancel  context.CancelFunc
	runDone chan error
}

func StartBus(parent context.Context, logger watermill.LoggerAdapter) (*Bus, error) {
	pubsub := gochannel.NewGoChannel(gochannel.Config{}, logger)
	router, err := message.NewRouter(message.RouterConfig{CloseTimeout: time.Second}, logger)
	if err != nil {
		return nil, err
	}
	audit := make(chan OrderCreated, 4)
	notify := make(chan OrderCreated, 4)

	router.AddNoPublisherHandler("audit", OrderCreatedTopic, pubsub, consume("audit", audit))
	router.AddNoPublisherHandler("notify", OrderCreatedTopic, pubsub, consume("notify", notify))

	ctx, cancel := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() { done <- router.Run(ctx) }()
	select {
	case <-router.Running():
	case <-time.After(time.Second):
		cancel()
		return nil, fmt.Errorf("router did not start")
	}
	return &Bus{router: router, pubsub: pubsub, Audit: audit, Notify: notify, cancel: cancel, runDone: done}, nil
}

func consume(name string, out chan<- OrderCreated) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var event OrderCreated
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			return fmt.Errorf("%s decode: %w", name, err)
		}
		out <- event
		return nil
	}
}

func (b *Bus) Publish(event OrderCreated) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	msg := message.NewMessage(watermill.NewUUID(), payload)
	msg.Metadata.Set("content-type", "application/json")
	return b.pubsub.Publish(OrderCreatedTopic, msg)
}

func (b *Bus) Stop() {
	b.cancel()
	select {
	case <-b.runDone:
	case <-time.After(2 * time.Second):
	}
	_ = b.pubsub.Close()
}

func main() {
	lab := labkit.New("Day 25", "Watermill：事件总线、解耦与至少一次投递")
	logger := watermill.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	bus, err := StartBus(context.Background(), logger)
	if err != nil {
		lab.Check(false, "start bus: %v", err)
		lab.Done()
		return
	}
	defer bus.Stop()

	lab.Section("场景 A：一个事件、两个独立消费者")
	event := OrderCreated{OrderID: "o-1", CustomerID: "c-1", Amount: 199}
	lab.Check(bus.Publish(event) == nil, "发布订单创建事件")
	audit := receive(bus.Audit)
	notify := receive(bus.Notify)
	lab.Infof("audit=%+v notify=%+v", audit, notify)
	lab.Check(audit.OrderID == "o-1" && notify.OrderID == "o-1", "审计和通知各自收到完整事件")
	lab.Check(audit.OrderID == notify.OrderID, "两个消费者互不阻塞")

	lab.Section("场景 B：事件契约")
	lab.Infof("topic=%s metadata content-type=application/json", OrderCreatedTopic)
	lab.Infof("生产环境换成 Kafka/RabbitMQ 时，业务 handler 不变，只替换 Publisher/Subscriber")
	lab.Infof("至少一次投递意味着消费者必须幂等；事件 ID 可用于去重")

	lab.Section("选型边界")
	lab.Infof("Watermill 适合事件驱动骨架；复杂流处理、分区、回溯优先 Kafka/Flink")
	lab.Infof("本地 pub/sub 不能跨进程；多实例必须接真实 broker")
	lab.Done()
}

func receive[T any](ch <-chan T) T {
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		var zero T
		return zero
	}
}
