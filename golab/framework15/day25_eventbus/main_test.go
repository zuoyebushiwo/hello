package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/stretchr/testify/require"
)

func start(t *testing.T) *Bus {
	t.Helper()
	logger := watermill.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	bus, err := StartBus(context.Background(), logger)
	require.NoError(t, err)
	t.Cleanup(bus.Stop)
	return bus
}

func TestFanOut(t *testing.T) {
	bus := start(t)
	require.NoError(t, bus.Publish(OrderCreated{OrderID: "o-1", CustomerID: "c-1", Amount: 10}))

	select {
	case event := <-bus.Audit:
		require.Equal(t, "o-1", event.OrderID)
	case <-time.After(time.Second):
		t.Fatal("audit consumer timed out")
	}
	select {
	case event := <-bus.Notify:
		require.Equal(t, "c-1", event.CustomerID)
	case <-time.After(time.Second):
		t.Fatal("notify consumer timed out")
	}
}

func TestMultipleEvents(t *testing.T) {
	bus := start(t)
	for _, id := range []string{"o-1", "o-2", "o-3"} {
		require.NoError(t, bus.Publish(OrderCreated{OrderID: id}))
	}
	var got []string
	for range 3 {
		select {
		case event := <-bus.Audit:
			got = append(got, event.OrderID)
		case <-time.After(time.Second):
			t.Fatal("audit consumer timed out")
		}
	}
	require.ElementsMatch(t, []string{"o-1", "o-2", "o-3"}, got)
}
