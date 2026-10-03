package db

import (
	"testing"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

func changesOf(rows []store.Row, id string) []store.Row {
	out := []store.Row{}
	for _, row := range rows {
		if row["row"] == id {
			out = append(out, row)
		}
	}
	return out
}

func TestAChangeMayNameARowOfADroppedTable(t *testing.T) {
	changes, _ := Lookup(ChangesTable)
	row := map[string]string{"id": "chgX7pQ2m9KdLr", "at": "2026-09-24 16:00:05", "actor": "import", "action": "delete", "table": "GONE", "row": "gctyCb75KlEPU7"}
	if err := changes.Check(row); err != nil {
		t.Fatal(err)
	}
}

func TestAChangeToAHiddenRowIsHidden(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": staff}, store.Row{"pronouns": "she/her"})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": staff}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	if shown := changesOf(s.Model().Shown(ChangesTable).All(), staff); len(shown) != 0 {
		t.Errorf("changes to a withheld person shown: %v", shown)
	}
}

func TestEveryWriteIsAChange(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"title": "Autumn Picnic"})); err != nil {
		t.Fatal(err)
	}
	changes := changesOf(s.Model().Table(ChangesTable).All(), "grp00000000040")
	if len(changes) != 3 {
		t.Fatalf("the picnic's changes = %v", changes)
	}
	last := changes[2]
	if last["action"] != "set" || last["table"] != "GROUP" || last["column"] != "title" || last["previous"] != "Fall Picnic" || last["actor"] != "test" {
		t.Errorf("the change = %v", last)
	}
	if _, err := cells.When(last["at"]); err != nil || len(last["at"]) != len(cells.StampFormat) {
		t.Errorf("the change's time %q is not a moment to the second: %v", last["at"], err)
	}
	if prefix, ok := ParseID(last["id"]); !ok || prefix != ChangePrefix {
		t.Errorf("the change's id %q", last["id"])
	}
	if got := as(t, s, staff, `(from CHANGES (where (= row "grp00000000040")) (order at desc))`); len(got) != 3 || got[0]["column"] != "title" || got[0]["previous"] != "Fall Picnic" {
		t.Errorf("a super admin reads the picnic's history as %v", got)
	}
	if got := as(t, s, parent, `(from CHANGES)`); len(got) != 0 {
		t.Errorf("a parent reads %d changes", len(got))
	}
}

func TestAChangeToAPrivateColumnIsHidden(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": staff}, store.Row{"vc_phone": "650-555-0100", "pronouns": "she/her"})); err != nil {
		t.Fatal(err)
	}
	if all := changesOf(s.Model().Table(ChangesTable).All(), staff); len(all) != 2 {
		t.Fatalf("the changes recorded = %v", all)
	}
	shown := changesOf(s.Model().Shown(ChangesTable).All(), staff)
	if len(shown) != 1 || shown[0]["column"] != "pronouns" {
		t.Errorf("the changes shown = %v", shown)
	}
}
