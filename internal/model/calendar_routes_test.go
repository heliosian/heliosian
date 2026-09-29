package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

var testNow = time.Date(2026, 9, 17, 12, 0, 0, 0, Location)

func pinnedClock() time.Time {
	return testNow
}

var sampleRoot string

func TestMain(m *testing.M) {
	now = pinnedClock
	root, err := filepath.Abs("../../sampledata")
	if err != nil {
		panic(err)
	}
	sampleRoot = root
	os.Exit(m.Run())
}

var (
	sheet *data.Dir
	queue *store.Queue
)

func sheetTables(t *testing.T) store.Tables {
	t.Helper()
	queue.Flush()
	return readTables(t, sheet)
}

func sampleCalendarCache(t *testing.T) *CalendarCache {
	t.Helper()
	t.Chdir("../..")
	sheet = &data.Dir{Root: "sampledata"}
	queue = store.NewQueue()
	cache, err := NewCalendarCache(sheet, sheet, func() Roster { return roster }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func testApp(t *testing.T) (http.Handler, *CalendarCache) {
	t.Helper()
	handler, cache, _ := testAppHooks(t)
	return handler, cache
}

func testAppHooks(t *testing.T) (http.Handler, *CalendarCache, CalendarHooks) {
	t.Helper()
	cache := sampleCalendarCache(t)
	d := calendarDirectory(t, "sampledata")
	parties, activities := linkedCaches(t, nil, nil)
	lists := linkedEmailLists(t, nil)
	mux := http.NewServeMux()
	hooks := RegisterCalendar(mux, CalendarDeps{
		Cache:      cache,
		Images:     memoryImages(),
		Directory:  func() *Directory { return d },
		Settings:   func() *Config { return &Config{} },
		Parties:    parties,
		Activities: activities,
		EmailLists: lists,
		Mail:       CalendarMail{Sender: keptMail().Mailgun},
		Style:      testStyle,
		Queue:      queue,
	})
	return served(mux, cache, hooks, func() *Directory { return d }, parties, activities, lists), cache, hooks
}

func memoryImages() blob.Images {
	return blob.NewImages(blob.New(blob.NewMemoryBucket()), "when", "celebrate", "team")
}

var (
	celebrateLinkedTabs = []string{partiesTab, hostsTab, ticketsTab, AdminsTab.Name, id.AliasesTab}
	teamLinkedTabs      = []string{activitiesTab, volunteersTab, id.AliasesTab}
)

func linkedCaches(t *testing.T, celebrate, team store.Tables) (*PartiesCache, *ActivitiesCache) {
	t.Helper()
	for _, tab := range celebrateLinkedTabs {
		replaceRows(t, partiesAppName, tab, celebrate[tab])
	}
	for _, tab := range teamLinkedTabs {
		replaceRows(t, activitiesAppName, tab, team[tab])
	}
	_, categories, err := sheet.Table(activitiesAppName, activityCategoriesTab)
	if err != nil {
		t.Fatal(err)
	}
	replaceRows(t, activitiesAppName, activityCategoriesTab, slices.DeleteFunc(categories, func(row store.Row) bool { return row["Event ID"] != "" }))
	parties, err := NewPartiesCache(sheet, sheet, testkit.All, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	activities, err := NewActivitiesCache(sheet, sheet, testkit.All, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return parties, activities
}

func linkedEmailLists(t *testing.T, groups store.Tables) *EmailListsCache {
	t.Helper()
	for _, tab := range append([]string{groupsTab, id.AliasesTab}, groupTabs...) {
		replaceRows(t, emailListsAppName, tab, groups[tab])
	}
	lists, err := NewEmailListsCache(sheet, sheet, func() []string { return nil }, queue, sampleKey)
	if err != nil {
		t.Fatal(err)
	}
	return lists
}

func replaceRows(t *testing.T, app, tab string, rows []store.Row) {
	t.Helper()
	_, old, err := sheet.Table(app, tab)
	if err != nil {
		t.Fatal(err)
	}
	if len(old) > 0 {
		if err := sheet.Delete(app, tab, map[string]string{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(rows) == 0 {
		return
	}
	if err := sheet.Insert(app, tab, rows); err != nil {
		t.Fatal(err)
	}
}

func calendarOver(t *testing.T, parties *PartiesCache, activities *ActivitiesCache, lists *EmailListsCache, directory func() *Directory) CalendarHooks {
	t.Helper()
	cache, err := NewCalendarCache(sheet, sheet, func() Roster { return roster }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return RegisterCalendar(http.NewServeMux(), CalendarDeps{
		Cache:      cache,
		Images:     memoryImages(),
		Directory:  directory,
		Settings:   noSettings,
		Parties:    parties,
		Activities: activities,
		EmailLists: lists,
		Mail:       CalendarMail{Sender: mailtest.Discard()},
		Style:      testStyle,
		Queue:      queue,
	})
}

var testStyle = CalendarCardStyle(func() string { return "Helios When" }, func() string { return "The school year, day by day" })

func changeLog(t *testing.T) []string {
	t.Helper()
	return testkit.ChangeLines(t, sheet, queue, CalendarApp)
}

func as(email string, handler http.Handler) http.Handler {
	return auth.Fixed(email, handler)
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "https://calendar.heliosiandev.com:8080"+path, strings.NewReader(body))
	req.Host = "calendar.heliosiandev.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestFeedRoute(t *testing.T) {
	mux, _ := testApp(t)
	rec := call(t, mux, http.MethodGet, "/open/feed/sample7feedtoken4jordan2whitfield.ics", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if body := rec.Body.String(); !strings.Contains(body, "X-WR-CALNAME:Whitfield school days") || !strings.Contains(body, "URL:https://calendar.heliosiandev.com:8080/e/gev0000000005") {
		t.Errorf("feed body: %s", body)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/nosuchtoken.ics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown token: %d", rec.Code)
	}
}

func TestFeedLifecycle(t *testing.T) {
	mux, cache := testApp(t)
	owner := as("jordan.whitfield@heliosschool.org", mux)
	other := as("mia.torres@heliosschool.org", mux)
	trip := `"tags":[` + quoted(t, "Trip") + `]`
	rec := call(t, owner, http.MethodPost, "/api/calendar-feeds", `{"name":"Just trips","classrooms":["Jays"],`+trip+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	key := created(t, rec)
	made := feedOf(t, owner, key)
	if len(made.Token) != 24 || made.URL != "/open/feed/"+made.Token+".ics" || key != feedKey("jordan.whitfield@heliosschool.org", made.Token) {
		t.Fatalf("add answered %s, the feed %+v", rec.Body.String(), made)
	}
	if len(cache.Model().Feeds) != 2 || cache.Model().Feed(made.Token).Email != "jordan.whitfield@heliosschool.org" {
		t.Fatalf("feeds after add: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SUMMARY:Jays and Ravens Camping") || strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("new feed: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(t, owner, http.MethodPost, "/api/calendar-feeds", `{"name":"just TRIPS","classrooms":["Jays"],`+trip+`}`)
	twin := created(t, rec)
	if twinFeed := feedOf(t, owner, twin); rec.Code != http.StatusOK || cache.Model().Feed(twinFeed.Token).Name != "just TRIPS 2" {
		t.Errorf("twin name: %d %+v", rec.Code, twinFeed)
	}
	call(t, owner, http.MethodDelete, "/api/calendar-feeds/"+twin, "")
	if rec := call(t, owner, http.MethodPost, "/api/calendar-feeds", `{"name":"Bad","classrooms":["Penguins"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown classroom: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/calendar-feeds", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", rec.Code)
	}
	if rec := call(t, other, http.MethodPost, "/api/calendar-feeds/"+key+"/edit", `{"name":"Theirs","classrooms":["Jays"],`+trip+`}`); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's change: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/calendar-feeds/"+key+"/edit", `{"name":"","classrooms":["Jays"],`+trip+`}`); rec.Code != http.StatusBadRequest {
		t.Errorf("change to no name: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/calendar-feeds/"+key+"/edit", `{"name":"Jays days","emoji":" 🚌 ","classrooms":["Jays"],"tags":[`+quoted(t, "Trip, Schedule")+`]}`); rec.Code != http.StatusNoContent {
		t.Errorf("owner's change: %d %s", rec.Code, rec.Body.String())
	}
	if f := cache.Model().Feed(made.Token); f == nil || f.Name != "Jays days" || f.Emoji != "🚌" || cells.JoinList(f.Tags) != calendarIDs(t, "Trip, Schedule") || f.Email != "jordan.whitfield@heliosschool.org" {
		t.Errorf("feed after change: %+v", f)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "X-WR-CALNAME:Jays days") || !strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("changed feed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, other, http.MethodDelete, "/api/calendar-feeds/"+key, ""); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's removal: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodDelete, "/api/calendar-feeds/"+key, ""); rec.Code != http.StatusNoContent {
		t.Errorf("owner's removal: %d %s", rec.Code, rec.Body.String())
	}
	if len(cache.Model().Feeds) != 1 || cache.Model().Feed(made.Token) != nil {
		t.Errorf("feeds after removal: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("removed feed still serves: %d", rec.Code)
	}
}

func TestCalendarRead(t *testing.T) {
	mux, _ := testApp(t)
	view := calendarOf(t, as("jordan.whitfield@heliosschool.org", mux))
	if view.User.Email != "jordan.whitfield@heliosschool.org" || len(view.Feeds) != 2 || !slices.ContainsFunc(view.Feeds, func(f Feed) bool { return f.Locked }) || len(view.Events) != 22 || len(view.Classrooms) != 9 {
		t.Errorf("view = user %s, %d feeds, %d events, %d classrooms", view.User.Email, len(view.Feeds), len(view.Events), len(view.Classrooms))
	}
}

func TestSharePreview(t *testing.T) {
	handler, _, hooks := testAppHooks(t)
	testkit.Previews(t, hooks.PreviewHead(), testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/e/gev0000000007",
		Want: []string{`property="og:title" content="International Night"`, `Thursday, September 24 · 4:00 – 6:00 PM`, `content="https://when.heliosiandev.com:8080/open/share/gev0000000007.png"`},
	}, testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/mine",
		Want: []string{`og:title" content="Helios When"`, "One calendar", "/open/share/upcoming.png"},
	})
	testkit.Cards(t, handler, []string{"/open/share/gev0000000007.png", "/open/share/upcoming.png"}, "/open/share/nope.png")
}

func TestOldIDsReachTheirEvents(t *testing.T) {
	handler, _, hooks := testAppHooks(t)
	jordan := as("jordan.whitfield@heliosschool.org", handler)
	for old, want := range map[string]string{"7QK2M4XN": "evt0000000001", "3pl9w2zc": "evt0000000002", "8RV4T6PK": "evt0000000003"} {
		rec := eventOf(t, jordan, old)
		var got struct{ Result string }
		json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != 200 || got.Result != want {
			t.Errorf("the event by its old ID %s: %d %s", old, rec.Code, got.Result)
		}
	}
	for path, want := range map[string]string{"/e/7QK2M4XN": "/e/evt0000000001", "/events/3PL9W2ZC": "/e/evt0000000002", "/e/8rv4t6pk?from=mail": "/e/evt0000000003?from=mail"} {
		if rec := call(t, handler, "GET", path, ""); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want a 301 to %s", path, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	for _, path := range []string{"/e/evt0000000001", "/events/evt0000000002", "/e/gev0000000007", "/e/no-such-event"} {
		if rec := call(t, handler, "GET", path, ""); rec.Code != 200 {
			t.Errorf("%s: %d, want the page", path, rec.Code)
		}
	}
	testkit.Cards(t, handler, []string{"/open/share/3PL9W2ZC.png", "/open/share/evt0000000002.png"}, "/open/share/3PL9W2ZD.png")
	testkit.Previews(t, hooks.PreviewHead(), testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/e/3PL9W2ZC",
		Want: []string{`property="og:title" content="HCA Meeting"`, `content="https://when.heliosiandev.com:8080/open/share/evt0000000002.png"`},
	})
	if rec := act(t, jordan, "3PL9W2ZC", "settings", `{"flyer":"sample/community.jpg"}`); rec.Code != 204 {
		t.Fatalf("a flyer by the old ID: %d %s", rec.Code, rec.Body)
	}
	for _, row := range sheetTables(t)[InvitationsTab] {
		if strings.EqualFold(row["Event ID"], "3PL9W2ZC") {
			t.Errorf("a write by the old ID made a row under it: %v", row)
		}
		if row["Event ID"] == "evt0000000002" && row["Flyer"] != "sample/community.jpg" {
			t.Errorf("the flyer did not reach the event's row: %v", row)
		}
	}
	for _, path := range []string{"/open/flyer/3PL9W2ZC", "/open/flyer/evt0000000002"} {
		if rec := call(t, handler, "GET", path, ""); rec.Code != 200 {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

func TestProvenanceForAdmins(t *testing.T) {
	handler, _ := testApp(t)
	view := calendarOf(t, as("jordan.whitfield@heliosschool.org", handler))
	if view.Provenance != nil {
		t.Errorf("a parent sees provenance: %v", view.Provenance)
	}
	var hand *Event
	for _, e := range view.Events {
		if e.ID == "evt0000000001" {
			hand = e
		}
	}
	if hand == nil || hand.AddedBy != "dana.hawkins@heliosschool.org" || hand.Added == "" {
		t.Errorf("hand-added event = %+v", hand)
	}
	view = calendarOf(t, as("dana.hawkins@heliosschool.org", handler))
	p := view.Provenance["gev0000000005"]
	if p == nil || p.Model == "" || len(p.Corrected) == 0 || p.Note != "Say which classrooms" {
		t.Errorf("admin provenance for gev0000000005 = %+v", p)
	}
}

func TestSavedView(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	rec := call(t, as(me, handler), "POST", settingsPath("save-view"), `{"classrooms":["Hawks"],"tags":[`+quoted(t, "Community, HCA")+`,"Nonsense"]}`)
	if rec.Code != 204 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	view := calendarOf(t, as(me, handler))
	if view.User.Saved == nil || strings.Join(view.User.Saved.Classrooms, ",") != "Hawks" || cells.JoinList(view.User.Saved.Tags) != calendarIDs(t, "Community, HCA") {
		t.Errorf("saved view = %+v", view.User.Saved)
	}
	for _, u := range cache.Model().UpcomingUnder(calendarDirectory(t, "sampledata"), me, nil, now(), 0, "") {
		if !strings.Contains(u.Title, "Hawks") && !strings.Contains(u.Title, "CAFE") && u.Title != "International Night" && u.Title != "Halloween Parade" && u.Title != "HCA Meeting" && u.Title != "All School Movie Night" && u.Title != "Cocoa & Cookies" && u.Title != "Talent Show" && u.Title != "Back to School Social" && u.Title != "Spring Celebration" && u.Title != "Fall Potluck at the Torres'" && u.Title != "Jays & Ravens Beach Picnic" {
			t.Errorf("upcoming under the saved view lists %q", u.Title)
		}
	}
	rec = call(t, as(me, handler), "POST", settingsPath("forget-view"), "")
	if rec.Code != 204 {
		t.Fatalf("forget: %d %s", rec.Code, rec.Body)
	}
	again := calendarOf(t, as(me, handler))
	if again.User.Saved != nil {
		t.Errorf("saved view survives forgetting: %+v", again.User.Saved)
	}
}

func TestDefaultCalendar(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	dir := calendarDirectory(t, "sampledata")
	found := false
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian, upcoming leaves out International Night")
	}
	if rec := call(t, viewer, "POST", settingsPath("default"), `{"token":"nonsense"}`); rec.Code != 400 {
		t.Errorf("a stranger's token: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", settingsPath("default"), `{"token":"sample7feedtoken4jordan2whitfield"}`); rec.Code != 204 {
		t.Fatalf("default: %d %s", rec.Code, rec.Body)
	}
	if chosen := cache.Model().DefaultCalendar(me); chosen == nil || chosen.Token != "sample7feedtoken4jordan2whitfield" {
		t.Errorf("default calendar = %+v", chosen)
	}
	if mine := cache.Model().MyCalendars(me); len(mine) != 2 || mine[0].Token != "sample7feedtoken4jordan2whitfield" || !mine[1].Locked {
		t.Errorf("rail = %+v", mine)
	}
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		if u.Title == "International Night" {
			t.Errorf("under the saved calendar, upcoming lists %q", u.Title)
		}
	}
	found = false
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, MyHeliosianToken) {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian by token, upcoming leaves out International Night")
	}
	inMonth := func(token string) bool {
		for _, u := range cache.Model().MonthUnder(dir, me, nil, now(), "2026-09", token).Events {
			if u.Title == "International Night" {
				return true
			}
		}
		return false
	}
	if inMonth("") || !inMonth(MyHeliosianToken) {
		t.Errorf("month under default %v, under My Heliosian %v; want false, true", inMonth(""), inMonth(MyHeliosianToken))
	}
	if rec := call(t, viewer, "POST", settingsPath("default"), `{"token":"`+MyHeliosianToken+`"}`); rec.Code != 204 || cache.Model().DefaultCalendar(me) != nil {
		t.Errorf("back to My Heliosian: %d, default %+v", rec.Code, cache.Model().DefaultCalendar(me))
	}
	if rec := call(t, viewer, "POST", "/api/calendar-feeds/"+feedKey(me, MyHeliosianToken)+"/edit", `{"name":"Home base","emoji":"🏠","classrooms":["Jays"],"tags":[`+quoted(t, "Trip")+`]}`); rec.Code != 204 {
		t.Errorf("rename My Heliosian: %d %s", rec.Code, rec.Body)
	}
	if home := cache.Model().MyHeliosian(me); home.Name != "Home base" || home.Emoji != "🏠" || !home.Locked || len(home.Classrooms) != 0 {
		t.Errorf("My Heliosian = %+v", home)
	}
	made := feedOf(t, viewer, created(t, call(t, viewer, "POST", "/api/calendar-feeds", `{"name":"Everything","classrooms":[],"tags":[]}`)))
	if rec := call(t, viewer, "POST", settingsPath("order-feeds"), `{"tokens":["`+made.Token+`"]}`); rec.Code != 400 {
		t.Errorf("a short order: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", settingsPath("order-feeds"), `{"tokens":["`+made.Token+`","`+MyHeliosianToken+`","sample7feedtoken4jordan2whitfield"]}`); rec.Code != 204 {
		t.Fatalf("order: %d %s", rec.Code, rec.Body)
	}
	mine := cache.Model().MyCalendars(me)
	if len(mine) != 3 || mine[0].Token != made.Token || mine[1].Token != MyHeliosianToken || mine[1].Name != "Home base" {
		t.Errorf("after ordering: %+v", mine)
	}
	if chosen := cache.Model().DefaultCalendar(me); chosen == nil || chosen.Token != made.Token {
		t.Errorf("the first is not the default: %+v", chosen)
	}
}

func keptMail() *mailtest.Recorder {
	return mailtest.NewRecorder("Helios When <when@example.org>")
}

func TestAdminsToldOfSharedEvents(t *testing.T) {
	cache := sampleCalendarCache(t)
	d := calendarDirectory(t, "sampledata")
	kept := keptMail()
	parties, activities := linkedCaches(t, nil, nil)
	lists := linkedEmailLists(t, nil)
	mux := http.NewServeMux()
	hooks := RegisterCalendar(mux, CalendarDeps{
		Cache:      cache,
		Images:     memoryImages(),
		Directory:  func() *Directory { return d },
		Settings:   func() *Config { return &Config{} },
		Parties:    parties,
		Activities: activities,
		EmailLists: lists,
		Mail:       CalendarMail{Sender: kept.Mailgun, Base: "https://when.heliosian.com"},
		Style:      testStyle,
		Queue:      queue,
	})
	served(mux, cache, hooks, func() *Directory { return d }, parties, activities, lists)
	parent := as("jordan.whitfield@heliosschool.org", mux)
	admin := as("dana.hawkins@heliosschool.org", mux)
	wait := func(n int) []mail.Message {
		for i := 0; i < 50 && len(kept.Messages()) < n; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		return kept.Messages()
	}
	call(t, parent, "POST", "/api/events", `{"title":"Bake sale","start":"2026-10-01 15:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Public"}`)
	sent := wait(1)
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Event to approve: Bake sale") || !slices.Contains(sent[0].To, "dana.hawkins@heliosschool.org") || !strings.Contains(sent[0].Text, "Jordan") || !strings.Contains(sent[0].HTML, "Review the event") {
		t.Errorf("public event mail = %+v", sent)
	}
	call(t, parent, "POST", "/api/events", `{"title":"Sam\u2019s party","start":"2026-10-03 14:00","tags":[`+quoted(t, "Jays")+`],"sharing":"Link"}`)
	sent = wait(2)
	if len(sent) != 2 || !strings.HasPrefix(sent[1].Subject, "Link event added: Sam") || !strings.Contains(sent[1].HTML, "See the event") || strings.Contains(sent[1].Text, "waiting for approval") {
		t.Errorf("link event mail = %+v", sent)
	}
	call(t, admin, "POST", "/api/events", `{"title":"Admin's own","start":"2026-10-05","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`)
	sent = wait(3)
	if len(sent) != 3 || !strings.HasPrefix(sent[2].Subject, "Event to approve: Admin's own") {
		t.Errorf("an admin's own public event waits and is mailed about too: %+v", sent)
	}
	v := calendarOf(t, parent)
	var party *Event
	for _, e := range v.Events {
		if strings.HasPrefix(e.Title, "Sam") {
			party = e
		}
	}
	if party == nil {
		t.Fatal("the party is not in the host's view")
	}
	if rec := act(t, parent, party.ID, "edit", `{"sharing":"Public"}`); rec.Code != 204 {
		t.Fatalf("switch to public: %d %s", rec.Code, rec.Body)
	}
	sent = wait(4)
	if e := cache.Model().Event(party.ID); e == nil || !e.Pending || len(sent) != 4 || !strings.HasPrefix(sent[3].Subject, "Event to approve: Sam") {
		t.Errorf("after the switch: event %+v, mail %d", e, len(sent))
	}
	other := as("robin.whitfield@heliosschool.org", mux)
	if rec := eventOf(t, other, party.ID); rec.Code != 200 {
		t.Errorf("the link of an event waiting for approval: %d", rec.Code)
	}
	if rec := act(t, other, party.ID, "answer", `{"answer":"yes"}`); rec.Code != 204 {
		t.Errorf("a yes by link while waiting: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminAddsAndCorrects(t *testing.T) {
	handler, cache, hooks := testAppHooks(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	rec := call(t, admin, "POST", "/api/events", `{"title":"Chess Club","start":"2026-10-01 15:30","end":"2026-10-01 16:30","tags":[`+quoted(t, "Jays, Clubs")+`],"sharing":"Public","repeatWeeks":4,"repeatTimes":2}`)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var made struct{ IDs []string }
	for _, e := range cache.Model().Pending {
		if e.Title == "Chess Club" {
			made.IDs = append(made.IDs, e.ID)
		}
	}
	if len(made.IDs) != 3 || made.IDs[0] != created(t, rec) {
		t.Fatalf("ids = %v, the first answered %s", made.IDs, rec.Body)
	}
	for _, id := range made.IDs {
		if rec := act(t, admin, id, "approve", ""); rec.Code != 204 {
			t.Fatalf("approve %s: %d", id, rec.Code)
		}
	}
	for _, key := range made.IDs {
		if parsed, ok := id.Parse(key); !ok || parsed != key {
			t.Errorf("a minted event ID does not parse: %q", key)
		}
	}
	starts := []string{}
	for _, id := range made.IDs {
		e := cache.Model().Event(id)
		if e == nil || e.Source != SourceSheet || e.AddedBy != "dana.hawkins@heliosschool.org" {
			t.Fatalf("event %s = %+v", id, e)
		}
		starts = append(starts, e.Start)
	}
	if strings.Join(starts, " ") != "2026-10-01 15:30 2026-10-29 15:30 2026-11-26 15:30" {
		t.Errorf("starts = %v", starts)
	}
	if rec := call(t, admin, "POST", "/api/events", `{"title":"Bad","start":"not a date","tags":[`+quoted(t, "Clubs")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("bad date accepted: %d", rec.Code)
	}
	rec = act(t, admin, made.IDs[0], "keywords", `{"keywords":["chess","board games"]}`)
	if rec.Code != 204 || strings.Join(cache.Model().Event(made.IDs[0]).Keywords, ",") != "chess,board games" {
		t.Errorf("keywords: %d %v", rec.Code, cache.Model().Event(made.IDs[0]).Keywords)
	}
	if p := cache.Model().Provenance[made.IDs[0]]; p == nil || strings.Join(p.Corrected, ",") != "Keywords" {
		t.Errorf("provenance = %+v", p)
	}
	rec = act(t, admin, made.IDs[0], "move", `{"start":"2026-10-02 16:00","end":"2026-10-02 17:00"}`)
	if rec.Code != 204 || cache.Model().Event(made.IDs[0]).Start != "2026-10-02 16:00" {
		t.Errorf("move: %d %s", rec.Code, cache.Model().Event(made.IDs[0]).Start)
	}
	if rec := act(t, admin, "gev0000000007", "move", `{"start":"2026-10-02 16:00"}`); rec.Code != 403 {
		t.Errorf("an imported event moved from the list: %d", rec.Code)
	}
	parent := as("jordan.whitfield@heliosschool.org", handler)
	if rec := act(t, parent, made.IDs[0], "keywords", `{"keywords":["x"]}`); rec.Code != 403 {
		t.Errorf("parent set keywords: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/events", `{"title":"Bake sale","start":"2026-10-01 15:00","end":"2026-10-01 17:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Public","repeatWeeks":1,"repeatTimes":3}`)
	shared := created(t, rec)
	if rec.Code != 200 || slices.ContainsFunc(cache.Model().Pending, func(e *Event) bool { return e.Title == "Bake sale" && e.ID != shared }) {
		t.Fatalf("parent shared an event: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared); e == nil || !e.Pending || !slices.Contains(cache.Model().Pending, e) {
		t.Errorf("shared event = %+v", e)
	}
	if cache.Model().AnswerOf("jordan.whitfield@heliosschool.org", shared) != AnswerYes {
		t.Errorf("the host is not going to their own event")
	}
	hostView := calendarOf(t, parent)
	if n := len(slices.DeleteFunc(slices.Clone(hostView.Events), func(e *Event) bool { return e.ID != shared })); n != 1 {
		t.Errorf("the host's view carries their event %d times", n)
	}
	for _, e := range cache.Model().Events {
		if e.ID == shared {
			t.Errorf("a pending event is on the calendar")
		}
	}
	sees := func(who http.Handler) bool {
		return slices.ContainsFunc(calendarOf(t, who).Events, func(e *Event) bool { return e.ID == shared && (e.Pending || e.Declined) })
	}
	other := as("robin.whitfield@heliosschool.org", handler)
	if !sees(parent) || !sees(admin) || sees(other) {
		t.Errorf("pending event seen by parent %v, admin %v, another %v", sees(parent), sees(admin), sees(other))
	}
	if rec := act(t, parent, shared, "approve", ""); rec.Code != 403 {
		t.Errorf("parent approved: %d", rec.Code)
	}
	if rec := act(t, admin, shared, "approve", ""); rec.Code != 204 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared); e == nil || e.Pending || e.Status != StatusApproved || !slices.Contains(cache.Model().Events, e) {
		t.Errorf("approved event = %+v", e)
	}
	if rec := act(t, parent, shared, "edit", `{"title":"Bake sale!","start":"2026-10-01 15:30","end":"2026-10-01 17:30","location":"Gym","image":"/category-images/cake.jpg"}`); rec.Code != 204 {
		t.Errorf("owner's edit: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared); e == nil || e.Title != "Bake sale!" || e.Location != "Gym" || e.Image != "/category-images/cake.jpg" || e.Start != "2026-10-01 15:30" || e.Pending {
		t.Errorf("edited event = %+v", e)
	}
	if rec := act(t, other, shared, "edit", `{"title":"Mine now"}`); rec.Code != 403 {
		t.Errorf("another parent's edit: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/events", `{"title":"Sam\u2019s birthday","start":"2026-10-03 14:00","end":"2026-10-03 16:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Link"}`)
	shared = created(t, rec)
	if rec.Code != 200 {
		t.Fatalf("link: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared); e == nil || e.Pending || e.Sharing != SharingLink || e.Status != "" {
		t.Errorf("link event = %+v", e)
	}
	if sees(other) {
		t.Errorf("a link event is on another's calendar before they answer")
	}
	if rec := eventOf(t, other, shared); rec.Code != 200 {
		t.Errorf("a link event by its link: %d", rec.Code)
	}
	if rec := act(t, other, shared, "answer", `{"answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("yes to a link event: %d %s", rec.Code, rec.Body)
	}
	otherView := calendarOf(t, other)
	if i := slices.IndexFunc(otherView.Events, func(e *Event) bool { return e.ID == shared }); i < 0 || !slices.Contains(otherView.Events[i].Tags, TagGoing) {
		t.Errorf("a yes did not put the link event under Going on the other's calendar")
	}
	found := false
	for _, u := range cache.Model().UpcomingUnder(calendarDirectory(t, "sampledata"), "robin.whitfield@heliosschool.org", nil, now(), 0, "") {
		found = found || u.ID == shared
	}
	if !found {
		t.Errorf("a yes did not put the link event in the other's Upcoming")
	}
	if rec := call(t, parent, "POST", "/api/events", `{"title":"Unsaid","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`]}`); rec.Code != 400 {
		t.Errorf("no sharing: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/events", `{"title":"Unsaid","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Private"}`); rec.Code != 400 {
		t.Errorf("an old sharing word: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/events", `{"address":"Sams-Party","title":"Sam\u2019s party","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Link"}`)
	shared = created(t, rec)
	party := cache.Model().Event("sams-party")
	if rec.Code != 200 || party == nil || party.ID != shared || party.Address != "sams-party" || EventPath(party) != "/e/sams-party" {
		t.Fatalf("chosen address: %d %s, event %+v", rec.Code, rec.Body, party)
	}
	if _, ok := id.Parse(party.ID); !ok {
		t.Errorf("an event with an address is still minted an ID: %q", party.ID)
	}
	if rec := call(t, handler, "GET", "/open/share/sams-party.png", ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("a link event's card: %d", rec.Code)
	}
	if head := hooks.PreviewHead()(httptest.NewRequest("GET", "https://when.heliosiandev.com:8080/e/sams-party", nil)); !strings.Contains(head, "Sam") || !strings.Contains(head, "/open/share/"+party.ID+".png") {
		t.Errorf("a link event's preview:\n%s", head)
	}
	if rec := call(t, parent, "POST", "/api/events", `{"address":"sams-party","title":"Again","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("a taken address: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/events", `{"address":"`+party.ID+`","title":"Again","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an address that is another event's ID: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/events", `{"address":"a/b","title":"Odd","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an ill-formed address: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/events", `{"address":"evt9999999999","title":"Odd","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an address that reads as an ID: %d", rec.Code)
	}
	shared = created(t, call(t, parent, "POST", "/api/events", `{"title":"Not this","start":"2026-10-02","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`))
	if rec := act(t, admin, shared, "decline", ""); rec.Code != 204 {
		t.Errorf("decline: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared); e == nil || !e.Declined || e.Status != StatusDeclined || slices.Contains(cache.Model().Events, e) {
		t.Errorf("declined event = %+v", e)
	}
	if !sees(parent) || sees(other) {
		t.Errorf("declined event seen by parent %v, another %v", sees(parent), sees(other))
	}
	if rec := eventOf(t, other, shared); rec.Code != 200 {
		t.Errorf("a declined event by its link: %d", rec.Code)
	}
	if rec := act(t, other, shared, "answer", `{"answer":"yes"}`); rec.Code != 204 || !sees(other) {
		t.Errorf("yes to a declined event: %d, on the calendar %v", rec.Code, sees(other))
	}
	if rec := act(t, admin, "sams-party", "decline", ""); rec.Code != 403 || cache.Model().Event("sams-party").Declined {
		t.Errorf("declined a link event: %d", rec.Code)
	}
	if rec := act(t, admin, shared, "approve", ""); rec.Code != 204 || cache.Model().Event(shared).Status != StatusApproved {
		t.Errorf("approve after decline: %d", rec.Code)
	}
}

func TestFeedTags(t *testing.T) {
	handler, _ := testApp(t)
	me := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, me, "POST", "/api/calendar-feeds", `{"name":"Odds and ends","classrooms":["Jays"],"tags":[`+quoted(t, "Misc, Trip")+`]}`); rec.Code != 200 {
		t.Errorf("feed with Misc: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/calendar-feeds", `{"name":"Parties","classrooms":["Jays"],"tags":[`+quoted(t, "Celebrate, Going")+`]}`); rec.Code != 200 {
		t.Errorf("feed with Celebrate and Going: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/calendar-feeds", `{"name":"Nope","classrooms":["Jays"],"tags":["Nonsense"]}`); rec.Code != 400 {
		t.Errorf("feed with a made-up tag: %d", rec.Code)
	}
}

func TestFeedCarriesLinked(t *testing.T) {
	handler, cache := testApp(t)
	linked := []Linked{{Source: SourceCelebrate, ID: "pty0000000009", EventID: "pty0000000009", Title: "Fondue Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineGoing}}
	f := &Feed{Token: "t", Email: "jordan.whitfield@heliosschool.org", Name: "Mine", Tags: []string{TagGoing}}
	out := string(ICS(cache.Model(), calendarDirectory(t, "sampledata"), f, linked, "https://when.heliosiandev.com:8080", now()))
	if !strings.Contains(out, "SUMMARY:Fondue Night") || !strings.Contains(out, "URL:https://when.heliosiandev.com:8080/e/pty0000000009") {
		t.Errorf("feed lacks the party:\n%s", out)
	}
	if strings.Contains(out, "SUMMARY:Halloween Parade") {
		t.Errorf("a Going feed carries an event the family is not in")
	}
	_ = handler
}

func TestAnswers(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":"perhaps"}`); rec.Code != 400 {
		t.Errorf("nonsense answer: %d", rec.Code)
	}
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":"maybe"}`); rec.Code != 204 || cache.Model().AnswerOf(me, "gev0000000007") != AnswerMaybe {
		t.Errorf("maybe: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":"hidden"}`); rec.Code != 204 {
		t.Fatalf("hide: %d %s", rec.Code, rec.Body)
	}
	view := calendarOf(t, viewer)
	if view.User.Answers["gev0000000007"] != AnswerHidden {
		t.Errorf("answers = %v", view.User.Answers)
	}
	dir := calendarDirectory(t, "sampledata")
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		if u.ID == "gev0000000007" {
			t.Errorf("a hidden event is in Upcoming")
		}
	}
	f := &Feed{Token: "t", Email: me, Name: "Mine"}
	if strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a hidden event is in the owner's feed")
	}
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":"no"}`); rec.Code != 204 {
		t.Fatalf("no: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("an event the owner said no to is in their feed")
	}
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":""}`); rec.Code != 204 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a cleared answer left the event out of the feed")
	}
	if rec := act(t, viewer, "gev0000000007", "answer", `{"answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("yes: %d %s", rec.Code, rec.Body)
	}
	found := false
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		if u.ID == "gev0000000007" {
			found = u.Answer == AnswerYes
		}
	}
	if !found {
		t.Errorf("a yes is not on the Upcoming card")
	}
	mine := calendarOf(t, viewer)
	if i := slices.IndexFunc(mine.Events, func(e *Event) bool { return e.ID == "gev0000000007" }); i < 0 || !slices.Contains(mine.Events[i].Tags, TagGoing) {
		t.Errorf("a yes is not under Going in the viewer's events")
	}
	other := calendarOf(t, as("dana.hawkins@heliosschool.org", handler))
	if i := slices.IndexFunc(other.Events, func(e *Event) bool { return e.ID == "gev0000000007" }); i < 0 || slices.Contains(other.Events[i].Tags, TagGoing) {
		t.Errorf("one viewer's yes is under Going for another")
	}
	if slices.Contains(cache.Model().Event("gev0000000007").Tags, TagGoing) {
		t.Errorf("a yes changed the model's own event")
	}
	going := &Feed{Token: "g", Email: me, Name: "Going", Tags: []string{TagGoing}}
	if !strings.Contains(string(ICS(cache.Model(), dir, going, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a yes is not in the owner's Going feed")
	}
	sender := calendarApp{mail: CalendarMail{ReplyTo: "Helios When <when@reply.heliosian.com>", Key: []byte("key")}}
	if got := string(sender.invite(cache.Model().Event("gev0000000007"), me, "https://when.heliosian.com/e/gev0000000007", mail.MethodRequest).Content); !strings.Contains(got, "METHOD:REQUEST") || !strings.Contains(got, "ORGANIZER;CN=Helios When:mailto:when+") || !strings.Contains(got, "ATTENDEE;CN="+me) {
		t.Errorf("invite:\n%s", got)
	}
}

func TestResponsesForAdmins(t *testing.T) {
	handler, _ := testApp(t)
	act(t, as("jordan.whitfield@heliosschool.org", handler), "gev0000000007", "answer", `{"answer":"yes"}`)
	act(t, as("dana.hawkins@heliosschool.org", handler), "gev0000000007", "answer", `{"answer":"no"}`)
	act(t, as("robin.whitfield@heliosschool.org", handler), "gev0000000007", "answer", `{"answer":"hidden"}`)
	view := calendarOf(t, as("dana.hawkins@heliosschool.org", handler))
	r := view.Responses["gev0000000007"]
	if r == nil || len(r.Yes) != 1 || r.Yes[0].Name != "Jordan Whitfield" || r.Yes[0].Line != "Parent to Sam (Grade 3), Ella (Grade 6)" || len(r.No) != 1 {
		t.Errorf("responses = %+v", r)
	}
	parent := calendarOf(t, as("jordan.whitfield@heliosschool.org", handler))
	if parent.Responses != nil {
		t.Errorf("a parent sees responses: %v", parent.Responses)
	}
}

func TestOverrideFromThePage(t *testing.T) {
	handler, cache := testApp(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	e := cache.Model().Event("gev0000000002")
	tags := slices.DeleteFunc(slices.Clone(e.Tags), BuiltInTag)
	body := func(title, start, end, location, note string) string {
		raw, _ := json.Marshal(map[string]any{"title": title, "start": start, "end": end, "location": location, "description": e.Description, "tags": tags, "keywords": e.Keywords, "note": note})
		return string(raw)
	}
	row := func() map[string]string {
		for _, r := range sheetTables(t)[OverridesTab] {
			if r["Event ID"] == "gev0000000002" {
				return r
			}
		}
		return nil
	}
	correct := func(who http.Handler, key, body string) *httptest.ResponseRecorder {
		return act(t, who, key, "correct", body)
	}
	if rec := correct(admin, "gev0000000002", body("Back to School Night", e.Start, e.End, e.Location, "Shorter")); rec.Code != 204 {
		t.Fatalf("override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r == nil || r["Title"] != "Back to School Night" || r["Start"] != "" || r["Location"] != "" || r["Tags"] != "" || r["Keywords"] != "" || r["Note"] != "Shorter" {
		t.Errorf("row after the title: %v", r)
	}
	if got := cache.Model().Event("gev0000000002"); got.Title != "Back to School Night" || strings.Join(cache.Model().Provenance["gev0000000002"].Corrected, ",") != "Title" {
		t.Errorf("event after: %q corrected %v", got.Title, cache.Model().Provenance["gev0000000002"].Corrected)
	}
	if rec := correct(admin, "gev0000000002", body("LS Back to School Night", "2026-08-27 18:30", "2026-08-27 20:00", "", "")); rec.Code != 204 {
		t.Fatalf("second override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r["Title"] != "" || r["Start"] != "2026-08-27 18:30" || r["End"] != "2026-08-27 20:00" || r["Location"] != Clear || r["Note"] != "" {
		t.Errorf("row after moving and clearing: %v", r)
	}
	if got := cache.Model().Event("gev0000000002"); got.Location != "" || got.Start != "2026-08-27 18:30" {
		t.Errorf("event after moving: %+v", got)
	}
	if rec := correct(admin, "gev0000000002", body("LS Back to School Night", e.Start, e.End, e.Location, "")); rec.Code != 204 || row() != nil {
		t.Errorf("everything put back: %d, row %v", rec.Code, row())
	}
	addressed := func(key, address string) *httptest.ResponseRecorder {
		return correct(admin, key, `{"address":"`+address+`"}`)
	}
	if rec := addressed("gev0000000002", "Back-To-School"); rec.Code != 204 {
		t.Fatalf("an address: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Event("back-to-school"); got == nil || got.ID != "gev0000000002" || EventPath(got) != "/e/back-to-school" || row()["Address"] != "back-to-school" || row()["Title"] != "" {
		t.Errorf("by its address: %+v, row %v", got, row())
	}
	if rec := addressed("gev0000000005", "back-to-school"); rec.Code != 400 || !strings.Contains(rec.Body.String(), "already another event's") {
		t.Errorf("a taken address: %d %s", rec.Code, rec.Body)
	}
	if rec := addressed("gev0000000005", "camping trip!"); rec.Code != 400 {
		t.Errorf("a bad address: %d %s", rec.Code, rec.Body)
	}
	if rec := addressed("back-to-school", ""); rec.Code != 204 || row() != nil || cache.Model().Event("back-to-school") != nil {
		t.Errorf("the address taken away: %d, row %v", rec.Code, row())
	}
	v := inviteView(t, admin, "gev0000000002")
	if !v.Host || len(v.Hosts) != 0 {
		t.Errorf("the admin's view: host %v hosts %v", v.Host, v.Hosts)
	}
	v = inviteView(t, as("jordan.whitfield@heliosschool.org", handler), "gev0000000002")
	if v.Host {
		t.Errorf("a parent's view: host %v", v.Host)
	}
	if !v.ListPrivate || v.Coming != nil {
		t.Errorf("a parent's view of a closed list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := act(t, admin, "gev0000000002", "settings", `{"publicList":true}`); rec.Code != 204 {
		t.Fatalf("opening the list: %d %s", rec.Code, rec.Body)
	}
	v = inviteView(t, as("jordan.whitfield@heliosschool.org", handler), "gev0000000002")
	if v.ListPrivate || v.Coming == nil {
		t.Errorf("a parent's view of an open list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := act(t, admin, "gev0000000002", "image", `{"image":"/category-images/night.jpg"}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "/category-images/night.jpg" || row()["Image"] != "category-images/night.jpg" {
		t.Errorf("a picture: %d %+v", rec.Code, row())
	}
	if rec := act(t, admin, "gev0000000002", "settings", `{"flyer":"sample/community.jpg"}`); rec.Code != 204 || cache.Model().Invitations["gev0000000002"] == nil || cache.Model().Invitations["gev0000000002"].Flyer != "sample/community.jpg" || len(cache.Model().Invitations["gev0000000002"].Hosts) != 0 {
		t.Errorf("a flyer: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, admin, "gev0000000002", "image", `{"image":""}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "" {
		t.Errorf("the picture taken away: %d", rec.Code)
	}
	if rec := act(t, as("jordan.whitfield@heliosschool.org", handler), "gev0000000002", "image", `{"image":"x.jpg"}`); rec.Code != 403 {
		t.Errorf("a parent's picture: %d", rec.Code)
	}
	if rec := correct(as("jordan.whitfield@heliosschool.org", handler), "gev0000000002", body("x", e.Start, e.End, "", "")); rec.Code != 403 {
		t.Errorf("a parent: %d", rec.Code)
	}
	if rec := correct(admin, "gev0000000002", `{"title":"x","start":"not a date"}`); rec.Code != 400 {
		t.Errorf("a bad date: %d", rec.Code)
	}
	if rec := correct(admin, "gev0000000002", body(e.Title, e.Start, e.End, e.Location, "The admins'")); rec.Code != 204 {
		t.Fatalf("an admin's note: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, admin, "gev0000000002", "settings", `{"hosts":["jordan.whitfield@heliosschool.org"]}`); rec.Code != 204 {
		t.Fatalf("a co-host: %d %s", rec.Code, rec.Body)
	}
	cohost := as("jordan.whitfield@heliosschool.org", handler)
	if rec := correct(cohost, "gev0000000002", body("Family Night", e.Start, e.End, e.Location, "Mine")); rec.Code != 204 {
		t.Fatalf("a co-host's edit: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r["Title"] != "Family Night" || r["Note"] != "The admins'" || cache.Model().Event("gev0000000002").Title != "Family Night" {
		t.Errorf("after the co-host's edit: %v", r)
	}
	if rec := act(t, cohost, "gev0000000002", "image", `{"image":"/category-images/night.jpg"}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "/category-images/night.jpg" {
		t.Errorf("a co-host's picture: %d", rec.Code)
	}
	raw, _ := json.Marshal(map[string]any{"title": "Family Night", "start": e.Start, "end": e.End, "location": e.Location, "description": e.Description, "tags": tags, "keywords": e.Keywords})
	if rec := correct(admin, "gev0000000002", string(raw)); rec.Code != 204 || row()["Note"] != "The admins'" {
		t.Errorf("an edit sending no note: %d %v", rec.Code, row())
	}
}

func TestMyHeliosianFeed(t *testing.T) {
	handler, cache := testApp(t)
	parent := as("jordan.whitfield@heliosschool.org", handler)
	token := func() string {
		rec := call(t, parent, "POST", settingsPath("feed-token"), "")
		made := cache.Model().Settings["jordan.whitfield@heliosschool.org"].FeedToken
		if rec.Code != 204 || made == "" {
			t.Fatalf("my heliosian token: %d %s", rec.Code, rec.Body)
		}
		return made
	}
	first := token()
	if again := token(); again != first {
		t.Errorf("a second ask minted another: %q then %q", first, again)
	}
	if cache.Model().Settings["jordan.whitfield@heliosschool.org"].FeedToken != first {
		t.Errorf("the token is not kept")
	}
	rec := call(t, parent, "GET", "/open/feed/"+first+".ics", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "BEGIN:VCALENDAR") || !strings.Contains(rec.Body.String(), "X-WR-CALNAME:My Heliosian") {
		t.Errorf("the feed: %d %s", rec.Code, rec.Body.String()[:min(200, rec.Body.Len())])
	}
	if rec := call(t, parent, "GET", "/open/feed/nobodys.ics", ""); rec.Code != 404 {
		t.Errorf("a token nobody holds: %d", rec.Code)
	}
}
