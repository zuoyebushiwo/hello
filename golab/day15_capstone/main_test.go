package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"golab/internal/fsm"
	"golab/internal/labkit"
)

func TestEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := labkit.NewRegistry()
	engine := NewEngine(ctx, specs(), logger, reg)
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	report := engine.Report()
	if report.Final != fsm.Success {
		t.Fatalf("final = %s", report.Final)
	}
	if report.Processed != 6 {
		t.Fatalf("processed = %d", report.Processed)
	}
	if report.Attempts["clean_b"] != 2 {
		t.Fatalf("clean_b attempts = %d", report.Attempts["clean_b"])
	}
	if report.Triggers["join"] != 1 {
		t.Fatalf("join triggers = %d", report.Triggers["join"])
	}
	if !bytes.Contains([]byte(reg.Render()), []byte(`capstone_node_total{node="join",status="success"} 1`)) {
		t.Fatalf("metrics missing join success: %s", reg.Render())
	}
}
