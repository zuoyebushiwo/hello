package main

import (
	"context"
	"errors"
	"testing"
)

func TestCopySuccess(t *testing.T) {
	sink := &MemorySink{}
	n, err := Copy(context.Background(), &CSVSource{rows: []string{"a", "b"}}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 || len(sink.Rows()) != 2 || !sink.flushed {
		t.Fatalf("n=%d rows=%v flushed=%v", n, sink.Rows(), sink.flushed)
	}
}

func TestCopyJoinsWriteAndCloseErrors(t *testing.T) {
	_, err := Copy(context.Background(), &CSVSource{rows: []string{"a"}}, &FailingSink{})
	if !errors.Is(err, ErrSinkFull) {
		t.Fatalf("err = %v, want ErrSinkFull", err)
	}
	if err.Error() == ErrSinkFull.Error() {
		t.Fatalf("close error was lost: %v", err)
	}
}

func TestNilInterfaceIsNotNil(t *testing.T) {
	var w Writer = (*brokenWriter)(nil)
	if w == nil {
		t.Fatal("typed nil interface must not compare equal to nil")
	}
}
