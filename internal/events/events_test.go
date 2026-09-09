package events

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

const (
	parent = "robin.whitfield@heliosschool.org"
	admin  = "jordan.whitfield@heliosschool.org"
	chair  = "mina.park@heliosschool.org"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

type fakeDirectory struct{}

func (fakeDirectory) Resolve(email string) string { return email }

func (fakeDirectory) Person(email string) (string, string, bool) {
	if email == parent {
		return "Robin Whitfield", "", true
	}
	return "", "", false
}

type bundled struct{}

func (bundled) Has(key string) bool { return strings.HasPrefix(key, "brand/") }

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, bundled{}, func(e string) bool { return e == admin }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return []string{admin} })
	return cache, mux
}

func call(t *testing.T, mux *http.ServeMux, as, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, req)
	return rec
}

func TestSampleLoads(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	if len(m.Categories) != 6 || len(m.Activities) != 15 {
		t.Fatalf("got %d categories, %d activities", len(m.Categories), len(m.Activities))
	}
	night := m.Activity("2026 - 2027", "International Night")
	if night == nil || len(night.Roles) != 6 || night.Role("India Performance") == nil || night.Role("India Performance").Parent != "India" {
		t.Fatalf("international night did not load as expected: %+v", night)
	}
	if len(night.CoChairs()) != 2 {
		t.Fatalf("co-chairs: %v", night.CoChairs())
	}
}

func TestRenderHidesWhatItShould(t *testing.T) {
	cache, _ := newServer(t)
	view := Render(cache.Model(), fakeDirectory{}, parent, false, now())
	for _, a := range view.Activities {
		if a.Status == StatusHidden || a.Status == StatusPending {
			t.Errorf("%s reached a parent as %s", a.Title, a.Status)
		}
		if a.Title == "Room Parents" {
			for _, v := range a.Volunteers {
				if v.Position != PositionCoChair {
					t.Errorf("room parents list leaked %s", v.Email)
				}
			}
			if a.Taken != 1 {
				t.Errorf("room parents taken %d", a.Taken)
			}
		}
	}
	if view.User.Name != "Robin Whitfield" || view.People != nil {
		t.Errorf("user %+v, people %v", view.User, view.People)
	}
	suggester := Render(cache.Model(), fakeDirectory{}, "elena.torres@heliosschool.org", false, now())
	found := false
	for _, a := range suggester.Activities {
		if a.Title == "Family Escape Room Night" {
			found = true
		}
	}
	if !found {
		t.Error("a suggester cannot see their own pending suggestion")
	}
	if got := Render(cache.Model(), fakeDirectory{}, "someone.new@heliosschool.org", false, now()).User.Name; got != "Someone New" {
		t.Errorf("display name %q", got)
	}
}

func TestSignUpAndRemove(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{"year": "2026 - 2027", "activity": "International Night", "role": "Clean Up Crew", "position": PositionOpen, "note": "happy to help"}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", body); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	role := cache.Model().Activity("2026 - 2027", "International Night").Role("Clean Up Crew")
	if len(role.Volunteers) != 1 || role.Volunteers[0].Email != parent || role.Volunteers[0].AddedBy != parent {
		t.Fatalf("volunteers after sign up: %+v", role.Volunteers)
	}
	body["position"] = PositionCoChair
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent named themselves co-chair: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/events/volunteer", map[string]any{"year": "2026 - 2027", "activity": "International Night", "role": "Clean Up Crew", "email": parent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not promote: %d %s", rec.Code, rec.Body)
	}
	full := map[string]any{"year": "2026 - 2027", "activity": "All School Movie Night", "role": "Tech Setup", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", full); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full role took a sign-up: %d", rec.Code)
	}
	direct := map[string]any{"year": "2026 - 2027", "activity": "International Night", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", direct); rec.Code != http.StatusBadRequest {
		t.Fatalf("an activity without direct sign-up took one: %d", rec.Code)
	}
	if rec := call(t, mux, "someone.else@heliosschool.org", "DELETE", "/api/events/volunteer", map[string]any{"year": "2026 - 2027", "activity": "International Night", "role": "Clean Up Crew", "email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed someone: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/events/volunteer", map[string]any{"year": "2026 - 2027", "activity": "International Night", "role": "Clean Up Crew", "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove self: %d %s", rec.Code, rec.Body)
	}
	if n := len(cache.Model().Activity("2026 - 2027", "International Night").Role("Clean Up Crew").Volunteers); n != 0 {
		t.Fatalf("%d volunteers left after removal", n)
	}
}

func TestSuggestApproveRenameDelete(t *testing.T) {
	cache, mux := newServer(t)
	suggestion := map[string]any{"year": "2026 - 2027", "title": "Kite Day", "category": "Just an Idea", "status": StatusOpen, "description": "Fly kites", "coChair": true, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/events/activity", suggestion); rec.Code != http.StatusNoContent {
		t.Fatalf("suggest: %d %s", rec.Code, rec.Body)
	}
	kite := cache.Model().Activity("2026 - 2027", "Kite Day")
	if kite == nil || kite.Status != StatusPending || kite.AddedBy != parent || len(kite.Volunteers) != 1 || kite.Volunteers[0].Position != PositionOpen {
		t.Fatalf("suggestion landed as %+v", kite)
	}
	edit := map[string]any{"original": map[string]string{"year": "2026 - 2027", "title": "Kite Day"}, "year": "2026 - 2027", "title": "Kite Festival", "category": "Activities", "status": StatusOpen, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/events/activity", edit); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-chair edited an activity: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("approve and rename: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("2026 - 2027", "Kite Day") != nil {
		t.Fatal("the old title survived the rename")
	}
	festival := cache.Model().Activity("2026 - 2027", "Kite Festival")
	if festival == nil || festival.Status != StatusOpen || len(festival.Volunteers) != 1 {
		t.Fatalf("renamed activity: %+v", festival)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/activity", map[string]string{"year": "2026 - 2027", "title": "Kite Festival"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete with a volunteer on it: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/volunteer", map[string]any{"year": "2026 - 2027", "activity": "Kite Festival", "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/activity", map[string]string{"year": "2026 - 2027", "title": "Kite Festival"}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("2026 - 2027", "Kite Festival") != nil {
		t.Fatal("the activity survived deletion")
	}
}

func TestRoleRenameCarriesChildren(t *testing.T) {
	cache, mux := newServer(t)
	edit := map[string]any{"year": "2026 - 2027", "activity": "International Night", "original": "India", "title": "India Booth", "group": "Place & Culture Booths", "status": StatusOpen, "coLeaderNeeded": true}
	if rec := call(t, mux, chair, "POST", "/api/events/role", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("rename role: %d %s", rec.Code, rec.Body)
	}
	night := cache.Model().Activity("2026 - 2027", "International Night")
	booth := night.Role("India Booth")
	if booth == nil || len(booth.Volunteers) != 1 || len(booth.Links) != 1 || len(booth.Roles) != 1 || booth.Roles[0].Parent != "India Booth" {
		t.Fatalf("renamed role: %+v", booth)
	}
	if rec := call(t, mux, chair, "DELETE", "/api/events/role", map[string]string{"year": "2026 - 2027", "activity": "International Night", "title": "India Booth"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a role with children and volunteers: %d", rec.Code)
	}
}

func TestCopyToNextYear(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, chair, "POST", "/api/events/copy", map[string]string{"year": "2026 - 2027", "title": "International Night"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair copied: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/copy", map[string]string{"year": "2026 - 2027", "title": "International Night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := cache.Model().Activity("2027 - 2028", "International Night")
	if next == nil || next.Status != StatusOpen || next.Start != "" || len(next.Volunteers) != 0 || len(next.AllRoles()) != 6 || next.Role("Cybertron") != nil || len(next.Links) != 2 {
		t.Fatalf("copied activity: %+v", next)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/copy", map[string]string{"year": "2026 - 2027", "title": "International Night"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("copied twice: %d", rec.Code)
	}
}

func TestYears(t *testing.T) {
	if got := SchoolYear(mustTime("2026-09-09")); got != "2026 - 2027" {
		t.Errorf("september: %s", got)
	}
	if got := SchoolYear(mustTime("2027-03-01")); got != "2026 - 2027" {
		t.Errorf("march: %s", got)
	}
	if got := ShiftYear("2026 - 2027", -1); got != "2025 - 2026" {
		t.Errorf("shift: %s", got)
	}
	if err := CheckYear("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}
