package model

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"heliosian/internal/testkit"
)

func keyOf(t *testing.T, r reply) string {
	t.Helper()
	var key string
	if err := json.Unmarshal(r.Result, &key); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return key
}

func keysOf(t *testing.T, r reply) []string {
	t.Helper()
	var keys []string
	if err := json.Unmarshal(r.Result, &keys); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return keys
}

func namesOf(r reply, kind, field string, keys []string) []string {
	out := []string{}
	for _, key := range keys {
		name, _ := r.Resources[kind][key][field].(string)
		out = append(out, name)
	}
	return out
}

func TestPeopleFiltersReachParentsThroughTheirChildren(t *testing.T) {
	s := newServer(t)
	whitfields := read(t, s.mux, jordan, "/api/people?q=whitfield")
	if names := namesOf(whitfields, "people", "fullName", keysOf(t, whitfields)); len(names) != 4 || !slices.Contains(names, "Sam Whitfield") {
		t.Fatalf("whitfields: %v", names)
	}
	parents := read(t, s.mux, jordan, "/api/people?role=parent&classroom=jays")
	names := namesOf(parents, "people", "fullName", keysOf(t, parents))
	if !slices.Contains(names, "Jordan Whitfield") || slices.Contains(names, "Sam Whitfield") {
		t.Fatalf("parents of Jays: %v", names)
	}
	if rec := testkit.Call(t, s.mux, jordan, "GET", "/api/people?role=teacher", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown role: %d", rec.Code)
	}
	me := read(t, s.mux, jordan, "/api/people/"+jordan+"?include=room-parent-for")
	grades := []string{}
	for _, key := range me.one(t, "people", s.personID(jordan))["room-parent-for"].([]any) {
		grades = append(grades, me.one(t, "grades", key.(string))["name"].(string))
	}
	if !slices.Contains(grades, "Grade 3") {
		t.Fatalf("room parent for %v", grades)
	}
}

func TestClassroomsCarryTheirBandTeachersAndRoomParents(t *testing.T) {
	s := newServer(t)
	out := read(t, s.mux, jordan, "/api/classrooms/jays?include=teachers,room-parents")
	room := out.one(t, "classrooms", keyOf(t, out))
	if room["band"] == nil || !slices.Contains(room["grades"].([]any), any("Grade 3")) || len(room["teachers"].([]any)) == 0 {
		t.Fatalf("jays: %v", room)
	}
	if !slices.Contains(room["room-parents"].([]any), any(s.personID(jordan))) {
		t.Fatalf("jays' room parents: %v", room["room-parents"])
	}
}

func TestEventFiltersReadTheCalendar(t *testing.T) {
	_, h := sampleEventsServer(t)
	thanks := read(t, h, jordan, "/api/events?q=thanksgiving&include=day-type")
	events := keysOf(t, thanks)
	if len(events) != 1 {
		t.Fatalf("thanksgiving: %v", events)
	}
	e := thanks.one(t, "events", events[0])
	if thanks.one(t, "day-types", e["day-type"].(string))["name"] != "No School" {
		t.Errorf("thanksgiving's day type: %v", e["day-type"])
	}
	if !slices.Contains(e["classrooms"].([]any), any("Jays")) {
		t.Errorf("thanksgiving's classrooms %v", e["classrooms"])
	}
	trips := read(t, h, jordan, "/api/events?from=2026-09-01&to=2026-09-30&tag=trip")
	if titles := namesOf(trips, "events", "title", keysOf(t, trips)); len(titles) != 1 || titles[0] != "Jays and Ravens Camping" {
		t.Errorf("trips in September: %v", titles)
	}
}

func TestActivityFiltersReadThePastAgainstTheQuery(t *testing.T) {
	_, mux := activitiesServer(t)
	was := now
	t.Cleanup(func() { now = was })
	now = func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, Location) }
	night := read(t, mux, jordan, "/api/activities/international-night")
	key := keyOf(t, night)
	if night.one(t, "activities", key)["past"] != true {
		t.Fatalf("international night on October 1: %v", night.one(t, "activities", key))
	}
	under := keysOf(t, read(t, mux, jordan, "/api/activities?under="+key))
	if len(under) == 0 {
		t.Fatal("international night has nothing under it")
	}
	if upcoming := keysOf(t, read(t, mux, jordan, "/api/activities?q=international&past=false")); len(upcoming) != 0 {
		t.Fatalf("things under a past event were listed as upcoming: %v", upcoming)
	}
	if past := keysOf(t, read(t, mux, jordan, "/api/activities?under="+key+"&past=true")); len(past) != len(under) {
		t.Fatalf("roles under a past event: %d past of %d", len(past), len(under))
	}
	categorized := read(t, mux, jordan, "/api/activities?include=category")
	named := 0
	for _, k := range keysOf(t, categorized) {
		if c, ok := categorized.one(t, "activities", k)["category"].(string); ok {
			if categorized.one(t, "activity-categories", c)["title"] == "" {
				t.Errorf("category %s has no title", c)
			}
			named++
		}
	}
	if named == 0 {
		t.Fatal("no activity names its category")
	}
}

func TestPartyFiltersReadThePastAgainstTheQuery(t *testing.T) {
	_, mux := partiesServer(t)
	all := keysOf(t, read(t, mux, jordan, "/api/parties"))
	upcoming := keysOf(t, read(t, mux, jordan, "/api/parties?past=false"))
	past := keysOf(t, read(t, mux, jordan, "/api/parties?past=true"))
	if len(upcoming) == 0 || len(past) == 0 || len(upcoming)+len(past) != len(all) {
		t.Fatalf("%d upcoming and %d past of %d", len(upcoming), len(past), len(all))
	}
	social := read(t, mux, jordan, "/api/parties?q=children+social")
	if titles := namesOf(social, "parties", "title", keysOf(t, social)); len(titles) != 1 || titles[0] != "Nerf Blaster Bash" {
		t.Fatalf("parties by category: %v", titles)
	}
}
