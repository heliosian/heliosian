package db

import (
	"strings"
	"testing"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestEveryWriteAppendsAChangeToTheSheet(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := NewStore(dir, dir, queue, NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"name": "Autumn Picnic"})); err != nil {
		t.Fatal(err)
	}
	changes := []store.Row{}
	for _, row := range testkit.Rows(t, dir, queue, GroupsSheet, ChangesTab) {
		if row["row"] == "grp00000000040" {
			changes = append(changes, row)
		}
	}
	if len(changes) != 3 {
		t.Fatalf("the picnic's changes = %v", changes)
	}
	last := changes[2]
	if last["action"] != "set" || last["table"] != "GROUP" || last["column"] != "name" || last["previous"] != "Fall Picnic" || last["actor"] != "test" {
		t.Errorf("the change = %v", last)
	}
	if _, err := cells.When(last["at"]); err != nil || len(last["at"]) != len(cells.StampFormat) {
		t.Errorf("the change's time %q is not a moment to the second: %v", last["at"], err)
	}
	if prefix, ok := ParseID(last["id"]); !ok || prefix != ChangePrefix {
		t.Errorf("the change's id %q", last["id"])
	}
}

func TestHistoryIsNeverRead(t *testing.T) {
	if _, ok := Lookup(ChangesTab); ok {
		t.Fatal("CHANGES is a table of the model")
	}
	if _, err := Parse(`(from CHANGES)`); err == nil || !strings.Contains(err.Error(), "CHANGES") {
		t.Fatalf("a query of CHANGES: %v", err)
	}
}
