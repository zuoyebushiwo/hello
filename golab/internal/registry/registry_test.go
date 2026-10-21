package registry

import (
	"errors"
	"testing"
)

func TestDefaultConversions(t *testing.T) {
	r := Default()
	cases := []struct {
		src, dst, in, want string
	}{
		{"mysql", "kudu", "VARCHAR(64)", "STRING"},
		{"mysql", "kudu", "BIGINT", "INT64"},
		{"mysql", "kudu", "INT", "INT32"},
		{"sqlserver", "tidb", "NVARCHAR(128)", "VARCHAR(128)"},
		{"sqlserver", "tidb", "BIT", "TINYINT(1)"},
	}
	for _, tc := range cases {
		got, err := r.Convert(tc.src, tc.dst, tc.in)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s->%s %s = %s, want %s", tc.src, tc.dst, tc.in, got, tc.want)
		}
	}
}

func TestUnsupported(t *testing.T) {
	_, err := Default().Convert("oracle", "kudu", "NUMBER")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestDuplicateRegistration(t *testing.T) {
	r := New()
	if err := r.Register("a", "b", func(s string) string { return s }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("a", "b", func(s string) string { return s }); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}
