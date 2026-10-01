package model

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/id"
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
	calendarLinkID = "hyp0000000002"
	parentPortalID = "hyp0000000003"
	jaysChatID     = "hyp0000000009"
	staffRoomID    = "hyp0000000010"
)

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

func linkTitles(c *Store, category string) []string {
	out := []string{}
	for _, l := range c.Model().Home.category(category).Links {
		out = append(out, l.Title)
	}
	return out
}

func TestRenamingKeepsLinksAndAudience(t *testing.T) {
	c, dir, mux := homeServer(t)
	chats := c.Model().Home.category(chatsID)
	before := len(homeChangeLog(t, dir))
	write(t, mux, homeAdmin, "POST", "/api/link-categories/"+chatsID+"/edit", map[string]any{"title": "Group Chats"})
	renamed := c.Model().Home.category(chatsID)
	if renamed == nil || renamed.Title != "Group Chats" || len(renamed.Links) != len(chats.Links) || len(renamed.Rules) != 1 || renamed.Order != chats.Order {
		t.Fatalf("the links and the audience did not stay with the renamed category: %+v", renamed)
	}
	log := homeChangeLog(t, dir)[before:]
	if len(log) != 1 || log[0]["Tab"] != homeCategoriesTab || log[0]["Column"] != "Title" || log[0]["Key"] != "Category ID="+chatsID || log[0]["Previous"] != "Chats" {
		t.Fatalf("change log = %v, want the title alone", log)
	}

	before = len(homeChangeLog(t, dir))
	write(t, mux, homeAdmin, "POST", "/api/links/"+jaysChatID+"/edit", map[string]any{"title": "Jays Parents Chat"})
	if got := c.Model().Home.link(jaysChatID); got == nil || got.Title != "Jays Parents Chat" || len(got.Rules) != 1 || got.Category != chatsID {
		t.Fatalf("the renamed link = %+v", got)
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Errorf("the renamed link has %d audience rows, want 1", n)
	}
	if log := homeChangeLog(t, dir)[before:]; len(log) != 1 || log[0]["Tab"] != homeLinksTab || log[0]["Column"] != "Title" {
		t.Fatalf("change log = %v, want the link's title alone", log)
	}
}

func TestDeletingALinkDropsItsAudience(t *testing.T) {
	c, dir, mux := homeServer(t)
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 1 {
		t.Fatalf("the sample chat has %d audience rows, want 1", n)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "DELETE", "/api/links/"+jaysChatID, nil); rec.Code != http.StatusForbidden {
		t.Errorf("a member's delete: %d", rec.Code)
	}
	write(t, mux, homeAdmin, "DELETE", "/api/links/"+jaysChatID, nil)
	if c.Model().Home.link(jaysChatID) != nil {
		t.Fatal("the link is still in the model")
	}
	if n := audienceRows(t, dir, thingLink+jaysChatID); n != 0 {
		t.Errorf("the deleted link left %d audience rows", n)
	}
	if n := audienceRows(t, dir, thingLink+parentPortalID); n != 1 {
		t.Errorf("another link's audience went too: %d rows", n)
	}
	if rec := testkit.Call(t, mux, homeAdmin, "DELETE", "/api/links/"+jaysChatID, nil); rec.Code != http.StatusNotFound {
		t.Errorf("deleted a deleted link: %d", rec.Code)
	}
}

func TestDeletingACategory(t *testing.T) {
	c, _, mux := homeServer(t)
	if got := read(t, mux, homeAdmin, "/api/link-categories/"+chatsID).one(t, "link-categories", chatsID); can(got, "delete") {
		t.Errorf("a category with links can be deleted: %v", got["can"])
	}
	if got := read(t, mux, homeAdmin, "/api/link-categories/"+EventsCategoryID).one(t, "link-categories", EventsCategoryID); can(got, "delete") || !can(got, "edit") {
		t.Errorf("the events section's can: %v", got["can"])
	}
	if rec := testkit.Call(t, mux, homeAdmin, "DELETE", "/api/link-categories/"+EventsCategoryID, nil); rec.Code != http.StatusBadRequest || c.Model().Home.category(EventsCategoryID) == nil {
		t.Errorf("deleting the events section: %d", rec.Code)
	}
	made := write(t, mux, homeAdmin, "POST", "/api/link-categories", map[string]any{"title": "Empty", "style": StyleTiles})
	write(t, mux, homeAdmin, "DELETE", "/api/link-categories/"+made, nil)
	if c.Model().Home.category(made) != nil {
		t.Errorf("the empty category is still there")
	}
}

func TestAddingMintsAnIDAndAKeyAfterTheLast(t *testing.T) {
	c, dir, mux := homeServer(t)
	parents := []Rule{{Kind: RuleInclude, Roles: []string{"Parent"}}}
	clubsID := write(t, mux, homeAdmin, "POST", "/api/link-categories", map[string]any{"title": "Clubs", "style": StyleTiles, "rules": parents, "order": "1"})
	clubs := c.Model().Home.category(clubsID)
	if clubs == nil || clubs.Title != "Clubs" {
		t.Fatalf("no Clubs category at %s", clubsID)
	}
	if key, ok := id.Parse(clubs.ID); !ok || key != clubs.ID || len(clubs.Rules) != 1 {
		t.Fatalf("the new category = %+v", clubs)
	}
	categories := c.Model().Home.Categories
	if last := categories[len(categories)-1]; last.ID != clubsID {
		t.Errorf("the new category is not last: %+v", last)
	}
	for _, other := range categories[:len(categories)-1] {
		if store.CompareKeys(other.Order, clubs.Order) >= 0 {
			t.Errorf("the new category's key %q is not after %s's %q", clubs.Order, other.Title, other.Order)
		}
	}
	chessID := write(t, mux, homeAdmin, "POST", "/api/links", map[string]any{"title": "Chess", "url": "https://chess.example.org/", "category": clubsID, "visible": true, "rules": parents})
	links := c.Model().Home.category(clubsID).Links
	if len(links) != 1 || links[0].ID != chessID || links[0].Title != "Chess" || links[0].Category != clubsID || len(links[0].Rules) != 1 || links[0].AddedBy != homeAdmin {
		t.Fatalf("the new category's links = %+v", links)
	}
	if key, ok := id.Parse(chessID); !ok || key != chessID || key == clubsID {
		t.Fatalf("the new link's id = %q", chessID)
	}
	if _, err := checkOrder(links[0].Order); err != nil {
		t.Errorf("the first link in a category has no key: %q", links[0].Order)
	}
	if n := audienceRows(t, dir, thingLink+chessID); n != 1 {
		t.Errorf("the new link has %d audience rows, want 1", n)
	}
	lunchID := write(t, mux, homeAdmin, "POST", "/api/links", map[string]any{"title": "Lunch Menu", "url": "https://lunch.example.org/", "category": schoolID, "visible": true, "order": "1"})
	if got := linkTitles(c, schoolID); !slices.Equal(got, []string{"Directory", "Calendar", "Parent Portal", "Staff Room", "Lunch Menu"}) {
		t.Errorf("school after an add = %v, want the new link last", got)
	}
	if lunch := c.Model().Home.link(lunchID); store.CompareKeys(lunch.Order, c.Model().Home.link(staffRoomID).Order) <= 0 {
		t.Errorf("the new link's key %q is not after the last's", lunch.Order)
	}
	if rec := testkit.Call(t, mux, homeAdmin, "POST", "/api/links", map[string]any{"title": "Nowhere", "url": "https://example.org/", "category": "Clubs", "visible": true}); rec.Code != http.StatusBadRequest {
		t.Errorf("a link filed under a category's title: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, homeAdmin, "POST", "/api/links", map[string]any{"title": "Nowhere", "url": "https://example.org/", "category": EventsCategoryID, "visible": true}); rec.Code != http.StatusBadRequest {
		t.Errorf("a link filed under the events section: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/links", map[string]any{"title": "Mine", "url": "https://example.org/", "category": schoolID, "visible": true}); rec.Code != http.StatusForbidden {
		t.Errorf("a member's add: %d", rec.Code)
	}
}

func TestAnEditSendingOrderAloneWritesOneCell(t *testing.T) {
	c, dir, mux := homeServer(t)
	for _, step := range []struct {
		path, tab, key string
	}{
		{"/api/links/" + parentPortalID + "/edit", homeLinksTab, "Link ID=" + parentPortalID},
		{"/api/link-categories/" + chatsID + "/edit", homeCategoriesTab, "Category ID=" + chatsID},
		{"/api/home-widgets/school/edit", homeWidgetsTab, "Widget=school"},
	} {
		before := len(homeChangeLog(t, dir))
		write(t, mux, homeAdmin, "POST", step.path, map[string]any{"order": "1"})
		log := homeLog(t, dir, before)
		if want := []string{homeAdmin + "|set|" + step.tab + "|" + step.key + "|" + store.OrderColumn}; !slices.Equal(log, want) {
			t.Errorf("%s wrote %v, want %v", step.path, log, want)
		}
	}
	if got := linkTitles(c, schoolID); !slices.Equal(got, []string{"Parent Portal", "Directory", "Calendar", "Staff Room"}) {
		t.Errorf("school after the move = %v", got)
	}
	if got := c.Model().Home.Categories[0].ID; got != chatsID {
		t.Errorf("the first category after the move = %s", got)
	}
	if got := c.Model().Home.WidgetOrder; !slices.Equal(got, []string{"school", "when", "team", "celebrate", "birthday"}) {
		t.Errorf("widgets after the move = %v", got)
	}
	for _, bad := range []string{"", "a b"} {
		for _, path := range []string{"/api/links/" + parentPortalID + "/edit", "/api/link-categories/" + chatsID + "/edit", "/api/home-widgets/school/edit"} {
			if rec := testkit.Call(t, mux, homeAdmin, "POST", path, map[string]any{"order": bad}); rec.Code != http.StatusBadRequest {
				t.Errorf("%s with order %q: %d", path, bad, rec.Code)
			}
		}
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/links/"+parentPortalID+"/edit", map[string]any{"order": "1"}); rec.Code != http.StatusForbidden {
		t.Errorf("a member's move: %d", rec.Code)
	}
}

func TestALinkMovedToAnotherCategoryLandsLast(t *testing.T) {
	c, dir, mux := homeServer(t)
	before := len(homeChangeLog(t, dir))
	write(t, mux, homeAdmin, "POST", "/api/links/"+parentPortalID+"/edit", map[string]any{"category": chatsID, "order": "1"})
	chats := linkTitles(c, chatsID)
	if chats[len(chats)-1] != "Parent Portal" || slices.Contains(linkTitles(c, schoolID), "Parent Portal") {
		t.Errorf("chats after the move = %v", chats)
	}
	moved := c.Model().Home.link(parentPortalID)
	if moved.Category != chatsID || store.CompareKeys(moved.Order, c.Model().Home.link(jaysChatID).Order) <= 0 {
		t.Errorf("the moved link = %+v", moved)
	}
	log := homeLog(t, dir, before)
	slices.Sort(log)
	if want := []string{homeAdmin + "|set|" + homeLinksTab + "|Link ID=" + parentPortalID + "|Category", homeAdmin + "|set|" + homeLinksTab + "|Link ID=" + parentPortalID + "|" + store.OrderColumn}; !slices.Equal(log, want) {
		t.Errorf("the move wrote %v, want %v", log, want)
	}
	if n := audienceRows(t, dir, thingLink+parentPortalID); n != 1 {
		t.Errorf("the moved link has %d audience rows", n)
	}
}

func linksFor(t *testing.T, mux http.Handler, as string) (map[string]map[string]any, map[string]map[string]any) {
	t.Helper()
	out := decoded[envelope](t, testkit.Call(t, mux, as, "POST", "/api/query", map[string]any{
		"links":      map[string]string{"path": "/api/links"},
		"categories": map[string]string{"path": "/api/link-categories"},
	}))
	decode := func(kind string) map[string]map[string]any {
		all := map[string]map[string]any{}
		for key, raw := range out.Resources[kind] {
			var v map[string]any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			all[key] = v
		}
		return all
	}
	return decode("links"), decode("link-categories")
}

func TestOnlyAdminsGetTheRules(t *testing.T) {
	c, _, mux := homeServer(t)
	const ruth = "ruth.amari@heliosschool.org"
	if c.Model().AdminList("home").IsAdmin(ruth) {
		t.Fatal("ruth is an admin in the sample data")
	}
	rules := func(links, categories map[string]map[string]any) (sections, linked int) {
		for _, cat := range categories {
			got, _ := cat["rules"].([]any)
			sections += len(got)
		}
		for _, l := range links {
			got, _ := l["rules"].([]any)
			linked += len(got)
		}
		return
	}
	links, categories := linksFor(t, mux, ruth)
	if links[staffRoomID] == nil {
		t.Errorf("ruth's links %v leave out the Staff Room kept to staff", links)
	}
	if links[parentPortalID] != nil || links["hyp0000000006"] != nil {
		t.Errorf("ruth sees a parents' link or a hidden one")
	}
	if sections, linked := rules(links, categories); sections != 0 || linked != 0 {
		t.Errorf("a non-admin reads %d section and %d link rules", sections, linked)
	}
	links, categories = linksFor(t, mux, homeAdmin)
	if sections, linked := rules(links, categories); sections == 0 || linked == 0 {
		t.Errorf("an admin reads %d section and %d link rules", sections, linked)
	}
	if links[staffRoomID]["forMe"] != false || links[parentPortalID]["forMe"] != nil || links["hyp0000000006"]["visible"] != false {
		t.Errorf("an admin's view of links not for them: %v %v %v", links[staffRoomID], links[parentPortalID], links["hyp0000000006"])
	}
}

func TestAnAdminsAliasIsTheAdmin(t *testing.T) {
	c, dir, mux := homeServer(t)
	const alias, admin = "facilities@heliosschool.org", "hank.morrow@heliosschool.org"
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Insert(AdminsTab.Name, store.Row{"Email": admin})); err != nil {
		t.Fatal(err)
	}
	setAppVisibility(t, c, "celebrate", AppVisibilityRow{Mode: VisibleToList, Emails: []string{admin}})
	const token, event = "sample7feedtoken4hank5morrow", "gev0000000007"
	if err := c.Commit(context.Background(), access.System("test"), CalendarApp, store.Insert(FeedsTab, store.Row{"Token": token, "Email": admin, "Name": "Facilities", "Created": "2026-09-01 08:00"})); err != nil {
		t.Fatal(err)
	}
	if chosen := c.Model().Calendar.DefaultCalendar(admin); chosen != nil {
		t.Fatalf("the admin starts with a default calendar: %+v", chosen)
	}
	before := len(homeChangeLog(t, dir))
	if c.Model().AdminList("home").IsAdmin(alias) {
		t.Fatal("the alias is listed as an admin itself")
	}
	if hidden := c.Model().HiddenApps(alias); slices.Contains(hidden, "celebrate") {
		t.Errorf("the alias is kept from an app listed for the admin by address: %v", hidden)
	}
	if got := read(t, mux, alias, "/api/apps/celebrate").one(t, "apps", appKey("celebrate")); got["me"].(map[string]any)["listed"] != true {
		t.Errorf("the alias's celebrate = %v", got)
	}
	write(t, mux, alias, "POST", "/api/events/"+event+"/answer", map[string]string{"answer": "yes"})
	write(t, mux, alias, "POST", settingsPath("default"), map[string]string{"token": token})
	calendar := c.Model().Calendar
	if calendar.AnswerOf(admin, event) != AnswerYes || calendar.AnswerOf(alias, event) != "" {
		t.Errorf("the alias's yes is the admin's %q, the alias's own %q; want the admin's", calendar.AnswerOf(admin, event), calendar.AnswerOf(alias, event))
	}
	if chosen := calendar.DefaultCalendar(admin); chosen == nil || chosen.Token != token {
		t.Errorf("the alias's default did not become the admin's: %+v", chosen)
	}
	if got := defaultFeedOf(t, mux, alias); got != feedKey(admin, token) {
		t.Errorf("the alias's first calendar %s is not the admin's", got)
	}
	var me struct {
		Email      string   `json:"email"`
		Allowances []string `json:"allowances"`
	}
	if err := json.Unmarshal(testkit.Call(t, mux, alias, "GET", "/api/me", nil).Body.Bytes(), &me); err != nil || me.Email != admin {
		t.Errorf("the alias's /api/me is not the admin's: %+v %v", me, err)
	}
	if !slices.Contains(me.Allowances, ConfigureHome.Name) {
		t.Errorf("the alias does not hold the admin's allowances: %v", me.Allowances)
	}
	write(t, mux, alias, "POST", "/api/links/"+parentPortalID+"/edit", map[string]any{"order": "1"})
	if log := homeChangeLog(t, dir)[before:]; len(log) != 1 || log[0]["Actor"] != admin {
		t.Errorf("change log = %v, want one row by the admin the alias resolves to", log)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/links/"+parentPortalID+"/edit", map[string]any{"order": "2"}); rec.Code != http.StatusForbidden {
		t.Errorf("a member's move: %d, want 403", rec.Code)
	}
}

func widgetsFor(t *testing.T, mux http.Handler, as string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, w := range listOf(t, mux, as, "/api/home-widgets") {
		out[w["key"].(string)] = w
	}
	return out
}

func shownTo(w map[string]any) bool {
	me, _ := w["me"].(map[string]any)
	return me["shown"] == true
}

func TestWidgetAudience(t *testing.T) {
	_, dir, mux := homeServer(t)
	parentsOnly := []Rule{{Kind: RuleInclude, Roles: []string{"Parent"}}}
	before := len(homeChangeLog(t, dir))
	write(t, mux, homeAdmin, "POST", "/api/home-widgets/team/edit", map[string]any{"rules": parentsOnly})
	if log := homeLog(t, dir, before); len(log) == 0 || slices.ContainsFunc(log, func(line string) bool { return !strings.Contains(line, "|"+homeAudienceTab+"|") }) {
		t.Errorf("a rules edit wrote %v, want the Audience tab alone", log)
	}
	if rec := testkit.Call(t, mux, homeAdmin, "POST", "/api/home-widgets/nope/edit", map[string]any{"rules": parentsOnly}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown widget: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/home-widgets/team/edit", map[string]any{"rules": []Rule{}}); rec.Code != http.StatusForbidden {
		t.Errorf("a member's edit: %d", rec.Code)
	}
	student := widgetsFor(t, mux, "sam.whitfield@heliosschool.org")
	if len(student) != len(HomeWidgets) || shownTo(student["team"]) || !shownTo(student["when"]) || student["team"]["rules"] != nil {
		t.Errorf("a student's widgets: %+v", student)
	}
	if parent := widgetsFor(t, mux, "robin.whitfield@heliosschool.org"); !shownTo(parent["team"]) {
		t.Errorf("a parent's widgets: %+v", parent)
	}
	got, _ := widgetsFor(t, mux, homeAdmin)["team"]["rules"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["roles"].([]any)[0] != "Parent" {
		t.Errorf("an admin's team rules: %+v", got)
	}
	keys := []string{}
	for _, w := range listOf(t, mux, homeAdmin, "/api/home-widgets") {
		keys = append(keys, w["key"].(string))
		if w["order"] == "" || w["order"] == nil {
			t.Errorf("a widget without its key: %v", w)
		}
	}
	if !slices.Equal(keys, []string{"when", "team", "celebrate", "school", "birthday"}) {
		t.Errorf("widgets in order = %v", keys)
	}
	if got := read(t, mux, homeAdmin, "/api/home-widgets/"+widgetKey("school")).one(t, "home-widgets", widgetKey("school")); got["key"] != "school" {
		t.Errorf("a widget by its ID = %v", got)
	}
}
