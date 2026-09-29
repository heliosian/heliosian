package model

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
	"heliosian/internal/id"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const (
	homeAdmin      = "jordan.whitfield@heliosschool.org"
	appsID         = "hcg0000000001"
	schoolID       = "hcg0000000002"
	eventsID       = "hcg0000000003"
	chatsID        = "hcg0000000004"
	directoryID    = "hyp0000000001"
	parentPortalID = "hyp0000000003"
	jaysChatID     = "hyp0000000009"
)

func callHome(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	auth.Fixed(homeAdmin, handler).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	return rec
}

func homeChangeLog(t *testing.T, s homeSheet) []store.Row {
	t.Helper()
	return testkit.ChangeLog(t, s.dir, s.queue, homeAppName)
}

func TestAudienceIsAListOfRules(t *testing.T) {
	c, _ := sampleHomeCache(t)
	links := map[string]HomeLink{}
	var chats HomeCategory
	for _, cat := range c.Model().Home.Categories {
		if cat.Title == "Chats" {
			chats = cat
		}
		for _, l := range cat.Links {
			links[l.Title] = l
		}
	}
	if got := links["Hawks and Falcons Chat"].Rules; len(got) != 1 || got[0].Kind != RuleInclude || got[0].Roles[0] != "Parent" || len(got[0].Classrooms) != 2 {
		t.Fatalf("the chat's rules = %+v", got)
	}
	const (
		jordan = "jordan.whitfield@heliosschool.org"
		sam    = "sam.whitfield@heliosschool.org"
		ruth   = "ruth.amari@heliosschool.org"
	)
	sees := func(rules []Rule, email string) bool {
		return len(rules) == 0 || c.Model().homeIncludes(rules, email)
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
	if hidden := c.Model().HiddenApps(jordan); slices.Contains(hidden, "celebrate") {
		t.Errorf("a Jays parent is kept from the celebration: %v", hidden)
	}
	if hidden := c.Model().HiddenApps(sam); !slices.Contains(hidden, "celebrate") {
		t.Errorf("a student sees the celebration: %v", hidden)
	}
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, audienceOps(thingLink+directoryID, nil, []Rule{{Kind: RuleExclude, Roles: []string{"Student"}}})...); err != nil {
		t.Fatal(err)
	}
	if got := c.Model().Home.link(directoryID).Rules; len(got) != 1 || got[0].Kind != RuleExclude {
		t.Errorf("the Directory's rules after a save = %+v", got)
	}
	for _, bad := range []store.Row{
		{"Thing": thingLink + directoryID, "Kind": "include", "Roles": "Teachers"},
		{"Thing": thingLink + directoryID, "Kind": "include"},
		{"Thing": thingLink + directoryID, "Kind": "include", "Tags": "Carpool"},
	} {
		if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Insert(homeAudienceTab, bad)); err == nil {
			t.Errorf("a bad rule %v loaded", bad)
		}
	}
}

func audienceRows(t *testing.T, s homeSheet, thing string) int {
	t.Helper()
	n := 0
	for _, row := range s.rows(t, homeAudienceTab) {
		if row["Thing"] == thing {
			n++
		}
	}
	return n
}

func TestRenamingKeepsLinksAndAudience(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	chats := c.Model().Home.category(chatsID)
	rec := callHome(t, serve.JSON(a.saveCategory), map[string]any{"id": chatsID, "title": "Group Chats", "emoji": chats.Emoji, "style": chats.Style, "rules": chats.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	renamed := c.Model().Home.category(chatsID)
	if renamed == nil || renamed.Title != "Group Chats" || len(renamed.Links) != len(chats.Links) || len(renamed.Rules) != 1 {
		t.Fatalf("the links and the audience did not stay with the renamed category: %+v", renamed)
	}
	log := homeChangeLog(t, dir)
	if len(log) != 1 || log[0]["Tab"] != homeCategoriesTab || log[0]["Column"] != "Title" || log[0]["Key"] != "Category ID="+chatsID || log[0]["Previous"] != "Chats" {
		t.Fatalf("change log = %v, want the title alone", log)
	}

	jays := c.Model().Home.link(jaysChatID)
	rec = callHome(t, serve.JSON(a.saveLink), map[string]any{"id": jaysChatID, "title": "Jays Parents Chat", "url": jays.URL, "category": jays.Category, "visible": jays.Visible, "rules": jays.Rules})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename a link: %d %s", rec.Code, rec.Body)
	}
	if got := c.Model().Home.link(jaysChatID); got == nil || got.Title != "Jays Parents Chat" || len(got.Rules) != 1 || got.Category != chatsID {
		t.Fatalf("the renamed link = %+v", got)
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Errorf("the renamed link has %d audience rows, want 1", n)
	}
	if log := homeChangeLog(t, dir)[1:]; len(log) != 1 || log[0]["Tab"] != homeLinksTab || log[0]["Column"] != "Title" {
		t.Fatalf("change log = %v, want the link's title alone", log)
	}
}

func TestDeletingALinkDropsItsAudience(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Fatalf("the sample chat has %d audience rows, want 1", n)
	}
	if rec := callHome(t, serve.JSON(a.deleteLink), map[string]any{"id": jaysChatID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if c.Model().Home.link(jaysChatID) != nil {
		t.Fatal("the link is still in the model")
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 0 {
		t.Errorf("the deleted link left %d audience rows", n)
	}
	if n := audienceRows(t, dir, thingLink+parentPortalID); n != 1 {
		t.Errorf("another link's audience went too: %d rows", n)
	}
	if rec := callHome(t, serve.JSON(a.deleteLink), map[string]any{"id": jaysChatID}); rec.Code != http.StatusNotFound {
		t.Errorf("deleted a deleted link: %d", rec.Code)
	}
}

func TestAddingMintsAnID(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	parents := []Rule{{Kind: RuleInclude, Roles: []string{"Parent"}}}
	if rec := callHome(t, serve.JSON(a.saveCategory), map[string]any{"title": "Clubs", "style": StyleTiles, "rules": parents}); rec.Code != http.StatusNoContent {
		t.Fatalf("add a category: %d %s", rec.Code, rec.Body)
	}
	var clubs *HomeCategory
	for _, cat := range c.Model().Home.Categories {
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
	if rec := callHome(t, serve.JSON(a.saveLink), map[string]any{"title": "Chess", "url": "https://chess.example.org/", "category": clubs.ID, "visible": true, "rules": parents}); rec.Code != http.StatusNoContent {
		t.Fatalf("add a link: %d %s", rec.Code, rec.Body)
	}
	links := c.Model().Home.category(clubs.ID).Links
	if len(links) != 1 || links[0].Title != "Chess" || links[0].Category != clubs.ID || len(links[0].Rules) != 1 {
		t.Fatalf("the new category's links = %+v", links)
	}
	if key, ok := id.Parse(links[0].ID); !ok || key != links[0].ID || key == clubs.ID {
		t.Fatalf("the new link's id = %q", links[0].ID)
	}
	if n := audienceRows(t, dir, thingLink+links[0].ID); n != 1 {
		t.Errorf("the new link has %d audience rows, want 1", n)
	}
	if rec := callHome(t, serve.JSON(a.saveLink), map[string]any{"title": "Nowhere", "url": "https://example.org/", "category": "Clubs", "visible": true}); rec.Code != http.StatusBadRequest {
		t.Errorf("a link filed under a category's title: %d", rec.Code)
	}
}

func TestMoveLinkTradesPlacesWithinItsCategory(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	if rec := callHome(t, serve.JSON(a.moveLink), map[string]any{"id": parentPortalID, "by": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	var school []string
	for _, l := range c.Model().Home.category(schoolID).Links {
		school = append(school, l.Title)
	}
	if want := []string{"Directory", "Calendar", "Staff Room", "Parent Portal"}; !slices.Equal(school, want) {
		t.Fatalf("school = %v, want %v", school, want)
	}
	got := []string{}
	for _, row := range homeChangeLog(t, dir) {
		got = append(got, row["Action"]+" "+row["Key"]+" "+row["Column"]+" "+row["Previous"])
	}
	if want := []string{"set Link ID=" + parentPortalID + " Order 6"}; !slices.Equal(got, want) {
		t.Fatalf("change log = %v, want %v", got, want)
	}
	if rec := callHome(t, serve.JSON(a.moveLink), map[string]any{"id": parentPortalID, "by": 1}); rec.Code != http.StatusNoContent || len(homeChangeLog(t, dir)) != 1 {
		t.Fatalf("a move past the end: %d, log %v", rec.Code, homeChangeLog(t, dir))
	}
}

func TestMoveLinkJumpsSeveralPlaces(t *testing.T) {
	c, _ := sampleHomeCache(t)
	a := homeApp{store: c}
	if rec := callHome(t, serve.JSON(a.moveLink), map[string]any{"id": parentPortalID, "by": -2}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	var school []string
	for _, l := range c.Model().Home.category(schoolID).Links {
		school = append(school, l.Title)
	}
	if want := []string{"Parent Portal", "Directory", "Calendar", "Staff Room"}; !slices.Equal(school, want) {
		t.Fatalf("school = %v, want %v", school, want)
	}
}

func TestARowWithNoOrderSortsLast(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	const lunchID = "hyp0000000099"
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Insert(homeLinksTab, store.Row{"Link ID": lunchID, "Title": "Lunch Menu", "URL": "https://lunch.example.org/", "Category": schoolID, "Visible": "Yes"}), store.Update(homeLinksTab, store.Row{"Link ID": directoryID}, store.Row{store.OrderColumn: ""})); err != nil {
		t.Fatal(err)
	}
	school := func() []string {
		out := []string{}
		for _, l := range c.Model().Home.category(schoolID).Links {
			out = append(out, l.Title)
		}
		return out
	}
	if want := []string{"Calendar", "Parent Portal", "Staff Room", "Directory", "Lunch Menu"}; !slices.Equal(school(), want) {
		t.Fatalf("school = %v, want %v", school(), want)
	}
	before := len(homeChangeLog(t, dir))
	if rec := callHome(t, serve.JSON(a.moveLink), map[string]any{"id": lunchID, "by": -1}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if want := []string{"Calendar", "Parent Portal", "Staff Room", "Lunch Menu", "Directory"}; !slices.Equal(school(), want) {
		t.Fatalf("school after the move = %v, want %v", school(), want)
	}
	if got := len(homeChangeLog(t, dir)) - before; got != 2 {
		t.Fatalf("the move keyed %d rows, want the two without one", got)
	}
}

func TestCategoryOrderKeysOnlyWhatMoved(t *testing.T) {
	c, dir := sampleHomeCache(t)
	a := homeApp{store: c}
	order := []string{appsID, EventsCategoryID, eventsID, schoolID, chatsID}
	if rec := callHome(t, serve.JSON(a.reorderCategories), map[string]any{"ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Home.Categories {
		got = append(got, category.ID)
	}
	if !slices.Equal(got, order) || len(homeChangeLog(t, dir)) != 1 {
		t.Fatalf("categories = %v, log %v", got, homeChangeLog(t, dir))
	}
	for _, bad := range [][]string{order[1:], append(slices.Clone(order[1:]), eventsID), append(slices.Clone(order[1:]), "Helios Community Apps")} {
		if rec := callHome(t, serve.JSON(a.reorderCategories), map[string]any{"ids": bad}); rec.Code != http.StatusBadRequest {
			t.Errorf("%v was taken as an order: %d", bad, rec.Code)
		}
	}
}

func TestTheEventsSectionIsWrittenWhereItStands(t *testing.T) {
	c, _ := sampleHomeCache(t)
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Delete(homeCategoriesTab, store.Row{"Category ID": EventsCategoryID})); err != nil {
		t.Fatal(err)
	}
	a := homeApp{store: c}
	if events := c.Model().Home.category(EventsCategoryID); events == nil || !events.Virtual || events.Title != EventsCategoryTitle {
		t.Fatalf("no synthesized events section: %+v", events)
	}
	order := []string{appsID, EventsCategoryID, schoolID, eventsID, chatsID}
	if rec := callHome(t, serve.JSON(a.reorderCategories), map[string]any{"ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, category := range c.Model().Home.Categories {
		got = append(got, category.ID)
	}
	if !slices.Equal(got, order) || c.Model().Home.category(EventsCategoryID).Virtual {
		t.Fatalf("categories = %v, want %v with the events row written", got, order)
	}
}

func TestAppOrderIsAKey(t *testing.T) {
	c, _ := sampleHomeCache(t)
	a := homeApp{store: c}
	order := []string{"ask", "who", "team", "celebrate", "birthday", "when", "loop"}
	if rec := callHome(t, serve.JSON(a.setAppOrder), map[string]any{"apps": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("order: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, app := range c.Model().Home.AppList() {
		got = append(got, app.Key)
	}
	if !slices.Equal(got, order) {
		t.Fatalf("apps = %v, want %v", got, order)
	}
}

func TestWidgetOrderIsAKey(t *testing.T) {
	c, _ := sampleHomeCache(t)
	a := homeApp{store: c}
	if got := c.Model().Home.WidgetOrder; !slices.Equal(got, HomeWidgets) {
		t.Fatalf("unset order = %v, want %v", got, HomeWidgets)
	}
	for _, order := range [][]string{{"school", "when", "team", "celebrate"}, {"school", "celebrate", "when", "team"}} {
		if rec := callHome(t, serve.JSON(a.setWidgetOrder), map[string]any{"widgets": order}); rec.Code != http.StatusNoContent {
			t.Fatalf("order: %d %s", rec.Code, rec.Body)
		}
		if got := c.Model().Home.WidgetOrder; !slices.Equal(got, order) {
			t.Fatalf("widgets = %v, want %v", got, order)
		}
	}
	if rec := callHome(t, serve.JSON(a.setWidgetOrder), map[string]any{"widgets": []string{"school", "when"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a partial order: %d, want 400", rec.Code)
	}
}

func TestOnlyAdminsGetTheRules(t *testing.T) {
	c, s := sampleHomeCache(t)
	a := homeApp{store: c, hooks: homeCalendar(c, s)}
	modelOf := func(email string) (view struct {
		Categories []HomeCategory `json:"categories"`
		User       homeUser       `json:"user"`
	}) {
		rec := httptest.NewRecorder()
		auth.Fixed(email, serve.JSON(a.view)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("model for %s: %d %s", email, rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	rules := func(categories []HomeCategory) (sections, links int) {
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
	if sections, links := rules(modelOf(homeAdmin).Categories); sections == 0 || links == 0 {
		t.Errorf("an admin's model has %d section and %d link rules", sections, links)
	}
}

func TestAnAdminsAliasIsTheAdmin(t *testing.T) {
	c, dir := sampleHomeCache(t)
	const alias, admin = "facilities@heliosschool.org", "hank.morrow@heliosschool.org"
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Insert(AdminsTab.Name, store.Row{"Email": admin})); err != nil {
		t.Fatal(err)
	}
	setAppVisibility(t, c, "celebrate", AppVisibilityRow{Mode: VisibleToList, Emails: []string{admin}})
	const token, event = "sample7feedtoken4hank5morrow", "gev0000000007"
	hooks := homeCalendar(c, dir)
	if err := c.Commit(context.Background(), access.System("test"), CalendarApp, store.Insert(FeedsTab, store.Row{"Token": token, "Email": admin, "Name": "Facilities", "Created": "2026-09-01 08:00"})); err != nil {
		t.Fatal(err)
	}
	if chosen := c.Model().Calendar.DefaultCalendar(admin); chosen != nil {
		t.Fatalf("the admin starts with a default calendar: %+v", chosen)
	}
	before := len(homeChangeLog(t, dir))
	a := homeApp{store: c, hooks: hooks}
	if c.Model().AdminList("home").IsAdmin(alias) {
		t.Fatal("the alias is listed as an admin itself")
	}
	if hidden := c.Model().HiddenApps(alias); slices.Contains(hidden, "celebrate") {
		t.Errorf("the alias is kept from an app listed for the admin by address: %v", hidden)
	}
	post := func(h http.HandlerFunc, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		auth.Fixed(alias, h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body))))
		if rec.Code != http.StatusNoContent {
			t.Errorf("the alias's post: %d %s", rec.Code, rec.Body)
		}
	}
	post(serve.JSON(a.rsvp), `{"id":"`+event+`","answer":"yes"}`)
	post(serve.JSON(a.setDefault), `{"token":"`+token+`"}`)
	calendar := c.Model().Calendar
	if calendar.AnswerOf(admin, event) != AnswerYes || calendar.AnswerOf(alias, event) != "" {
		t.Errorf("the alias's yes is the admin's %q, the alias's own %q; want the admin's", calendar.AnswerOf(admin, event), calendar.AnswerOf(alias, event))
	}
	if chosen := calendar.DefaultCalendar(admin); chosen == nil || chosen.Token != token {
		t.Errorf("the alias's default did not become the admin's: %+v", chosen)
	}
	get := func(h http.HandlerFunc, into any) {
		t.Helper()
		rec := httptest.NewRecorder()
		auth.Fixed(alias, h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
		}
	}
	var month HomeMonth
	get(serve.JSON(a.calendar), &month)
	var ahead HomeUpcoming
	get(serve.JSON(a.upcomingUnder), &ahead)
	if month.Calendar != token || ahead.Default != token || ahead.Calendar != token {
		t.Errorf("the alias's month under %q and upcoming %+v are not the admin's calendars", month.Calendar, ahead)
	}
	var view struct {
		User             homeUser      `json:"user"`
		Calendar         HomeMonth     `json:"calendar"`
		UpcomingCalendar *HomeUpcoming `json:"upcomingCalendar"`
	}
	get(serve.JSON(a.view), &view)
	if !view.User.IsAdmin || view.User.Email != admin {
		t.Errorf("the alias's model is not the admin's: %+v", view.User)
	}
	if view.Calendar.Calendar != token || view.UpcomingCalendar == nil || view.UpcomingCalendar.Default != token {
		t.Errorf("the alias's model does not read the admin's calendars: month under %q, upcoming %+v", view.Calendar.Calendar, view.UpcomingCalendar)
	}
	raw, err := json.Marshal(map[string]any{"id": parentPortalID, "by": 1})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	auth.Fixed(alias, serve.JSON(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the alias's move: %d %s", rec.Code, rec.Body)
	}
	if log := homeChangeLog(t, dir)[before:]; len(log) != 1 || log[0]["Actor"] != admin {
		t.Errorf("change log = %v, want one row by the admin the alias resolves to", log)
	}
	rec = httptest.NewRecorder()
	auth.Fixed("robin.whitfield@heliosschool.org", serve.JSON(a.moveLink)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member's move: %d, want 403", rec.Code)
	}
}

func TestWidgetAudience(t *testing.T) {
	c, s := sampleHomeCache(t)
	a := homeApp{store: c, hooks: homeCalendar(c, s)}
	parentsOnly := []Rule{{Kind: RuleInclude, Roles: []string{"Parent"}}}
	if rec := callHome(t, serve.JSON(a.saveWidgetAudience), map[string]any{"widget": "team", "rules": parentsOnly}); rec.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := callHome(t, serve.JSON(a.saveWidgetAudience), map[string]any{"widget": "nope", "rules": parentsOnly}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown widget: %d", rec.Code)
	}
	widgets := func(email string) map[string]widgetView {
		rec := httptest.NewRecorder()
		auth.Fixed(email, serve.JSON(a.view)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/apps/model", nil))
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
	if got := widgets(homeAdmin)["team"].Rules; len(got) != 1 || got[0].Roles[0] != "Parent" {
		t.Errorf("an admin's team rules: %+v", got)
	}
}
