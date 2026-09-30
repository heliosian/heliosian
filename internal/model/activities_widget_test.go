package model

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"heliosian/internal/testkit"
)

func clockAt(t *testing.T, at time.Time) {
	t.Helper()
	was := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = was })
}

func teamWidgetOf(t *testing.T, mux http.Handler, as string) map[string][]map[string]any {
	t.Helper()
	r := read(t, mux, as, "/api/team-settings?include=mine,needed,priority")
	settings := r.one(t, "team-settings", teamSettingsKeyOf())
	out := map[string][]map[string]any{}
	for _, name := range []string{"mine", "needed", "priority"} {
		out[name] = []map[string]any{}
		list, _ := settings[name].([]any)
		for _, key := range list {
			out[name] = append(out[name], r.one(t, "activities", key.(string)))
		}
	}
	return out
}

func positionOf(a map[string]any) string {
	me, _ := a["me"].(map[string]any)
	position, _ := me["position"].(string)
	return position
}

func TestWidget(t *testing.T) {
	_, mux := activitiesServer(t)
	clockAt(t, time.Date(2026, 9, 25, 12, 0, 0, 0, Location))
	w := teamWidgetOf(t, mux, jordan)
	if len(w["mine"]) != 3 {
		t.Fatalf("mine: %+v", w["mine"])
	}
	if m := w["mine"][0]; m["title"] != "Tech Setup" || m["under"] != "All School Movie Night" || m["day"] != "2026-11-06" || positionOf(m) != PositionVolunteer || m["path"] != "/activities/act0000000003/act0000000026" {
		t.Errorf("a role under a dated event: %+v", m)
	}
	if m := w["mine"][2]; m["title"] != "Tech Team" || m["day"] != nil || m["dayTiming"] != "All Year" || positionOf(m) != PositionOpen {
		t.Errorf("all-year last: %+v", m)
	}
	if n := w["needed"]; len(n) == 0 || len(n) > widgetNeeded || n[0]["title"] != "All School Movie Night" || n[0]["wants"] != "Co-chair wanted" {
		t.Errorf("needed: %+v", n)
	}
	for _, o := range w["needed"] {
		if o["title"] == "Tech Team" {
			t.Errorf("needed lists what they are on: %+v", o)
		}
	}
	if p := w["priority"]; len(p) != 2 || p[0]["title"] != "Spring Celebration" || p[1]["title"] != "Helios Cares" || p[0]["wants"] == nil || p[0]["wants"] == "" {
		t.Errorf("priority: %+v", p)
	}
	if none := teamWidgetOf(t, mux, activitiesParent); len(none["mine"]) != 0 || len(none["needed"]) == 0 {
		t.Errorf("on nothing: %+v", none)
	}
	clockAt(t, time.Date(2026, 11, 7, 12, 0, 0, 0, Location))
	for _, m := range teamWidgetOf(t, mux, jordan)["mine"] {
		if m["title"] == "Tech Setup" {
			t.Errorf("passed sign-up still listed: %+v", m)
		}
	}
}

func approvableOf(t *testing.T, mux http.Handler, as string) []string {
	t.Helper()
	ids := []string{}
	json.Unmarshal(read(t, mux, as, "/api/activities?can=approve").Result, &ids)
	return ids
}

func TestApprovalsFollowCan(t *testing.T) {
	cache, mux := activitiesServer(t)
	if got := cache.Model().Activities.Activity("act0000000022"); got == nil || got.Status != StatusPending || got.Parent == "" {
		t.Fatalf("act0000000022 is not a pending child: %+v", got)
	}
	if ids := approvableOf(t, mux, chair); !slices.Contains(ids, "act0000000022") || slices.Contains(ids, "act0000000012") {
		t.Errorf("a co-chair's approvals: %v", ids)
	}
	if ids := approvableOf(t, mux, jordan); !slices.Contains(ids, "act0000000022") || !slices.Contains(ids, "act0000000012") {
		t.Errorf("an admin's approvals: %v", ids)
	}
	if ids := approvableOf(t, mux, activitiesParent); len(ids) != 0 {
		t.Errorf("a plain member's approvals: %v", ids)
	}
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000022", "approve"), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair's approval: %d %s", rec.Code, rec.Body)
	}
	if ids := approvableOf(t, mux, chair); slices.Contains(ids, "act0000000022") {
		t.Errorf("an approved child is still to approve: %v", ids)
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
