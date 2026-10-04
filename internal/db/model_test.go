package db

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

func sampleWithQueue(t *testing.T) (*Store, *store.Queue) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := NewStore(dir, dir, queue, NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	return s, queue
}

func sample(t *testing.T) *Store {
	t.Helper()
	s, _ := sampleWithQueue(t)
	return s
}

func commit(s *Store, sheet string, ops ...store.Op) error {
	return s.Commit(context.Background(), access.System("test"), sheet, ops...)
}

func TestSampleLoads(t *testing.T) {
	m := sample(t).Model()
	if n := m.Table("PERSON").Len(); n != 4 {
		t.Fatalf("PERSON has %d rows", n)
	}
	if n := len(m.Table("MEMBER").Referencing("group", "grp00000000040")); n != 3 {
		t.Fatalf("the picnic has %d member rows", n)
	}
	if n := len(m.Table("COLLECTION_GROUP").Referencing("group", "grp00000000010")); n != 1 {
		t.Fatalf("%d collections name the Hummingbirds", n)
	}
	if row, ok := m.Table("MEMBER").Find("grp00000000020", "per00000000002"); !ok || row["member"] != "yes" || row["id"] != "mem00000000010" {
		t.Fatalf("Rowan's row in the Ashdowns is %v", row)
	}
	if g, _ := m.Table("GROUP").Get("grp00000000020"); g["managed_by"] != "grp00000000020" {
		t.Fatalf("the Ashdowns are managed by %q", g["managed_by"])
	}
	if !m.Has("grp00000000030") || m.Has("grp99999999999") {
		t.Fatal("Has is wrong")
	}
}

func TestGeneratedSchoolFieldsTakeTheOverride(t *testing.T) {
	s := sample(t)
	juni, _ := s.Model().Table("PERSON").Get("per00000000001")
	if juni["grade"] != "3" || juni["classroom"] != "grp00000000010" {
		t.Fatalf("Veracross's grade and classroom read %q %q", juni["grade"], juni["classroom"])
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000001"}, store.Row{"grade_override": "4", "job_title_override": "Reader"})); err != nil {
		t.Fatal(err)
	}
	juni, _ = s.Model().Table("PERSON").Get("per00000000001")
	if juni["grade"] != "4" || juni["vc_grade"] != "3" || juni["job_title"] != "Reader" || juni["classroom"] != "grp00000000010" {
		t.Fatalf("after overrides: grade %q vc_grade %q job_title %q classroom %q", juni["grade"], juni["vc_grade"], juni["job_title"], juni["classroom"])
	}
}

func TestGeneratedNames(t *testing.T) {
	s := sample(t)
	people := s.Model().Table("PERSON")
	for id, want := range map[string]string{"per00000000001": "Juni Ashdown", "per00000000004": "Guest"} {
		row, _ := people.Get(id)
		if row["name_show"] != want {
			t.Fatalf("%s shows as %q, want %q", id, row["name_show"], want)
		}
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000001"}, store.Row{"name_long_override": "June Ashdown"})); err != nil {
		t.Fatal(err)
	}
	row, _ := s.Model().Table("PERSON").Get("per00000000001")
	if row["name_long"] != "June Ashdown" || row["name_short"] != "Juni" {
		t.Fatalf("after an override: long %q short %q", row["name_long"], row["name_short"])
	}
	if row, _ := people.Get("per00000000001"); row["name_long"] != "Juni Ashdown" {
		t.Fatalf("the earlier model changed: %q", row["name_long"])
	}
}

func TestCommitsTheModelRefuses(t *testing.T) {
	for name, c := range map[string]struct {
		sheet string
		op    store.Op
		want  string
	}{
		"missing person":   {GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000099", "group": "grp00000000040", "person": "per99999999999", "member": "yes"}), "names no row"},
		"wrong table":      {GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000099", "group": "grp00000000040", "person": "grp00000000001", "member": "yes"}), "not a PERSON id"},
		"duplicate member": {GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000099", "group": "grp00000000040", "person": "per00000000002", "note": "again"}), "two rows have the same group="},
		"duplicate id":     {GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000001", "group": "grp00000000040", "person": "per00000000001", "member": "yes"}), "two rows have the id"},
		"no id":            {GroupsSheet, store.Insert("MEMBER", store.Row{"group": "grp00000000040", "person": "per00000000001", "member": "yes"}), "id is required"}, "second primary": {PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "ro@example.net", "person": "per00000000002", "primary": "Yes", "source": "manual"}), "2 primary"},
		"no primary":       {PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "juni@example.net", "person": "per00000000001", "source": "manual"}), "0 primary"},
		"bad enum":         {GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"status": "maybe"}), "not one of"},
		"referenced alias": {ConfigSheet, store.Insert("ALIAS", store.Row{"id": "als00000000099", "alias": "old", "target": "doc99999999999"}), "names no row"},
		"still named":      {PeopleSheet, store.Delete("PERSON", store.Row{"id": "per00000000004"}), "names no row"},
		"private property": {GroupsSheet, store.Insert("RULE", store.Row{"id": "rul00000000099", "group": "grp00000000030", "order": "j", "property": "vc_phone", "value": "555-0100"}), "is private"},
		"guarded property": {GroupsSheet, store.Insert("RULE", store.Row{"id": "rul00000000099", "group": "grp00000000030", "order": "j", "property": "phone_consent", "value": "shared"}), "not open to everyone"},
		"kind and parent":  {DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000099", "kind": "newsletter", "relation": "attachment", "parent": "doc00000000001"}), "a kind and no parent"},
		"orphan part":      {DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000099", "relation": "attachment"}), "a kind and no parent"},
		"neither":          {DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000099"}), "a kind and no parent"},
	} {
		t.Run(name, func(t *testing.T) {
			err := commit(sample(t), c.sheet, c.op)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestCommitAcrossSheets(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Insert("PERSON", store.Row{"id": "per00000000005", "source": "manual", "name_long_override": "Sam Ortiz"})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000099", "group": "grp00000000040", "person": "per00000000005", "member": "yes"})); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Model().Table("MEMBER").Referencing("person", "per00000000005")); n != 1 {
		t.Fatalf("Sam has %d member rows", n)
	}
}
