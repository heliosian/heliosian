package id

import (
	"strings"
	"testing"
)

func TestNewIsAnID(t *testing.T) {
	for range 1000 {
		s := New(func(string) bool { return false })
		if got, ok := Parse(s); !ok || got != s {
			t.Fatalf("New() = %q, which does not parse", s)
		}
	}
}

func TestNewRedrawsTaken(t *testing.T) {
	first := ""
	calls := 0
	s := New(func(c string) bool {
		calls++
		if first == "" {
			first = c
			return true
		}
		return false
	})
	if calls != 2 || s == first {
		t.Fatalf("New drew %q after refusing %q in %d calls", s, first, calls)
	}
}

func TestOfIsStableAndKeyed(t *testing.T) {
	a := Of([]byte("k"), "person", "pat@example.org")
	if a != Of([]byte("k"), "person", "pat@example.org") {
		t.Fatal("Of is not stable")
	}
	if a == Of([]byte("other"), "person", "pat@example.org") {
		t.Fatal("Of ignores its key")
	}
	if a == Of([]byte("k"), "family", "pat@example.org") {
		t.Fatal("Of ignores its kind")
	}
	if _, ok := Parse(a); !ok {
		t.Fatalf("Of() = %q, which does not parse", a)
	}
}

func TestParse(t *testing.T) {
	if got, ok := Parse(" K7M2Q9X4V1BNC "); !ok || got != "k7m2q9x4v1bnc" {
		t.Fatalf("Parse lowercases and trims: got %q %v", got, ok)
	}
	for _, bad := range []string{"", "k7m2q9x4v1bn", "k7m2q9x4v1bnci", "k7m2q9x4v1bnu", "k7m2q9x4v1bno"} {
		if _, ok := Parse(bad); ok {
			t.Fatalf("Parse(%q) accepted", bad)
		}
	}
}

func TestAliases(t *testing.T) {
	a, err := ParseAliases([]map[string]string{{"Alias": " P001 ", "ID": "PTY0000000001"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Resolve("p001"); got != "pty0000000001" {
		t.Fatalf("Resolve(p001) = %q", got)
	}
	if got := a.Resolve("other"); got != "other" {
		t.Fatalf("Resolve(other) = %q", got)
	}
	for _, rows := range [][]map[string]string{
		{{"Alias": "", "ID": "pty0000000001"}},
		{{"Alias": "x", "ID": "short"}},
		{{"Alias": "x", "ID": "pty0000000001"}, {"Alias": "X", "ID": "pty0000000002"}},
	} {
		if _, err := ParseAliases(rows); err == nil {
			t.Fatalf("ParseAliases(%v) accepted", rows)
		}
	}
}

func TestTokenUsesAlphabet(t *testing.T) {
	tok := Token()
	if len(tok) != tokenLength {
		t.Fatalf("Token() = %q", tok)
	}
	for _, r := range tok {
		if !strings.ContainsRune(Alphabet, r) {
			t.Fatalf("Token() = %q holds %q", tok, r)
		}
	}
}
