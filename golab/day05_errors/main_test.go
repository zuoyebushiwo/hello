package main

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"404", "not_found"},
		{"bad", "validation"},
		{"flaky", "retryable"},
		{"ok", "ok"},
	}
	for _, tc := range cases {
		if got := classify(loadUser(tc.id)); got != tc.want {
			t.Errorf("classify(%s) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestErrorsAsAfterWrap(t *testing.T) {
	err := processMessage()
	var target *nackError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %v", err)
	}
	if _, ok := err.(*nackError); ok {
		t.Fatal("direct assertion unexpectedly succeeded")
	}
}

func TestJoin(t *testing.T) {
	joined := errors.Join(ErrNotFound, &ValidationError{Field: "x", Reason: "y"})
	if !errors.Is(joined, ErrNotFound) {
		t.Fatal("joined error lost ErrNotFound")
	}
	var ve *ValidationError
	if !errors.As(joined, &ve) {
		t.Fatal("joined error lost ValidationError")
	}
}
