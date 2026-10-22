package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

func TestRetryTransient(t *testing.T) {
	var calls int32
	n, err := Retry(context.Background(), backoff.NewConstantBackOff(time.Millisecond), func(context.Context) error {
		if atomic.AddInt32(&calls, 1) < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, n)
}

func TestRetryPermanent(t *testing.T) {
	var calls int32
	_, err := Retry(context.Background(), backoff.NewConstantBackOff(time.Millisecond), func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return &PermanentError{Err: errors.New("bad credentials")}
	})
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestRetryRespectsContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := Retry(ctx, backoff.NewConstantBackOff(100*time.Millisecond), func(context.Context) error {
		return errors.New("temporary")
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSingleFlightCollapses(t *testing.T) {
	loader := &Loader{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := loader.Load(context.Background(), "k", func(context.Context) (string, error) {
				time.Sleep(20 * time.Millisecond)
				return "v", nil
			})
			require.NoError(t, err)
			require.Equal(t, "v", value)
		}()
	}
	wg.Wait()
	require.Equal(t, int64(1), loader.Calls())
}

func TestBoundedPool(t *testing.T) {
	peak, err := RunBounded(context.Background(), 3, 12, func(context.Context) {
		time.Sleep(5 * time.Millisecond)
	})
	require.NoError(t, err)
	require.LessOrEqual(t, peak, int64(3))
	require.GreaterOrEqual(t, peak, int64(2))
}
