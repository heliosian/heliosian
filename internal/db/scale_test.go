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
		groups = append(groups, map[string]string{"id": family, "kind": "family", "title": fmt.Sprintf("Family %d", f), "status": "open", "visibility": "everyone", "vc_address": fmt.Sprintf("%d Main St", f), "consent": "listed", "address_consent": "shared", "phone_consent": "shared"})
		for k := range 4 {
			n++
			id := fmt.Sprintf("per%011d", n)
			people = append(people, map[string]string{"id": id, "source": "veracross", "vc_name": fmt.Sprintf("Person %d", n), "name_long_import": fmt.Sprintf("Person %d", n), "name_short_import": fmt.Sprintf("P%d", n), "name_sort_import": fmt.Sprintf("%d, Person", n), "consent": "listed", "address_consent": "shared", "phone_consent": "shared", "vc_phone": "555-0100"})
			emails = append(emails, map[string]string{"id": fmt.Sprintf("eml%011d", n), "address": fmt.Sprintf("p%d@example.org", n), "person": id, "primary": "Yes", "source": "veracross"})
			manager, roleGroup := "Yes", "grp00000000002"
			if k >= 2 {
				manager, roleGroup = "", "grp00000000001"
			}
			m++
			members = append(members, map[string]string{"id": fmt.Sprintf("mem%011d", m), "group": family, "person": id, "manager": manager, "member": "yes"})
			m++
			members = append(members, map[string]string{"id": fmt.Sprintf("mem%011d", m), "group": roleGroup, "person": id, "member": "yes"})
		}
	}
	appendRows(tb, filepath.Join(root, "datapeople", "PERSON.csv"), people)
	appendRows(tb, filepath.Join(root, "datapeople", "PERSON_EMAIL.csv"), emails)
	appendRows(tb, filepath.Join(root, "datagroups", "GROUP.csv"), groups)
	appendRows(tb, filepath.Join(root, "datagroups", "MEMBER.csv"), members)
	dir := &data.Dir{Root: root}
	s, err := NewStore(dir, dir, store.NewQueue())
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
		m.Run(q, env)
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
