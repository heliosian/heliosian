package calendar

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	Register(mux, cache, dir, directQueue{}, nil, d, func() []string { return nil }, func(string) []Linked { return nil }, ImageSearch{})
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
	if rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":"Bad","classrooms":["Penguins"]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown classroom: %d", rec.Code)
	}
	if rec := call(t, owner, http.MethodPost, "/api/calendar/feeds", `{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("no name: %d", rec.Code)
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
	if !strings.Contains(tags, `og:title" content="Helios Calendar"`) || !strings.Contains(tags, "/share/upcoming.png") {
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
