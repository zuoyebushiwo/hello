package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golab/internal/labkit"
)

// Small capabilities, not a god interface. Readers only read, writers only
// write, and only the pipeline needs the composite Source/Sink.
type Reader interface {
	Read(ctx context.Context) ([]string, error)
}

type Writer interface {
	Write(ctx context.Context, rows []string) error
}

type Closer interface {
	Close() error
}

type Source interface {
	Reader
	Closer
}

type Sink interface {
	Writer
	Closer
}

// Optional capability: callers discover it with a type assertion.
type Flusher interface {
	Flush() error
}

var ErrSinkClosed = errors.New("sink is closed")
var ErrSinkFull = errors.New("sink is full")

type CSVSource struct {
	rows   []string
	closed bool
}

func (s *CSVSource) Read(ctx context.Context) ([]string, error) {
	if s.closed {
		return nil, errors.New("source is closed")
	}
	if !labkit.Sleep(ctx, 10*time.Millisecond) {
		return nil, ctx.Err()
	}
	return append([]string(nil), s.rows...), nil
}

func (s *CSVSource) Close() error {
	s.closed = true
	return nil
}

type MemorySink struct {
	mu      sync.Mutex
	rows    []string
	closed  bool
	flushed bool
}

func (s *MemorySink) Write(ctx context.Context, rows []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSinkClosed
	}
	s.rows = append(s.rows, rows...)
	return nil
}

func (s *MemorySink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushed = true
	return nil
}

func (s *MemorySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *MemorySink) Rows() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.rows...)
}

type FailingSink struct {
	closed bool
}

func (s *FailingSink) Write(ctx context.Context, rows []string) error {
	if !labkit.Sleep(ctx, 5*time.Millisecond) {
		return ctx.Err()
	}
	return fmt.Errorf("write batch of %d rows: %w", len(rows), ErrSinkFull)
}

func (s *FailingSink) Close() error {
	s.closed = true
	return errors.New("close failed after a write error")
}

var _ Source = (*CSVSource)(nil)
var _ Sink = (*MemorySink)(nil)
var _ Sink = (*FailingSink)(nil)
var _ Flusher = (*MemorySink)(nil)

// Copy depends on two tiny capabilities. It does not know whether the source is
// CSV, Kafka or MySQL, and it does not know whether the sink is memory or S3.
func Copy(ctx context.Context, src Source, dst Sink) (n int, err error) {
	defer func() {
		if e := src.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close source: %w", e))
		}
		if e := dst.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close sink: %w", e))
		}
	}()

	rows, err := src.Read(ctx)
	if err != nil {
		return 0, err
	}
	if err := dst.Write(ctx, rows); err != nil {
		return 0, err
	}
	if f, ok := dst.(Flusher); ok {
		if err := f.Flush(); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// brokenWriter demonstrates the nil-interface trap. The interface value is not
// nil even though the concrete pointer inside it is.
type brokenWriter struct{}

func (b *brokenWriter) Write(ctx context.Context, rows []string) error { return nil }

func main() {
	lab := labkit.New("Day 03", "小接口：能力隔离与 nil interface 陷阱")

	lab.Section("场景 A：同一条管道落到两种 sink")
	src := &CSVSource{rows: []string{"id,name", "1,Ada", "2,Linus"}}
	sink := &MemorySink{}
	n, err := Copy(context.Background(), src, sink)
	lab.Infof("copied=%d rows=%v flushed=%v err=%v", n, sink.Rows(), sink.flushed, err)
	lab.Check(err == nil && n == 3, "Copy 只依赖 Reader/Writer/Closer 三个能力")
	lab.Check(sink.flushed, "Flusher 是可选的，通过 type assertion 发现")

	lab.Section("场景 B：写入失败 + Close 失败，错误必须同时保留")
	_, err = Copy(context.Background(), &CSVSource{rows: []string{"a", "b"}}, &FailingSink{})
	lab.Infof("joined error = %v", err)
	lab.Check(errors.Is(err, ErrSinkFull), "errors.Is 能穿透包装找到业务错误")
	lab.Check(err != nil && errors.Is(err, ErrSinkFull), "Close 的错误没有覆盖 Write 的错误")

	lab.Section("场景 C：nil interface 不等于 nil 指针")
	var w Writer = (*brokenWriter)(nil)
	lab.Infof("w == nil -> %v", w == nil)
	lab.Check(w != nil, "接口包含类型信息，所以非 nil；返回 error 时尤其危险")
	lab.Check(w.Write(context.Background(), nil) == nil, "nil receiver 方法若不解引用仍可调用")

	lab.Section("场景 D：为什么不要定义 30 个方法的 God Interface")
	lab.Infof("CSVSource 只需要 Read/Close；MemorySink 只需要 Write/Flush/Close")
	lab.Infof("让调用方定义它真正使用的最小接口，fake 才会只有 2-3 个方法")

	lab.Section("Java → Go 对照")
	lab.Infof("class X implements Reader  ≈  隐式满足接口，不需要 implements 声明")
	lab.Infof("接口定义在实现方                   ≈  Go 习惯定义在消费方")
	lab.Infof("instanceof Flusher                 ≈  f, ok := dst.(Flusher)")
	lab.Done()
}
