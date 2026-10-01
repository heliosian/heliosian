package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/db"
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

func TestAddressesSkipPlaceholders(t *testing.T) {
	if got := addresses(" Juni@Example.org ", "juni@example.org", "x.noemail@example.org", ""); !slices.Equal(got, []string{"juni@example.org"}) {
		t.Errorf("addresses = %v", got)
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
	db.Register(mux, s, queue, []byte(testKey), func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return client{url: srv.URL + "/api/q", key: testKey}, s
}

func sampleExport(withMaya bool) *export {
	x := &export{entries: []entry{
		{role: student, name: "Juni (Juniper) Ashdown", grade: "3", classroom: "Hummingbirds"},
		{role: parent, name: "Rowan Ashdown", addresses: []string{"rowan.ashdown@example.org"}, phone: "555-0100"},
		{role: parent, name: "Sam (Samuel) Ashdown", addresses: []string{"sam@example.org"}},
		{role: student, name: "Wren (Wrennie) Ashdown", grade: "K", classroom: "Oak", crew: "Acorn"},
		{role: parent, name: "Rowan Ashdown", addresses: []string{"rowan.ashdown@example.org"}, phone: "555-0100"},
		{role: parent, name: "Sam (Samuel) Ashdown", addresses: []string{"sam@example.org"}},
		{role: staff, name: "Ines Okafor", addresses: []string{"ines@example.org"}, jobTitle: "Librarian", bio: "Reads."},
		{role: staff, name: "Sam Ashdown", addresses: []string{"sam@example.org"}, jobTitle: "Coach"},
	}}
	x.households = []householdRow{
		{adults: []int{1, 2}, kid: 0, address: "12 Elm St"},
		{adults: []int{4, 5}, kid: 3, address: "12 Elm St"},
	}
	if withMaya {
		x.entries = append(x.entries, entry{role: staff, name: "Maya Lindqvist", addresses: []string{"maya.lindqvist@example.org"}, jobTitle: "Teacher"})
	}
	return x
}

func importOnce(t *testing.T, c client, x *export) ([]write, map[string]int) {
	t.Helper()
	st, err := c.read()
	if err != nil {
		t.Fatal(err)
	}
	batch, counts, err := plan(x, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) > 0 {
		if _, err := c.write(batch); err != nil {
			t.Fatal(err)
		}
	}
	return batch, counts
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
	_, counts := importOnce(t, c, sampleExport(true))
	if counts["people added"] != 3 || counts["people deactivated"] != 0 {
		t.Fatalf("the first run: %v", counts)
	}
	if juni := personNamed(s, "Juni (Juniper) Ashdown"); juni["id"] != "per00000000001" {
		t.Fatalf("Juni was not matched by name: %v", juni)
	}
	wren := personNamed(s, "Wren (Wrennie) Ashdown")
	if wren == nil || wren["name_sort_import"] != "Ashdown, Wren" || wren["vc_grade"] != "K" {
		t.Fatalf("Wren reads %v", wren)
	}
	want := []string{"band:Hummingbirds:member", "classroom:Oak:member", "crew:Acorn:member", "family::member", "grade:Kindergartengrade-k:member", "role:Everyoneeveryone:member", "role:Studentsstudents:member"}
	if got := groupsOf(s, wren["id"]); !slices.Equal(got, want) {
		t.Fatalf("Wren is in %v", got)
	}
	family, _ := s.Model().Table("GROUP").Get("grp00000000020")
	if family["vc_title"] != "Ashdown Family" || family["vc_address"] != "12 Elm St" {
		t.Fatalf("the Ashdowns' family reads %v", family)
	}
	sam := personNamed(s, "Sam (Samuel) Ashdown")
	if sam == nil || sam["vc_job_title"] != "Coach" || sam["vc_legal_name"] != "Samuel Ashdown" {
		t.Fatalf("Sam, a parent and staff, reads %v", sam)
	}
	if _, ok := s.Model().Table("MEMBER").Find("grp00000000020", sam["id"], "lead"); !ok {
		t.Fatal("Sam does not lead the Ashdowns' family")
	}
	if email, ok := s.Model().Table("PERSON_EMAIL").Find("sam@example.org"); !ok || email["primary"] != "Yes" || email["source"] != "veracross" {
		t.Fatalf("Sam's address reads %v", email)
	}

	if batch, counts := importOnce(t, c, sampleExport(true)); len(batch) != 0 {
		t.Fatalf("a second run changed %v: %v", counts, batch)
	}

	_, counts = importOnce(t, c, sampleExport(false))
	maya, _ := s.Model().Table("PERSON").Get("per00000000003")
	if counts["people deactivated"] != 1 || maya["deactivated"] != "2026-09-30 12:00" {
		t.Fatalf("Maya gone from the export: %v, %v", counts, maya)
	}
	for _, g := range groupsOf(s, "per00000000003") {
		if strings.HasPrefix(g, "role:") {
			t.Fatalf("deactivated, Maya is still in %v", g)
		}
	}
}
