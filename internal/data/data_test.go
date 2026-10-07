package data

import (
	"context"
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

func TestARaggedRowReadsItsMissingCellsBlank(t *testing.T) {
	d := dirWith(t, "Title,Image,Style\nSchool,a.png\nEvents\nChats,c.png,tiles\n")
	_, rows, err := d.Table("apps", "Categories")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0]["Image"] != "a.png" || rows[0]["Style"] != "" || rows[1]["Title"] != "Events" || rows[1]["Image"] != "" || rows[2]["Style"] != "tiles" {
		t.Fatalf("rows = %v", rows)
	}
}

func TestTabsReadsTablesWholeAndHeadersAlone(t *testing.T) {
	d := dirWith(t, categories)
	tabs, err := d.Tabs(context.Background(), "apps", []string{"Categories"}, []string{"Categories"})
	if err != nil {
		t.Fatalf("tabs: %v", err)
	}
	if got := tabs["Categories"]; len(got.Header) != 3 || got.Header[0] != "Title" {
		t.Errorf("header = %v", got.Header)
	}
	whole, err := d.Tabs(context.Background(), "apps", []string{"Categories"}, nil)
	if err != nil || len(whole["Categories"].Rows) != 3 {
		t.Errorf("rows = %v, %v", whole["Categories"].Rows, err)
	}
	if _, err := d.Tabs(context.Background(), "apps", []string{"Nope"}, nil); err == nil {
		t.Errorf("a missing tab read")
	}
}
