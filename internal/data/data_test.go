package data

import (
	"os"
	"path/filepath"
	"testing"
)

func dirWith(t *testing.T, csv string) *Dir {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps"), 0o755); err != nil {
		t.Fatalf("make app dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "apps", "Categories.csv"), []byte(csv), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return &Dir{Root: root}
}

const categories = "Title,Image,Style\nSchool,a.png,cards\nEvents,,tiles\nChats,c.png,tiles\n"

func titles(t *testing.T, d *Dir) []string {
	t.Helper()
	_, rows, err := d.Table("apps", "Categories")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row["Title"])
	}
	return out
}

func TestReorderMovesRowsAndKeepsTheirOtherCells(t *testing.T) {
	d := dirWith(t, categories)
	if err := d.Reorder("apps", "Categories", "Title", []string{"Chats", "School", "Events"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	got := titles(t, d)
	want := []string{"Chats", "School", "Events"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v, want %v", got, want)
		}
	}
	_, rows, _ := d.Table("apps", "Categories")
	if rows[0]["Image"] != "c.png" || rows[0]["Style"] != "tiles" {
		t.Errorf("a moved row lost its other cells: %v", rows[0])
	}
}

// The guard that matters: an order that is not a permutation must be refused
// outright rather than silently dropping or duplicating a row.
func TestReorderRefusesAnythingButAPermutation(t *testing.T) {
	cases := map[string][]string{
		"a missing row":   {"School", "Events"},
		"an unknown row":  {"School", "Events", "Nope"},
		"a duplicate row": {"School", "School", "Events"},
		"an extra row":    {"School", "Events", "Chats", "School"},
	}
	for name, keys := range cases {
		d := dirWith(t, categories)
		if err := d.Reorder("apps", "Categories", "Title", keys); err == nil {
			t.Errorf("%s was accepted", name)
		}
		got := titles(t, d)
		if len(got) != 3 || got[0] != "School" || got[2] != "Chats" {
			t.Errorf("%s: a refused reorder still changed the tab: %v", name, got)
		}
	}
}
