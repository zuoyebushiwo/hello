package main

import (
	"testing"
	"time"

	"golab/internal/labkit"
)

func TestNaiveDoubleTrigger(t *testing.T) {
	total := 0
	for i := 0; i < 5; i++ {
		total += runNaiveRound([]string{"a", "b"}, 2)
	}
	if total != 10 {
		t.Fatalf("naive triggered = %d, want 10 (double per round)", total)
	}
}

func TestSafeSingleTrigger(t *testing.T) {
	total := 0
	for i := 0; i < 20; i++ {
		total += runSafeRound([]string{"a", "b"}, 4)
	}
	if total != 20 {
		t.Fatalf("safe triggered = %d, want 20 (once per round)", total)
	}
}

func TestExpiredHolderCannotDeleteNewLock(t *testing.T) {
	kv := labkit.NewKV()
	old, ok := kv.AcquireLock("lease", "t1", 30*time.Millisecond, false)
	if !ok {
		t.Fatal("acquire failed")
	}
	time.Sleep(45 * time.Millisecond)
	fresh, ok := kv.AcquireLock("lease", "t2", time.Second, false)
	if !ok {
		t.Fatal("fresh acquire failed")
	}
	if old.Release() {
		t.Fatal("old holder deleted a newer lock")
	}
	if token, _ := kv.Get("lease"); token != "t2" {
		t.Fatalf("token = %q, want t2", token)
	}
	fresh.Release()
}
