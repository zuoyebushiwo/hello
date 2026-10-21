package tx

import "testing"

func TestBatchVsNPlusOne(t *testing.T) {
	rows := make([]Row, 100)
	for i := range rows {
		rows[i] = Row{"name": "row"}
	}
	n := NewDB()
	if err := n.InsertOneByOne("t", rows); err != nil {
		t.Fatal(err)
	}
	b := NewDB()
	if err := b.BatchInsert("t", rows); err != nil {
		t.Fatal(err)
	}
	ns, bs := n.Stats(), b.Stats()
	if ns.Statements != 100 || bs.Statements != 1 {
		t.Fatalf("n+1 statements=%d batch statements=%d", ns.Statements, bs.Statements)
	}
	if ns.Transactions != 100 || bs.Transactions != 1 {
		t.Fatalf("n+1 tx=%d batch tx=%d", ns.Transactions, bs.Transactions)
	}
	if len(b.Rows("t")) != 100 {
		t.Fatalf("batch rows=%d", len(b.Rows("t")))
	}
}

func TestRollback(t *testing.T) {
	db := NewDB()
	tx := db.Begin()
	if err := tx.Insert("t", Row{"name": "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Insert("t", Row{}); err == nil {
		t.Fatal("expected validation error")
	}
	tx.Rollback()
	if got := len(db.Rows("t")); got != 0 {
		t.Fatalf("rows after rollback = %d", got)
	}
	if db.Stats().Rollbacks != 1 {
		t.Fatalf("rollbacks = %d", db.Stats().Rollbacks)
	}
}

func TestSnapshotIsolation(t *testing.T) {
	db := NewDB()
	t1 := db.Begin()
	t2 := db.Begin()
	if err := t2.Insert("t", Row{"name": "from-t2"}); err != nil {
		t.Fatal(err)
	}
	if err := t2.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := len(t1.Select("t")); got != 0 {
		t.Fatalf("t1 saw t2's commit: %d rows", got)
	}
	if got := len(db.Rows("t")); got != 1 {
		t.Fatalf("db rows = %d", got)
	}
	t1.Rollback()
}

func TestOwnWritesAreVisible(t *testing.T) {
	db := NewDB()
	tx := db.Begin()
	if err := tx.Insert("t", Row{"name": "mine"}); err != nil {
		t.Fatal(err)
	}
	if got := len(tx.Select("t")); got != 1 {
		t.Fatalf("tx did not see its own write: %d", got)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := len(db.Rows("t")); got != 1 {
		t.Fatalf("db rows = %d", got)
	}
}
