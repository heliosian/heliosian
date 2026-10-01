package db

import (
	"strings"
	"testing"
)

func TestMintParses(t *testing.T) {
	for range 1000 {
		s := Mint(PersonPrefix, func(string) bool { return false })
		if prefix, ok := ParseID(s); !ok || prefix != PersonPrefix {
			t.Fatalf("Mint() = %q, which does not parse as a person id", s)
		}
	}
}

func TestMintUsesBothCases(t *testing.T) {
	seen := ""
	for range 200 {
		seen += Mint(GroupPrefix, func(string) bool { return false })[prefixLength:]
	}
	if strings.ToLower(seen) == seen || strings.ToUpper(seen) == seen {
		t.Fatalf("200 ids drew from one case only: %q", seen)
	}
}

func TestMintRedrawsTaken(t *testing.T) {
	first := ""
	calls := 0
	s := Mint(GroupPrefix, func(c string) bool {
		calls++
		if first == "" {
			first = c
			return true
		}
		return false
	})
	if calls != 2 || s == first {
		t.Fatalf("Mint drew %q after refusing %q in %d calls", s, first, calls)
	}
}

func TestMintRefusesUnknownPrefix(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Mint accepted an unknown prefix")
		}
	}()
	Mint("zzz", func(string) bool { return false })
}

func TestParseID(t *testing.T) {
	if prefix, ok := ParseID("grpX7pQ2m9KdLr"); !ok || prefix != GroupPrefix {
		t.Fatalf("ParseID(grpX7pQ2m9KdLr) = %q %v", prefix, ok)
	}
	for _, bad := range []string{"", "grpX7pQ2m9KdL", "grpX7pQ2m9KdLrr", " grpX7pQ2m9KdLr", "GRPX7pQ2m9KdLr", "zzzX7pQ2m9KdLr", "grpX7pQ2m9Kd-r"} {
		if _, ok := ParseID(bad); ok {
			t.Fatalf("ParseID(%q) accepted", bad)
		}
	}
}

func TestTableOf(t *testing.T) {
	for s, want := range map[string]string{
		"perX7pQ2m9KdLr": "PERSON",
		"grpX7pQ2m9KdLr": "GROUP",
		"tokX7pQ2m9KdLr": "RECIPIENT",
		"fedX7pQ2m9KdLr": "SAVED_VIEW",
		"bdyX7pQ2m9KdLr": "BIRTHDAY_YEAR",
	} {
		if got, ok := TableOf(s); !ok || got != want {
			t.Fatalf("TableOf(%s) = %q %v, want %s", s, got, ok, want)
		}
	}
	if got, ok := TableOf("purX7pQ2m9KdLr"); ok {
		t.Fatalf("TableOf(purchase) = %q, but no table is keyed by purchases", got)
	}
}
