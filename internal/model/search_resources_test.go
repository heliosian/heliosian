package model

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
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

func hitsOf(t *testing.T, r reply) []api.Hit {
	t.Helper()
	var hits []api.Hit
	if err := json.Unmarshal(r.Result, &hits); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return hits
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

func TestNearRanksLocatedFamiliesByMiles(t *testing.T) {
	s := newServer(t)
	d := s.directory()
	from := d.FamilyKeysOf(jordan)[0]
	offsets := map[string]float64{from: 0}
	for key := range d.Families {
		if _, taken := offsets[key]; !taken && len(offsets) < 4 {
			offsets[key] = []float64{0.02, 0.005, 0.01}[len(offsets)-1]
		}
	}
	unlocated := ""
	for key, f := range d.Families {
		f.Address, f.Lat, f.Lng = "", 0, 0
		if offset, ok := offsets[key]; ok {
			f.Address, f.Lat, f.Lng = "1 Main St, Berkeley, CA", 37.87, -122.27+offset
		} else {
			unlocated = key
		}
		d.Families[key] = f
	}
	hits := hitsOf(t, read(t, s.mux, jordan, "/api/families?near="+from))
	if len(hits) != 3 {
		t.Fatalf("hits: %v", hits)
	}
	for i, h := range hits {
		if h.ID == from || h.Score <= 0 || (i > 0 && h.Score < hits[i-1].Score) {
			t.Errorf("hit %d: %v", i, hits)
		}
	}
	close := hitsOf(t, read(t, s.mux, jordan, "/api/families?near="+from+"&within=0.6"))
	if len(close) != 2 || close[1].Score > 0.6 {
		t.Fatalf("within 0.6 miles: %v", close)
	}
	if rec := testkit.Call(t, s.mux, jordan, "GET", "/api/families?near="+unlocated, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("measured from a family with no street address: %d", rec.Code)
	}
}

func TestDayPlansAndEventFiltersReadTheCalendar(t *testing.T) {
	_, h := sampleEventsServer(t)
	labor := read(t, h, jordan, "/api/day-plans?date=2026-09-07&mine&include=day-type,set-by")
	plans := keysOf(t, labor)
	if len(plans) != 2 {
		t.Fatalf("jordan's classrooms on labor day: %v", plans)
	}
	for _, key := range plans {
		plan := labor.one(t, "day-plans", key)
		if name := labor.one(t, "day-types", plan["day-type"].(string))["name"]; name != "No School" || plan["weekday"] != "Monday" {
			t.Errorf("labor day: %v %v", plan, name)
		}
	}
	if none := keysOf(t, read(t, h, jordan, "/api/day-plans?date=2026-09-06")); len(none) != 0 {
		t.Fatalf("a sunday has plans: %v", none)
	}
	thanks := read(t, h, jordan, "/api/events?q=thanksgiving&include=calendar-tags,day-type")
	events := keysOf(t, thanks)
	if len(events) != 1 {
		t.Fatalf("thanksgiving: %v", events)
	}
	e := thanks.one(t, "events", events[0])
	if thanks.one(t, "day-types", e["day-type"].(string))["name"] != "No School" {
		t.Errorf("thanksgiving's day type: %v", e["day-type"])
	}
	tags := []string{}
	for _, key := range e["calendar-tags"].([]any) {
		tags = append(tags, thanks.one(t, "calendar-tags", key.(string))["name"].(string))
	}
	if !slices.Contains(tags, "Schedule") || !slices.Contains(e["classrooms"].([]any), any("Jays")) {
		t.Errorf("thanksgiving's tags %v and classrooms %v", tags, e["classrooms"])
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

func documentsServer(t *testing.T) (*DocumentFiler, http.Handler) {
	t.Helper()
	in, _, _, queue := testInbox(t)
	mux := http.NewServeMux()
	typedRegistry(in.store, queue, DirectoryResources(in.store), DocumentResources(in)).Register(mux)
	files, err := filepath.Glob(filepath.Join(documentSamples, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := in.FileSaved(context.Background(), access.System("test"), file); err != nil {
			t.Fatal(err)
		}
	}
	for _, group := range []string{"hummingbird-families", "middle-school-parents", "soccer-team"} {
		if err := in.Post(context.Background(), access.System("loop mailer"), group, []byte(personalMail)); err != nil {
			t.Fatal(err)
		}
	}
	queue.Flush()
	return in, mux
}

func TestDocumentsFollowWhoMayReadAListsMail(t *testing.T) {
	_, mux := documentsServer(t)
	for _, c := range []struct {
		email string
		reads []string
	}{
		{jordan, []string{"middle-school-parents", "soccer-team"}},
		{"nobody@heliosschool.org", nil},
	} {
		out := read(t, mux, c.email, "/api/documents")
		groups := []string{}
		for _, key := range keysOf(t, out) {
			if d := out.one(t, "documents", key); d["kind"] == DocumentKindGroup {
				groups = append(groups, d["emailList"].(string))
			}
		}
		if len(groups) != len(c.reads) {
			t.Errorf("%s reads %v", c.email, groups)
		}
		for _, name := range c.reads {
			if !slices.ContainsFunc(groups, func(g string) bool { return strings.Contains(g, name+"@") }) {
				t.Errorf("%s does not read %s: %v", c.email, name, groups)
			}
		}
		hidden := DocumentKey("hummingbird-families/personal-1@example.org")
		if rec := testkit.Call(t, mux, c.email, "GET", "/api/documents/"+hidden, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s read hummingbird mail: %d", c.email, rec.Code)
		}
	}
}

func TestPassageSearchRanksWhatTheViewerMayRead(t *testing.T) {
	_, mux := documentsServer(t)
	out := read(t, mux, jordan, "/api/document-passages?search=international+night+booths&limit=3&include=document")
	hits := hitsOf(t, out)
	if len(hits) != 3 || hits[0].Score < hits[1].Score {
		t.Fatalf("hits: %v", hits)
	}
	first := out.one(t, "document-passages", hits[0].ID)
	if first["section"] != "HCA NEWSLETTER" || !strings.Contains(first["text"].(string), "booth") {
		t.Fatalf("first passage: %v", first)
	}
	if doc := out.one(t, "documents", first["document"].(string)); doc["title"] != "Helios Weekly Newsletter 2026 Sep 11" {
		t.Fatalf("its document: %v", doc)
	}
	early := read(t, mux, jordan, "/api/document-passages?search=labor+day&until=2026-09-04&include=document")
	if len(hitsOf(t, early)) == 0 {
		t.Fatal("nothing before September 4")
	}
	for _, h := range hitsOf(t, early) {
		if d := early.one(t, "documents", early.one(t, "document-passages", h.ID)["document"].(string)); d["date"].(string) > "2026-09-04" {
			t.Errorf("a passage from %v", d["date"])
		}
	}
	if none := hitsOf(t, read(t, mux, jordan, "/api/document-passages?search=x&since=2027-01-01")); len(none) != 0 {
		t.Fatalf("passages from 2027: %v", none)
	}
	for _, path := range []string{"/api/document-passages?search=x&since=last+summer", "/api/document-passages?search=+"} {
		if rec := testkit.Call(t, mux, jordan, "GET", path, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}
