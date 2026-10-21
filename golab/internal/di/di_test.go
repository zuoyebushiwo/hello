package di

import (
	"errors"
	"testing"
)

type cfg struct{ name string }
type db struct{ dsn string }

func TestLazyBuildAndReverseClose(t *testing.T) {
	c := New()
	var events []string
	cfgKey := NewKey[cfg]("config")
	dbKey := NewKey[db]("db")
	if err := Provide(c, cfgKey, func(*Container) (cfg, error) {
		events = append(events, "build-config")
		return cfg{name: "prod"}, nil
	}, func(cfg) error { events = append(events, "close-config"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Provide(c, dbKey, func(ctn *Container) (db, error) {
		events = append(events, "build-db")
		conf, err := Get(ctn, cfgKey)
		if err != nil {
			return db{}, err
		}
		return db{dsn: conf.name}, nil
	}, func(db) error { events = append(events, "close-db"); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("providers should be lazy")
	}
	got, err := Get(c, dbKey)
	if err != nil || got.dsn != "prod" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	// Build functions are entered parent-first, but the dependency graph is
	// resolved depth-first: db starts building, then asks for config.
	want := []string{"build-db", "build-config", "close-db", "close-config"}
	if len(events) != len(want) {
		t.Fatalf("events = %v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestBuildFailure(t *testing.T) {
	c := New()
	k := NewKey[db]("db")
	boom := errors.New("dial failed")
	_ = Provide(c, k, func(*Container) (db, error) { return db{}, boom }, nil)
	if _, err := Get(c, k); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	_ = c.Close()
}

func TestCycleDetected(t *testing.T) {
	c := New()
	a := NewKey[cfg]("a")
	b := NewKey[db]("b")
	_ = Provide(c, a, func(ctn *Container) (cfg, error) {
		_, err := Get(ctn, b)
		return cfg{}, err
	}, nil)
	_ = Provide(c, b, func(ctn *Container) (db, error) {
		_, err := Get(ctn, a)
		return db{}, err
	}, nil)
	if _, err := Get(c, a); err == nil {
		t.Fatal("expected cycle error")
	}
}
