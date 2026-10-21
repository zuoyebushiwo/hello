package main

import "testing"

func TestPromotedMethodIsNotVirtual(t *testing.T) {
	sql := &SQLExecutor{BaseExecutor: &BaseExecutor{Name: "orders"}, SQL: "select 1"}
	if got := sql.Run(); got != "run(orders)" {
		t.Fatalf("Run() = %q, want run(orders)", got)
	}
}

func TestExplicitComposition(t *testing.T) {
	sql := &FixedSQLExecutor{BaseExecutor: &BaseExecutor{Name: "orders"}, SQL: "select 1"}
	if got := sql.Run(); got != "run(sql:orders)" {
		t.Fatalf("Run() = %q, want run(sql:orders)", got)
	}
}

func TestPromotedValidate(t *testing.T) {
	if err := (&SQLExecutor{BaseExecutor: &BaseExecutor{}}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
