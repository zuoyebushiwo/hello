package workerpool

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
)

var (
	ErrTaskPanic = errors.New("task panicked")
	ErrNilTask   = errors.New("task function is nil")
)

// PanicError is returned through Future.Get when a task panics. The panic is
// still propagated to the pool after the Future is completed, so Pool.Stats
// continues to count it and the worker is restarted normally.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("%v: %v", ErrTaskPanic, e.Value)
}

func (e *PanicError) Unwrap() error { return ErrTaskPanic }

// Future is the result handle returned by SubmitFuture. It mirrors the useful
// part of java.util.concurrent.Future without hiding task errors:
//
//	value, err := future.Get(ctx)
type Future[T any] struct {
	done  chan struct{}
	once  sync.Once
	value T
	err   error
}

func (f *Future[T]) complete(value T, err error) {
	f.once.Do(func() {
		f.value = value
		f.err = err
		close(f.done)
	})
}

// Done is closed when the task finishes, successfully or otherwise.
func (f *Future[T]) Done() <-chan struct{} { return f.done }

func (f *Future[T]) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// Get waits for the task result. A cancelled ctx only stops this wait; it does
// not cancel the task, matching Future.get(timeout) semantics. Use
// context.Background() when you want to wait without a deadline.
func (f *Future[T]) Get(ctx context.Context) (T, error) {
	select {
	case <-f.done:
		return f.value, f.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// SubmitFuture is the typed equivalent of submit(Callable<T>). It is a
// package-level generic function rather than a method because this module keeps
// its language baseline at Go 1.22, where generic methods are unavailable.
//
// submitCtx bounds only admission to the queue. The task itself runs with the
// pool's lifetime context; callers can stop waiting with Future.Get(ctx).
func SubmitFuture[T any](p *Pool, submitCtx context.Context, fn func(context.Context) (T, error)) (*Future[T], error) {
	if fn == nil {
		return nil, ErrNilTask
	}
	future := &Future[T]{done: make(chan struct{})}
	err := p.SubmitContext(submitCtx, func(ctx context.Context) {
		var value T
		var taskErr error
		defer func() {
			if r := recover(); r != nil {
				future.complete(value, &PanicError{Value: r, Stack: debug.Stack()})
				panic(r)
			}
			future.complete(value, taskErr)
		}()
		value, taskErr = fn(ctx)
	})
	if err != nil {
		return nil, err
	}
	return future, nil
}
