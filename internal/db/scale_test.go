package db

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
	s, err := NewStore(dir, dir, store.NewQueue(), NewSearchIndex())
	if err != nil {
		tb.Fatal(err)
	}
	return s
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
		`(from GROUP @g (where (sidebar_group @g) (manages @g)) (order name asc) (include parent parent.parent parent.parent.parent))`,
		`(from GROUP @g (where (sidebar_group @g) (not (manages @g)) (or (in id ` + mine + `) (in rsvp_yes ` + mine + `))) (order name asc) (include parent parent.parent parent.parent.parent))`,
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
	s := scaleStore(b)
	i := 0
	for b.Loop() {
		i++
		if err := commit(s, DocumentsSheet, store.Update("DOCUMENT", store.Row{"id": "doc00000100001"}, store.Row{"name": fmt.Sprintf("Mail %d", i)})); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRebuild(b *testing.B) {
	s := scaleStore(b)
	i := 0
	for b.Loop() {
		i++
		if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000150"}, store.Row{"facts": fmt.Sprintf("fact %d", i)})); err != nil {
			b.Fatal(err)
		}
	}
}
