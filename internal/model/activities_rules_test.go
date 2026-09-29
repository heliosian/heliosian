package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/testkit"
)

func whole(t *testing.T, b activityBody) activityPatch {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	patch := activityPatch{}
	if err := json.Unmarshal(raw, &patch); err != nil {
		t.Fatal(err)
	}
	return patch
}

func TestSaveActivityRules(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model()
	const india = "deepa.natarajan@heliosschool.org"
	existing := func(id string) activityBody {
		act := m.Activity(id)
		return activityBody{ID: id, Year: act.Year, Title: act.Title, Parent: act.Parent, Category: act.Category, Status: act.Status, DirectSignUp: act.DirectSignUpOwn, PrettyID: act.PrettyID}
	}
	with := func(b activityBody, change func(*activityBody)) activityBody {
		change(&b)
		return b
	}
	suggestion := activityBody{Year: "2026 - 2027", Title: "Kite Day", Category: "tcg0000000006", Status: StatusOpen, SignUp: PositionOpen}
	cases := []struct {
		name   string
		actor  access.Actor
		body   activityBody
		status int
		want   string
		ops    int
	}{
		{"a parent's suggestion waits for approval", activityViewerOf(activitiesParent, false), suggestion, http.StatusOK, StatusPending, 2},
		{"a suggestion needs a category", activityViewerOf(activitiesParent, false), with(suggestion, func(b *activityBody) { b.Category = "" }), http.StatusBadRequest, "", 0},
		{"a sign-up is volunteer, open or none", activityViewerOf(activitiesParent, false), with(suggestion, func(b *activityBody) { b.SignUp = PositionCoChair }), http.StatusBadRequest, "", 0},
		{"an admin's add keeps its status", activityViewerOf(jordan, true), with(suggestion, func(b *activityBody) { b.Status = StatusHidden; b.SignUp = "" }), http.StatusOK, StatusHidden, 1},
		{"a parent cannot edit", activityViewerOf(activitiesParent, false), existing("act0000000002"), http.StatusForbidden, "", 0},
		{"an admin edits", activityViewerOf(jordan, true), existing("act0000000002"), http.StatusOK, m.Activity("act0000000002").Status, 1},
		{"no such activity", activityViewerOf(jordan, true), activityBody{ID: "nope"}, http.StatusNotFound, "", 0},
		{"a co-chair hides their own", activityViewerOf(chair, false), with(existing("act0000000020"), func(b *activityBody) { b.Status = StatusHidden }), http.StatusOK, StatusHidden, 1},
		{"a co-chair cannot hide what waits for approval", activityViewerOf(chair, false), with(existing("act0000000022"), func(b *activityBody) { b.Status = StatusHidden }), http.StatusBadRequest, "", 0},
		{"a co-chair may leave it pending", activityViewerOf(chair, false), with(existing("act0000000022"), func(b *activityBody) { b.Status = StatusPending }), http.StatusOK, StatusPending, 1},
		{"the event's co-chair approves", activityViewerOf(chair, false), with(existing("act0000000022"), func(b *activityBody) { b.Status = StatusOpen }), http.StatusOK, StatusOpen, 1},
		{"a co-chair moves only under their own", activityViewerOf(india, false), with(existing("act0000000020"), func(b *activityBody) { b.Parent = "act0000000002" }), http.StatusForbidden, "", 0},
		{"a co-chair cannot make an event of it", activityViewerOf(india, false), with(existing("act0000000020"), func(b *activityBody) { b.Parent = "" }), http.StatusForbidden, "", 0},
		{"no loops", activityViewerOf(jordan, true), with(existing("act0000000001"), func(b *activityBody) { b.Parent = "act0000000020"; b.Category = "" }), http.StatusBadRequest, "", 0},
		{"a parent's booth under a Yes category opens", activityViewerOf(activitiesParent, false), activityBody{Year: "2026 - 2027", Title: "Sweden", Parent: "act0000000001", Category: "tcg0000000008"}, http.StatusOK, StatusOpen, 1},
		{"a parent's booth under a closed category is refused", activityViewerOf(activitiesParent, false), activityBody{Year: "2026 - 2027", Title: "Sweden", Parent: "act0000000001", Category: "tcg0000000007"}, http.StatusBadRequest, "", 0},
		{"the co-chair adds under a closed category", activityViewerOf(chair, false), activityBody{Year: "2026 - 2027", Title: "Sweden", Parent: "act0000000001", Category: "tcg0000000007", Status: StatusOpen}, http.StatusOK, StatusOpen, 1},
		{"a child cannot take a page heading", activityViewerOf(chair, false), activityBody{Year: "2026 - 2027", Title: "Sweden", Parent: "act0000000001", Category: "tcg0000000001"}, http.StatusBadRequest, "", 0},
		{"a malformed address", activityViewerOf(jordan, true), with(existing("act0000000002"), func(b *activityBody) { b.PrettyID = "Bad Address!" }), http.StatusBadRequest, "", 0},
	}
	for _, c := range cases {
		s, err := m.saveActivity(c.actor, whole(t, c.body))
		if got := testkit.Status(t, err); got != c.status {
			t.Errorf("%s: status %d, want %d (%v)", c.name, got, c.status, err)
			continue
		}
		if err == nil && (s.status != c.want || len(s.ops) != c.ops) {
			t.Errorf("%s: saved as %q with %d ops, want %q with %d", c.name, s.status, len(s.ops), c.want, c.ops)
		}
	}
}

func TestSaveActivityPrettyConflicts(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model()
	act := m.Activity("act0000000002")
	body := activityBody{ID: "act0000000002", Year: act.Year, Title: act.Title, Category: act.Category, Status: act.Status, DirectSignUp: "Yes", PrettyID: "international-night"}
	_, err := m.saveActivity(activityViewerOf(jordan, true), whole(t, body))
	var refusal *access.Refusal
	if !errors.As(err, &refusal) || refusal.Status != http.StatusConflict {
		t.Fatalf("same-year clash: %v", err)
	}
	if c, ok := refusal.Body.(*activityPrettyConflict); !ok || c.ID != "act0000000001" || c.Prior || c.Message == "" {
		t.Fatalf("same-year clash body: %+v", refusal.Body)
	}
	last := byTitle(m, "2025 - 2026", "International Night")
	body.PrettyID = "international-night-2025"
	_, err = m.saveActivity(activityViewerOf(jordan, true), whole(t, body))
	if !errors.As(err, &refusal) || refusal.Status != http.StatusConflict {
		t.Fatalf("prior-year clash: %v", err)
	}
	if c, ok := refusal.Body.(*activityPrettyConflict); !ok || c.ID != last.ID || !c.Prior || c.Renamed != "international-night-2025-2025" {
		t.Fatalf("prior-year clash body: %+v", refusal.Body)
	}
	body.TakeOver = true
	s, err := m.saveActivity(activityViewerOf(jordan, true), whole(t, body))
	if err != nil || len(s.ops) != 2 {
		t.Fatalf("take-over: %v, %d ops", err, len(s.ops))
	}
	if err := cache.Commit(context.Background(), activityViewerOf(jordan, true), s.ops...); err != nil {
		t.Fatal(err)
	}
	if m = cache.Model(); m.Activity("act0000000002").PrettyID != "international-night-2025" || m.Activity(last.ID).PrettyID != "international-night-2025-2025" {
		t.Fatalf("after take-over: %q %q", m.Activity("act0000000002").PrettyID, m.Activity(last.ID).PrettyID)
	}
}

func TestSaveActivityPriority(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model()
	act := m.Activity("act0000000022")
	body := activityBody{ID: act.ID, Year: act.Year, Title: act.Title, Parent: act.Parent, Category: act.Category, Status: act.Status, DirectSignUp: act.DirectSignUpOwn}
	save := func(who string, isAdmin, priority, complete bool) bool {
		t.Helper()
		b := body
		b.Priority, b.VolunteersComplete = priority, complete
		s, err := cache.Model().saveActivity(activityViewerOf(who, isAdmin), whole(t, b))
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Commit(context.Background(), activityViewerOf(who, isAdmin), s.ops...); err != nil {
			t.Fatal(err)
		}
		return cache.Model().Activity(act.ID).Priority
	}
	if act.Priority {
		t.Fatal("the sample activity starts with a priority")
	}
	if save(chair, false, true, false) {
		t.Fatal("a co-chair set the priority")
	}
	if !save(jordan, true, true, false) {
		t.Fatal("an admin could not set the priority")
	}
	if !save(chair, false, false, false) {
		t.Fatal("a co-chair's save cleared the priority")
	}
	if save(jordan, true, true, true) {
		t.Fatal("a complete activity kept its priority")
	}
}
