package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const (
	rowan   = "per00000000002"
	maya    = "per00000000003"
	ashdown = "grp00000000020"
	picnic  = "grp00000000040"
	camping = "doc00000000106"
)

var sampleNow = time.Date(2026, 9, 28, 9, 0, 0, 0, db.School)

func write(t *testing.T, s *db.Store, queue *store.Queue, edits ...db.Edit) []string {
	t.Helper()
	env := db.Env{System: "test", Whole: true, Now: sampleNow}
	ids, err := db.Write(context.Background(), s, queue, access.System(env.System), env, db.Batch{Batch: edits})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func schoolDayEdits() []db.Edit {
	edits := []db.Edit{}
	for _, in := range testkit.SchoolDay() {
		edits = append(edits, db.Edit{Insert: in.Table, As: in.As, Row: in.Row})
	}
	return edits
}

func sample(t *testing.T) (*Set, *db.Store, *store.Queue) {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.ClaudeHost, testkit.SearchClaude())
	vertex, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s, err := db.NewStore(dir, dir, queue, db.NewSearchIndex(), func(app string) string { return "https://" + app + ".heliosian.com" })
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	if err := bucket.FillFrom("../../sampledata/bucket"); err != nil {
		t.Fatal(err)
	}
	search := db.NewSearcher(s, queue, bucket, vertex)
	return New(Deps{Data: s, Search: search, Bucket: bucket, Now: func() time.Time { return sampleNow }}), s, queue
}

func run(t *testing.T, set *Set, s *db.Store, name string, input string) (Answer, error) {
	t.Helper()
	return set.Run(context.Background(), s.Model(), rowan, name, json.RawMessage(input))
}

func TestEachToolAnswersTheSampleParent(t *testing.T) {
	set, s, queue := sample(t)
	write(t, s, queue, schoolDayEdits()...)
	for _, c := range []struct {
		tool, input string
		wants       []string
		fails       bool
	}{
		{"helios_whoami", `{}`, []string{`"id":"` + rowan + `"`, `"roles":["parent"]`, `"Ashdown Family"`, `"Juni Ashdown"`, `"roles":["student"]`, `"International Night"`, `"lead":true`, `"rowan@example.com"`}, false},
		{"helios_find_people", `{"role": "staff"}`, []string{`"Maya Lindqvist"`, `"job_title":"Teacher"`, `"classroom":"Hummingbirds"`, `"email":"maya.lindqvist@example.org"`}, false},
		{"helios_find_people", `{}`, []string{"name at least one"}, true},
		{"helios_classroom", `{"name": "Hummingbirds"}`, []string{`"gradeband":"Jayvens"`, `"color":"#5b8def"`, `"grades":["3","4"]`, `"teachers":[{"id":"` + maya, `"Robins"`, `"Juni Ashdown"`, `"Jayvens Room Parents"`, `"Rowan Ashdown"`}, false},
		{"helios_classroom", `{}`, []string{`"Hummingbirds"`, `"Maya Lindqvist"`}, false},
		{"helios_classroom", `{"name": "Penguins"}`, []string{"no classroom named"}, true},
		{"helios_group", `{"id": "` + ashdown + `"}`, []string{`"roles":["parent"]`, `"roles":["student"]`, `"memberCount":4`}, false},
		{"helios_group", `{"id": "grp00000000512"}`, []string{`"lead":true`, `"via":"International Night Managers"`}, false},
		{"helios_get", `{"id": "` + rowan + `"}`, []string{`"families"`, `"Ashdown Family"`, `"PERSON_EMAIL.person"`}, false},
		{"helios_nearby_families", `{}`, []string{`"from":{"id":"` + ashdown, `"Lindqvist Family"`, `"milesAway":0.7`, `"Maya Lindqvist"`}, false},
		{"helios_nearby_families", `{"grade": "5"}`, []string{`"name":"Abernathy Family"`, `"name":"Okonkwo Family"`, `"name":"Whitaker Family"`, `"name":"Lund Family"`, `"milesAway":0.2`}, false},
		{"helios_events", `{"from": "2026-10-01"}`, []string{`"Fall Picnic"`, `"category":"Community"`, `"yourAnswer":"yes"`, `"forYourHousehold":true`}, false},
		{"helios_events", `{"words": "taco"}`, []string{`"Taco Night"`, `"kind":"party"`}, false},
		{"helios_events", `{"from": "tomorrow"}`, []string{"YYYY-MM-DD"}, true},
		{"helios_days", `{}`, []string{`"type":"Regular"`, `"name":"School","start":"08:30","end":"15:00"`, `"about":"A full school day with aftercare."`, `"schoolYear":"2026-2027"`}, false},
		{"helios_days", `{"date": "2026-09-27"}`, []string{`"plans":[]`, `"weekday":"Sunday"`}, false},
		{"helios_activities", `{}`, []string{`"year":"2026 - 2027"`, `"International Night"`, `"heading":"Headline Events"`, `"leads":["Rowan Ashdown"]`, `"yourPlace":"co-chair"`, `"expenseForm":"https://example.com/expenses"`}, false},
		{"helios_activities", `{"year": "2019 - 2020"}`, []string{"holds no school year starting in 2019"}, true},
		{"helios_parties", `{}`, []string{`"Taco Night"`, `"celebration":"Spring Celebration 2027"`, `"hosts":["Maya Lindqvist"]`, `"yourHouseholdTickets":["Rowan Ashdown"]`, `"currentCelebration":"Spring Celebration 2027"`, `"ticketNote"`}, false},
		{"helios_lists", `{}`, []string{`"Hummingbirds Parents"`, `"address":"hummingbirds-parents@loop.heliosian.com"`, `"Jayvens Room Parents"`, `"youAreIn":true`}, false},
		{"helios_links", `{}`, []string{`"Lunch Ordering"`, `"url":"https://example.com/lunch"`, `"section":"Quick Links"`}, false},
		{"helios_links", `{"words": "nothing like it"}`, []string{"no link matches"}, true},
		{"helios_read_document", `{"id": "` + camping + `"}`, []string{"# Camping Trips", "https://wiki.heliosian.com/p/Activities/Camping-Trips"}, false},
		{"helios_query", `{"query": "(from GROUP (where (= kind \"crew\")))"}`, []string{`"Robins"`}, false},
		{"helios_query_json", `{"query": ["from", "GROUP", ["where", ["=", {"n": "kind"}, "crew"]]]}`, []string{`"Robins"`}, false},
		{"helios_query_json", `{"query": ["from", "GROUP", ["where", ["=", {"path": "kind"}, "crew"]]]}`, []string{`is an object other than {"n": name}`}, true},
	} {
		answer, err := run(t, set, s, c.tool, c.input)
		text := answer.Text
		if err != nil {
			text = err.Error()
		}
		if (err != nil) != c.fails {
			t.Errorf("%s %s: failed %v: %s", c.tool, c.input, err != nil, text)
			continue
		}
		for _, want := range c.wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s %s: no %s in %s", c.tool, c.input, want, text)
			}
		}
	}
}

func TestAnEventIsForWhomItsAudienceHolds(t *testing.T) {
	set, s, queue := sample(t)
	write(t, s, queue,
		db.Edit{Insert: "GROUP", As: "e", Row: map[string]any{"kind": "event", "name": "Hummingbirds Hike", "start": "2026-10-05 09:00", "status": "open", "visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
		db.Edit{Insert: "GROUP", As: "a", Row: map[string]any{"kind": "group", "parent": "@e", "name": "Hummingbirds Hike Audience", "status": "open", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
		db.Edit{Set: "@e", Cells: map[string]any{"audience": "@a"}},
		db.Edit{Insert: "RULE", Row: map[string]any{"group": "@a", "order": "i", "target": "grp00000000010", "replace_with": "household"}},
	)
	for _, input := range []string{`{"words": "hike"}`, `{"words": "hike", "classroom": "Hummingbirds"}`} {
		answer, err := run(t, set, s, "helios_events", input)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(answer.Text, `"Hummingbirds Hike"`) || !strings.Contains(answer.Text, `"forYourHousehold":true`) {
			t.Errorf("%s: %s", input, answer.Text)
		}
	}
}

func TestADayIsOnePlanPerClassroom(t *testing.T) {
	set, s, queue := sample(t)
	write(t, s, queue, schoolDayEdits()...)
	day := func(name string) []db.Edit {
		return []db.Edit{
			{Insert: "GROUP", As: "t", Row: map[string]any{"kind": "category", "name": name, "status": "open", "visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
			{Insert: "GROUP", As: "d", Row: map[string]any{"kind": "day", "parent": "@t", "name": name, "start": "2026-09-28", "all_day": "Yes", "status": "open", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
		}
	}
	plans := func() []schoolDay {
		t.Helper()
		answer, err := run(t, set, s, "helios_days", `{}`)
		if err != nil {
			t.Fatal(err)
		}
		var out struct{ Plans []schoolDay }
		if err := json.Unmarshal([]byte(answer.Text), &out); err != nil {
			t.Fatal(err)
		}
		return out.Plans
	}
	write(t, s, queue, append(day("Early Dismissal"), db.Edit{Insert: "RULE", Row: map[string]any{"group": "@d", "order": "i", "target": "grp00000000010"}})...)
	got := plans()
	if len(got) != 2 || got[0].Type != "Regular" || !slices.Equal(got[0].Classrooms, []string{"Condors", "Egrets", "Falcons", "Hawks", "Herons", "Jays", "Ospreys", "Ravens"}) || got[0].ForYou || got[1].Type != "Early Dismissal" || !slices.Equal(got[1].Classrooms, []string{"Hummingbirds"}) || !got[1].ForYou {
		t.Errorf("an early dismissal for the Hummingbirds over a regular day: %+v", got)
	}
	write(t, s, queue, day("No School")...)
	if got := plans(); len(got) != 1 || got[0].Type != "No School" || len(got[0].Classrooms) != 0 {
		t.Errorf("no school for everyone over the rest: %+v", got)
	}
}

func TestACityOnlyFamilyIsNeverMeasured(t *testing.T) {
	set, s, queue := sample(t)
	vega := write(t, s, queue,
		db.Edit{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "family", "status": "open", "name": "Vega Family", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "vc_address": "Moss Beach, CA", "address_consent": "shared", "consent": "listed"}},
		db.Edit{Insert: "GEOCODE", Row: map[string]any{"address": "Moss Beach, CA", "lat": "37.5272", "lng": "-122.5131"}},
	)[0]
	answer, err := run(t, set, s, "helios_nearby_families", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer.Text, "Vega Family") || !strings.Contains(answer.Text, "Lindqvist Family") {
		t.Fatalf("nearby: %s", answer.Text)
	}
	if _, err := run(t, set, s, "helios_nearby_families", `{"family": "`+vega+`"}`); err == nil || !strings.Contains(err.Error(), "only a city") {
		t.Fatalf("measured from a city: %v", err)
	}
}

func TestEveryHrefIsNotedWithItsRow(t *testing.T) {
	set, s, _ := sample(t)
	answer, err := run(t, set, s, "helios_classroom", `{"name": "Hummingbirds"}`)
	if err != nil {
		t.Fatal(err)
	}
	for href, id := range map[string]string{
		"https://who.heliosian.com/g/hummingbirds":          "grp00000000010",
		"https://who.heliosian.com/p/maya.lindqvist":        maya,
		"https://loop.heliosian.com/g/jayvens-room-parents": "grp00000000502",
	} {
		if answer.Links[href] != id {
			t.Errorf("%s noted as %q; every link: %v", href, answer.Links[href], answer.Links)
		}
	}
}

func TestEveryToolHasWordsAndAnObjectSchema(t *testing.T) {
	set, _, _ := sample(t)
	for _, tool := range set.Tools {
		if tool.Words == "" || tool.Schema == nil || tool.Schema.Type != "object" || !strings.HasPrefix(tool.Name, "helios_") {
			t.Errorf("%s: words %q, schema %+v", tool.Name, tool.Words, tool.Schema)
		}
	}
}
