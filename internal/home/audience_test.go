package home

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const admin = "jordan.whitfield@heliosschool.org"

type noFiles struct{}

func (noFiles) Has(string) (bool, error) { return false, nil }

func (noFiles) Prefetch(context.Context, []string) error { return nil }

type sampleDirectory struct {
	model   *who.Model
	aliases map[string]string
}

func (d sampleDirectory) Sources() filter.Sources {
	return filter.Sources{Directory: d.model}
}

func (d sampleDirectory) Resolve(email string) string {
	if to, ok := d.aliases[email]; ok {
		return to
	}
	return d.model.Resolve(email)
}

func directoryOf(t *testing.T) sampleDirectory {
	t.Helper()
	model, err := who.LoadModel(&data.Dir{Root: "../../sampledata"}, nil, noFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return sampleDirectory{model: model}
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

func changeLog(t *testing.T, s sheet) []store.Row {
	t.Helper()
	return s.rows(t, store.ChangeLogTab)
}

func TestAudienceIsAListOfRules(t *testing.T) {
	c, _ := sampleCache(t)
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
	if err := c.Commit(context.Background(), access.System("test"), audience(thingLink+"Directory", nil, []filter.Rule{{Kind: filter.KindExclude, Roles: []string{"Student"}}})...); err != nil {
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
		if err := c.Commit(context.Background(), access.System("test"), store.Insert(audienceTab, bad)); err == nil {
			t.Errorf("a bad rule %v loaded", bad)
		}
	}
}

func TestCategoryRenameCarriesItsLinksAndAudience(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, directory: c.directory}
	chats := c.Model().category("Chats")
	rec := call(t, a.saveCategory, map[string]any{"original": "Chats", "title": "Group Chats", "emoji": chats.Emoji, "style": chats.Style, "rules": chats.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	renamed := c.Model().category("Group Chats")
	if renamed == nil || len(renamed.Links) != len(chats.Links) || len(renamed.Rules) != 1 || c.Model().category("Chats") != nil {
		t.Fatalf("the links and the audience did not follow the rename in memory: %+v", renamed)
	}
	for _, row := range dir.rows(t, linksTab) {
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
	a := app{cache: c, directory: c.directory}
	if rec := call(t, a.moveLink, map[string]any{"title": "Parent Portal", "by": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	var school []string
	for _, l := range c.Model().category("School").Links {
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
	a := app{cache: c, directory: c.directory}
	if err := c.Commit(context.Background(), access.System("test"), store.Insert(linksTab, store.Row{"Title": "Lunch Menu", "URL": "https://lunch.example.org/", "Category": "School", "Visible": "Yes"}), store.Update(linksTab, store.Row{"Title": "Directory"}, store.Row{store.OrderColumn: ""})); err != nil {
		t.Fatal(err)
	}
	school := func() []string {
		out := []string{}
		for _, l := range c.Model().category("School").Links {
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
	if err := c.Commit(context.Background(), access.System("test"), store.Delete(categoriesTab, store.Row{"Title": EventsTitle})); err != nil {
		t.Fatal(err)
	}
	a := app{cache: c, directory: c.directory}
	if !c.Model().virtualEvents(EventsTitle) {
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
	if !slices.Equal(got, titles) || c.Model().virtualEvents(EventsTitle) {
		t.Fatalf("categories = %v, want %v with the events row written", got, titles)
	}
}

func TestAppOrderIsAKey(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{cache: c, directory: c.directory}
	order := []string{"ask", "who", "team", "celebrate", "birthday", "when", "loop"}
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

func TestWidgetOrderIsAKey(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{cache: c, directory: c.directory}
	if got := c.Model().WidgetOrder; !slices.Equal(got, Widgets) {
		t.Fatalf("unset order = %v, want %v", got, Widgets)
	}
	for _, order := range [][]string{{"school", "when", "team", "celebrate"}, {"school", "celebrate", "when", "team"}} {
		if rec := call(t, a.setWidgetOrder, map[string]any{"widgets": order}); rec.Code != http.StatusNoContent {
			t.Fatalf("order: %d %s", rec.Code, rec.Body)
		}
		if got := c.Model().WidgetOrder; !slices.Equal(got, order) {
			t.Fatalf("widgets = %v, want %v", got, order)
		}
	}
	if rec := call(t, a.setWidgetOrder, map[string]any{"widgets": []string{"school", "when"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a partial order: %d, want 400", rec.Code)
	}
}

func TestOnlyAdminsGetTheRules(t *testing.T) {
	c, _ := sampleCache(t)
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

func TestAnAdminsAliasIsTheAdmin(t *testing.T) {
	c, dir := sampleCache(t)
	const alias = "jw@example.org"
	directory := directoryOf(t)
	directory.aliases = map[string]string{alias: admin}
	c.directory = directory
	seen := map[string]string{}
	a := app{
		cache: c, directory: directory,
		heroPhoto: func(email string) string { seen["photo"] = email; return "" },
		alerts:    func(string) ([]string, []string) { return nil, nil },
		upcoming:  func(email, _ string) Upcoming { seen["upcoming"] = email; return Upcoming{} },
		month:     func(email, _, _ string) Month { seen["month"] = email; return Month{} },
		answer: func(_ context.Context, email, _, _ string) error {
			seen["answer"] = email
			return nil
		},
		makeDefault: func(_ context.Context, email, _ string) error {
			seen["default"] = email
			return nil
		},
	}
	if c.IsAdmin(alias) {
		t.Fatal("the alias is listed as an admin itself")
	}
	if hidden := c.HiddenApps(alias); slices.Contains(hidden, "celebrate") {
		t.Errorf("the alias is kept from an app listed for the admin by address: %v", hidden)
	}
	for _, h := range []http.HandlerFunc{a.calendar, a.upcomingUnder} {
		auth.Fixed(alias, h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
	for _, h := range []http.HandlerFunc{a.rsvp, a.setDefault} {
		rec := httptest.NewRecorder()
		auth.Fixed(alias, h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`))))
		if rec.Code != http.StatusNoContent {
			t.Errorf("the alias's post: %d %s", rec.Code, rec.Body)
		}
	}
	for _, key := range []string{"upcoming", "month", "answer", "default"} {
		if seen[key] != admin {
			t.Errorf("%s got %q, want the admin the alias resolves to", key, seen[key])
		}
	}
	clear(seen)
	rec := httptest.NewRecorder()
	auth.Fixed(alias, http.HandlerFunc(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
	var view struct {
		User user `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.User.IsAdmin || view.User.Email != admin {
		t.Errorf("the alias's model is not the admin's: %s", rec.Body)
	}
	for _, key := range []string{"photo", "upcoming", "month"} {
		if seen[key] != admin {
			t.Errorf("%s got %q, want the admin the alias resolves to", key, seen[key])
		}
	}
	raw, err := json.Marshal(map[string]any{"title": "Parent Portal", "by": 1})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	auth.Fixed(alias, http.HandlerFunc(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the alias's move: %d %s", rec.Code, rec.Body)
	}
	if log := changeLog(t, dir); len(log) != 1 || log[0]["Actor"] != admin {
		t.Errorf("change log = %v, want one row by the admin the alias resolves to", log)
	}
	rec = httptest.NewRecorder()
	auth.Fixed("robin.whitfield@heliosschool.org", http.HandlerFunc(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member's move: %d, want 403", rec.Code)
	}
}

func TestWidgetAudience(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{
		cache: c, directory: c.directory,
		heroPhoto: func(string) string { return "" },
		alerts:    func(string) ([]string, []string) { return nil, nil },
		upcoming:  func(string, string) Upcoming { return Upcoming{} },
		month:     func(string, string, string) Month { return Month{} },
	}
	parentsOnly := []filter.Rule{{Kind: filter.KindInclude, Roles: []string{"Parent"}}}
	if rec := call(t, a.saveWidgetAudience, map[string]any{"widget": "team", "rules": parentsOnly}); rec.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, a.saveWidgetAudience, map[string]any{"widget": "nope", "rules": parentsOnly}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown widget: %d", rec.Code)
	}
	widgets := func(email string) map[string]widgetView {
		rec := httptest.NewRecorder()
		auth.Fixed(email, http.HandlerFunc(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
		var view struct {
			Widgets map[string]widgetView `json:"widgets"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view.Widgets
	}
	student := widgets("sam.whitfield@heliosschool.org")
	if student["team"].ForMe || !student["when"].ForMe || len(student["team"].Rules) != 0 {
		t.Errorf("a student's widgets: %+v", student)
	}
	if parent := widgets("jordan.whitfield@heliosschool.org"); !parent["team"].ForMe {
		t.Errorf("a parent's widgets: %+v", parent)
	}
	if got := widgets(admin)["team"].Rules; len(got) != 1 || got[0].Roles[0] != "Parent" {
		t.Errorf("an admin's team rules: %+v", got)
	}
}
