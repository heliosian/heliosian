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
	cache, err := NewCache(dir, func() Roster { return roster }, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	d := fakeDirectory{
		people: map[string]Person{"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true}},
		kids:   map[string][]Person{},
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, directQueue{}, d, func() []string { return nil }, func() []Linked { return nil })
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
