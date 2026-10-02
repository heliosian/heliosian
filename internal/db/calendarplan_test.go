package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	hummingbirds = "grp00000000010"
	ospreys      = "grp00000000013"
	conference   = "cat00000000002"
)

func calendarSample(t *testing.T) (*Store, *store.Queue) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	categories := []store.Op{store.Insert("CATEGORY", store.Row{"id": conference, "scope": "event", "title": "Conference", "description": "Family and teacher conferences."})}
	n := 10
	for _, part := range []string{"Dropoff", "School", "Pickup", "Aftercare"} {
		n++
		categories = append(categories, store.Insert("CATEGORY", store.Row{"id": "cat000000000" + itoa2(n), "scope": "day_part", "title": part}))
	}
	for name := range DayTemplates {
		n++
		categories = append(categories, store.Insert("CATEGORY", store.Row{"id": "cat000000000" + itoa2(n), "scope": "day_type", "title": name}))
	}
	if err := commit(s, ConfigSheet, categories...); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, GroupsSheet, store.Insert("GROUP", store.Row{"id": ospreys, "kind": "classroom", "title": "Ospreys", "status": "open", "visibility": "everyone"})); err != nil {
		t.Fatal(err)
	}
	return s, queue
}

func itoa2(n int) string {
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func apply(t *testing.T, s *Store, queue *store.Queue, edits []Edit) {
	t.Helper()
	if len(edits) == 0 {
		return
	}
	env := Env{System: importReader, Now: testNow}
	if _, err := Write(context.Background(), s, queue, access.System(importReader), env, Batch{Batch: edits}); err != nil {
		t.Fatal(err)
	}
}

func groupsOf(m *Model, kind string) []store.Row {
	out := []store.Row{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] == kind {
			out = append(out, g)
		}
	}
	return out
}

func sourcesKeyed(m *Model, key string) []store.Row {
	out := []store.Row{}
	for _, row := range m.Table("GROUP_SOURCE").All() {
		if row["calendar_event"] == key {
			out = append(out, row)
		}
	}
	return out
}

func TestGooglePlan(t *testing.T) {
	s, queue := calendarSample(t)
	from, to := feedWindow(testNow)
	cafe := GoogleEvent{CalendarItem: CalendarItem{Key: "a@google", Title: "Hummingbirds CAFE", Start: "2026-10-02 08:30", End: "2026-10-02 09:30"}}
	early := GoogleEvent{CalendarItem: CalendarItem{Key: "b@google", Title: "Kindergarten early dismissal", Start: "2026-10-09", End: "2026-10-09", AllDay: true}}
	first := GoogleEvent{Series: "c@google", CalendarItem: CalendarItem{Key: "c@google/20261005T150000", Title: "Parent coffee", Start: "2026-10-05 15:00", End: "2026-10-05 16:00"}}
	second := GoogleEvent{Series: "c@google", CalendarItem: CalendarItem{Key: "c@google/20261012T150000", Title: "Parent coffee", Start: "2026-10-12 15:00", End: "2026-10-12 16:00"}}
	answers := map[string]Classification{
		cafe.Key:   {Classrooms: []string{hummingbirds}, Who: "parents"},
		early.Key:  {Classrooms: []string{hummingbirds}, Who: "families", DayType: "Early Dismissal"},
		first.Key:  {Classrooms: []string{hummingbirds, ospreys}, Who: "parents", Categories: []string{conference}},
		second.Key: {Classrooms: []string{hummingbirds, ospreys}, Who: "parents", Categories: []string{conference}},
	}
	sync := func(feed []GoogleEvent) {
		t.Helper()
		rows := s.Model().calendarRows()
		v, err := NewVocabulary(rows)
		if err != nil {
			t.Fatal(err)
		}
		classified := map[string]Classification{}
		for _, it := range GoogleToClassify(rows, v, feed) {
			classified[it.Key] = answers[it.Key]
		}
		apply(t, s, queue, GooglePlan(rows, v, feed, from, to, classified))
	}

	before := len(groupsOf(s.Model(), "event"))
	sync([]GoogleEvent{cafe, early, first, second})
	m := s.Model()
	if n := len(groupsOf(m, "event")) - before; n != 4 {
		t.Fatalf("%d events added, want 4", n)
	}
	series := groupsOf(m, "series")
	if len(series) != 1 {
		t.Fatalf("%d series, want 1", len(series))
	}
	instances := 0
	for _, g := range groupsOf(m, "event") {
		if g["parent"] == series[0]["id"] {
			instances++
		}
	}
	if instances != 2 {
		t.Fatalf("the series has %d instances, want 2", instances)
	}
	cafeGroup := sourcesKeyed(m, cafe.Key)[0]["group"]
	rules := m.Table("RULE").Referencing("group", cafeGroup)
	if len(rules) != 1 || rules[0]["target"] != hummingbirds || rules[0]["expand"] != "parents" {
		t.Fatalf("the cafe's rules are %v", rules)
	}
	days := groupsOf(m, "day")
	if len(days) != 1 || days[0]["start"] != "2026-10-09" {
		t.Fatalf("days %v, want one on 2026-10-09", days)
	}
	if parts := len(m.Table("GROUP").Referencing("parent", days[0]["id"])); parts != len(DayTemplates["Early Dismissal"]) {
		t.Fatalf("the early dismissal day has %d parts", parts)
	}
	if rules := m.Table("RULE").Referencing("group", days[0]["id"]); len(rules) != 1 || rules[0]["target"] != hummingbirds {
		t.Fatalf("the day's rules are %v", rules)
	}

	rows := s.Model().calendarRows()
	v, _ := NewVocabulary(rows)
	feed := []GoogleEvent{cafe, early, first, second}
	if pending := GoogleToClassify(rows, v, feed); len(pending) != 0 {
		t.Fatalf("an unchanged feed asks to classify %v", pending)
	}
	if edits := GooglePlan(rows, v, feed, from, to, nil); len(edits) != 0 {
		t.Fatalf("an unchanged feed plans %v", edits)
	}

	sync([]GoogleEvent{cafe, second})
	m = s.Model()
	if n := len(groupsOf(m, "event")) - before; n != 2 {
		t.Fatalf("%d events left after two left the feed, want 2", n)
	}
	if n := len(groupsOf(m, "day")) + len(groupsOf(m, "day_part")); n != 0 {
		t.Fatalf("%d days and parts outlived their event", n)
	}
	if n := len(groupsOf(m, "series")); n != 1 {
		t.Fatalf("the series went with one instance left: %d", n)
	}

	sync([]GoogleEvent{cafe})
	if n := len(groupsOf(s.Model(), "series")); n != 0 {
		t.Fatalf("a series with no instances stayed: %d", n)
	}
}

func TestPDFPlan(t *testing.T) {
	s, queue := calendarSample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000002", "kind": "calendar", "hash": "v1"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000003", "kind": "calendar", "hash": "v2"}),
	); err != nil {
		t.Fatal(err)
	}
	from, to := feedWindow(testNow)
	btsnGoogle := GoogleEvent{CalendarItem: CalendarItem{Key: "btsn@google", Title: "Back to School Night", Start: "2026-08-27", End: "2026-08-27", AllDay: true}}
	rows := s.Model().calendarRows()
	v, err := NewVocabulary(rows)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, s, queue, GooglePlan(rows, v, []GoogleEvent{btsnGoogle}, from, to, map[string]Classification{btsnGoogle.Key: {Classrooms: []string{hummingbirds, ospreys}, Who: "parents"}}))

	both := []string{hummingbirds, ospreys}
	year := func(doc string, entries ...YearEntry) YearCalendar {
		return YearCalendar{Document: doc, Year: "2026-2027", Hash: "reading", Entries: append([]YearEntry{
			{Key: "first", Title: "First day of school", Start: "2026-08-26", End: "2026-08-26", Classrooms: both, Marker: "first_day"},
			{Key: "last", Title: "Last day of school", Start: "2026-09-04", End: "2026-09-04", Classrooms: both, Marker: "last_day"},
			{Key: "k", Title: "K early dismissal", Start: "2026-09-02", End: "2026-09-02", DayType: "Early Dismissal", Classrooms: []string{hummingbirds}},
		}, entries...), Shaded: []ShadedDay{{Date: "2026-09-01", DayType: "No School"}}}
	}
	btsn := YearEntry{Key: "btsn", Title: "Back to School Night", Start: "2026-08-27", End: "2026-08-27", Classrooms: both}
	picnic := YearEntry{Key: "picnic", Title: "Picnic", Start: "2026-08-28", End: "2026-08-28", Classrooms: both}
	run := func(cal YearCalendar) {
		t.Helper()
		rows := s.Model().calendarRows()
		v, _ := NewVocabulary(rows)
		classified := map[string]Classification{}
		for _, it := range PDFToClassify(rows, v, cal) {
			classified[it.Key] = Classification{Who: "families"}
		}
		edits, err := PDFPlan(rows, v, cal, classified)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, s, queue, edits)
	}

	run(year("doc00000000002", btsn, picnic))
	m := s.Model()
	days := groupsOf(m, "day")
	if len(days) != 9 {
		t.Fatalf("%d days, want 9: eight school days, one split for kindergarten", len(days))
	}
	byDate := map[string][]store.Row{}
	for _, d := range days {
		byDate[d["start"]] = append(byDate[d["start"]], d)
	}
	if len(byDate["2026-09-02"]) != 2 || len(byDate["2026-09-01"]) != 1 || byDate["2026-09-01"][0]["title"] != "No School" {
		t.Fatalf("days by date %v", byDate)
	}
	markers := 0
	for _, src := range m.Table("GROUP_SOURCE").All() {
		if src["marker"] != "" {
			markers++
		}
	}
	if markers != 2 {
		t.Fatalf("%d marked days, want 2", markers)
	}
	btsnGroup := sourcesKeyed(m, btsnGoogle.Key)[0]["group"]
	if n := len(m.Table("GROUP_SOURCE").Referencing("group", btsnGroup)); n != 2 {
		t.Fatalf("Back to School Night has %d sources, want the feed's and the pdf's", n)
	}
	events := len(groupsOf(m, "event"))

	run(year("doc00000000003", btsn))
	m = s.Model()
	if n := len(groupsOf(m, "event")); n != events-1 {
		t.Fatalf("%d events after the picnic left the pdf, want %d", n, events-1)
	}
	if _, ok := m.Table("GROUP").Get(btsnGroup); !ok {
		t.Fatal("Back to School Night went with the old version")
	}
	for _, src := range m.Table("GROUP_SOURCE").All() {
		if src["document"] == "doc00000000002" {
			t.Fatalf("a source still names the old version: %v", src)
		}
	}
	if n := len(groupsOf(m, "day")); n != 9 {
		t.Fatalf("%d days after the new version, want 9", n)
	}
}
