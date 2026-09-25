package store

import (
	"slices"
	"testing"
)

func sorted(t *testing.T, keys []string) {
	t.Helper()
	for i, key := range keys {
		if err := CheckKey(key); key == "" || err != nil {
			t.Fatalf("key %d of %q: %v", i, keys, err)
		}
		if i > 0 && keys[i-1] >= key {
			t.Fatalf("keys out of order: %q", keys)
		}
	}
}

func TestOrderKeysEveryRow(t *testing.T) {
	for _, n := range []int{1, 2, 7, 40, 500} {
		keys := Order(make([]string, n))
		sorted(t, keys)
		if n <= 40 && len(slices.MaxFunc(keys, func(a, b string) int { return len(a) - len(b) })) > 2 {
			t.Errorf("%d rows got keys as long as %q", n, keys)
		}
	}
}

func TestOrderKeepsEveryKeyItCan(t *testing.T) {
	got := Order([]string{"2", "4", "k", "6", ""})
	sorted(t, got)
	if got[0] != "2" || got[1] != "4" || got[2] != "k" || got[3] == "6" {
		t.Fatalf("moving one row rewrote others: %q", got)
	}
	got = Order([]string{"b", "a"})
	sorted(t, got)
	if got[0] != "b" && got[1] != "a" {
		t.Fatalf("a swap kept neither key: %q", got)
	}
}

func TestOrderAlwaysFindsRoom(t *testing.T) {
	keys := []string{"1", "2"}
	for range 200 {
		keys = Order([]string{keys[0], "", keys[1]})
		sorted(t, keys)
		keys = keys[:2]
	}
	for range 200 {
		keys = Order([]string{"", keys[0], keys[1]})
		sorted(t, keys)
		keys = keys[:2]
	}
}

func TestCheckKey(t *testing.T) {
	for _, bad := range []string{"A", "a-b", "10", " 1"} {
		if CheckKey(bad) == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if CheckKey("1") != nil || CheckKey("") != nil {
		t.Error("a good key failed")
	}
}

func TestCompareKeysPutsBlankLast(t *testing.T) {
	keys := []string{"", "b", "", "a"}
	slices.SortStableFunc(keys, CompareKeys)
	if !slices.Equal(keys, []string{"a", "b", "", ""}) {
		t.Fatalf("%q", keys)
	}
}
