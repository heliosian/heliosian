package id

import "testing"

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
