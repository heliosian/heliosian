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
	"heliosian/internal/admins"
	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/id"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/who"
)

const (
	admin          = "jordan.whitfield@heliosschool.org"
	appsID         = "hcg0000000001"
	schoolID       = "hcg0000000002"
	eventsID       = "hcg0000000003"
	chatsID        = "hcg0000000004"
	directoryID    = "hyp0000000001"
	parentPortalID = "hyp0000000003"
	jaysChatID     = "hyp0000000009"
)

func sampleSources(t *testing.T) func() filter.Sources {
	t.Helper()
	model, err := who.LoadModel(&data.Dir{Root: "../../sampledata"}, nil, testkit.None, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return func() filter.Sources { return filter.Sources{Directory: model} }
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
	return testkit.ChangeLog(t, s.dir, s.queue, appName)
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
	if err := c.Commit(context.Background(), access.System("test"), audience(thingLink+directoryID, nil, []filter.Rule{{Kind: filter.KindExclude, Roles: []string{"Student"}}})...); err != nil {
		t.Fatal(err)
	}
	if got := c.Model().link(directoryID).Rules; len(got) != 1 || got[0].Kind != filter.KindExclude {
		t.Errorf("the Directory's rules after a save = %+v", got)
	}
	for _, bad := range []store.Row{
		{"Thing": thingLink + directoryID, "Kind": "include", "Roles": "Teachers"},
		{"Thing": thingLink + directoryID, "Kind": "include"},
		{"Thing": thingLink + directoryID, "Kind": "include", "Tags": "Carpool"},
	} {
		if err := c.Commit(context.Background(), access.System("test"), store.Insert(audienceTab, bad)); err == nil {
			t.Errorf("a bad rule %v loaded", bad)
		}
	}
}

func audienceRows(t *testing.T, s sheet, thing string) int {
	t.Helper()
	n := 0
	for _, row := range s.rows(t, audienceTab) {
		if row["Thing"] == thing {
			n++
		}
	}
	return n
}

func TestRenamingKeepsLinksAndAudience(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	chats := c.Model().category(chatsID)
	rec := call(t, serve.JSON(a.saveCategory), map[string]any{"id": chatsID, "title": "Group Chats", "emoji": chats.Emoji, "style": chats.Style, "rules": chats.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	renamed := c.Model().category(chatsID)
	if renamed == nil || renamed.Title != "Group Chats" || len(renamed.Links) != len(chats.Links) || len(renamed.Rules) != 1 {
		t.Fatalf("the links and the audience did not stay with the renamed category: %+v", renamed)
	}
	log := changeLog(t, dir)
	if len(log) != 1 || log[0]["Tab"] != categoriesTab || log[0]["Column"] != "Title" || log[0]["Key"] != "Category ID="+chatsID || log[0]["Previous"] != "Chats" {
		t.Fatalf("change log = %v, want the title alone", log)
	}

	jays := c.Model().link(jaysChatID)
	rec = call(t, serve.JSON(a.saveLink), map[string]any{"id": jaysChatID, "title": "Jays Parents Chat", "url": jays.URL, "category": jays.Category, "visible": jays.Visible, "rules": jays.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename a link: %d %s", rec.Code, rec.Body)
	}
	if got := c.Model().link(jaysChatID); got == nil || got.Title != "Jays Parents Chat" || len(got.Rules) != 1 || got.Category != chatsID {
		t.Fatalf("the renamed link = %+v", got)
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Errorf("the renamed link has %d audience rows, want 1", n)
	}
	if log := changeLog(t, dir)[1:]; len(log) != 1 || log[0]["Tab"] != linksTab || log[0]["Column"] != "Title" {
		t.Fatalf("change log = %v, want the link's title alone", log)
	}
}

func TestDeletingALinkDropsItsAudience(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Fatalf("the sample chat has %d audience rows, want 1", n)
	}
	if rec := call(t, serve.JSON(a.deleteLink), map[string]any{"id": jaysChatID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if c.Model().link(jaysChatID) != nil {
		t.Fatal("the link is still in the model")
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 0 {
		t.Errorf("the deleted link left %d audience rows", n)
	}
	if n := audienceRows(t, dir, thingLink+parentPortalID); n != 1 {
		t.Errorf("another link's audience went too: %d rows", n)
	}
	if rec := call(t, serve.JSON(a.deleteLink), map[string]any{"id": jaysChatID}); rec.Code != http.StatusNotFound {
		t.Errorf("deleted a deleted link: %d", rec.Code)
	}
}

func TestAddingMintsAnID(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	parents := []filter.Rule{{Kind: filter.KindInclude, Roles: []string{"Parent"}}}
	if rec := call(t, serve.JSON(a.saveCategory), map[string]any{"title": "Clubs", "style": StyleTiles, "rules": parents}); rec.Code != http.StatusNoContent {
		t.Fatalf("add a category: %d %s", rec.Code, rec.Body)
	}
	var clubs *Category
	for _, cat := range c.Model().Categories {
		if cat.Title == "Clubs" {
			clubs = &cat
		}
	}
	if clubs == nil {
		t.Fatal("no Clubs category")
	}
	if key, ok := id.Parse(clubs.ID); !ok || key != clubs.ID || len(clubs.Rules) != 1 {
		t.Fatalf("the new category = %+v", clubs)
	}
	if rec := call(t, serve.JSON(a.saveLink), map[string]any{"title": "Chess", "url": "https://chess.example.org/", "category": clubs.ID, "visible": true, "rules": parents}); rec.Code != http.StatusNoContent {
		t.Fatalf("add a link: %d %s", rec.Code, rec.Body)
	}
	links := c.Model().category(clubs.ID).Links
	if len(links) != 1 || links[0].Title != "Chess" || links[0].Category != clubs.ID || len(links[0].Rules) != 1 {
		t.Fatalf("the new category's links = %+v", links)
	}
	if key, ok := id.Parse(links[0].ID); !ok || key != links[0].ID || key == clubs.ID {
		t.Fatalf("the new link's id = %q", links[0].ID)
	}
	if n := audienceRows(t, dir, thingLink+links[0].ID); n != 1 {
		t.Errorf("the new link has %d audience rows, want 1", n)
	}
	if rec := call(t, serve.JSON(a.saveLink), map[string]any{"title": "Nowhere", "url": "https://example.org/", "category": "Clubs", "visible": true}); rec.Code != http.StatusBadRequest {
		t.Errorf("a link filed under a category's title: %d", rec.Code)
	}
}

func TestMoveLinkTradesPlacesWithinItsCategory(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	if rec := call(t, serve.JSON(a.moveLink), map[string]any{"id": parentPortalID, "by": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	var school []string
	for _, l := range c.Model().category(schoolID).Links {
		school = append(school, l.Title)
	}
	if want := []string{"Directory", "Calendar", "Staff Room", "Parent Portal"}; !slices.Equal(school, want) {
		t.Fatalf("school = %v, want %v", school, want)
	}
	got := []string{}
	for _, row := range changeLog(t, dir) {
		got = append(got, row["Action"]+" "+row["Key"]+" "+row["Column"]+" "+row["Previous"])
	}
	if want := []string{"set Link ID=" + parentPortalID + " Order 6"}; !slices.Equal(got, want) {
		t.Fatalf("change log = %v, want %v", got, want)
	}
	if rec := call(t, serve.JSON(a.moveLink), map[string]any{"id": parentPortalID, "by": 1}); rec.Code != http.StatusNoContent || len(changeLog(t, dir)) != 1 {
		t.Fatalf("a move past the end: %d, log %v", rec.Code, changeLog(t, dir))
	}
}

func TestARowWithNoOrderSortsLast(t *testing.T) {
	c, dir := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	const lunchID = "hyp0000000099"
	if err := c.Commit(context.Background(), access.System("test"), store.Insert(linksTab, store.Row{"Link ID": lunchID, "Title": "Lunch Menu", "URL": "https://lunch.example.org/", "Category": schoolID, "Visible": "Yes"}), store.Update(linksTab, store.Row{"Link ID": directoryID}, store.Row{store.OrderColumn: ""})); err != nil {
		t.Fatal(err)
	}
	school := func() []string {
		out := []string{}
		for _, l := range c.Model().category(schoolID).Links {
			out = append(out, l.Title)
		}
		return out
	}
	if want := []string{"Calendar", "Parent Portal", "Staff Room", "Directory", "Lunch Menu"}; !slices.Equal(school(), want) {
		t.Fatalf("school = %v, want %v", school(), want)
	}
	before := len(changeLog(t, dir))
	if rec := call(t, serve.JSON(a.moveLink), map[string]any{"id": lunchID, "by": -1}); rec.Code != http.StatusNoContent {
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
	a := app{cache: c, sources: c.sources}
	order := []string{appsID, EventsID, eventsID, schoolID, chatsID}
	if rec := call(t, serve.JSON(a.reorderCategories), map[string]any{"ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Categories {
		got = append(got, category.ID)
	}
	if !slices.Equal(got, order) || len(changeLog(t, dir)) != 1 {
		t.Fatalf("categories = %v, log %v", got, changeLog(t, dir))
	}
	for _, bad := range [][]string{order[1:], append(slices.Clone(order[1:]), eventsID), append(slices.Clone(order[1:]), "Helios Community Apps")} {
		if rec := call(t, serve.JSON(a.reorderCategories), map[string]any{"ids": bad}); rec.Code != http.StatusBadRequest {
			t.Errorf("%v was taken as an order: %d", bad, rec.Code)
		}
	}
}

func TestTheEventsSectionIsWrittenWhereItStands(t *testing.T) {
	c, _ := sampleCache(t)
	if err := c.Commit(context.Background(), access.System("test"), store.Delete(categoriesTab, store.Row{"Category ID": EventsID})); err != nil {
		t.Fatal(err)
	}
	a := app{cache: c, sources: c.sources}
	if events := c.Model().category(EventsID); events == nil || !events.Virtual || events.Title != EventsTitle {
		t.Fatalf("no synthesized events section: %+v", events)
	}
	order := []string{appsID, EventsID, schoolID, eventsID, chatsID}
	if rec := call(t, serve.JSON(a.reorderCategories), map[string]any{"ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Categories {
		got = append(got, category.ID)
	}
	if !slices.Equal(got, order) || c.Model().category(EventsID).Virtual {
		t.Fatalf("categories = %v, want %v with the events row written", got, order)
	}
}

func TestAppOrderIsAKey(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{cache: c, sources: c.sources}
	order := []string{"ask", "who", "team", "celebrate", "birthday", "when", "loop"}
	if rec := call(t, serve.JSON(a.setAppOrder), map[string]any{"apps": order}); rec.Code != http.StatusNoContent {
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
	a := app{cache: c, sources: c.sources}
	if got := c.Model().WidgetOrder; !slices.Equal(got, Widgets) {
		t.Fatalf("unset order = %v, want %v", got, Widgets)
	}
	for _, order := range [][]string{{"school", "when", "team", "celebrate"}, {"school", "celebrate", "when", "team"}} {
		if rec := call(t, serve.JSON(a.setWidgetOrder), map[string]any{"widgets": order}); rec.Code != http.StatusNoContent {
			t.Fatalf("order: %d %s", rec.Code, rec.Body)
		}
		if got := c.Model().WidgetOrder; !slices.Equal(got, order) {
			t.Fatalf("widgets = %v, want %v", got, order)
		}
	}
	if rec := call(t, serve.JSON(a.setWidgetOrder), map[string]any{"widgets": []string{"school", "when"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a partial order: %d, want 400", rec.Code)
	}
}

func TestOnlyAdminsGetTheRules(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{
		cache: c, sources: c.sources,
		upcoming: func(string, string) Upcoming { return Upcoming{} },
		month:    func(string, string, string) Month { return Month{} },
	}
	modelOf := func(email string) (view struct {
		Categories []Category `json:"categories"`
		User       user       `json:"user"`
	}) {
		rec := httptest.NewRecorder()
		auth.Fixed(email, serve.JSON(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
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
	const alias, admin = "facilities@heliosschool.org", "hank.morrow@heliosschool.org"
	if err := c.Commit(context.Background(), access.System("test"), store.Insert(admins.Tab, store.Row{"Email": admin})); err != nil {
		t.Fatal(err)
	}
	setVisibility(t, c, "celebrate", Visibility{Mode: VisibleToList, Emails: []string{admin}})
	before := len(changeLog(t, dir))
	seen := map[string]string{}
	a := app{
		cache: c, sources: c.sources,
		upcoming: func(email, _ string) Upcoming { seen["upcoming"] = email; return Upcoming{} },
		month:    func(email, _, _ string) Month { seen["month"] = email; return Month{} },
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
	for _, h := range []http.HandlerFunc{serve.JSON(a.calendar), serve.JSON(a.upcomingUnder)} {
		auth.Fixed(alias, h).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
	for _, h := range []http.HandlerFunc{serve.JSON(a.rsvp), serve.JSON(a.setDefault)} {
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
	auth.Fixed(alias, serve.JSON(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
	var view struct {
		User user `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.User.IsAdmin || view.User.Email != admin {
		t.Errorf("the alias's model is not the admin's: %s", rec.Body)
	}
	for _, key := range []string{"upcoming", "month"} {
		if seen[key] != admin {
			t.Errorf("%s got %q, want the admin the alias resolves to", key, seen[key])
		}
	}
	raw, err := json.Marshal(map[string]any{"id": parentPortalID, "by": 1})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	auth.Fixed(alias, serve.JSON(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the alias's move: %d %s", rec.Code, rec.Body)
	}
	if log := changeLog(t, dir)[before:]; len(log) != 1 || log[0]["Actor"] != admin {
		t.Errorf("change log = %v, want one row by the admin the alias resolves to", log)
	}
	rec = httptest.NewRecorder()
	auth.Fixed("robin.whitfield@heliosschool.org", serve.JSON(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member's move: %d, want 403", rec.Code)
	}
}

func TestWidgetAudience(t *testing.T) {
	c, _ := sampleCache(t)
	a := app{
		cache: c, sources: c.sources,
		upcoming: func(string, string) Upcoming { return Upcoming{} },
		month:    func(string, string, string) Month { return Month{} },
	}
	parentsOnly := []filter.Rule{{Kind: filter.KindInclude, Roles: []string{"Parent"}}}
	if rec := call(t, serve.JSON(a.saveWidgetAudience), map[string]any{"widget": "team", "rules": parentsOnly}); rec.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, serve.JSON(a.saveWidgetAudience), map[string]any{"widget": "nope", "rules": parentsOnly}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown widget: %d", rec.Code)
	}
	widgets := func(email string) map[string]widgetView {
		rec := httptest.NewRecorder()
		auth.Fixed(email, serve.JSON(a.model)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
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
