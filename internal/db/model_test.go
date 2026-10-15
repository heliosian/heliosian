package db

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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

func apply(t testing.TB, s *Store, queue *store.Queue, edits []Edit) []string {
	t.Helper()
	if len(edits) == 0 {
		return nil
	}
	ids, err := applyErr(s, queue, edits)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func applyErr(s *Store, queue *store.Queue, edits []Edit) ([]string, error) {
	return Write(context.Background(), s, queue, access.System(setupEnv.System), setupEnv, Batch{Batch: edits})
}

func memberRow(t testing.TB, s *Store, group, person string) string {
	t.Helper()
	row, ok := s.Model().Table("MEMBER").Find(group, person)
	if !ok {
		t.Fatalf("%s has no member row for %s", group, person)
	}
	return row["id"]
}

func TestSampleLoads(t *testing.T) {
	m := sample(t).Model()
	if n := m.Table("PERSON").Len(); n != 101 {
		t.Fatalf("PERSON has %d rows", n)
	}
	if n := len(m.Table("MEMBER").Referencing("group", "grp00000000040")); n != 3 {
		t.Fatalf("the picnic has %d member rows", n)
	}
	if n := len(m.Table("SAVED_SEARCH").Referencing("person", "per00000000002")); n != 1 {
		t.Fatalf("Rowan has %d saved searches", n)
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
	s, queue := sampleWithQueue(t)
	juni, _ := s.Model().Table("PERSON").Get("per00000000001")
	if juni["grade"] != "3" || juni["classroom"] != "grp00000000010" {
		t.Fatalf("Veracross's grade and classroom read %q %q", juni["grade"], juni["classroom"])
	}
	apply(t, s, queue, []Edit{
		{Set: "per00000000001", Cells: map[string]any{"grade_override": "4", "job_title_override": "Reader"}},
	})
	juni, _ = s.Model().Table("PERSON").Get("per00000000001")
	if juni["grade"] != "4" || juni["vc_grade"] != "3" || juni["job_title"] != "Reader" || juni["classroom"] != "grp00000000010" {
		t.Fatalf("after overrides: grade %q vc_grade %q job_title %q classroom %q", juni["grade"], juni["vc_grade"], juni["job_title"], juni["classroom"])
	}
}

func TestDocumentsKnowTheirSourceAndTerminal(t *testing.T) {
	rows := []store.Row{
		{"id": "mail", "kind": "mail"},
		{"id": "body", "parent": "mail", "relation": "part"},
		{"id": "bodyText", "parent": "body", "relation": "extract"},
		{"id": "pdf", "parent": "mail", "relation": "part", "filename": "rules.pdf"},
		{"id": "pages1", "parent": "pdf", "relation": "pages"},
		{"id": "pages1Text", "parent": "pages1", "relation": "extract"},
		{"id": "page", "parent": "body", "relation": "linked"},
		{"id": "pageText", "parent": "page", "relation": "extract"},
		{"id": "file", "kind": "file"},
		{"id": "filePages", "parent": "file", "relation": "pages"},
		{"id": "filePagesText", "parent": "filePages", "relation": "extract"},
	}
	documentGenerated(rows)
	for _, want := range []struct{ id, source, terminal string }{
		{"mail", "", ""},
		{"body", "", "mail"},
		{"bodyText", "body", "mail"},
		{"pages1", "pdf", "mail"},
		{"pages1Text", "pdf", "mail"},
		{"pageText", "page", "mail"},
		{"file", "", ""},
		{"filePagesText", "file", "file"},
	} {
		i := slices.IndexFunc(rows, func(r store.Row) bool { return r["id"] == want.id })
		if rows[i]["source"] != want.source || rows[i]["terminal"] != want.terminal {
			t.Errorf("%s: source %q terminal %q, want %q %q", want.id, rows[i]["source"], rows[i]["terminal"], want.source, want.terminal)
		}
	}
}

func TestGeneratedNames(t *testing.T) {
	s, queue := sampleWithQueue(t)
	people := s.Model().Table("PERSON")
	for id, want := range map[string]string{"per00000000001": "Juni Ashdown", "per00000000004": "Guest"} {
		row, _ := people.Get(id)
		if row["name_show"] != want {
			t.Fatalf("%s shows as %q, want %q", id, row["name_show"], want)
		}
	}
	apply(t, s, queue, []Edit{{Set: "per00000000001", Cells: map[string]any{"name_long_override": "June Ashdown"}}})
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
		edit Edit
		want string
	}{
		"missing person":   {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": "per99999999999", "member": "yes"}}, "names no row"},
		"wrong table":      {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": "grp00000000001", "member": "yes"}}, "not a PERSON id"},
		"duplicate member": {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": "per00000000002", "note": "again"}}, "two rows have the same group="},
		"duplicate slug":   {Edit{Set: "grp00000000011", Cells: map[string]any{"slug": "Hummingbirds"}}, `two rows have the slug "Hummingbirds"`},
		"second primary":   {Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"address": "ro@example.net", "person": "per00000000002", "primary": "Yes", "source": "manual"}}, "2 primary"},
		"no primary":       {Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"address": "juni@example.net", "person": "per00000000001", "source": "manual"}}, "0 primary"},
		"bad enum":         {Edit{Set: "grp00000000040", Cells: map[string]any{"status": "maybe"}}, "not one of"},
		"referenced alias": {Edit{Insert: "ALIAS", Row: map[string]any{"alias": "old", "target": "doc99999999999"}}, "names no row"},
		"still named":      {Edit{Delete: "per00000000004"}, "names no row"},
		"private property": {Edit{Insert: "RULE", Row: map[string]any{"group": "grp00000000030", "order": "j", "property": "vc_phone", "value": "555-0100"}}, "is private"},
		"guarded property": {Edit{Insert: "RULE", Row: map[string]any{"group": "grp00000000030", "order": "j", "property": "phone_consent", "value": "shared"}}, "not open to everyone"},
		"kind and parent":  {Edit{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "relation": "part", "parent": "doc00000000001"}}, "a kind and no parent"},
		"orphan part":      {Edit{Insert: "DOCUMENT", Row: map[string]any{"relation": "part"}}, "a kind and no parent"},
		"neither":          {Edit{Insert: "DOCUMENT", Row: map[string]any{}}, "a kind and no parent"},
	} {
		t.Run(name, func(t *testing.T) {
			s, queue := sampleWithQueue(t)
			_, err := applyErr(s, queue, []Edit{c.edit})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestALoadRefusesRowsWithoutTheirOwnID(t *testing.T) {
	for name, c := range map[string]struct {
		row  map[string]string
		want string
	}{
		"duplicate id": {map[string]string{"id": "grp00000000040", "kind": "group", "status": "open", "name": "Again", "posting": "members", "replying": "members"}, "two rows have the id"},
		"no id":        {map[string]string{"kind": "group", "status": "open", "name": "Nameless", "posting": "members", "replying": "members"}, "id is required"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS("../../sampledata")); err != nil {
				t.Fatal(err)
			}
			appendRows(t, filepath.Join(root, GroupsSheet, "GROUP.csv"), []map[string]string{c.row})
			dir := &data.Dir{Root: root}
			if _, err := NewStore(dir, dir, store.NewQueue(), NewSearchIndex()); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

func TestARefreshRefusesAHandEditThatLeavesADanglingReference(t *testing.T) {
	for name, edit := range map[string]func(dir *data.Dir) error{
		"a person still named deleted": func(dir *data.Dir) error {
			return dir.Delete(PeopleSheet, "PERSON", map[string]string{"id": "per00000000004"})
		},
		"a membership naming nobody added": func(dir *data.Dir) error {
			return dir.Insert(GroupsSheet, "MEMBER", []map[string]string{{"id": "mem00000000999", "group": "grp00000000040", "person": "per99999999999", "member": "yes"}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(root, os.DirFS("../../sampledata")); err != nil {
				t.Fatal(err)
			}
			dir := &data.Dir{Root: root}
			queue := store.NewQueue()
			s, err := NewStore(dir, dir, queue, NewSearchIndex())
			if err != nil {
				t.Fatal(err)
			}
			before := s.Model()
			if err := edit(dir); err != nil {
				t.Fatal(err)
			}
			queue.Refresh()
			queue.Flush()
			if s.Model() != before {
				t.Fatal("a refresh swapped in a model with a reference that names no row")
			}
		})
	}
}

func TestCommitAcrossSheets(t *testing.T) {
	s, queue := sampleWithQueue(t)
	sam := apply(t, s, queue, []Edit{
		{Insert: "PERSON", As: "sam", Row: map[string]any{"source": "manual", "name_long_override": "Sam Ortiz"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": "@sam", "member": "yes"}},
	})[0]
	if n := len(s.Model().Table("MEMBER").Referencing("person", sam)); n != 1 {
		t.Fatalf("Sam has %d member rows", n)
	}
}
