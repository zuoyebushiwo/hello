package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskProcessed(t *testing.T) {
	received := make(chan EmailPayload, 1)
	queue, err := NewTaskQueue(1, func(_ context.Context, payload EmailPayload) error {
		received <- payload
		return nil
	})
	require.NoError(t, err)
	defer queue.Close()

	id, err := queue.Enqueue(context.Background(), EmailPayload{To: "a@b.c", Subject: "hi"})
	require.NoError(t, err)
	require.NotEmpty(t, id)
	select {
	case payload := <-received:
		require.Equal(t, "a@b.c", payload.To)
	case <-time.After(2 * time.Second):
		t.Fatal("task was not processed")
	}
}

func TestConcurrentTasks(t *testing.T) {
	done := make(chan struct{}, 3)
	queue, err := NewTaskQueue(3, func(context.Context, EmailPayload) error {
		done <- struct{}{}
		return nil
	})
	require.NoError(t, err)
	defer queue.Close()

	for i := 0; i < 3; i++ {
		_, err := queue.Enqueue(context.Background(), EmailPayload{To: "a@b.c"})
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("not all tasks were processed")
		}
	}
}
