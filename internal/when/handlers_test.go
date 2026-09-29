package when

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
	"heliosian/internal/who"
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

func sampleCache(t *testing.T) *Cache {
	t.Helper()
	t.Chdir("../..")
	sheet = &data.Dir{Root: "sampledata"}
	queue = store.NewQueue()
	cache, err := NewCache(sheet, sheet, func() Roster { return roster }, nil, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func testApp(t *testing.T) (http.Handler, *Cache) {
	t.Helper()
	cache := sampleCache(t)
	d := sampleDirectory(t, "sampledata")
	mux := http.NewServeMux()
	Register(mux, Deps{
		Cache:     cache,
		Images:    memoryImages(),
		Directory: func() *who.Model { return d },
		Settings:  func() *config.Settings { return &config.Settings{} },
		Lists:     func(string) []List { return nil },
		Linked:    func(string) []Linked { return nil },
		SourceID:  noSource,
		Celebrate: noCelebrate(),
		Sources:   newSampleSources(t).sources,
		Mail:      Mail{Sender: keptMail().Mailgun},
		Style:     testStyle,
		Queue:     queue,
	})
	return mux, cache
}

func noSource(string, string) string {
	return ""
}

func memoryImages() blob.Images {
	return blob.NewImages(blob.New(blob.NewMemoryBucket()), "when", "celebrate", "team")
}

func noCelebrate() Celebrate {
	return Celebrate{
		Party:       func(string) *PartyPeople { return nil },
		IsAdmin:     func(string) bool { return false },
		MoveAddress: func(context.Context, access.Actor, string, string, string) error { return nil },
	}
}

var testStyle = CardStyle(func() string { return "Helios When" }, func() string { return "The school year, day by day" })

func changeLog(t *testing.T) []string {
	t.Helper()
	return testkit.ChangeLines(t, sheet, queue, appName)
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
	rec := call(t, owner, http.MethodPost, "/api/when/feeds", `{"name":"Just trips","classrooms":["Jays"],`+trip+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	var made struct{ Token, URL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || len(made.Token) != 24 || made.URL != "https://calendar.heliosiandev.com:8080/open/feed/"+made.Token+".ics" {
		t.Fatalf("add answered %s", rec.Body.String())
	}
	if len(cache.Model().Feeds) != 2 || cache.Model().Feed(made.Token).Email != "jordan.whitfield@heliosschool.org" {
		t.Fatalf("feeds after add: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SUMMARY:Jays and Ravens Camping") || strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("new feed: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(t, owner, http.MethodPost, "/api/when/feeds", `{"name":"just TRIPS","classrooms":["Jays"],`+trip+`}`)
	var twin struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &twin)
	if rec.Code != http.StatusOK || cache.Model().Feed(twin.Token).Name != "just TRIPS 2" {
		t.Errorf("twin name: %d %+v", rec.Code, cache.Model().Feed(twin.Token))
	}
	call(t, owner, http.MethodDelete, "/api/when/feeds", `{"token":"`+twin.Token+`"}`)
	if rec := call(t, owner, http.MethodPost, "/api/when/feeds", `{"name":"Bad","classrooms":["Penguins"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown classroom: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/when/feeds", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", rec.Code)
	}
	if rec := call(t, other, http.MethodPut, "/api/when/feeds", `{"token":"`+made.Token+`","name":"Theirs","classrooms":["Jays"],`+trip+`}`); rec.Code != http.StatusForbidden {
		t.Errorf("someone else's change: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPut, "/api/when/feeds", `{"token":"`+made.Token+`","name":"","classrooms":["Jays"],`+trip+`}`); rec.Code != http.StatusBadRequest {
		t.Errorf("change to no name: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPut, "/api/when/feeds", `{"token":"`+made.Token+`","name":"Jays days","emoji":" 🚌 ","classrooms":["Jays"],"tags":[`+quoted(t, "Trip, Schedule")+`]}`); rec.Code != http.StatusNoContent {
		t.Errorf("owner's change: %d %s", rec.Code, rec.Body.String())
	}
	if f := cache.Model().Feed(made.Token); f == nil || f.Name != "Jays days" || f.Emoji != "🚌" || cells.JoinList(f.Tags) != ids(t, "Trip, Schedule") || f.Email != "jordan.whitfield@heliosschool.org" {
		t.Errorf("feed after change: %+v", f)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "X-WR-CALNAME:Jays days") || !strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("changed feed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, other, http.MethodDelete, "/api/when/feeds", `{"token":"`+made.Token+`"}`); rec.Code != http.StatusForbidden {
		t.Errorf("someone else's removal: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodDelete, "/api/when/feeds", `{"token":"`+made.Token+`"}`); rec.Code != http.StatusNoContent {
		t.Errorf("owner's removal: %d %s", rec.Code, rec.Body.String())
	}
	if len(cache.Model().Feeds) != 1 || cache.Model().Feed(made.Token) != nil {
		t.Errorf("feeds after removal: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("removed feed still serves: %d", rec.Code)
	}
}

func TestModelRoute(t *testing.T) {
	mux, _ := testApp(t)
	rec := call(t, as("jordan.whitfield@heliosschool.org", mux), http.MethodGet, "/api/when/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model: %d", rec.Code)
	}
	var view View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.User.Email != "jordan.whitfield@heliosschool.org" || len(view.Feeds) != 1 || len(view.Events) != 22 || len(view.Classrooms) != 9 {
		t.Errorf("view = user %s, %d feeds, %d events, %d classrooms", view.User.Email, len(view.Feeds), len(view.Events), len(view.Classrooms))
	}
}

func TestSharePreview(t *testing.T) {
	handler, cache := testApp(t)
	testkit.Previews(t, PreviewHead(cache, func(string) []Linked { return nil }, noSource, testStyle), testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/e/gev0000000007",
		Want: []string{`property="og:title" content="International Night"`, `Thursday, September 24 · 4:00 – 6:00 PM`, `content="https://when.heliosiandev.com:8080/open/share/gev0000000007.png"`},
	}, testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/mine",
		Want: []string{`og:title" content="Helios When"`, "One calendar", "/open/share/upcoming.png"},
	})
	testkit.Cards(t, handler, []string{"/open/share/gev0000000007.png", "/open/share/upcoming.png"}, "/open/share/nope.png")
}

func TestOldIDsReachTheirEvents(t *testing.T) {
	handler, cache := testApp(t)
	jordan := as("jordan.whitfield@heliosschool.org", handler)
	for old, want := range map[string]string{"7QK2M4XN": "evt0000000001", "3pl9w2zc": "evt0000000002", "8RV4T6PK": "evt0000000003"} {
		rec := call(t, jordan, "GET", "/api/when/event?id="+old, "")
		var e Event
		json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != 200 || e.ID != want {
			t.Errorf("the event by its old ID %s: %d %s", old, rec.Code, e.ID)
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
	testkit.Previews(t, PreviewHead(cache, func(string) []Linked { return nil }, noSource, testStyle), testkit.Preview{
		URL:  "https://when.heliosiandev.com:8080/e/3PL9W2ZC",
		Want: []string{`property="og:title" content="HCA Meeting"`, `content="https://when.heliosiandev.com:8080/open/share/evt0000000002.png"`},
	})
	if rec := call(t, jordan, "PUT", "/api/when/invites/settings", `{"id":"3PL9W2ZC","flyer":"sample/community.jpg"}`); rec.Code != 204 {
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
	var view View
	rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/when/model", "")
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
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
	rec = call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/when/model", "")
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	p := view.Provenance["gev0000000005"]
	if p == nil || p.Model == "" || len(p.Corrected) == 0 || p.Note != "Say which classrooms" {
		t.Errorf("admin provenance for gev0000000005 = %+v", p)
	}
}

func TestSavedView(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	rec := call(t, as(me, handler), "POST", "/api/when/settings", `{"classrooms":["Hawks"],"tags":[`+quoted(t, "Community, HCA")+`,"Nonsense"]}`)
	if rec.Code != 204 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	var view View
	rec = call(t, as(me, handler), "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	if view.User.Saved == nil || strings.Join(view.User.Saved.Classrooms, ",") != "Hawks" || cells.JoinList(view.User.Saved.Tags) != ids(t, "Community, HCA") {
		t.Errorf("saved view = %+v", view.User.Saved)
	}
	for _, u := range cache.Model().UpcomingUnder(sampleDirectory(t, "sampledata"), me, nil, now(), 0, "") {
		if !strings.Contains(u.Title, "Hawks") && !strings.Contains(u.Title, "CAFE") && u.Title != "International Night" && u.Title != "Halloween Parade" && u.Title != "HCA Meeting" && u.Title != "All School Movie Night" && u.Title != "Cocoa & Cookies" && u.Title != "Talent Show" && u.Title != "Back to School Social" && u.Title != "Spring Celebration" && u.Title != "Fall Potluck at the Torres'" && u.Title != "Jays & Ravens Beach Picnic" {
			t.Errorf("upcoming under the saved view lists %q", u.Title)
		}
	}
	rec = call(t, as(me, handler), "DELETE", "/api/when/settings", "")
	if rec.Code != 204 {
		t.Fatalf("forget: %d %s", rec.Code, rec.Body)
	}
	var again View
	rec = call(t, as(me, handler), "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&again)
	if again.User.Saved != nil {
		t.Errorf("saved view survives forgetting: %+v", again.User.Saved)
	}
}

func TestDefaultCalendar(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	dir := sampleDirectory(t, "sampledata")
	found := false
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian, upcoming leaves out International Night")
	}
	if rec := call(t, viewer, "POST", "/api/when/default", `{"token":"nonsense"}`); rec.Code != 400 {
		t.Errorf("a stranger's token: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", "/api/when/default", `{"token":"sample7feedtoken4jordan2whitfield"}`); rec.Code != 204 {
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
	if rec := call(t, viewer, "POST", "/api/when/default", `{"token":"`+MyHeliosianToken+`"}`); rec.Code != 204 || cache.Model().DefaultCalendar(me) != nil {
		t.Errorf("back to My Heliosian: %d, default %+v", rec.Code, cache.Model().DefaultCalendar(me))
	}
	if rec := call(t, viewer, "PUT", "/api/when/feeds", `{"token":"`+MyHeliosianToken+`","name":"Home base","emoji":"🏠","classrooms":["Jays"],"tags":[`+quoted(t, "Trip")+`]}`); rec.Code != 204 {
		t.Errorf("rename My Heliosian: %d %s", rec.Code, rec.Body)
	}
	if home := cache.Model().MyHeliosian(me); home.Name != "Home base" || home.Emoji != "🏠" || !home.Locked || len(home.Classrooms) != 0 {
		t.Errorf("My Heliosian = %+v", home)
	}
	rec := call(t, viewer, "POST", "/api/when/feeds", `{"name":"Everything","classrooms":[],"tags":[]}`)
	var made struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec := call(t, viewer, "PUT", "/api/when/feeds/order", `{"tokens":["`+made.Token+`"]}`); rec.Code != 400 {
		t.Errorf("a short order: %d", rec.Code)
	}
	if rec := call(t, viewer, "PUT", "/api/when/feeds/order", `{"tokens":["`+made.Token+`","`+MyHeliosianToken+`","sample7feedtoken4jordan2whitfield"]}`); rec.Code != 204 {
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
	cache := sampleCache(t)
	d := sampleDirectory(t, "sampledata")
	kept := keptMail()
	mux := http.NewServeMux()
	Register(mux, Deps{
		Cache:     cache,
		Images:    memoryImages(),
		Directory: func() *who.Model { return d },
		Settings:  func() *config.Settings { return &config.Settings{} },
		Lists:     func(string) []List { return nil },
		Linked:    func(string) []Linked { return nil },
		SourceID:  noSource,
		Celebrate: noCelebrate(),
		Sources:   newSampleSources(t).sources,
		Mail:      Mail{Sender: kept.Mailgun, Base: "https://when.heliosian.com"},
		Style:     testStyle,
		Queue:     queue,
	})
	parent := as("jordan.whitfield@heliosschool.org", mux)
	admin := as("dana.hawkins@heliosschool.org", mux)
	wait := func(n int) []mail.Message {
		for i := 0; i < 50 && len(kept.Messages()) < n; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		return kept.Messages()
	}
	call(t, parent, "POST", "/api/when/events", `{"title":"Bake sale","start":"2026-10-01 15:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Public"}`)
	sent := wait(1)
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Event to approve: Bake sale") || !slices.Contains(sent[0].To, "dana.hawkins@heliosschool.org") || !strings.Contains(sent[0].Text, "Jordan") || !strings.Contains(sent[0].HTML, "Review the event") {
		t.Errorf("public event mail = %+v", sent)
	}
	call(t, parent, "POST", "/api/when/events", `{"title":"Sam\u2019s party","start":"2026-10-03 14:00","tags":[`+quoted(t, "Jays")+`],"sharing":"Link"}`)
	sent = wait(2)
	if len(sent) != 2 || !strings.HasPrefix(sent[1].Subject, "Link event added: Sam") || !strings.Contains(sent[1].HTML, "See the event") || strings.Contains(sent[1].Text, "waiting for approval") {
		t.Errorf("link event mail = %+v", sent)
	}
	call(t, admin, "POST", "/api/when/events", `{"title":"Admin's own","start":"2026-10-05","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`)
	sent = wait(3)
	if len(sent) != 3 || !strings.HasPrefix(sent[2].Subject, "Event to approve: Admin's own") {
		t.Errorf("an admin's own public event waits and is mailed about too: %+v", sent)
	}
	var v View
	rec := call(t, parent, "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&v)
	var party *Event
	for _, e := range v.Events {
		if strings.HasPrefix(e.Title, "Sam") {
			party = e
		}
	}
	if party == nil {
		t.Fatal("the party is not in the host's view")
	}
	if rec := call(t, parent, "PUT", "/api/when/events", `{"id":"`+party.ID+`","title":"`+party.Title+`","start":"2026-10-03 14:00","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 204 {
		t.Fatalf("switch to public: %d %s", rec.Code, rec.Body)
	}
	sent = wait(4)
	if e := cache.Model().Event(party.ID); e == nil || !e.Pending || len(sent) != 4 || !strings.HasPrefix(sent[3].Subject, "Event to approve: Sam") {
		t.Errorf("after the switch: event %+v, mail %d", e, len(sent))
	}
	other := as("robin.whitfield@heliosschool.org", mux)
	if rec := call(t, other, "GET", "/api/when/event?id="+party.ID, ""); rec.Code != 200 {
		t.Errorf("the link of an event waiting for approval: %d", rec.Code)
	}
	if rec := call(t, other, "POST", "/api/when/rsvp", `{"id":"`+party.ID+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("a yes by link while waiting: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminAddsAndCorrects(t *testing.T) {
	handler, cache := testApp(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	rec := call(t, admin, "POST", "/api/when/events", `{"title":"Chess Club","start":"2026-10-01 15:30","end":"2026-10-01 16:30","tags":[`+quoted(t, "Jays, Clubs")+`],"sharing":"Public","repeatWeeks":4,"repeatTimes":2}`)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var made struct{ IDs []string }
	json.NewDecoder(rec.Body).Decode(&made)
	for _, id := range made.IDs {
		if e := cache.Model().Event(id); e == nil || !e.Pending {
			t.Errorf("an admin's event went straight on: %+v", e)
		}
		if rec := call(t, admin, "POST", "/api/when/events/approve", `{"id":"`+id+`"}`); rec.Code != 204 {
			t.Fatalf("approve %s: %d", id, rec.Code)
		}
	}
	if len(made.IDs) != 3 {
		t.Fatalf("ids = %v", made.IDs)
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
	if rec := call(t, admin, "POST", "/api/when/events", `{"title":"Bad","start":"not a date","tags":[`+quoted(t, "Clubs")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("bad date accepted: %d", rec.Code)
	}
	rec = call(t, admin, "POST", "/api/when/keywords", `{"id":"`+made.IDs[0]+`","keywords":["chess","board games"]}`)
	if rec.Code != 204 || strings.Join(cache.Model().Event(made.IDs[0]).Keywords, ",") != "chess,board games" {
		t.Errorf("keywords: %d %v", rec.Code, cache.Model().Event(made.IDs[0]).Keywords)
	}
	if p := cache.Model().Provenance[made.IDs[0]]; p == nil || strings.Join(p.Corrected, ",") != "Keywords" {
		t.Errorf("provenance = %+v", p)
	}
	rec = call(t, admin, "POST", "/api/when/events/when", `{"id":"`+made.IDs[0]+`","start":"2026-10-02 16:00","end":"2026-10-02 17:00"}`)
	if rec.Code != 204 || cache.Model().Event(made.IDs[0]).Start != "2026-10-02 16:00" {
		t.Errorf("move: %d %s", rec.Code, cache.Model().Event(made.IDs[0]).Start)
	}
	if rec := call(t, admin, "POST", "/api/when/events/when", `{"id":"gev0000000007","start":"2026-10-02 16:00"}`); rec.Code != 400 {
		t.Errorf("an imported event moved from the list: %d", rec.Code)
	}
	parent := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, parent, "POST", "/api/when/keywords", `{"id":"`+made.IDs[0]+`","keywords":["x"]}`); rec.Code != 403 {
		t.Errorf("parent set keywords: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/when/events", `{"title":"Bake sale","start":"2026-10-01 15:00","end":"2026-10-01 17:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Public","repeatWeeks":1,"repeatTimes":3}`)
	var shared struct {
		IDs     []string `json:"ids"`
		Pending bool     `json:"pending"`
	}
	json.Unmarshal(rec.Body.Bytes(), &shared)
	if rec.Code != 200 || !shared.Pending || len(shared.IDs) != 1 {
		t.Fatalf("parent shared an event: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || !e.Pending || !slices.Contains(cache.Model().Pending, e) {
		t.Errorf("shared event = %+v", e)
	}
	if cache.Model().AnswerOf("jordan.whitfield@heliosschool.org", shared.IDs[0]) != AnswerYes {
		t.Errorf("the host is not going to their own event")
	}
	var hostView View
	rec = call(t, parent, "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&hostView)
	if n := len(slices.DeleteFunc(slices.Clone(hostView.Events), func(e *Event) bool { return e.ID != shared.IDs[0] })); n != 1 {
		t.Errorf("the host's view carries their event %d times", n)
	}
	for _, e := range cache.Model().Events {
		if e.ID == shared.IDs[0] {
			t.Errorf("a pending event is on the calendar")
		}
	}
	sees := func(who http.Handler) bool {
		var v View
		rec := call(t, who, "GET", "/api/when/model", "")
		json.NewDecoder(rec.Body).Decode(&v)
		return slices.ContainsFunc(v.Events, func(e *Event) bool { return e.ID == shared.IDs[0] && (e.Pending || e.Declined) })
	}
	other := as("robin.whitfield@heliosschool.org", handler)
	if !sees(parent) || !sees(admin) || sees(other) {
		t.Errorf("pending event seen by parent %v, admin %v, another %v", sees(parent), sees(admin), sees(other))
	}
	if rec := call(t, parent, "POST", "/api/when/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 403 {
		t.Errorf("parent approved: %d", rec.Code)
	}
	if rec := call(t, admin, "POST", "/api/when/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || e.Pending || e.Status != StatusApproved || !slices.Contains(cache.Model().Events, e) {
		t.Errorf("approved event = %+v", e)
	}
	if rec := call(t, parent, "PUT", "/api/when/events", `{"id":"`+shared.IDs[0]+`","title":"Bake sale!","start":"2026-10-01 15:30","end":"2026-10-01 17:30","location":"Gym","tags":[`+quoted(t, "Jays, Community")+`],"image":"/category-images/cake.jpg","sharing":"Public"}`); rec.Code != 204 {
		t.Errorf("owner's edit: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || e.Title != "Bake sale!" || e.Location != "Gym" || e.Image != "/category-images/cake.jpg" || e.Start != "2026-10-01 15:30" || e.Pending {
		t.Errorf("edited event = %+v", e)
	}
	if rec := call(t, other, "PUT", "/api/when/events", `{"id":"`+shared.IDs[0]+`","title":"Mine now","start":"2026-10-01","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 403 {
		t.Errorf("another parent's edit: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/when/events", `{"title":"Sam\u2019s birthday","start":"2026-10-03 14:00","end":"2026-10-03 16:00","tags":[`+quoted(t, "Jays, Community")+`],"sharing":"Link"}`)
	json.Unmarshal(rec.Body.Bytes(), &shared)
	if rec.Code != 200 || shared.Pending {
		t.Fatalf("link: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || e.Sharing != SharingLink || e.Status != "" {
		t.Errorf("link event = %+v", e)
	}
	if sees(other) {
		t.Errorf("a link event is on another's calendar before they answer")
	}
	if rec := call(t, other, "GET", "/api/when/event?id="+shared.IDs[0], ""); rec.Code != 200 {
		t.Errorf("a link event by its link: %d", rec.Code)
	}
	if rec := call(t, other, "POST", "/api/when/rsvp", `{"id":"`+shared.IDs[0]+`","answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("yes to a link event: %d %s", rec.Code, rec.Body)
	}
	var otherView View
	rec = call(t, other, "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&otherView)
	if i := slices.IndexFunc(otherView.Events, func(e *Event) bool { return e.ID == shared.IDs[0] }); i < 0 || !slices.Contains(otherView.Events[i].Tags, TagGoing) {
		t.Errorf("a yes did not put the link event under Going on the other's calendar")
	}
	found := false
	for _, u := range cache.Model().UpcomingUnder(sampleDirectory(t, "sampledata"), "robin.whitfield@heliosschool.org", nil, now(), 0, "") {
		found = found || u.ID == shared.IDs[0]
	}
	if !found {
		t.Errorf("a yes did not put the link event in the other's Upcoming")
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"title":"Unsaid","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`]}`); rec.Code != 400 {
		t.Errorf("no sharing: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"title":"Unsaid","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Private"}`); rec.Code != 400 {
		t.Errorf("an old sharing word: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/when/events", `{"id":"sams-party","address":"Sams-Party","title":"Sam\u2019s party","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Link"}`)
	json.Unmarshal(rec.Body.Bytes(), &shared)
	party := cache.Model().Event("sams-party")
	if rec.Code != 200 || party == nil || party.ID != shared.IDs[0] || party.Address != "sams-party" || EventPath(party) != "/e/sams-party" {
		t.Fatalf("chosen address: %d %s, event %+v", rec.Code, rec.Body, party)
	}
	if _, ok := id.Parse(party.ID); !ok {
		t.Errorf("an event with an address is still minted an ID: %q", party.ID)
	}
	if rec := call(t, handler, "GET", "/open/share/sams-party.png", ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("a link event's card: %d", rec.Code)
	}
	if head := PreviewHead(cache, func(string) []Linked { return nil }, noSource, testStyle)(httptest.NewRequest("GET", "https://when.heliosiandev.com:8080/e/sams-party", nil)); !strings.Contains(head, "Sam") || !strings.Contains(head, "/open/share/"+party.ID+".png") {
		t.Errorf("a link event's preview:\n%s", head)
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"address":"sams-party","title":"Again","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("a taken address: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"address":"`+party.ID+`","title":"Again","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an address that is another event's ID: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"address":"a/b","title":"Odd","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an ill-formed address: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/when/events", `{"address":"evt9999999999","title":"Odd","start":"2026-10-04","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an address that reads as an ID: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/when/events", `{"title":"Not this","start":"2026-10-02","tags":[`+quoted(t, "Jays")+`],"sharing":"Public"}`)
	json.Unmarshal(rec.Body.Bytes(), &shared)
	if rec := call(t, admin, "POST", "/api/when/events/decline", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 {
		t.Errorf("decline: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || !e.Declined || e.Status != StatusDeclined || slices.Contains(cache.Model().Events, e) {
		t.Errorf("declined event = %+v", e)
	}
	if !sees(parent) || sees(other) {
		t.Errorf("declined event seen by parent %v, another %v", sees(parent), sees(other))
	}
	if rec := call(t, other, "GET", "/api/when/event?id="+shared.IDs[0], ""); rec.Code != 200 {
		t.Errorf("a declined event by its link: %d", rec.Code)
	}
	if rec := call(t, other, "POST", "/api/when/rsvp", `{"id":"`+shared.IDs[0]+`","answer":"yes"}`); rec.Code != 204 || !sees(other) {
		t.Errorf("yes to a declined event: %d, on the calendar %v", rec.Code, sees(other))
	}
	if rec := call(t, admin, "POST", "/api/when/events/decline", `{"id":"sams-party"}`); rec.Code != 400 || cache.Model().Event("sams-party").Declined {
		t.Errorf("declined a link event: %d", rec.Code)
	}
	if rec := call(t, admin, "POST", "/api/when/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 || cache.Model().Event(shared.IDs[0]).Status != StatusApproved {
		t.Errorf("approve after decline: %d", rec.Code)
	}
}

func TestFeedTags(t *testing.T) {
	handler, _ := testApp(t)
	me := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, me, "POST", "/api/when/feeds", `{"name":"Odds and ends","classrooms":["Jays"],"tags":[`+quoted(t, "Misc, Trip")+`]}`); rec.Code != 200 {
		t.Errorf("feed with Misc: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/when/feeds", `{"name":"Parties","classrooms":["Jays"],"tags":[`+quoted(t, "Celebrate, Going")+`]}`); rec.Code != 200 {
		t.Errorf("feed with Celebrate and Going: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/when/feeds", `{"name":"Nope","classrooms":["Jays"],"tags":["Nonsense"]}`); rec.Code != 400 {
		t.Errorf("feed with a made-up tag: %d", rec.Code)
	}
}

func TestFeedCarriesLinked(t *testing.T) {
	handler, cache := testApp(t)
	linked := []Linked{{Source: SourceCelebrate, ID: "pty0000000009", EventID: "pty0000000009", Title: "Fondue Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineGoing}}
	f := &Feed{Token: "t", Email: "jordan.whitfield@heliosschool.org", Name: "Mine", Tags: []string{TagGoing}}
	out := string(ICS(cache.Model(), sampleDirectory(t, "sampledata"), f, linked, "https://when.heliosiandev.com:8080", now()))
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
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"perhaps"}`); rec.Code != 400 {
		t.Errorf("nonsense answer: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"maybe"}`); rec.Code != 204 || cache.Model().AnswerOf(me, "gev0000000007") != AnswerMaybe {
		t.Errorf("maybe: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"hidden"}`); rec.Code != 204 {
		t.Fatalf("hide: %d %s", rec.Code, rec.Body)
	}
	var view View
	rec := call(t, viewer, "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	if view.User.Answers["gev0000000007"] != AnswerHidden {
		t.Errorf("answers = %v", view.User.Answers)
	}
	dir := sampleDirectory(t, "sampledata")
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, "") {
		if u.ID == "gev0000000007" {
			t.Errorf("a hidden event is in Upcoming")
		}
	}
	f := &Feed{Token: "t", Email: me, Name: "Mine"}
	if strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a hidden event is in the owner's feed")
	}
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"no"}`); rec.Code != 204 {
		t.Fatalf("no: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("an event the owner said no to is in their feed")
	}
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":""}`); rec.Code != 204 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(string(ICS(cache.Model(), dir, f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a cleared answer left the event out of the feed")
	}
	if rec := call(t, viewer, "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"yes"}`); rec.Code != 204 {
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
	var mine View
	rec = call(t, viewer, "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&mine)
	if i := slices.IndexFunc(mine.Events, func(e *Event) bool { return e.ID == "gev0000000007" }); i < 0 || !slices.Contains(mine.Events[i].Tags, TagGoing) {
		t.Errorf("a yes is not under Going in the viewer's events")
	}
	var other View
	rec = call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&other)
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
	sender := app{mail: Mail{ReplyTo: "Helios When <when@reply.heliosian.com>", Key: []byte("key")}}
	if got := string(sender.invite(cache.Model().Event("gev0000000007"), me, "https://when.heliosian.com/e/gev0000000007", mail.MethodRequest).Content); !strings.Contains(got, "METHOD:REQUEST") || !strings.Contains(got, "ORGANIZER;CN=Helios When:mailto:when+") || !strings.Contains(got, "ATTENDEE;CN="+me) {
		t.Errorf("invite:\n%s", got)
	}
}

func TestResponsesForAdmins(t *testing.T) {
	handler, _ := testApp(t)
	call(t, as("jordan.whitfield@heliosschool.org", handler), "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"yes"}`)
	call(t, as("dana.hawkins@heliosschool.org", handler), "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"no"}`)
	call(t, as("robin.whitfield@heliosschool.org", handler), "POST", "/api/when/rsvp", `{"id":"gev0000000007","answer":"hidden"}`)
	var view View
	rec := call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/when/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	r := view.Responses["gev0000000007"]
	if r == nil || len(r.Yes) != 1 || r.Yes[0].Name != "Jordan Whitfield" || r.Yes[0].Line != "Parent to Sam (Grade 3), Ella (Grade 6)" || len(r.No) != 1 {
		t.Errorf("responses = %+v", r)
	}
	rec = call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/when/model", "")
	var parent View
	json.NewDecoder(rec.Body).Decode(&parent)
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
		raw, _ := json.Marshal(map[string]any{"id": "gev0000000002", "title": title, "start": start, "end": end, "location": location, "description": e.Description, "tags": tags, "keywords": e.Keywords, "note": note})
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
	if rec := call(t, admin, "PUT", "/api/when/overrides", body("Back to School Night", e.Start, e.End, e.Location, "Shorter")); rec.Code != 204 {
		t.Fatalf("override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r == nil || r["Title"] != "Back to School Night" || r["Start"] != "" || r["Location"] != "" || r["Tags"] != "" || r["Keywords"] != "" || r["Note"] != "Shorter" {
		t.Errorf("row after the title: %v", r)
	}
	if got := cache.Model().Event("gev0000000002"); got.Title != "Back to School Night" || strings.Join(cache.Model().Provenance["gev0000000002"].Corrected, ",") != "Title" {
		t.Errorf("event after: %q corrected %v", got.Title, cache.Model().Provenance["gev0000000002"].Corrected)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", body("LS Back to School Night", "2026-08-27 18:30", "2026-08-27 20:00", "", "")); rec.Code != 204 {
		t.Fatalf("second override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r["Title"] != "" || r["Start"] != "2026-08-27 18:30" || r["End"] != "2026-08-27 20:00" || r["Location"] != Clear || r["Note"] != "" {
		t.Errorf("row after moving and clearing: %v", r)
	}
	if got := cache.Model().Event("gev0000000002"); got.Location != "" || got.Start != "2026-08-27 18:30" {
		t.Errorf("event after moving: %+v", got)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", body("LS Back to School Night", e.Start, e.End, e.Location, "")); rec.Code != 204 || row() != nil {
		t.Errorf("everything put back: %d, row %v", rec.Code, row())
	}
	addressed := func(id, address string) string {
		ev := cache.Model().Event(id)
		raw, _ := json.Marshal(map[string]any{"id": id, "title": ev.Title, "start": ev.Start, "end": ev.End, "location": ev.Location, "description": ev.Description, "tags": slices.DeleteFunc(slices.Clone(ev.Tags), BuiltInTag), "keywords": ev.Keywords, "address": address})
		return string(raw)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", addressed("gev0000000002", "Back-To-School")); rec.Code != 204 {
		t.Fatalf("an address: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Event("back-to-school"); got == nil || got.ID != "gev0000000002" || EventPath(got) != "/e/back-to-school" || row()["Address"] != "back-to-school" || row()["Title"] != "" {
		t.Errorf("by its address: %+v, row %v", got, row())
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", addressed("gev0000000005", "back-to-school")); rec.Code != 400 || !strings.Contains(rec.Body.String(), "already another event's") {
		t.Errorf("a taken address: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", addressed("gev0000000005", "camping trip!")); rec.Code != 400 {
		t.Errorf("a bad address: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", addressed("gev0000000002", "")); rec.Code != 204 || row() != nil || cache.Model().Event("back-to-school") != nil {
		t.Errorf("the address taken away: %d, row %v", rec.Code, row())
	}
	var v InviteView
	json.NewDecoder(call(t, admin, "GET", "/api/when/invites?id=gev0000000002", "").Body).Decode(&v)
	if !v.Host || len(v.Hosts) != 0 {
		t.Errorf("the admin's view: host %v hosts %v", v.Host, v.Hosts)
	}
	v = InviteView{}
	json.NewDecoder(call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/when/invites?id=gev0000000002", "").Body).Decode(&v)
	if v.Host {
		t.Errorf("a parent's view: host %v", v.Host)
	}
	if !v.ListPrivate || v.Coming != nil {
		t.Errorf("a parent's view of a closed list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := call(t, admin, "PUT", "/api/when/invites/settings", `{"id":"gev0000000002","publicList":true}`); rec.Code != 204 {
		t.Fatalf("opening the list: %d %s", rec.Code, rec.Body)
	}
	v = InviteView{}
	json.NewDecoder(call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/when/invites?id=gev0000000002", "").Body).Decode(&v)
	if v.ListPrivate || v.Coming == nil {
		t.Errorf("a parent's view of an open list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides/image", `{"id":"gev0000000002","image":"/category-images/night.jpg"}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "/category-images/night.jpg" || row()["Image"] != "category-images/night.jpg" {
		t.Errorf("a picture: %d %+v", rec.Code, row())
	}
	if rec := call(t, admin, "PUT", "/api/when/invites/settings", `{"id":"gev0000000002","flyer":"sample/community.jpg"}`); rec.Code != 204 || cache.Model().Invitations["gev0000000002"] == nil || cache.Model().Invitations["gev0000000002"].Flyer != "sample/community.jpg" || len(cache.Model().Invitations["gev0000000002"].Hosts) != 0 {
		t.Errorf("a flyer: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides/image", `{"id":"gev0000000002","image":""}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "" {
		t.Errorf("the picture taken away: %d", rec.Code)
	}
	if rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "PUT", "/api/when/overrides/image", `{"id":"gev0000000002","image":"x.jpg"}`); rec.Code != 403 {
		t.Errorf("a parent's picture: %d", rec.Code)
	}
	if rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "PUT", "/api/when/overrides", body("x", e.Start, e.End, "", "")); rec.Code != 403 {
		t.Errorf("a parent: %d", rec.Code)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", `{"id":"gev0000000002","title":"x","start":"not a date"}`); rec.Code != 400 {
		t.Errorf("a bad date: %d", rec.Code)
	}
	if rec := call(t, admin, "PUT", "/api/when/overrides", body(e.Title, e.Start, e.End, e.Location, "The admins'")); rec.Code != 204 {
		t.Fatalf("an admin's note: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/when/invites/settings", `{"id":"gev0000000002","hosts":["jordan.whitfield@heliosschool.org"]}`); rec.Code != 204 {
		t.Fatalf("a co-host: %d %s", rec.Code, rec.Body)
	}
	cohost := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, cohost, "PUT", "/api/when/overrides", body("Family Night", e.Start, e.End, e.Location, "Mine")); rec.Code != 204 {
		t.Fatalf("a co-host's edit: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r["Title"] != "Family Night" || r["Note"] != "The admins'" || cache.Model().Event("gev0000000002").Title != "Family Night" {
		t.Errorf("after the co-host's edit: %v", r)
	}
	if rec := call(t, cohost, "PUT", "/api/when/overrides/image", `{"id":"gev0000000002","image":"/category-images/night.jpg"}`); rec.Code != 204 || cache.Model().Event("gev0000000002").Image != "/category-images/night.jpg" {
		t.Errorf("a co-host's picture: %d", rec.Code)
	}
	raw, _ := json.Marshal(map[string]any{"id": "gev0000000002", "title": "Family Night", "start": e.Start, "end": e.End, "location": e.Location, "description": e.Description, "tags": tags, "keywords": e.Keywords})
	if rec := call(t, admin, "PUT", "/api/when/overrides", string(raw)); rec.Code != 204 || row()["Note"] != "The admins'" {
		t.Errorf("an edit sending no note: %d %v", rec.Code, row())
	}
}

func TestMyHeliosianFeed(t *testing.T) {
	handler, cache := testApp(t)
	parent := as("jordan.whitfield@heliosschool.org", handler)
	token := func() string {
		rec := call(t, parent, "POST", "/api/when/feeds/my-heliosian", `{}`)
		var body struct {
			Token string `json:"token"`
		}
		json.NewDecoder(rec.Body).Decode(&body)
		if rec.Code != 200 || body.Token == "" {
			t.Fatalf("my heliosian token: %d %s", rec.Code, rec.Body)
		}
		return body.Token
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
