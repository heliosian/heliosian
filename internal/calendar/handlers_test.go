package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

type directQueue struct{}

func (directQueue) Add(f func()) { f() }

func testApp(t *testing.T) (http.Handler, *Cache) {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, func() Roster { return roster }, nil, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	d := fakeDirectory{
		people: map[string]Person{"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true}},
		kids:   map[string][]Person{},
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, directQueue{}, nil, d, func() []string { return nil }, func(string) []Linked { return nil }, ImageSearch{}, Mail{})
	return mux, cache
}

func as(email string, handler http.Handler) http.Handler {
	return auth.Fixed(email, handler)
}

func call(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "https://calendar.local.heliosian.com:8080"+path, strings.NewReader(body))
	req.Host = "calendar.local.heliosian.com:8080"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestFeedRoute(t *testing.T) {
	mux, _ := testApp(t)
	rec := call(t, mux, http.MethodGet, "/feed/sample7feedtoken4jordan2whitfield.ics", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if body := rec.Body.String(); !strings.Contains(body, "X-WR-CALNAME:Whitfield school days") || !strings.Contains(body, "URL:https://calendar.local.heliosian.com:8080/events/a5@sample") {
		t.Errorf("feed body: %s", body)
	}
	if rec := call(t, mux, http.MethodGet, "/feed/nosuchtoken.ics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown token: %d", rec.Code)
	}
}

func TestFeedLifecycle(t *testing.T) {
	mux, cache := testApp(t)
	owner := as("jordan.whitfield@heliosschool.org", mux)
	other := as("mia.torres@heliosschool.org", mux)
	rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":"Just trips","classrooms":["Jays"],"tags":["Trip"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}
	var made struct{ Token, URL string }
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || len(made.Token) != 24 || made.URL != "https://calendar.local.heliosian.com:8080/feed/"+made.Token+".ics" {
		t.Fatalf("add answered %s", rec.Body.String())
	}
	if len(cache.Model().Feeds) != 2 || cache.Model().Feed(made.Token).Email != "jordan.whitfield@heliosschool.org" {
		t.Fatalf("feeds after add: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SUMMARY:Jays and Ravens Camping") || strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("new feed: %d %s", rec.Code, rec.Body.String())
	}
	// A second feed under a name the owner already uses gets a number.
	rec = call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":"just TRIPS","classrooms":["Jays"],"tags":["Trip"]}`)
	var twin struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &twin)
	if rec.Code != http.StatusOK || cache.Model().Feed(twin.Token).Name != "just TRIPS 2" {
		t.Errorf("twin name: %d %+v", rec.Code, cache.Model().Feed(twin.Token))
	}
	call(t, owner, http.MethodDelete, "/api/calendar/feeds", `{"token":"`+twin.Token+`"}`)
	if rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":"Bad","classrooms":["Penguins"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown classroom: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", rec.Code)
	}
	// A change keeps the address and carries the new filter from the next
	// fetch; someone else's change, or an empty name, is refused.
	if rec := call(t, other, http.MethodPut, "/api/calendar/feeds", `{"token":"`+made.Token+`","name":"Theirs","classrooms":["Jays"],"tags":["Trip"]}`); rec.Code != http.StatusForbidden {
		t.Errorf("someone else's change: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPut, "/api/calendar/feeds", `{"token":"`+made.Token+`","name":"","classrooms":["Jays"],"tags":["Trip"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("change to no name: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPut, "/api/calendar/feeds", `{"token":"`+made.Token+`","name":"Jays days","emoji":" 🚌 ","classrooms":["Jays"],"tags":["Trip","Schedule"]}`); rec.Code != http.StatusNoContent {
		t.Errorf("owner's change: %d %s", rec.Code, rec.Body.String())
	}
	if f := cache.Model().Feed(made.Token); f == nil || f.Name != "Jays days" || f.Emoji != "🚌" || strings.Join(f.Tags, ",") != "Trip,Schedule" || f.Email != "jordan.whitfield@heliosschool.org" {
		t.Errorf("feed after change: %+v", f)
	}
	if rec := call(t, mux, http.MethodGet, "/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "X-WR-CALNAME:Jays days") || !strings.Contains(rec.Body.String(), "Labor Day") {
		t.Errorf("changed feed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(t, other, http.MethodDelete, "/api/calendar/feeds", `{"token":"`+made.Token+`"}`); rec.Code != http.StatusForbidden {
		t.Errorf("someone else's removal: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodDelete, "/api/calendar/feeds", `{"token":"`+made.Token+`"}`); rec.Code != http.StatusNoContent {
		t.Errorf("owner's removal: %d %s", rec.Code, rec.Body.String())
	}
	if len(cache.Model().Feeds) != 1 || cache.Model().Feed(made.Token) != nil {
		t.Errorf("feeds after removal: %+v", cache.Model().Feeds)
	}
	if rec := call(t, mux, http.MethodGet, "/feed/"+made.Token+".ics", ""); rec.Code != http.StatusNotFound {
		t.Errorf("removed feed still serves: %d", rec.Code)
	}
}

func TestModelRoute(t *testing.T) {
	mux, _ := testApp(t)
	rec := call(t, as("jordan.whitfield@heliosschool.org", mux), http.MethodGet, "/api/calendar/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model: %d", rec.Code)
	}
	var view View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.User.Email != "jordan.whitfield@heliosschool.org" || len(view.Feeds) != 1 || len(view.Events) != 20 || len(view.Classrooms) != 9 {
		t.Errorf("view = user %s, %d feeds, %d events, %d classrooms", view.User.Email, len(view.Feeds), len(view.Events), len(view.Classrooms))
	}
}

// A shared link previews: the sign-in page's tags name the event at the
// path, or the calendar itself elsewhere, and the cards behind them draw.
func TestSharePreview(t *testing.T) {
	handler, cache := testApp(t)
	head := PreviewHead(cache, func(string) []Linked { return nil })
	req := httptest.NewRequest("GET", "https://when.local.heliosian.com:8080/events/a7@sample", nil)
	req.Host = "when.local.heliosian.com:8080"
	tags := head(req)
	for _, want := range []string{`property="og:title" content="International Night"`, `Thursday, September 24 · 4:00 – 6:00 PM`, `content="https://when.local.heliosian.com:8080/share/a7@sample.png"`} {
		if !strings.Contains(tags, want) {
			t.Errorf("event tags lack %s:\n%s", want, tags)
		}
	}
	req = httptest.NewRequest("GET", "https://when.local.heliosian.com:8080/feeds", nil)
	req.Host = "when.local.heliosian.com:8080"
	tags = head(req)
	if !strings.Contains(tags, `og:title" content="Helios When"`) || !strings.Contains(tags, "One calendar") || !strings.Contains(tags, "/share/upcoming.png") {
		t.Errorf("site tags:\n%s", tags)
	}
	for _, path := range []string{"/share/a7@sample.png", "/share/upcoming.png"} {
		rec := call(t, handler, "GET", path, "")
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 1000 {
			t.Errorf("%s: %d %s %d bytes", path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
	if rec := call(t, handler, "GET", "/share/nope.png", ""); rec.Code != 404 {
		t.Errorf("missing card: %d", rec.Code)
	}
}

// Provenance - the classifier's filing and the admins' corrections - is in
// the view for an admin and absent for anyone else; the source links and
// who added an event are for everyone.
func TestProvenanceForAdmins(t *testing.T) {
	handler, _ := testApp(t)
	var view View
	rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/calendar/model", "")
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Provenance != nil {
		t.Errorf("a parent sees provenance: %v", view.Provenance)
	}
	var hand *Event
	for _, e := range view.Events {
		if e.ID == "7QK2M4XN" {
			hand = e
		}
	}
	// The fake directory knows no Dana, so no name comes with the address.
	if hand == nil || hand.AddedBy != "dana.hawkins@heliosschool.org" || hand.Added == "" {
		t.Errorf("hand-added event = %+v", hand)
	}
	rec = call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/calendar/model", "")
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	p := view.Provenance["a5@sample"]
	if p == nil || p.Model == "" || len(p.Corrected) == 0 || p.Note != "Say which classrooms" {
		t.Errorf("admin provenance for a5@sample = %+v", p)
	}
}

// A saved view is the person's defaults on every device - in the view as
// user.saved and under Heliosian's Upcoming Events - until forgotten.
func TestSavedView(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	rec := call(t, as(me, handler), "POST", "/api/calendar/settings", `{"classrooms":["Hawks"],"tags":["Community","HCA","Nonsense"]}`)
	if rec.Code != 204 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	var view View
	rec = call(t, as(me, handler), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	if view.User.Saved == nil || strings.Join(view.User.Saved.Classrooms, ",") != "Hawks" || strings.Join(view.User.Saved.Tags, ",") != "Community,HCA" {
		t.Errorf("saved view = %+v", view.User.Saved)
	}
	for _, u := range cache.Model().Upcoming(fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}, me, nil, now(), 0) {
		if !strings.Contains(u.Title, "Hawks") && !strings.Contains(u.Title, "CAFE") && u.Title != "International Night" && u.Title != "Halloween Parade" && u.Title != "HCA Meeting" && u.Title != "All School Movie Night" && u.Title != "Cocoa & Cookies" && u.Title != "Talent Show" && u.Title != "Back to School Social" && u.Title != "Spring Celebration" {
			t.Errorf("upcoming under the saved view lists %q", u.Title)
		}
	}
	rec = call(t, as(me, handler), "DELETE", "/api/calendar/settings", "")
	if rec.Code != 204 {
		t.Fatalf("forget: %d %s", rec.Code, rec.Body)
	}
	var again View
	rec = call(t, as(me, handler), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&again)
	if again.User.Saved != nil {
		t.Errorf("saved view survives forgetting: %+v", again.User.Saved)
	}
}

// Everyone's default calendar is the first in their rail - My Heliosian,
// the calendar's own defaults, to start; then whichever they make default
// or drag to the top. The page opens to it and Upcoming is read under it.
func TestDefaultCalendar(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	dir := fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}
	// Under My Heliosian - every classroom for someone the directory does
	// not know, the default categories - Community events are in.
	found := false
	for _, u := range cache.Model().Upcoming(dir, me, nil, now(), 0) {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian, upcoming leaves out International Night")
	}
	// The sample calendar carries Schedule, Trip and Celebration of Learning
	// for Jays and Ospreys: no Community events once it is the default.
	if rec := call(t, viewer, "POST", "/api/calendar/default", `{"token":"nonsense"}`); rec.Code != 400 {
		t.Errorf("a stranger's token: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", "/api/calendar/default", `{"token":"sample7feedtoken4jordan2whitfield"}`); rec.Code != 204 {
		t.Fatalf("default: %d %s", rec.Code, rec.Body)
	}
	if chosen := cache.Model().DefaultCalendar(me); chosen == nil || chosen.Token != "sample7feedtoken4jordan2whitfield" {
		t.Errorf("default calendar = %+v", chosen)
	}
	if mine := cache.Model().MyCalendars(me); len(mine) != 2 || mine[0].Token != "sample7feedtoken4jordan2whitfield" || !mine[1].Locked {
		t.Errorf("rail = %+v", mine)
	}
	for _, u := range cache.Model().Upcoming(dir, me, nil, now(), 0) {
		if u.Title == "International Night" {
			t.Errorf("under the saved calendar, upcoming lists %q", u.Title)
		}
	}
	// Under My Heliosian by its token, the picker's way, the Community
	// event is back; making it the default again puts it first.
	found = false
	for _, u := range cache.Model().UpcomingUnder(dir, me, nil, now(), 0, MyHeliosianToken) {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian by token, upcoming leaves out International Night")
	}
	// The rail's month reads the same way: the default leaves the event
	// out of September, My Heliosian by token puts it back.
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
	if rec := call(t, viewer, "POST", "/api/calendar/default", `{"token":"`+MyHeliosianToken+`"}`); rec.Code != 204 || cache.Model().DefaultCalendar(me) != nil {
		t.Errorf("back to My Heliosian: %d, default %+v", rec.Code, cache.Model().DefaultCalendar(me))
	}
	// My Heliosian takes a name and a mark of the person's own; its filters
	// stay the calendar's.
	if rec := call(t, viewer, "PUT", "/api/calendar/feeds", `{"token":"`+MyHeliosianToken+`","name":"Home base","emoji":"🏠","classrooms":["Jays"],"tags":["Trip"]}`); rec.Code != 204 {
		t.Errorf("rename My Heliosian: %d %s", rec.Code, rec.Body)
	}
	if home := cache.Model().MyHeliosian(me); home.Name != "Home base" || home.Emoji != "🏠" || !home.Locked || len(home.Classrooms) != 0 {
		t.Errorf("My Heliosian = %+v", home)
	}
	// Dragging another to the top, as the order route says it, makes it
	// the default; an order that is not each of theirs once is refused.
	rec := call(t, viewer, "POST", "/api/calendar/feeds", `{"name":"Everything","classrooms":[],"tags":[]}`)
	var made struct{ Token string }
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec := call(t, viewer, "PUT", "/api/calendar/feeds/order", `{"tokens":["`+made.Token+`"]}`); rec.Code != 400 {
		t.Errorf("a short order: %d", rec.Code)
	}
	if rec := call(t, viewer, "PUT", "/api/calendar/feeds/order", `{"tokens":["`+made.Token+`","`+MyHeliosianToken+`","sample7feedtoken4jordan2whitfield"]}`); rec.Code != 204 {
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

// An admin adds an event, repeated every so many weeks, and replaces an
// event's search words; a parent can do neither.
func TestAdminAddsAndCorrects(t *testing.T) {
	handler, cache := testApp(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	rec := call(t, admin, "POST", "/api/calendar/events", `{"title":"Chess Club","start":"2026-10-01 15:30","end":"2026-10-01 16:30","tags":["Jays","Clubs"],"repeatWeeks":4,"repeatTimes":2}`)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var made struct{ IDs []string }
	json.NewDecoder(rec.Body).Decode(&made)
	if len(made.IDs) != 3 {
		t.Fatalf("ids = %v", made.IDs)
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
	if rec := call(t, admin, "POST", "/api/calendar/events", `{"title":"Bad","start":"not a date","tags":["Clubs"]}`); rec.Code != 400 {
		t.Errorf("bad date accepted: %d", rec.Code)
	}
	rec = call(t, admin, "POST", "/api/calendar/keywords", `{"id":"`+made.IDs[0]+`","keywords":["chess","board games"]}`)
	if rec.Code != 204 || strings.Join(cache.Model().Event(made.IDs[0]).Keywords, ",") != "chess,board games" {
		t.Errorf("keywords: %d %v", rec.Code, cache.Model().Event(made.IDs[0]).Keywords)
	}
	if p := cache.Model().Provenance[made.IDs[0]]; p == nil || strings.Join(p.Corrected, ",") != "Keywords" {
		t.Errorf("provenance = %+v", p)
	}
	rec = call(t, admin, "POST", "/api/calendar/events/when", `{"id":"`+made.IDs[0]+`","start":"2026-10-02 16:00","end":"2026-10-02 17:00"}`)
	if rec.Code != 204 || cache.Model().Event(made.IDs[0]).Start != "2026-10-02 16:00" {
		t.Errorf("move: %d %s", rec.Code, cache.Model().Event(made.IDs[0]).Start)
	}
	if rec := call(t, admin, "POST", "/api/calendar/events/when", `{"id":"a7@sample","start":"2026-10-02 16:00"}`); rec.Code != 400 {
		t.Errorf("an imported event moved from the list: %d", rec.Code)
	}
	parent := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, parent, "POST", "/api/calendar/keywords", `{"id":"`+made.IDs[0]+`","keywords":["x"]}`); rec.Code != 403 {
		t.Errorf("parent set keywords: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"title":"X","start":"2026-10-01","tags":["Clubs"]}`); rec.Code != 403 {
		t.Errorf("parent added an event: %d", rec.Code)
	}
}

// A feed takes Misc, the one built-in tag a sheet event wears, and refuses
// the built-ins no feed could carry.
func TestFeedTags(t *testing.T) {
	handler, _ := testApp(t)
	me := as("jordan.whitfield@heliosschool.org", handler)
	if rec := call(t, me, "POST", "/api/calendar/feeds", `{"name":"Odds and ends","classrooms":["Jays"],"tags":["Misc","Trip"]}`); rec.Code != 200 {
		t.Errorf("feed with Misc: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/calendar/feeds", `{"name":"Parties","classrooms":["Jays"],"tags":["Celebrate","Going"]}`); rec.Code != 200 {
		t.Errorf("feed with Celebrate and Going: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, me, "POST", "/api/calendar/feeds", `{"name":"Nope","classrooms":["Jays"],"tags":["Nonsense"]}`); rec.Code != 400 {
		t.Errorf("feed with a made-up tag: %d", rec.Code)
	}
}

// A feed carries the other apps' events, read for its owner: a party the
// owner is going to comes through under Celebrate, and a feed on Going
// alone is the family's own calendar.
func TestFeedCarriesLinked(t *testing.T) {
	handler, cache := testApp(t)
	linked := []Linked{{Source: SourceCelebrate, ID: "P9", Title: "Fondue Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineGoing}}
	f := &Feed{Token: "t", Email: "jordan.whitfield@heliosschool.org", Name: "Mine", Tags: []string{TagGoing}}
	out := string(ICS(cache.Model(), f, linked, "https://when.local.heliosian.com:8080", now()))
	if !strings.Contains(out, "SUMMARY:Fondue Night") || !strings.Contains(out, "URL:https://when.local.heliosian.com:8080/events/celebrate%2FP9") {
		t.Errorf("feed lacks the party:\n%s", out)
	}
	if strings.Contains(out, "SUMMARY:Halloween Parade") {
		t.Errorf("a Going feed carries an event the family is not in")
	}
	_ = handler
}

// An answer is the viewer's: yes and no mark the event and hidden takes
// it out of Upcoming and the feeds while it stays on the calendar; a blank
// answer clears it, and nonsense is refused.
func TestAnswers(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"maybe"}`); rec.Code != 400 {
		t.Errorf("nonsense answer: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"hidden"}`); rec.Code != 204 {
		t.Fatalf("hide: %d %s", rec.Code, rec.Body)
	}
	var view View
	rec := call(t, viewer, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	if view.User.Answers["a7@sample"] != AnswerHidden {
		t.Errorf("answers = %v", view.User.Answers)
	}
	dir := fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}
	for _, u := range cache.Model().Upcoming(dir, me, nil, now(), 0) {
		if u.ID == "a7@sample" {
			t.Errorf("a hidden event is in Upcoming")
		}
	}
	f := &Feed{Token: "t", Email: me, Name: "Mine"}
	if strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.local.heliosian.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a hidden event is in the owner's feed")
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"no"}`); rec.Code != 204 {
		t.Fatalf("no: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.local.heliosian.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("an event the owner said no to is in their feed")
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":""}`); rec.Code != 204 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.local.heliosian.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a cleared answer left the event out of the feed")
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("yes: %d %s", rec.Code, rec.Body)
	}
	found := false
	for _, u := range cache.Model().Upcoming(dir, me, nil, now(), 0) {
		if u.ID == "a7@sample" {
			found = u.Answer == AnswerYes
		}
	}
	if !found {
		t.Errorf("a yes is not on the Upcoming card")
	}
	// A yes files the event under Going for this viewer - in the view, in
	// their feed - and for nobody else; the model's own event stays as loaded.
	var mine View
	rec = call(t, viewer, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&mine)
	if i := slices.IndexFunc(mine.Events, func(e *Event) bool { return e.ID == "a7@sample" }); i < 0 || !slices.Contains(mine.Events[i].Tags, TagGoing) {
		t.Errorf("a yes is not under Going in the viewer's events")
	}
	var other View
	rec = call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&other)
	if i := slices.IndexFunc(other.Events, func(e *Event) bool { return e.ID == "a7@sample" }); i < 0 || slices.Contains(other.Events[i].Tags, TagGoing) {
		t.Errorf("one viewer's yes is under Going for another")
	}
	if slices.Contains(cache.Model().Event("a7@sample").Tags, TagGoing) {
		t.Errorf("a yes changed the model's own event")
	}
	going := &Feed{Token: "g", Email: me, Name: "Going", Tags: []string{TagGoing}}
	if !strings.Contains(string(ICS(cache.Model(), going, nil, "https://when.local.heliosian.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a yes is not in the owner's Going feed")
	}
	if got := invite("Helios Calendar <when@heliosian.com>", me, cache.Model().Event("a7@sample"), "https://when.heliosian.com/events/a7@sample", now()); !strings.Contains(got, "METHOD:REQUEST") || !strings.Contains(got, "ORGANIZER;CN=Helios Calendar:mailto:when@heliosian.com") || !strings.Contains(got, "ATTENDEE;CN="+me) {
		t.Errorf("invite:\n%s", got)
	}
}

// Admins see who said yes and no to an event, by name; nobody sees who
// hid one, and a parent sees none of it.
func TestResponsesForAdmins(t *testing.T) {
	handler, _ := testApp(t)
	call(t, as("jordan.whitfield@heliosschool.org", handler), "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"yes"}`)
	call(t, as("dana.hawkins@heliosschool.org", handler), "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"no"}`)
	call(t, as("robin.whitfield@heliosschool.org", handler), "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"hidden"}`)
	var view View
	rec := call(t, as("dana.hawkins@heliosschool.org", handler), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	r := view.Responses["a7@sample"]
	if r == nil || len(r.Yes) != 1 || r.Yes[0].Name != "Jordan" || len(r.No) != 1 {
		t.Errorf("responses = %+v", r)
	}
	rec = call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/calendar/model", "")
	var parent View
	json.NewDecoder(rec.Body).Decode(&parent)
	if parent.Responses != nil {
		t.Errorf("a parent sees responses: %v", parent.Responses)
	}
}
