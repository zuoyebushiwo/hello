package workerpool

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubmitFutureReturnsValue(t *testing.T) {
	p := New(context.Background(), 2, 4)
	defer p.Close()

	f, err := SubmitFuture(p, context.Background(), func(context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get(context.Background())
	if err != nil || got != 42 {
		t.Fatalf("value=%d err=%v", got, err)
	}
	if !f.IsDone() {
		t.Fatal("future should be done after Get returns")
	}
}

func TestSubmitFuturePropagatesError(t *testing.T) {
	p := New(context.Background(), 1, 1)
	defer p.Close()
	want := errors.New("invalid row")

	f, err := SubmitFuture(p, context.Background(), func(context.Context) (string, error) {
		return "", want
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, got := f.Get(context.Background()); !errors.Is(got, want) {
		t.Fatalf("err = %v, want %v", got, want)
	}
}

func TestFutureGetTimeoutDoesNotCancelTask(t *testing.T) {
	p := New(context.Background(), 1, 1)
	defer p.Close()
	started := make(chan struct{})
	release := make(chan struct{})

	f, err := SubmitFuture(p, context.Background(), func(context.Context) (string, error) {
		close(started)
		<-release
		return "late result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := f.Get(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Get err = %v, want deadline exceeded", err)
	}
	if f.IsDone() {
		t.Fatal("Get timeout must not mark the task done")
	}

	close(release)
	long, cancelLong := context.WithTimeout(context.Background(), time.Second)
	defer cancelLong()
	got, err := f.Get(long)
	if err != nil || got != "late result" {
		t.Fatalf("value=%q err=%v", got, err)
	}
}

func TestFutureConvertsPanicToError(t *testing.T) {
	p := New(context.Background(), 1, 1)
	f, err := SubmitFuture(p, context.Background(), func(context.Context) (int, error) {
		panic("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	_, got := f.Get(context.Background())
	if !errors.Is(got, ErrTaskPanic) {
		t.Fatalf("err = %v, want ErrTaskPanic", got)
	}
	var panicErr *PanicError
	if !errors.As(got, &panicErr) || panicErr.Value != "boom" || len(panicErr.Stack) == 0 {
		t.Fatalf("panic detail = %#v", panicErr)
	}

	p.Close()
	if got := p.Stats().Panics; got != 1 {
		t.Fatalf("panics = %d, want 1", got)
	}
}

func TestSubmitFutureRejectsClosedPool(t *testing.T) {
	p := New(context.Background(), 1, 1)
	p.Close()
	_, err := SubmitFuture(p, context.Background(), func(context.Context) (int, error) {
		return 0, nil
	})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
}

func TestSubmitFutureRejectsNilTask(t *testing.T) {
	p := New(context.Background(), 1, 1)
	defer p.Close()
	if _, err := SubmitFuture[int](p, context.Background(), nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("err = %v, want ErrNilTask", err)
	}
}
