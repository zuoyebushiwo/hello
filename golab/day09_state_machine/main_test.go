package main

import (
	"testing"

	"golab/internal/fsm"
)

func TestScenarioHappyPath(t *testing.T) {
	f := build(1)
	for _, id := range []string{"start", "extract", "clean", "join", "publish"} {
		if err := f.Start(id); err != nil {
			t.Fatal(err)
		}
		if err := f.Succeed(id); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.Final(); got != fsm.Success {
		t.Fatalf("final = %s", got)
	}
}

func TestScenarioRetryExhausted(t *testing.T) {
	f := build(2)
	for _, id := range []string{"start", "extract"} {
		_ = f.Start(id)
		_ = f.Succeed(id)
	}
	_ = f.Start("clean")
	_, _ = f.Fail("clean")
	_ = f.Start("clean")
	_, _ = f.Fail("clean")
	if f.Final() != fsm.Failed {
		t.Fatalf("final = %s", f.Final())
	}
	if f.State("publish") != fsm.Skipped {
		t.Fatalf("publish = %s", f.State("publish"))
	}
}
