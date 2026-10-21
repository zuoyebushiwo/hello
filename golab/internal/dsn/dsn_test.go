package dsn

import (
	"net/url"
	"testing"
)

func TestSpecialCharactersRoundTrip(t *testing.T) {
	raw, err := Build("dm", `BL:ZY@user`, `Poly@BLZY467\:%end`, "172.16.203.10", "5236", "BLZY", url.Values{
		"schema":     []string{"BLZY"},
		"autoCommit": []string{"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	u, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	if u.User.Username() != `BL:ZY@user` {
		t.Fatalf("username = %q", u.User.Username())
	}
	if pw, _ := u.User.Password(); pw != `Poly@BLZY467\:%end` {
		t.Fatalf("password = %q", pw)
	}
	if u.Host != "172.16.203.10:5236" {
		t.Fatalf("host = %q", u.Host)
	}
	if u.Query().Get("autoCommit") != "true" {
		t.Fatalf("query = %v", u.Query())
	}
}

func TestIPv6(t *testing.T) {
	raw, err := Build("postgres", "u", "p", "::1", "5432", "db", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := Parse(raw)
	if u.Host != "[::1]:5432" {
		t.Fatalf("host = %q", u.Host)
	}
}

func TestMissingFields(t *testing.T) {
	if _, err := Build("", "u", "p", "h", "1", "db", nil); err == nil {
		t.Fatal("expected validation error")
	}
}
