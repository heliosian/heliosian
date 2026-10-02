package main

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/qclient"
	"heliosian/internal/store"
)

func TestFlattenBioSeparatesParagraphsAndDecodesEntities(t *testing.T) {
	bio, err := flattenBio("<p>She holds a Bachelor&#39;s degree.</p>\n<p>Outside work, she\n  surfs.</p>")
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	want := "She holds a Bachelor's degree.\n\nOutside work, she surfs."
	if bio != want {
		t.Errorf("bio = %q, want %q", bio, want)
	}
}

func TestFlattenBioKeepsLinkTextAndDropsMarkup(t *testing.T) {
	bio, err := flattenBio(`<p>He works at <a href="https://example.org/">the lab</a>.<br>Ask him about it.</p>`)
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	want := "He works at the lab.\nAsk him about it."
	if bio != want {
		t.Errorf("bio = %q, want %q", bio, want)
	}
}

func TestFlattenBioOfNothingIsEmpty(t *testing.T) {
	bio, err := flattenBio("")
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if bio != "" {
		t.Errorf("bio = %q, want empty", bio)
	}
}

func TestParseName(t *testing.T) {
	for raw, want := range map[string]names{
		"Juni (Juniper)  Ashdown": {long: "Juni Ashdown", legal: "Juniper Ashdown", short: "Juni", sort: "Ashdown, Juni"},
		"Rowan Ashdown":           {long: "Rowan Ashdown", legal: "Rowan Ashdown", short: "Rowan", sort: "Ashdown, Rowan"},
		"Maya Ruth Lindqvist":     {long: "Maya Ruth Lindqvist", legal: "Maya Ruth Lindqvist", short: "Maya", sort: "Lindqvist, Maya Ruth"},
		"Cher":                    {long: "Cher", legal: "Cher", short: "Cher", sort: "Cher"},
	} {
		if got := parseName(raw); got != want {
			t.Errorf("%q: %+v, want %+v", raw, got, want)
		}
	}
}

func TestEmailsSkipPlaceholders(t *testing.T) {
	if got := emails(" Juni@Example.org ", "juni@example.org", "x.noemail@example.org", ""); !slices.Equal(got, []string{"juni@example.org"}) {
		t.Errorf("emails = %v", got)
	}
}

const testKey = "test-import-key"

func sampleServer(t *testing.T) (client, *db.Store) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := db.NewStore(dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	db.Register(mux, s, queue, db.NewPictures(s, queue, blob.NewMemoryBucket()), []byte(testKey), func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client{qclient.Client{Base: srv.URL, Key: testKey}}, s
}

func writePNG(t *testing.T, dir string, size int) string {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	name := blob.Name(buf.Bytes(), "png")
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

func sampleExport(t *testing.T, withMaya bool) *export {
	t.Helper()
	dir := t.TempDir()
	x := &export{photoDir: dir, entries: []entry{
		{role: student, name: "Juni (Juniper) Ashdown", grade: "3", classroom: "Hummingbirds", photos: []string{writePNG(t, dir, 2)}},
		{role: parent, name: "Rowan Ashdown", emails: []string{"rowan.ashdown@example.org"}, phone: "555-0100"},
		{role: parent, name: "Sam (Samuel) Ashdown", emails: []string{"sam@example.org"}},
		{role: student, name: "Wren (Wrennie) Ashdown", grade: "K", classroom: "Oak", crew: "Acorn"},
		{role: parent, name: "Rowan Ashdown", emails: []string{"rowan.ashdown@example.org"}, phone: "555-0100"},
		{role: parent, name: "Sam (Samuel) Ashdown", emails: []string{"sam@example.org"}},
		{role: staff, name: "Ines Okafor", emails: []string{"ines@example.org"}, jobTitle: "Librarian", department: "Specials", photos: []string{writePNG(t, dir, 3)}},
		{role: staff, name: "Sam Ashdown", emails: []string{"sam@example.org"}, jobTitle: "Coach"},
	}}
	x.households = []householdRow{
		{adults: []int{1, 2}, kid: 0, address: "12 Elm St"},
		{adults: []int{4, 5}, kid: 3, address: "12 Elm St"},
	}
	x.website = []websiteRow{
		{name: "Ines Okafor", bio: "Reads.", photos: []string{writePNG(t, dir, 4)}},
		{name: "Maya Lindqvist-Berg", emails: []string{"maya@example.com"}, bio: "Teaches."},
		{name: "Nobody Here", emails: []string{"nobody@example.org"}, bio: "Gone."},
	}
	if withMaya {
		x.entries = append(x.entries, entry{role: staff, name: "Maya Lindqvist", emails: []string{"maya.lindqvist@example.org"}, jobTitle: "Teacher"})
	}
	return x
}

func TestOneNameTwoPeople(t *testing.T) {
	for _, kidEmails := range [][]string{{"marco.jr@example.org"}, {}} {
		c, s := sampleServer(t)
		x := sampleExport(t, true)
		x.households = append(x.households, householdRow{adults: []int{len(x.entries) + 1}, kid: len(x.entries)})
		x.entries = append(x.entries,
			entry{role: student, name: "Marco Mena", emails: kidEmails, grade: "K", classroom: "Oak"},
			entry{role: parent, name: "Marco Mena", emails: []string{"marco@example.org"}},
		)
		importOnce(t, c, x)
		marcos := 0
		for _, r := range s.Model().Table("PERSON").All() {
			if r["vc_name"] == "Marco Mena" {
				marcos++
			}
		}
		if marcos != 2 {
			t.Fatalf("with the student's emails %v, %d Marco Menas", kidEmails, marcos)
		}
		if p, _ := importOnce(t, c, x); len(p.batch) != 0 {
			t.Fatalf("with the student's emails %v, a second run changed %v", kidEmails, p.counts)
		}
	}
	x := sampleExport(t, true)
	x.entries = append(x.entries,
		entry{role: parent, name: "Lee Park", emails: []string{"lee@example.org"}},
		entry{role: staff, name: "Lee Park"},
	)
	x.households = append(x.households, householdRow{adults: []int{len(x.entries) - 2}, kid: 0})
	c, _ := sampleServer(t)
	st, err := c.read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan(x, st); err == nil || !strings.Contains(err.Error(), "one has no email") {
		t.Fatalf("a parent and a staff member of one name, one without an email: %v", err)
	}
}

func importOnce(t *testing.T, c client, x *export) (*planner, int) {
	t.Helper()
	p, err := planned(c, x)
	if err != nil {
		t.Fatal(err)
	}
	added, err := apply(c, x, p)
	if err != nil {
		t.Fatal(err)
	}
	return p, added
}

func photosOf(s *db.Store, person string) []map[string]string {
	rows := s.Model().Table("PHOTO").Referencing("person", person)
	slices.SortFunc(rows, func(a, b map[string]string) int { return store.CompareKeys(a["order"], b["order"]) })
	return rows
}

func personNamed(s *db.Store, name string) map[string]string {
	for _, r := range s.Model().Table("PERSON").All() {
		if r["vc_name"] == name {
			return r
		}
	}
	return nil
}

func groupsOf(s *db.Store, person string) []string {
	m := s.Model()
	out := []string{}
	for _, r := range m.Table("MEMBER").Referencing("person", person) {
		g, _ := m.Table("GROUP").Get(r["group"])
		out = append(out, g["kind"]+":"+g["title"]+g["slug"]+":"+r["role"])
	}
	slices.Sort(out)
	return out
}

func TestImportAgainstTheSample(t *testing.T) {
	c, s := sampleServer(t)
	x := sampleExport(t, true)
	p, photos := importOnce(t, c, x)
	if p.counts["people added"] != 3 || p.counts["people deactivated"] != 0 || photos != 3 {
		t.Fatalf("the first run: %v, %d photos", p.counts, photos)
	}
	ines := personNamed(s, "Ines Okafor")
	if got := photosOf(s, ines["id"]); len(got) != 2 || got[0]["photo"] != x.entries[6].photos[0] || got[1]["photo"] != x.website[0].photos[0] {
		t.Fatalf("Ines's photos, Veracross's first: %v", got)
	}
	if ines["vc_bio"] != "Reads." {
		t.Fatalf("Ines's bio, matched to the staff page by name: %v", ines)
	}
	if got := groupsOf(s, ines["id"]); !slices.Contains(got, "department:Specials:member") {
		t.Fatalf("Ines, in Veracross's Specials department, is in %v", got)
	}
	if maya, _ := s.Model().Table("PERSON").Get("per00000000003"); maya["vc_bio"] != "Teaches." {
		t.Fatalf("Maya's bio, matched to the staff page by another email on file: %v", maya)
	}
	if juni := personNamed(s, "Juni (Juniper) Ashdown"); juni["id"] != "per00000000001" {
		t.Fatalf("Juni was not matched by name: %v", juni)
	}
	wren := personNamed(s, "Wren (Wrennie) Ashdown")
	if wren == nil || wren["name_sort_import"] != "Ashdown, Wren" || wren["vc_grade"] != "K" {
		t.Fatalf("Wren reads %v", wren)
	}
	want := []string{"classroom:Oak:member", "crew:Acorn:member", "family:Ashdown Family:member", "grade:Kindergartengrade-k:member", "group:Everyoneeveryone:member", "group:Studentsstudents:member"}
	if got := groupsOf(s, wren["id"]); !slices.Equal(got, want) {
		t.Fatalf("Wren is in %v", got)
	}
	groups := s.Model().Table("GROUP")
	for classroom, title := range map[string]string{"grp00000000010": "Jayvens", wren["vc_classroom"]: "Hummingbirds"} {
		c, _ := groups.Get(classroom)
		band, _ := groups.Get(c["parent"])
		if band["kind"] != "band" || band["title"] != title {
			t.Fatalf("classroom %s sits under %v, not the %s band", c["title"], band, title)
		}
		rules := s.Model().Table("RULE").Referencing("group", band["id"])
		if len(rules) != 1 || rules[0]["target"] != band["id"] || rules[0]["descend"] != "Yes" {
			t.Fatalf("the %s band's rules: %v", title, rules)
		}
	}
	x.entries = append(x.entries, entry{role: student, name: "Ada Ashdown", grade: "4", classroom: "Oak"})
	x.households = append(x.households, householdRow{adults: []int{1, 2}, kid: len(x.entries) - 1, address: "12 Elm St"})
	if _, err := planned(c, x); err == nil || !strings.Contains(err.Error(), "Oak has students in the Hummingbirds and Jayvens bands") {
		t.Fatalf("a classroom spanning two bands: %v", err)
	}
	x.entries, x.households = x.entries[:len(x.entries)-1], x.households[:len(x.households)-1]
	family, _ := s.Model().Table("GROUP").Get("grp00000000020")
	if family["title"] != "Ashdown Family" || family["vc_address"] != "12 Elm St" {
		t.Fatalf("the Ashdowns' family reads %v", family)
	}
	sam := personNamed(s, "Sam (Samuel) Ashdown")
	if sam == nil || sam["vc_job_title"] != "Coach" || sam["vc_legal_name"] != "Samuel Ashdown" {
		t.Fatalf("Sam, a parent and staff, reads %v", sam)
	}
	if _, ok := s.Model().Table("MEMBER").Find("grp00000000020", sam["id"], "manager"); !ok {
		t.Fatal("Sam does not manage the Ashdowns' family")
	}
	if email, ok := s.Model().Table("PERSON_EMAIL").Find("sam@example.org"); !ok || email["primary"] != "Yes" || email["source"] != "veracross" {
		t.Fatalf("Sam's address reads %v", email)
	}

	if p, photos := importOnce(t, c, x); len(p.batch) != 0 || photos != 0 {
		t.Fatalf("a second run changed %v and added %d photos: %v", p.counts, photos, p.batch)
	}

	x.entries = x.entries[:len(x.entries)-1]
	p, _ = importOnce(t, c, x)
	maya, _ := s.Model().Table("PERSON").Get("per00000000003")
	if p.counts["people deactivated"] != 1 || maya["deactivated"] != "2026-09-30 12:00" {
		t.Fatalf("Maya gone from the export: %v, %v", p.counts, maya)
	}
	for _, g := range groupsOf(s, "per00000000003") {
		if strings.HasPrefix(g, "group:") {
			t.Fatalf("deactivated, Maya is still in %v", g)
		}
	}
}
