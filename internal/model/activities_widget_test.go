package model

import (
	"net/http"
	"testing"
	"time"

	"heliosian/internal/testkit"
)

func TestWidget(t *testing.T) {
	cache, _ := activitiesServer(t)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, Location)
	w := cache.Model().Activities.Widget("jordan.whitfield@heliosschool.org", at)
	if len(w.Mine) != 3 {
		t.Fatalf("mine: %+v", w.Mine)
	}
	if m := w.Mine[0]; m.Title != "Tech Setup" || m.Under != "All School Movie Night" || m.Start != "2026-11-06" || m.Position != PositionVolunteer || m.Path != "/activities/act0000000003/act0000000026" {
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
	later := cache.Model().Activities.Widget("jordan.whitfield@heliosschool.org", time.Date(2026, 11, 7, 12, 0, 0, 0, Location))
	for _, m := range later.Mine {
		if m.Title == "Tech Setup" {
			t.Errorf("passed sign-up still listed: %+v", later.Mine)
		}
	}
	if none := cache.Model().Activities.Widget(activitiesParent, at); len(none.Mine) != 0 || len(none.Open) == 0 {
		t.Errorf("on nothing: %+v", none)
	}
	if p := w.Priority; len(p) != 2 || p[0].Title != "Spring Celebration" || p[1].Title != "Helios Cares" || p[0].Note == "" {
		t.Errorf("priority: %+v", p)
	}
}

func TestPriorityIsAnAdmins(t *testing.T) {
	cache, mux := activitiesServer(t)
	path := activityAction("act0000000020", "edit")
	edit := map[string]any{
		"year": "2026 - 2027", "title": "India", "parent": "act0000000001",
		"category": "tcg0000000008", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": "Yes", "priority": true,
	}
	if rec := testkit.Call(t, mux, chair, "POST", path, edit); rec.Code != http.StatusNoContent {
		t.Fatalf("co-chair save: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activities.Activity("act0000000020").Priority {
		t.Fatal("a co-chair marked a priority")
	}
	if rec := testkit.Call(t, mux, jordan, "POST", path, edit); rec.Code != http.StatusNoContent {
		t.Fatalf("admin save: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Activities.Activity("act0000000020").Priority {
		t.Fatal("an admin's mark did not stick")
	}
	edit["priority"] = false
	if rec := testkit.Call(t, mux, chair, "POST", path, edit); rec.Code != http.StatusNoContent {
		t.Fatalf("co-chair save: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Activities.Activity("act0000000020").Priority {
		t.Fatal("a co-chair cleared an admin's mark")
	}
	edit["volunteersComplete"] = true
	if rec := testkit.Call(t, mux, chair, "POST", path, edit); rec.Code != http.StatusNoContent {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activities.Activity("act0000000020").Priority {
		t.Fatal("a complete thing kept its priority")
	}
	edit["priority"] = true
	if rec := testkit.Call(t, mux, jordan, "POST", path, edit); rec.Code != http.StatusNoContent {
		t.Fatalf("admin save: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activities.Activity("act0000000020").Priority {
		t.Fatal("an admin marked a complete thing")
	}
}
