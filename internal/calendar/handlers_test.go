package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type directQueue struct{}

func (directQueue) Add(f func()) { f() }

var testNow = time.Date(2026, 9, 17, 12, 0, 0, 0, Location)

func pinnedClock() time.Time {
	return testNow
}

func TestMain(m *testing.M) {
	now = pinnedClock
	os.Exit(m.Run())
}

var sheet *data.Dir

func sampleCache(t *testing.T) *Cache {
	t.Helper()
	t.Chdir("../..")
	sheet = &data.Dir{Root: "sampledata"}
	cache, err := NewCache(sheet, sheet, func() Roster { return roster }, nil, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func testApp(t *testing.T) (http.Handler, *Cache) {
	t.Helper()
	cache := sampleCache(t)
	d := fakeDirectory{
		people: map[string]Person{"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true}},
		kids:   map[string][]Person{},
	}
	mux := http.NewServeMux()
	Register(mux, cache, nil, d, func() []string { return nil }, func(string) []Linked { return nil }, nil, nil, ImageSearch{}, Mail{})
	return mux, cache
}

func changeLog(t *testing.T) []string {
	t.Helper()
	_, rows, err := sheet.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, row := range rows {
		out = append(out, row["Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"]+"|"+row["Previous"])
	}
	return out
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
	if body := rec.Body.String(); !strings.Contains(body, "X-WR-CALNAME:Whitfield school days") || !strings.Contains(body, "URL:https://calendar.heliosiandev.com:8080/e/a5@sample") {
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
	rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":"Just trips","classrooms":["Jays"],"tags":["Trip"]}`)
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
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "X-WR-CALNAME:Jays days") || !strings.Contains(rec.Body.String(), "Labor Day") {
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
	if rec := call(t, mux, http.MethodGet, "/open/feed/"+made.Token+".ics", ""); rec.Code != http.StatusNotFound {
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

func TestSharePreview(t *testing.T) {
	handler, cache := testApp(t)
	head := PreviewHead(cache, func(string) []Linked { return nil })
	req := httptest.NewRequest("GET", "https://when.heliosiandev.com:8080/e/a7@sample", nil)
	req.Host = "when.heliosiandev.com:8080"
	tags := head(req)
	for _, want := range []string{`property="og:title" content="International Night"`, `Thursday, September 24 · 4:00 – 6:00 PM`, `content="https://when.heliosiandev.com:8080/open/share/a7@sample.png"`} {
		if !strings.Contains(tags, want) {
			t.Errorf("event tags lack %s:\n%s", want, tags)
		}
	}
	req = httptest.NewRequest("GET", "https://when.heliosiandev.com:8080/feeds", nil)
	req.Host = "when.heliosiandev.com:8080"
	tags = head(req)
	if !strings.Contains(tags, `og:title" content="Helios When"`) || !strings.Contains(tags, "One calendar") || !strings.Contains(tags, "/open/share/upcoming.png") {
		t.Errorf("site tags:\n%s", tags)
	}
	for _, path := range []string{"/open/share/a7@sample.png", "/open/share/upcoming.png"} {
		rec := call(t, handler, "GET", path, "")
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 1000 {
			t.Errorf("%s: %d %s %d bytes", path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
	if rec := call(t, handler, "GET", "/open/share/nope.png", ""); rec.Code != 404 {
		t.Errorf("missing card: %d", rec.Code)
	}
}

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

func TestDefaultCalendar(t *testing.T) {
	handler, cache := testApp(t)
	me := "jordan.whitfield@heliosschool.org"
	viewer := as(me, handler)
	dir := fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}
	found := false
	for _, u := range cache.Model().Upcoming(dir, me, nil, now(), 0) {
		found = found || u.Title == "International Night"
	}
	if !found {
		t.Errorf("under My Heliosian, upcoming leaves out International Night")
	}
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
	if rec := call(t, viewer, "POST", "/api/calendar/default", `{"token":"`+MyHeliosianToken+`"}`); rec.Code != 204 || cache.Model().DefaultCalendar(me) != nil {
		t.Errorf("back to My Heliosian: %d, default %+v", rec.Code, cache.Model().DefaultCalendar(me))
	}
	if rec := call(t, viewer, "PUT", "/api/calendar/feeds", `{"token":"`+MyHeliosianToken+`","name":"Home base","emoji":"🏠","classrooms":["Jays"],"tags":["Trip"]}`); rec.Code != 204 {
		t.Errorf("rename My Heliosian: %d %s", rec.Code, rec.Body)
	}
	if home := cache.Model().MyHeliosian(me); home.Name != "Home base" || home.Emoji != "🏠" || !home.Locked || len(home.Classrooms) != 0 {
		t.Errorf("My Heliosian = %+v", home)
	}
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

type keptMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (k *keptMail) Send(ctx context.Context, m mail.Message) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sent = append(k.sent, m)
	return nil
}

func (k *keptMail) all() []mail.Message {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Clone(k.sent)
}

func TestAdminsToldOfSharedEvents(t *testing.T) {
	cache := sampleCache(t)
	d := fakeDirectory{people: map[string]Person{"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true}}, kids: map[string][]Person{}}
	kept := &keptMail{}
	mux := http.NewServeMux()
	Register(mux, cache, nil, d, func() []string { return nil }, func(string) []Linked { return nil }, nil, nil, ImageSearch{}, Mail{Sender: kept, From: "Helios When <when@example.org>"})
	parent := as("jordan.whitfield@heliosschool.org", mux)
	admin := as("dana.hawkins@heliosschool.org", mux)
	wait := func(n int) []mail.Message {
		for i := 0; i < 50 && len(kept.all()) < n; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		return kept.all()
	}
	call(t, parent, "POST", "/api/calendar/events", `{"title":"Bake sale","start":"2026-10-01 15:00","tags":["Jays","Community"],"sharing":"Public"}`)
	sent := wait(1)
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Event to approve: Bake sale") || !slices.Contains(sent[0].To, "dana.hawkins@heliosschool.org") || !strings.Contains(sent[0].Text, "Jordan") || !strings.Contains(sent[0].HTML, "Review the event") {
		t.Errorf("public event mail = %+v", sent)
	}
	call(t, parent, "POST", "/api/calendar/events", `{"title":"Sam\u2019s party","start":"2026-10-03 14:00","tags":["Jays"],"sharing":"Link"}`)
	sent = wait(2)
	if len(sent) != 2 || !strings.HasPrefix(sent[1].Subject, "Link event added: Sam") || !strings.Contains(sent[1].HTML, "See the event") || strings.Contains(sent[1].Text, "waiting for approval") {
		t.Errorf("link event mail = %+v", sent)
	}
	call(t, admin, "POST", "/api/calendar/events", `{"title":"Admin's own","start":"2026-10-05","tags":["Jays"],"sharing":"Public"}`)
	sent = wait(3)
	if len(sent) != 3 || !strings.HasPrefix(sent[2].Subject, "Event to approve: Admin's own") {
		t.Errorf("an admin's own public event waits and is mailed about too: %+v", sent)
	}
	var v View
	rec := call(t, parent, "GET", "/api/calendar/model", "")
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
	if rec := call(t, parent, "PUT", "/api/calendar/events", `{"id":"`+party.ID+`","title":"`+party.Title+`","start":"2026-10-03 14:00","tags":["Jays"],"sharing":"Public"}`); rec.Code != 204 {
		t.Fatalf("switch to public: %d %s", rec.Code, rec.Body)
	}
	sent = wait(4)
	if e := cache.Model().Event(party.ID); e == nil || !e.Pending || len(sent) != 4 || !strings.HasPrefix(sent[3].Subject, "Event to approve: Sam") {
		t.Errorf("after the switch: event %+v, mail %d", e, len(sent))
	}
	other := as("robin.whitfield@heliosschool.org", mux)
	if rec := call(t, other, "GET", "/api/calendar/event?id="+party.ID, ""); rec.Code != 200 {
		t.Errorf("the link of an event waiting for approval: %d", rec.Code)
	}
	if rec := call(t, other, "POST", "/api/calendar/rsvp", `{"id":"`+party.ID+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("a yes by link while waiting: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminAddsAndCorrects(t *testing.T) {
	handler, cache := testApp(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	rec := call(t, admin, "POST", "/api/calendar/events", `{"title":"Chess Club","start":"2026-10-01 15:30","end":"2026-10-01 16:30","tags":["Jays","Clubs"],"sharing":"Public","repeatWeeks":4,"repeatTimes":2}`)
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var made struct{ IDs []string }
	json.NewDecoder(rec.Body).Decode(&made)
	for _, id := range made.IDs {
		if e := cache.Model().Event(id); e == nil || !e.Pending {
			t.Errorf("an admin's event went straight on: %+v", e)
		}
		if rec := call(t, admin, "POST", "/api/calendar/events/approve", `{"id":"`+id+`"}`); rec.Code != 204 {
			t.Fatalf("approve %s: %d", id, rec.Code)
		}
	}
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
	if rec := call(t, admin, "POST", "/api/calendar/events", `{"title":"Bad","start":"not a date","tags":["Clubs"],"sharing":"Public"}`); rec.Code != 400 {
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
	rec = call(t, parent, "POST", "/api/calendar/events", `{"title":"Bake sale","start":"2026-10-01 15:00","end":"2026-10-01 17:00","tags":["Jays","Community"],"sharing":"Public","repeatWeeks":1,"repeatTimes":3}`)
	var shared struct {
		IDs     []string `json:"ids"`
		Pending bool     `json:"pending"`
	}
	json.Unmarshal(rec.Body.Bytes(), &shared)
	if rec.Code != 200 || !shared.Pending || len(shared.IDs) != 1 {
		t.Fatalf("parent shared an event: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || !e.Pending || cache.Model().Event(shared.IDs[0]) != cache.Model().Pending[len(cache.Model().Pending)-1] {
		t.Errorf("shared event = %+v", e)
	}
	if cache.Model().AnswerOf("jordan.whitfield@heliosschool.org", shared.IDs[0]) != AnswerYes {
		t.Errorf("the host is not going to their own event")
	}
	var hostView View
	rec = call(t, parent, "GET", "/api/calendar/model", "")
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
		rec := call(t, who, "GET", "/api/calendar/model", "")
		json.NewDecoder(rec.Body).Decode(&v)
		return slices.ContainsFunc(v.Events, func(e *Event) bool { return e.ID == shared.IDs[0] && (e.Pending || e.Declined) })
	}
	other := as("robin.whitfield@heliosschool.org", handler)
	if !sees(parent) || !sees(admin) || sees(other) {
		t.Errorf("pending event seen by parent %v, admin %v, another %v", sees(parent), sees(admin), sees(other))
	}
	if rec := call(t, parent, "POST", "/api/calendar/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 403 {
		t.Errorf("parent approved: %d", rec.Code)
	}
	if rec := call(t, admin, "POST", "/api/calendar/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || e.Pending || e.Status != StatusApproved || !slices.Contains(cache.Model().Events, e) {
		t.Errorf("approved event = %+v", e)
	}
	if rec := call(t, parent, "PUT", "/api/calendar/events", `{"id":"`+shared.IDs[0]+`","title":"Bake sale!","start":"2026-10-01 15:30","end":"2026-10-01 17:30","location":"Gym","tags":["Jays","Community"],"image":"/category-images/cake.jpg","sharing":"Public"}`); rec.Code != 204 {
		t.Errorf("owner's edit: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || e.Title != "Bake sale!" || e.Location != "Gym" || e.Image != "/category-images/cake.jpg" || e.Start != "2026-10-01 15:30" || e.Pending {
		t.Errorf("edited event = %+v", e)
	}
	if rec := call(t, other, "PUT", "/api/calendar/events", `{"id":"`+shared.IDs[0]+`","title":"Mine now","start":"2026-10-01","tags":["Jays"],"sharing":"Public"}`); rec.Code != 403 {
		t.Errorf("another parent's edit: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/calendar/events", `{"title":"Sam\u2019s birthday","start":"2026-10-03 14:00","end":"2026-10-03 16:00","tags":["Jays","Community"],"sharing":"Link"}`)
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
	if rec := call(t, other, "GET", "/api/calendar/event?id="+shared.IDs[0], ""); rec.Code != 200 {
		t.Errorf("a link event by its link: %d", rec.Code)
	}
	if rec := call(t, other, "POST", "/api/calendar/rsvp", `{"id":"`+shared.IDs[0]+`","answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("yes to a link event: %d %s", rec.Code, rec.Body)
	}
	var otherView View
	rec = call(t, other, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&otherView)
	if i := slices.IndexFunc(otherView.Events, func(e *Event) bool { return e.ID == shared.IDs[0] }); i < 0 || !slices.Contains(otherView.Events[i].Tags, TagGoing) {
		t.Errorf("a yes did not put the link event under Going on the other's calendar")
	}
	found := false
	dir := fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}
	for _, u := range cache.Model().Upcoming(dir, "robin.whitfield@heliosschool.org", nil, now(), 0) {
		found = found || u.ID == shared.IDs[0]
	}
	if !found {
		t.Errorf("a yes did not put the link event in the other's Upcoming")
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"title":"Unsaid","start":"2026-10-04","tags":["Jays"]}`); rec.Code != 400 {
		t.Errorf("no sharing: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"title":"Unsaid","start":"2026-10-04","tags":["Jays"],"sharing":"Private"}`); rec.Code != 400 {
		t.Errorf("an old sharing word: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"id":"Sams-Party","title":"Sam\u2019s party","start":"2026-10-04","tags":["Jays"],"sharing":"Link"}`); rec.Code != 200 || cache.Model().Event("sams-party") == nil {
		t.Errorf("chosen address: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, handler, "GET", "/open/share/sams-party.png", ""); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("a link event's card: %d", rec.Code)
	}
	if head := PreviewHead(cache, func(string) []Linked { return nil })(httptest.NewRequest("GET", "https://when.heliosiandev.com:8080/e/sams-party", nil)); !strings.Contains(head, "Sam") || !strings.Contains(head, "/open/share/sams-party.png") {
		t.Errorf("a link event's preview:\n%s", head)
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"id":"sams-party","title":"Again","start":"2026-10-04","tags":["Jays"],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("a taken address: %d", rec.Code)
	}
	if rec := call(t, parent, "POST", "/api/calendar/events", `{"id":"a/b","title":"Odd","start":"2026-10-04","tags":["Jays"],"sharing":"Public"}`); rec.Code != 400 {
		t.Errorf("an ill-formed address: %d", rec.Code)
	}
	rec = call(t, parent, "POST", "/api/calendar/events", `{"title":"Not this","start":"2026-10-02","tags":["Jays"],"sharing":"Public"}`)
	json.Unmarshal(rec.Body.Bytes(), &shared)
	if rec := call(t, admin, "POST", "/api/calendar/events/decline", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 {
		t.Errorf("decline: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event(shared.IDs[0]); e == nil || !e.Declined || e.Status != StatusDeclined || slices.Contains(cache.Model().Events, e) {
		t.Errorf("declined event = %+v", e)
	}
	if !sees(parent) || sees(other) {
		t.Errorf("declined event seen by parent %v, another %v", sees(parent), sees(other))
	}
	if rec := call(t, admin, "POST", "/api/calendar/events/approve", `{"id":"`+shared.IDs[0]+`"}`); rec.Code != 204 || cache.Model().Event(shared.IDs[0]).Status != StatusApproved {
		t.Errorf("approve after decline: %d", rec.Code)
	}
}

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

func TestFeedCarriesLinked(t *testing.T) {
	handler, cache := testApp(t)
	linked := []Linked{{Source: SourceCelebrate, ID: "P9", Title: "Fondue Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineGoing}}
	f := &Feed{Token: "t", Email: "jordan.whitfield@heliosschool.org", Name: "Mine", Tags: []string{TagGoing}}
	out := string(ICS(cache.Model(), f, linked, "https://when.heliosiandev.com:8080", now()))
	if !strings.Contains(out, "SUMMARY:Fondue Night") || !strings.Contains(out, "URL:https://when.heliosiandev.com:8080/e/celebrate/P9") {
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
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"perhaps"}`); rec.Code != 400 {
		t.Errorf("nonsense answer: %d", rec.Code)
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"maybe"}`); rec.Code != 204 || cache.Model().AnswerOf(me, "a7@sample") != AnswerMaybe {
		t.Errorf("maybe: %d %s", rec.Code, rec.Body)
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
	if strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a hidden event is in the owner's feed")
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":"no"}`); rec.Code != 204 {
		t.Fatalf("no: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("an event the owner said no to is in their feed")
	}
	if rec := call(t, viewer, "POST", "/api/calendar/rsvp", `{"id":"a7@sample","answer":""}`); rec.Code != 204 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(string(ICS(cache.Model(), f, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
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
	if !strings.Contains(string(ICS(cache.Model(), going, nil, "https://when.heliosiandev.com:8080", now())), "SUMMARY:International Night") {
		t.Errorf("a yes is not in the owner's Going feed")
	}
	if got := invite("Helios When <when@reply.heliosian.com>", me, cache.Model().Event("a7@sample"), "https://when.heliosian.com/e/a7@sample", now()); !strings.Contains(got, "METHOD:REQUEST") || !strings.Contains(got, "ORGANIZER;CN=Helios When:mailto:when@reply.heliosian.com") || !strings.Contains(got, "ATTENDEE;CN="+me) {
		t.Errorf("invite:\n%s", got)
	}
}

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

func TestOverrideFromThePage(t *testing.T) {
	handler, cache := testApp(t)
	admin := as("dana.hawkins@heliosschool.org", handler)
	e := cache.Model().Event("a2@sample")
	builtIn := map[string]bool{}
	for _, tag := range cache.Model().Tags {
		builtIn[tag.Name] = tag.BuiltIn
	}
	tags := slices.DeleteFunc(slices.Clone(e.Tags), func(t string) bool { return builtIn[t] })
	body := func(title, start, end, location, note string) string {
		raw, _ := json.Marshal(map[string]any{"id": "a2@sample", "title": title, "start": start, "end": end, "location": location, "description": e.Description, "tags": tags, "keywords": e.Keywords, "note": note})
		return string(raw)
	}
	row := func() map[string]string {
		for _, r := range readTables(t, sheet)[OverridesTab] {
			if r["Event ID"] == "a2@sample" {
				return r
			}
		}
		return nil
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", body("Back to School Night", e.Start, e.End, e.Location, "Shorter")); rec.Code != 204 {
		t.Fatalf("override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r == nil || r["Title"] != "Back to School Night" || r["Start"] != "" || r["Location"] != "" || r["Tags"] != "" || r["Keywords"] != "" || r["Note"] != "Shorter" {
		t.Errorf("row after the title: %v", r)
	}
	if got := cache.Model().Event("a2@sample"); got.Title != "Back to School Night" || strings.Join(cache.Model().Provenance["a2@sample"].Corrected, ",") != "Title" {
		t.Errorf("event after: %q corrected %v", got.Title, cache.Model().Provenance["a2@sample"].Corrected)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", body("LS Back to School Night", "2026-08-27 18:30", "2026-08-27 20:00", "", "")); rec.Code != 204 {
		t.Fatalf("second override: %d %s", rec.Code, rec.Body)
	}
	if r := row(); r["Title"] != "" || r["Start"] != "2026-08-27 18:30" || r["End"] != "2026-08-27 20:00" || r["Location"] != Clear || r["Note"] != "" {
		t.Errorf("row after moving and clearing: %v", r)
	}
	if got := cache.Model().Event("a2@sample"); got.Location != "" || got.Start != "2026-08-27 18:30" {
		t.Errorf("event after moving: %+v", got)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", body("LS Back to School Night", e.Start, e.End, e.Location, "")); rec.Code != 204 || row() != nil {
		t.Errorf("everything put back: %d, row %v", rec.Code, row())
	}
	addressed := func(id, address string) string {
		ev := cache.Model().Event(id)
		raw, _ := json.Marshal(map[string]any{"id": id, "title": ev.Title, "start": ev.Start, "end": ev.End, "location": ev.Location, "description": ev.Description, "tags": slices.DeleteFunc(slices.Clone(ev.Tags), func(t string) bool { return builtIn[t] }), "keywords": ev.Keywords, "address": address})
		return string(raw)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", addressed("a2@sample", "Back-To-School")); rec.Code != 204 {
		t.Fatalf("an address: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Event("back-to-school"); got == nil || got.ID != "a2@sample" || EventPath(got) != "/e/back-to-school" || row()["Address"] != "back-to-school" || row()["Title"] != "" {
		t.Errorf("by its address: %+v, row %v", got, row())
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", addressed("a5@sample", "back-to-school")); rec.Code != 400 || !strings.Contains(rec.Body.String(), "already another event's") {
		t.Errorf("a taken address: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", addressed("a5@sample", "camping trip!")); rec.Code != 400 {
		t.Errorf("a bad address: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", addressed("a2@sample", "")); rec.Code != 204 || row() != nil || cache.Model().Event("back-to-school") != nil {
		t.Errorf("the address taken away: %d, row %v", rec.Code, row())
	}
	var v InviteView
	json.NewDecoder(call(t, admin, "GET", "/api/calendar/invites?id=a2@sample", "").Body).Decode(&v)
	if !v.Host || !v.AdminHost || len(v.Hosts) != 0 {
		t.Errorf("the admin's view: host %v adminHost %v hosts %v", v.Host, v.AdminHost, v.Hosts)
	}
	v = InviteView{}
	json.NewDecoder(call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/calendar/invites?id=a2@sample", "").Body).Decode(&v)
	if v.Host || v.AdminHost {
		t.Errorf("a parent's view: host %v adminHost %v", v.Host, v.AdminHost)
	}
	if !v.ListPrivate || v.Coming != nil {
		t.Errorf("a parent's view of a closed list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/invites/settings", `{"id":"a2@sample","publicList":true}`); rec.Code != 204 {
		t.Fatalf("opening the list: %d %s", rec.Code, rec.Body)
	}
	v = InviteView{}
	json.NewDecoder(call(t, as("jordan.whitfield@heliosschool.org", handler), "GET", "/api/calendar/invites?id=a2@sample", "").Body).Decode(&v)
	if v.ListPrivate || v.Coming == nil {
		t.Errorf("a parent's view of an open list: private %v coming %v", v.ListPrivate, v.Coming)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides/image", `{"id":"a2@sample","image":"/category-images/night.jpg"}`); rec.Code != 204 || cache.Model().Event("a2@sample").Image != "/category-images/night.jpg" || row()["Image"] != "category-images/night.jpg" {
		t.Errorf("a picture: %d %+v", rec.Code, row())
	}
	if rec := call(t, admin, "PUT", "/api/calendar/invites/settings", `{"id":"a2@sample","flyer":"sample/flyer.jpg"}`); rec.Code != 204 || cache.Model().Invitations["a2@sample"] == nil || cache.Model().Invitations["a2@sample"].Flyer != "sample/flyer.jpg" || len(cache.Model().Invitations["a2@sample"].Hosts) != 0 {
		t.Errorf("a flyer: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides/image", `{"id":"a2@sample","image":""}`); rec.Code != 204 || cache.Model().Event("a2@sample").Image != "" {
		t.Errorf("the picture taken away: %d", rec.Code)
	}
	if rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "PUT", "/api/calendar/overrides/image", `{"id":"a2@sample","image":"x.jpg"}`); rec.Code != 403 {
		t.Errorf("a parent's picture: %d", rec.Code)
	}
	if rec := call(t, as("jordan.whitfield@heliosschool.org", handler), "PUT", "/api/calendar/overrides", body("x", e.Start, e.End, "", "")); rec.Code != 403 {
		t.Errorf("a parent: %d", rec.Code)
	}
	if rec := call(t, admin, "PUT", "/api/calendar/overrides", `{"id":"a2@sample","title":"x","start":"not a date"}`); rec.Code != 400 {
		t.Errorf("a bad date: %d", rec.Code)
	}
}
