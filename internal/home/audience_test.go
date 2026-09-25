package home

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const admin = "jordan.whitfield@heliosschool.org"

type noFiles struct{}

func (noFiles) Has(string) (bool, error) { return false, nil }

func (noFiles) Prefetch([]string) error { return nil }

type sampleDirectory struct{ model *who.Model }

func (d sampleDirectory) Sources() filter.Sources {
	return filter.Sources{Directory: d.model}
}

func directoryOf(t *testing.T) sampleDirectory {
	t.Helper()
	tables, err := who.ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(tables, nil, noFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return sampleDirectory{model}
}

func call(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	auth.Fixed(admin, handler).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	return rec
}

func changeLog(t *testing.T, dir *data.Dir) []store.Row {
	t.Helper()
	_, rows, err := dir.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestAudienceIsAListOfRules(t *testing.T) {
	c, _ := sampleCache(t)
	c.directory = directoryOf(t)
	links := map[string]Link{}
	var chats Category
	for _, cat := range c.Model().Categories {
		if cat.Title == "Chats" {
			chats = cat
		}
		for _, l := range cat.Links {
			links[l.Title] = l
		}
	}
	if got := links["Hawks and Falcons Chat"].Rules; len(got) != 1 || got[0].Kind != filter.KindInclude || got[0].Roles[0] != "Parent" || len(got[0].Classrooms) != 2 {
		t.Fatalf("the chat's rules = %+v", got)
	}
	const (
		jordan = "jordan.whitfield@heliosschool.org"
		sam    = "sam.whitfield@heliosschool.org"
		ruth   = "ruth.amari@heliosschool.org"
	)
	sees := func(rules []filter.Rule, email string) bool {
		return len(rules) == 0 || c.includes(rules, email)
	}
	for _, tc := range []struct {
		title string
		email string
		want  bool
	}{
		{"Directory", sam, true},
		{"Parent Portal", jordan, true},
		{"Parent Portal", sam, false},
		{"Staff Room", ruth, true},
		{"Staff Room", jordan, false},
		{"Hummingbirds Chat", ruth, false},
		{"Hawks and Falcons Chat", jordan, false},
		{"Jays Chat", jordan, true},
		{"Jays Chat", sam, false},
	} {
		if got := sees(links[tc.title].Rules, tc.email); got != tc.want {
			t.Errorf("%s for %s = %v, want %v", tc.title, tc.email, got, tc.want)
		}
	}
	if len(chats.Rules) != 1 || sees(chats.Rules, sam) || !sees(chats.Rules, ruth) || !sees(chats.Rules, jordan) {
		t.Errorf("Chats = %+v, want kept to parents and staff", chats.Rules)
	}
	if hidden := c.HiddenApps(jordan); slices.Contains(hidden, "celebrate") {
		t.Errorf("a Jays parent is kept from the celebration: %v", hidden)
	}
	if hidden := c.HiddenApps(sam); !slices.Contains(hidden, "celebrate") {
		t.Errorf("a student sees the celebration: %v", hidden)
	}
	if err := c.Commit(context.Background(), "test", audience(thingLink+"Directory", nil, []filter.Rule{{Kind: filter.KindExclude, Roles: []string{"Student"}}})...); err != nil {
		t.Fatal(err)
	}
	if got := c.Model().Categories[2].Links[0].Rules; len(got) != 1 || got[0].Kind != filter.KindExclude {
		t.Errorf("the Directory's rules after a save = %+v", got)
	}
	for _, bad := range []store.Row{
		{"Thing": "link:Directory", "Kind": "include", "Roles": "Teachers"},
		{"Thing": "link:Directory", "Kind": "include"},
		{"Thing": "link:Directory", "Kind": "include", "Tags": "Carpool"},
	} {
		if err := c.Commit(context.Background(), "test", store.Insert(audienceTab, bad)); err == nil {
			t.Errorf("a bad rule %v loaded", bad)
		}
	}
}

func TestCategoryRenameCarriesItsLinksAndAudience(t *testing.T) {
	c, dir := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{cache: c, directory: c.directory}
	chats := a.category("Chats")
	rec := call(t, a.saveCategory, map[string]any{"original": "Chats", "title": "Group Chats", "emoji": chats.Emoji, "style": chats.Style, "rules": chats.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	renamed := a.category("Group Chats")
	if renamed == nil || len(renamed.Links) != len(chats.Links) || len(renamed.Rules) != 1 || a.category("Chats") != nil {
		t.Fatalf("the links and the audience did not follow the rename in memory: %+v", renamed)
	}
	_, links, err := dir.Table(appName, linksTab)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range links {
		if row["Category"] == "Chats" {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	tabs := map[string]int{}
	for _, row := range changeLog(t, dir) {
		if row["Actor"] != admin || row["Action"] != "set" || (row["Previous"] != "Chats" && row["Previous"] != thingCategory+"Chats") {
			t.Errorf("log row %v", row)
		}
		tabs[row["Tab"]+"/"+row["Column"]]++
	}
	if tabs[categoriesTab+"/Title"] != 1 || tabs[linksTab+"/Category"] != len(chats.Links) || tabs[audienceTab+"/Thing"] != 1 || len(tabs) != 3 {
		t.Fatalf("logged %v", tabs)
	}
}

func TestMoveLinkTradesPlacesWithinItsCategory(t *testing.T) {
	c, dir := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{cache: c, directory: c.directory}
	if rec := call(t, a.moveLink, map[string]any{"title": "Parent Portal", "by": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	var school []string
	for _, l := range a.category("School").Links {
		school = append(school, l.Title)
	}
	if want := []string{"Directory", "Calendar", "Staff Room", "Parent Portal"}; !slices.Equal(school, want) {
		t.Fatalf("school = %v, want %v", school, want)
	}
	_, links, err := dir.Table(appName, linksTab)
	if err != nil {
		t.Fatal(err)
	}
	if links[2]["Title"] != "Staff Room" || links[9]["Title"] != "Parent Portal" {
		t.Fatalf("the sheet's order: %v", links)
	}
	got := []string{}
	for _, row := range changeLog(t, dir) {
		got = append(got, row["Action"]+" "+row["Key"]+" "+row["Previous"])
	}
	if want := []string{"reorder Title=Staff Room 10", "reorder Title=Parent Portal 3"}; !slices.Equal(got, want) {
		t.Fatalf("change log = %v, want %v", got, want)
	}
}
