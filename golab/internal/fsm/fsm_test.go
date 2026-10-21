package fsm

import "testing"

func buildFlow(t *testing.T, retries int) *Flow {
	t.Helper()
	f := New()
	for _, id := range []string{"a", "b", "c", "d"} {
		n := retries
		if id != "b" {
			n = 1
		}
		if err := f.AddNode(id, n); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}} {
		if err := f.AddEdge(e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	f.Init()
	return f
}

func TestHappyPath(t *testing.T) {
	f := buildFlow(t, 1)
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := f.Start(id); err != nil {
			t.Fatalf("start %s: %v (state=%s)", id, err, f.State(id))
		}
		if err := f.Succeed(id); err != nil {
			t.Fatalf("succeed %s: %v", id, err)
		}
	}
	if got := f.Final(); got != Success {
		t.Fatalf("final = %s, want success", got)
	}
}

func TestRetryThenFailSkipsDownstream(t *testing.T) {
	f := buildFlow(t, 2)
	if err := f.Start("a"); err != nil {
		t.Fatal(err)
	}
	_ = f.Succeed("a")
	if err := f.Start("b"); err != nil {
		t.Fatal(err)
	}
	retry, err := f.Fail("b")
	if err != nil || !retry {
		t.Fatalf("first fail retry=%v err=%v", retry, err)
	}
	if err := f.Start("b"); err != nil {
		t.Fatal(err)
	}
	retry, err = f.Fail("b")
	if err != nil || retry {
		t.Fatalf("second fail retry=%v err=%v", retry, err)
	}
	if got := f.State("d"); got != Skipped {
		t.Fatalf("d = %s, want skipped", got)
	}
	if got := f.Final(); got != Failed {
		t.Fatalf("final = %s, want failed", got)
	}
}

func TestInvalidTransition(t *testing.T) {
	f := buildFlow(t, 1)
	if err := f.Succeed("a"); err == nil {
		t.Fatal("expected invalid transition from ready to success")
	}
}
