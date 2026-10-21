package main

import (
	"context"
	"testing"
	"time"
)

func TestDAGExecution(t *testing.T) {
	g := build()
	layers, err := g.Layers()
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 5 || len(layers[2]) != 2 {
		t.Fatalf("unexpected layers: %v", layers)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n, err := execute(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("executed = %d, want 6", n)
	}
}
