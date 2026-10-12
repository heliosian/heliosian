package db

import (
	"encoding/csv"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

const (
	scaleFamilies = 400
	scaleEvents   = 600
	scaleMails    = 2000
	scaleViewer   = "per00000000101"
)

func appendRows(tb testing.TB, path string, rows []map[string]string) {
	tb.Helper()
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	header, err := csv.NewReader(f).Read()
	f.Close()
	if err != nil {
		tb.Fatal(err)
	}
	out, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		tb.Fatal(err)
	}
	defer out.Close()
	w := csv.NewWriter(out)
	for _, row := range rows {
		rec := make([]string, len(header))
		for i, h := range header {
			rec[i] = row[h]
		}
		if err := w.Write(rec); err != nil {
			tb.Fatal(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		tb.Fatal(err)
	}
}

func scaleStore(tb testing.TB) *Store {
	tb.Helper()
	s, _, _ := scaleStoreIn(tb)
	return s
}

func scaleStoreIn(tb testing.TB) (*Store, *store.Queue, *data.Dir) {
	tb.Helper()
	root := tb.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../sampledata")); err != nil {
		tb.Fatal(err)
	}
	people, emails, groups, members := []map[string]string{}, []map[string]string{}, []map[string]string{}, []map[string]string{}
	n, m := 100, 100
	for f := range scaleFamilies {
		family := fmt.Sprintf("grp%011d", 1000+f)
		groups = append(groups, map[string]string{"id": family, "kind": "family", "name": fmt.Sprintf("Family %d", f), "status": "open", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "managed_by": family, "vc_address": fmt.Sprintf("%d Main St", f), "consent": "listed", "address_consent": "shared", "phone_consent": "shared", "posting": "members", "replying": "members"})
		for k := range 4 {
			n++
			id := fmt.Sprintf("per%011d", n)
			people = append(people, map[string]string{"id": id, "source": "veracross", "vc_name": fmt.Sprintf("Person %d", n), "vc_name_long": fmt.Sprintf("Person %d", n), "vc_name_short": fmt.Sprintf("P%d", n), "vc_name_sort": fmt.Sprintf("%d, Person", n), "consent": "listed", "address_consent": "shared", "phone_consent": "shared", "vc_phone": "555-0100"})
			emails = append(emails, map[string]string{"id": fmt.Sprintf("eml%011d", n), "address": fmt.Sprintf("p%d@example.org", n), "person": id, "primary": "Yes", "source": "veracross"})
			roleGroup := "grp00000000002"
			if k >= 2 {
				roleGroup = "grp00000000001"
			}
			m++
			members = append(members, map[string]string{"id": fmt.Sprintf("mem%011d", m), "group": family, "person": id, "member": "yes"})
			m++
			members = append(members, map[string]string{"id": fmt.Sprintf("mem%011d", m), "group": roleGroup, "person": id, "member": "yes"})
		}
	}
	for e := range scaleEvents {
		event, managers, going, notGoing := fmt.Sprintf("grp%011d", 5000+4*e), fmt.Sprintf("grp%011d", 5001+4*e), fmt.Sprintf("grp%011d", 5002+4*e), fmt.Sprintf("grp%011d", 5003+4*e)
		open := func(id, kind, name string, cells map[string]string) map[string]string {
			row := map[string]string{"id": id, "kind": kind, "name": name, "status": "open", "visible_to": "grp00000000004", "posting": "members", "replying": "members"}
			for k, v := range cells {
				row[k] = v
			}
			return row
		}
		groups = append(groups,
			open(event, "event", fmt.Sprintf("Event %d", e), map[string]string{"managed_by": managers, "rsvp_yes": going, "rsvp_no": notGoing, "start": "2026-10-20 18:00"}),
			open(managers, "group", fmt.Sprintf("Event %d Managers", e), map[string]string{"managed_by": managers}),
			open(going, "group", fmt.Sprintf("Event %d Going", e), map[string]string{"parent": event}),
			open(notGoing, "group", fmt.Sprintf("Event %d Not Going", e), map[string]string{"parent": event}))
		m++
		members = append(members, map[string]string{"id": fmt.Sprintf("mem%011d", m), "group": going, "person": fmt.Sprintf("per%011d", 101+e%(4*scaleFamilies)), "member": "yes"})
	}
	appendRows(tb, filepath.Join(root, "datapeople", "PERSON.csv"), people)
	appendRows(tb, filepath.Join(root, "datapeople", "PERSON_EMAIL.csv"), emails)
	appendRows(tb, filepath.Join(root, "datagroups", "GROUP.csv"), groups)
	appendRows(tb, filepath.Join(root, "datagroups", "MEMBER.csv"), members)
	documents, sentTo := []map[string]string{}, []map[string]string{}
	d := 100000
	for i := range scaleMails {
		d++
		mail := fmt.Sprintf("doc%011d", d)
		documents = append(documents, map[string]string{"id": mail, "kind": "mail", "name": fmt.Sprintf("Mail %d", i), "published": "2026-09-01 09:00:00"})
		sentTo = append(sentTo, map[string]string{"id": fmt.Sprintf("dgr%011d", d), "document": mail, "group": "grp00000001001", "relation": "sent_to"})
		for range 4 {
			d++
			documents = append(documents, map[string]string{"id": fmt.Sprintf("doc%011d", d), "parent": mail, "relation": "part"})
		}
	}
	appendRows(tb, filepath.Join(root, "datadocuments", "DOCUMENT.csv"), documents)
	appendRows(tb, filepath.Join(root, "datadocuments", "DOCUMENT_GROUP.csv"), sentTo)
	dir := &data.Dir{Root: root}
	queue := store.NewQueue()
	s, err := NewStore(dir, dir, queue, NewSearchIndex())
	if err != nil {
		tb.Fatal(err)
	}
	return s, queue, dir
}

func benchQuery(b *testing.B, src string) {
	m := scaleStore(b).Model()
	q, err := Parse(src)
	if err != nil {
		b.Fatal(err)
	}
	env := Env{Viewer: scaleViewer, Now: testNow}
	for b.Loop() {
		m.Run(b.Context(), q, env)
	}
}

func BenchmarkEveryPerson(b *testing.B) {
	benchQuery(b, `(from PERSON)`)
}

func BenchmarkEveryMember(b *testing.B) {
	benchQuery(b, `(from MEMBER (include person group))`)
}

func BenchmarkOneFamily(b *testing.B) {
	benchQuery(b, `(from MEMBER (where (= group "grp00000001000")) (include person))`)
}

func BenchmarkOnePerson(b *testing.B) {
	benchQuery(b, `(from PERSON (where (= id "per00000000150")))`)
}

func BenchmarkWikiPages(b *testing.B) {
	benchQuery(b, `(from DOCUMENT (where (= kind "wiki")))`)
}

func BenchmarkWikiSides(b *testing.B) {
	benchQuery(b, `(from DOCUMENT (where (= relation "side")))`)
}

func BenchmarkSidebar(b *testing.B) {
	m := scaleStore(b).Model()
	mine := `(select EFFECTIVE_MEMBER.group (= person @viewer))`
	queries := []*Query{}
	for _, src := range []string{
		`(from GROUP @g (where (manages @g) (sidebar_group @g)) (order name asc) (include parent parent.parent parent.parent.parent))`,
		`(from GROUP @g (where (or (in id ` + mine + `) (in rsvp_yes ` + mine + `)) (not (manages @g)) (sidebar_group @g)) (order name asc) (include parent parent.parent parent.parent.parent))`,
	} {
		q, err := Parse(src)
		if err != nil {
			b.Fatal(err)
		}
		queries = append(queries, q)
	}
	env := Env{Viewer: scaleViewer, Now: testNow}
	for b.Loop() {
		m.RunAll(b.Context(), queries, env)
	}
}

func BenchmarkRebuildDocuments(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "doc00000100001", Cells: map[string]any{"name": fmt.Sprintf("Mail %d", i)}}}
	})
}

func TestAnIncrementalRefreshMatchesAFullLoad(t *testing.T) {
	s, queue, dir := scaleStoreIn(t)
	apply(t, s, queue, []Edit{{Set: "per00000000150", Cells: map[string]any{"facts": "committed before the refresh"}}})
	queue.Flush()
	if err := dir.Insert(DocumentsSheet, "DOCUMENT", []map[string]string{{"id": "doc00000900001", "kind": "mail", "name": "Added by hand", "published": "2026-09-02 09:00:00"}}); err != nil {
		t.Fatal(err)
	}
	if err := dir.Delete(GroupsSheet, "MEMBER", map[string]string{"id": "mem00000000101"}); err != nil {
		t.Fatal(err)
	}
	if err := dir.Update(GroupsSheet, "GROUP", map[string]string{"id": "grp00000005000"}, map[string]string{"name": "Renamed by hand"}); err != nil {
		t.Fatal(err)
	}
	before := s.Model()
	queue.Refresh()
	queue.Flush()
	after := s.Model()
	if after.People["PERSON"] != before.People["PERSON"] {
		t.Error("the refresh built the People sheet again, though nothing in it was edited by hand")
	}
	if after.Groups["GROUP"] == before.Groups["GROUP"] {
		t.Error("the refresh kept the Groups sheet a hand edit changed")
	}
	fresh, err := NewStore(dir, dir, store.NewQueue(), NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	want := fresh.Model()
	same := func(table string, got, wanted []store.Row) {
		t.Helper()
		if !slices.EqualFunc(got, wanted, func(a, b store.Row) bool { return maps.Equal(a, b) }) {
			t.Errorf("%s differs from a full load: %d rows against %d", table, len(got), len(wanted))
		}
	}
	for i := range Tables {
		table := &Tables[i]
		if table.Generated {
			continue
		}
		same(table.Name, after.Table(table.Name).All(), want.Table(table.Name).All())
		same(table.Name+" shown", after.Shown(table.Name).All(), want.Shown(table.Name).All())
	}
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	same("EFFECTIVE_MEMBER", after.generated(effective).rows, want.generated(effective).rows)
}

func benchWrite(b *testing.B, edits func(i int) []Edit) {
	s, queue, _ := scaleStoreIn(b)
	i := 0
	for b.Loop() {
		i++
		if _, err := Write(b.Context(), s, queue, access.System(setupEnv.System), setupEnv, Batch{Batch: edits(i)}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWritePerson(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "per00000000150", Cells: map[string]any{"facts": fmt.Sprintf("fact %d", i)}}}
	})
}

func BenchmarkWriteAnswer(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "mem00000003301", Cells: map[string]any{"member": []string{"yes", "cancelled"}[i%2]}}}
	})
}

func BenchmarkWriteEventName(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "grp00000005000", Cells: map[string]any{"name": fmt.Sprintf("Event 0 take %d", i)}}}
	})
}

func BenchmarkWriteMailName(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "doc00000100001", Cells: map[string]any{"name": fmt.Sprintf("Mail %d", i)}}}
	})
}

func BenchmarkSearchFullPass(b *testing.B) {
	m := scaleStore(b).Model()
	for b.Loop() {
		m.SearchInputs(map[string]string{})
	}
}

func BenchmarkSearchEventRenamed(b *testing.B) {
	m := scaleStore(b).Model()
	event, _ := m.Table("GROUP").Get("grp00000005000")
	c := Change{Table: "GROUP", Old: event, New: event}
	for b.Loop() {
		excluded := m.searchExcluded()
		for _, id := range m.searchReach(c) {
			m.searchRowOf(id, map[string]string{}, excluded)
		}
	}
}

func BenchmarkEffectiveAfterAnswer(b *testing.B) {
	s, queue, _ := scaleStoreIn(b)
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	s.Model().generated(effective)
	i := 0
	for b.Loop() {
		i++
		if _, err := Write(b.Context(), s, queue, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Set: "mem00000003301", Cells: map[string]any{"member": []string{"yes", "cancelled"}[i%2]}}}}); err != nil {
			b.Fatal(err)
		}
		s.Model().generated(effective)
	}
}

func BenchmarkRefreshUnchanged(b *testing.B) {
	_, queue, _ := scaleStoreIn(b)
	for b.Loop() {
		queue.Refresh()
		queue.Flush()
	}
}

func BenchmarkRebuild(b *testing.B) {
	benchWrite(b, func(i int) []Edit {
		return []Edit{{Set: "per00000000150", Cells: map[string]any{"facts": fmt.Sprintf("fact %d", i)}}}
	})
}
