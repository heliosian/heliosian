package tools

import (
	"context"
	"encoding/json"
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

func sample(t *testing.T) (*Set, *db.Store, *db.Searcher) {
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
	s, err := db.NewStore(dir, dir, queue, db.NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	if err := bucket.FillFrom("../../sampledata/bucket"); err != nil {
		t.Fatal(err)
	}
	origin := func(app string) string { return "https://" + app + ".heliosian.com" }
	search := db.NewSearcher(s, queue, bucket, vertex, origin)
	return New(Deps{Data: s, Search: search, Bucket: bucket, Origin: origin, Now: func() time.Time { return sampleNow }}), s, search
}

func run(t *testing.T, set *Set, s *db.Store, name string, input string) (Answer, error) {
	t.Helper()
	return set.Run(context.Background(), s.Model(), rowan, name, json.RawMessage(input))
}

func TestEachToolAnswersTheSampleParent(t *testing.T) {
	set, s, _ := sample(t)
	if err := s.Commit(context.Background(), access.System("test"), db.GroupsSheet, testkit.SchoolDay()...); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		tool, input string
		wants       []string
		fails       bool
	}{
		{"helios_whoami", `{}`, []string{`"id":"` + rowan + `"`, `"roles":["parent"]`, `"Ashdown Family"`, `"Juni Ashdown"`, `"roles":["student"]`, `"International Night"`, `"lead":true`, `"rowan@example.com"`}, false},
		{"helios_find_people", `{"role": "staff"}`, []string{`"Maya Lindqvist"`, `"job_title":"Teacher"`, `"classroom":"Hummingbirds"`, `"email":"maya.lindqvist@example.org"`}, false},
		{"helios_find_people", `{}`, []string{"name at least one"}, true},
		{"helios_classroom", `{"name": "Hummingbirds"}`, []string{`"band":"Jayvens"`, `"color":"#5b8def"`, `"grades":["3"]`, `"teachers":[{"id":"` + maya, `"Robins"`, `"Juni Ashdown"`, `"Jayvens Room Parents"`, `"Rowan Ashdown"`}, false},
		{"helios_classroom", `{}`, []string{`"Hummingbirds"`, `"Maya Lindqvist"`}, false},
		{"helios_classroom", `{"name": "Ravens"}`, []string{"no classroom named"}, true},
		{"helios_group", `{"id": "` + ashdown + `"}`, []string{`"roles":["parent"]`, `"roles":["student"]`, `"memberCount":2`}, false},
		{"helios_group", `{"id": "grp00000000512"}`, []string{`"lead":true`, `"via":"International Night Managers"`}, false},
		{"helios_get", `{"id": "` + rowan + `"}`, []string{`"families"`, `"Ashdown Family"`, `"PERSON_EMAIL.person"`}, false},
		{"helios_nearby_families", `{}`, []string{`"from":{"id":"` + ashdown, `"Lindqvist Family"`, `"milesAway":0.7`, `"Maya Lindqvist"`}, false},
		{"helios_nearby_families", `{"grade": "5"}`, []string{`"families":[]`}, false},
		{"helios_events", `{"from": "2026-10-01"}`, []string{`"Fall Picnic"`, `"category":"Community"`, `"yourAnswer":"yes"`, `"forYourHousehold":true`}, false},
		{"helios_events", `{"words": "taco"}`, []string{`"Taco Night"`, `"kind":"party"`}, false},
		{"helios_events", `{"from": "tomorrow"}`, []string{"YYYY-MM-DD"}, true},
		{"helios_days", `{}`, []string{`"type":"Regular"`, `"name":"School","start":"08:30","end":"15:00"`, `"about":"A full school day with aftercare."`, `"schoolYear":"2026-2027"`}, false},
		{"helios_days", `{"date": "2026-09-27"}`, []string{`"plans":[]`, `"weekday":"Sunday"`}, false},
		{"helios_activities", `{}`, []string{`"year":"2026 - 2027"`, `"International Night"`, `"heading":"Headline Events"`, `"leadNeeded":true`, `"leads":["Rowan Ashdown"]`, `"yourPlace":"co-chair"`, `"expenseForm":"https://example.com/expenses"`}, false},
		{"helios_activities", `{"year": "2019 - 2020"}`, []string{"holds no school year starting in 2019"}, true},
		{"helios_parties", `{}`, []string{`"Taco Night"`, `"celebration":"Spring Celebration 2027"`, `"hosts":["Maya Lindqvist"]`, `"yourHouseholdTickets":["Rowan Ashdown"]`, `"currentCelebration":"Spring Celebration 2027"`, `"ticketNote"`}, false},
		{"helios_lists", `{}`, []string{`"Hummingbirds Parents"`, `"address":"hummingbirds-parents@loop.heliosian.com"`, `"Jayvens Room Parents"`, `"youAreIn":true`}, false},
		{"helios_links", `{}`, []string{`"Lunch Ordering"`, `"url":"https://example.com/lunch"`, `"section":"Quick Links"`}, false},
		{"helios_links", `{"words": "nothing like it"}`, []string{"no link matches"}, true},
		{"helios_read_document", `{"id": "` + camping + `"}`, []string{"# Camping Trips", "https://wiki.heliosian.com/p/Activities/Camping-Trips"}, false},
		{"helios_query", `{"query": "(from GROUP (where (= kind \"crew\")))"}`, []string{`"Robins"`}, false},
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

func TestACityOnlyFamilyIsNeverMeasured(t *testing.T) {
	set, s, _ := sample(t)
	if err := s.Commit(context.Background(), access.System("test"), db.GroupsSheet,
		store.Insert("GROUP", store.Row{"id": "grp00000000540", "kind": "family", "status": "open", "name": "Vega Family", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "vc_address": "Moss Beach, CA", "address_consent": "shared", "consent": "listed"}),
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(context.Background(), access.System("test"), db.ConfigSheet,
		store.Insert("GEOCODE", store.Row{"id": "geo00000000009", "address": "Moss Beach, CA", "lat": "37.5272", "lng": "-122.5131"}),
	); err != nil {
		t.Fatal(err)
	}
	answer, err := run(t, set, s, "helios_nearby_families", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer.Text, "Vega Family") || !strings.Contains(answer.Text, "Lindqvist Family") {
		t.Fatalf("nearby: %s", answer.Text)
	}
	if _, err := run(t, set, s, "helios_nearby_families", `{"family": "grp00000000540"}`); err == nil || !strings.Contains(err.Error(), "only a city") {
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
		"https://who.heliosian.com/classrooms/grp00000000010":    "grp00000000010",
		"https://who.heliosian.com/people/" + maya:               maya,
		"https://loop.heliosian.com/groups/jayvens-room-parents": "grp00000000502",
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
