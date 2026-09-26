package team

import (
	"net/http"
	"testing"
	"time"
)

// TestWidget checks the home page's HCA-Team widget on the sample's 25
// September: a role takes its day from the event it sits under, the dated
// come before the all-year, the position rides along, and what needs people
// leaves out what the viewer is already on - someone on nothing gets the
// list alone.
func TestWidget(t *testing.T) {
	cache, _ := newServer(t)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, local)
	w := cache.Widget("jordan.whitfield@heliosschool.org", at)
	if len(w.Mine) != 3 {
		t.Fatalf("mine: %+v", w.Mine)
	}
	if m := w.Mine[0]; m.Title != "Tech Setup" || m.Under != "All School Movie Night" || m.Start != "2026-11-06" || m.Position != PositionVolunteer || m.Path != "/activities/E003/E026" {
		t.Errorf("a role under a dated event: %+v", m)
	}
	if m := w.Mine[2]; m.Title != "Tech Team" || m.Start != "" || m.Timing != "All Year" || m.Position != PositionOpen {
		t.Errorf("all-year last: %+v", m)
	}
	if len(w.Open) == 0 || len(w.Open) > widgetOpen || w.Open[0].Title != "All School Movie Night" || w.Open[0].Note != "Co-chair wanted" {
		t.Errorf("open: %+v", w.Open)
	}
	for _, o := range w.Open {
		if o.Title == "Tech Team" {
			t.Errorf("open lists what they are on: %+v", o)
		}
	}
	// A day past the Movie Night, its role has passed.
	later := cache.Widget("jordan.whitfield@heliosschool.org", time.Date(2026, 11, 7, 12, 0, 0, 0, local))
	for _, m := range later.Mine {
		if m.Title == "Tech Setup" {
			t.Errorf("passed sign-up still listed: %+v", later.Mine)
		}
	}
	if none := cache.Widget(parent, at); len(none.Mine) != 0 || len(none.Open) == 0 {
		t.Errorf("on nothing: %+v", none)
	}
	// The sample marks Spring Celebration and Helios Cares a priority; only
	// this year's come through, dated first.
	if p := w.Priority; len(p) != 2 || p[0].Title != "Spring Celebration" || p[1].Title != "Helios Cares" || p[0].Note == "" {
		t.Errorf("priority: %+v", p)
	}
}

// TestPriorityIsAnAdmins checks that marking something a priority is an
// admin's alone: a co-chair's save of the thing they run keeps the mark as
// it was, whatever the body says, and an admin's sets and clears it.
func TestPriorityIsAnAdmins(t *testing.T) {
	cache, mux := newServer(t)
	edit := map[string]any{
		"id": "E020", "year": "2026 - 2027", "title": "India", "parent": "E001",
		"category": "C08", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": true, "priority": true,
	}
	if rec := call(t, mux, chair, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("co-chair save: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("E020").Priority {
		t.Fatal("a co-chair marked a priority")
	}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("admin save: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Activity("E020").Priority {
		t.Fatal("an admin's mark did not stick")
	}
	edit["priority"] = false
	if rec := call(t, mux, chair, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("co-chair save: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Activity("E020").Priority {
		t.Fatal("a co-chair cleared an admin's mark")
	}
	// Volunteers complete takes the mark off, whoever saves it, and an admin
	// cannot mark a complete thing.
	edit["volunteersComplete"] = true
	if rec := call(t, mux, chair, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("E020").Priority {
		t.Fatal("a complete thing kept its priority")
	}
	edit["priority"] = true
	if rec := call(t, mux, admin, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("admin save: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("E020").Priority {
		t.Fatal("an admin marked a complete thing")
	}
}
