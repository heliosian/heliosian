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
	model, err := who.LoadModel(&data.Dir{Root: "../../sampledata"}, nil, noFiles{}, []byte("test"))
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
	got := []string{}
	for _, row := range changeLog(t, dir) {
		got = append(got, row["Action"]+" "+row["Key"]+" "+row["Column"]+" "+row["Previous"])
	}
	if want := []string{"set Title=Parent Portal Order 6"}; !slices.Equal(got, want) {
		t.Fatalf("change log = %v, want %v", got, want)
	}
	if rec := call(t, a.moveLink, map[string]any{"title": "Parent Portal", "by": 1}); rec.Code != http.StatusNoContent || len(changeLog(t, dir)) != 1 {
		t.Fatalf("a move past the end: %d, log %v", rec.Code, changeLog(t, dir))
	}
}

func TestARowWithNoOrderSortsLast(t *testing.T) {
	c, dir := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{cache: c, directory: c.directory}
	if err := c.Commit(context.Background(), "test", store.Insert(linksTab, store.Row{"Title": "Lunch Menu", "URL": "https://lunch.example.org/", "Category": "School", "Visible": "Yes"}), store.Update(linksTab, store.Row{"Title": "Directory"}, store.Row{store.OrderColumn: ""})); err != nil {
		t.Fatal(err)
	}
	school := func() []string {
		out := []string{}
		for _, l := range a.category("School").Links {
			out = append(out, l.Title)
		}
		return out
	}
	if want := []string{"Calendar", "Parent Portal", "Staff Room", "Directory", "Lunch Menu"}; !slices.Equal(school(), want) {
		t.Fatalf("school = %v, want %v", school(), want)
	}
	before := len(changeLog(t, dir))
	if rec := call(t, a.moveLink, map[string]any{"title": "Lunch Menu", "by": -1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if want := []string{"Calendar", "Parent Portal", "Staff Room", "Lunch Menu", "Directory"}; !slices.Equal(school(), want) {
		t.Fatalf("school after the move = %v, want %v", school(), want)
	}
	if got := len(changeLog(t, dir)) - before; got != 2 {
		t.Fatalf("the move keyed %d rows, want the two without one", got)
	}
}

func TestCategoryOrderKeysOnlyWhatMoved(t *testing.T) {
	c, dir := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{cache: c, directory: c.directory}
	titles := []string{"Helios Community Apps", "Upcoming Events", "Events", "School", "Chats"}
	if rec := call(t, a.reorderCategories, map[string]any{"titles": titles}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Categories {
		got = append(got, category.Title)
	}
	if !slices.Equal(got, titles) || len(changeLog(t, dir)) != 1 {
		t.Fatalf("categories = %v, log %v", got, changeLog(t, dir))
	}
	for _, bad := range [][]string{titles[1:], append(slices.Clone(titles[1:]), "Events"), append(slices.Clone(titles[1:]), "Nowhere")} {
		if rec := call(t, a.reorderCategories, map[string]any{"titles": bad}); rec.Code != http.StatusBadRequest {
			t.Errorf("%v was taken as an order: %d", bad, rec.Code)
		}
	}
}

func TestTheEventsSectionIsWrittenWhereItStands(t *testing.T) {
	c, _ := sampleCache(t)
	c.directory = directoryOf(t)
	if err := c.Commit(context.Background(), "test", store.Delete(categoriesTab, store.Row{"Title": EventsTitle})); err != nil {
		t.Fatal(err)
	}
	a := app{cache: c, directory: c.directory}
	if !a.virtualEvents(EventsTitle) {
		t.Fatal("no synthesized events section")
	}
	titles := []string{"Helios Community Apps", EventsTitle, "School", "Events", "Chats"}
	if rec := call(t, a.reorderCategories, map[string]any{"titles": titles}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Categories {
		got = append(got, category.Title)
	}
	if !slices.Equal(got, titles) || a.virtualEvents(EventsTitle) {
		t.Fatalf("categories = %v, want %v with the events row written", got, titles)
	}
}

func TestAppOrderIsAKey(t *testing.T) {
	c, _ := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{cache: c, directory: c.directory}
	order := []string{"ask", "who", "team", "celebrate", "birthday", "calendar", "loop"}
	if rec := call(t, a.setAppOrder, map[string]any{"apps": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("order: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, app := range c.AppList() {
		got = append(got, app.Key)
	}
	if !slices.Equal(got, order) {
		t.Fatalf("apps = %v, want %v", got, order)
	}
}

func TestOnlyAdminsGetTheRules(t *testing.T) {
	c, _ := sampleCache(t)
	c.directory = directoryOf(t)
	a := app{
		cache: c, directory: c.directory,
		heroPhoto: func(string) string { return "" },
		alerts:    func(string) ([]string, []string) { return nil, nil },
		upcoming:  func(string, string) Upcoming { return Upcoming{} },
		month:     func(string, string, string) Month { return Month{} },
	}
	modelOf := func(email string) (view struct {
		Categories []Category `json:"categories"`
		User       user       `json:"user"`
	}) {
		rec := httptest.NewRecorder()
		auth.Fixed(email, http.HandlerFunc(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("model for %s: %d %s", email, rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	rules := func(categories []Category) (sections, links int) {
		for _, cat := range categories {
			sections += len(cat.Rules)
			for _, l := range cat.Links {
				links += len(l.Rules)
			}
		}
		return
	}
	staff := modelOf("ruth.amari@heliosschool.org")
	if staff.User.IsAdmin {
		t.Fatal("ruth is an admin in the sample data")
	}
	titles := []string{}
	for _, cat := range staff.Categories {
		for _, l := range cat.Links {
			titles = append(titles, l.Title)
		}
	}
	if !slices.Contains(titles, "Staff Room") {
		t.Errorf("ruth's links %v leave out the Staff Room kept to staff", titles)
	}
	if sections, links := rules(staff.Categories); sections != 0 || links != 0 {
		t.Errorf("a non-admin's model has %d section and %d link rules", sections, links)
	}
	if sections, links := rules(modelOf(admin).Categories); sections == 0 || links == 0 {
		t.Errorf("an admin's model has %d section and %d link rules", sections, links)
	}
}
