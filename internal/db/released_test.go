package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReleasedQueriesParse(t *testing.T) {
	files, err := filepath.Glob("testdata/released/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no released queries in testdata/released")
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		queries := map[string]string{}
		if err := json.Unmarshal(raw, &queries); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		version := strings.TrimSuffix(filepath.Base(file), ".json")
		names := []string{}
		for name := range queries {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if _, err := Parse(queries[name]); err != nil {
				t.Errorf("version %s, query %s: %v\n%s", version, name, err, queries[name])
			}
		}
	}
}
