package main

import (
	"testing"

	"golab/internal/dsn"
	"golab/internal/registry"
)

func TestDefaultRegistry(t *testing.T) {
	r := registry.Default()
	got, err := r.Convert("mysql", "kudu", "VARCHAR(32)")
	if err != nil || got != "STRING" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestDSNRoundTrip(t *testing.T) {
	raw, err := dsn.Build("dm", "u@x", "p:y", "127.0.0.1", "5236", "db", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := dsn.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.User.Username() != "u@x" {
		t.Fatalf("user=%q", u.User.Username())
	}
	if pw, _ := u.User.Password(); pw != "p:y" {
		t.Fatalf("password=%q", pw)
	}
}
