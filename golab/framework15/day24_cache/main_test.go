package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCacheHit(t *testing.T) {
	cache := NewUserCache(10, time.Minute)
	value, hit, err := cache.Get(context.Background(), "k", func(context.Context) (string, error) { return "v", nil })
	require.NoError(t, err)
	require.False(t, hit)
	require.Equal(t, "v", value)
	value, hit, err = cache.Get(context.Background(), "k", func(context.Context) (string, error) { return "other", nil })
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, "v", value)
	require.Equal(t, int64(1), cache.Loads())
}

func TestLRUEviction(t *testing.T) {
	cache := NewUserCache(2, time.Minute)
	ctx := context.Background()
	_, _, _ = cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, _, _ = cache.Get(ctx, "b", func(context.Context) (string, error) { return "B", nil })
	_, _, _ = cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, _, _ = cache.Get(ctx, "c", func(context.Context) (string, error) { return "C", nil })
	_, hitA, _ := cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, hitB, _ := cache.Get(ctx, "b", func(context.Context) (string, error) { return "B", nil })
	require.True(t, hitA)
	require.False(t, hitB)
}

func TestTTL(t *testing.T) {
	cache := NewUserCache(2, 20*time.Millisecond)
	ctx := context.Background()
	_, _, _ = cache.Get(ctx, "k", func(context.Context) (string, error) { return "old", nil })
	time.Sleep(30 * time.Millisecond)
	value, hit, _ := cache.Get(ctx, "k", func(context.Context) (string, error) { return "new", nil })
	require.False(t, hit)
	require.Equal(t, "new", value)
}

func TestSingleFlight(t *testing.T) {
	cache := NewUserCache(10, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = cache.Get(context.Background(), "hot", func(context.Context) (string, error) {
				time.Sleep(20 * time.Millisecond)
				return "v", nil
			})
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), cache.Loads())
}
