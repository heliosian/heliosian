package db

import (
	"context"
	"maps"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	hummingbirds = "grp00000000010"
	ospreys      = "grp00000000013"
	conference   = "grp00000000070"
)

func calendarSample(t *testing.T) (*Store, *store.Queue) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	ops := []store.Op{
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": ospreys, "kind": "classroom", "name": "Ospreys", "status": "open", "visible_to": "grp00000000004"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": conference, "kind": "category", "name": "Conference", "description": "Family and teacher conferences.", "status": "open", "visible_to": "grp00000000004"}),
	}
	n := 70
	for name := range DayTemplates {
		n++
		ops = append(ops, store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp000000000" + itoa2(n), "kind": "category", "name": name, "status": "open", "visible_to": "grp00000000004"}))
	}
	if err := commit(s, GroupsSheet, ops...); err != nil {
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
	env := setupEnv
	if _, err := Write(context.Background(), s, queue, newPictures(s, queue), access.System(env.System), env, Batch{Batch: edits}); err != nil {
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

func eventsOf(m *Model, recurring bool) []store.Row {
	out := []store.Row{}
	for _, g := range groupsOf(m, "event") {
		if (g["start"] == "") == recurring {
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
		first.Key:  {Classrooms: []string{hummingbirds, ospreys}, Who: "parents", Category: conference},
		second.Key: {Classrooms: []string{hummingbirds, ospreys}, Who: "parents", Category: conference},
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

	before := len(eventsOf(s.Model(), false))
	sync([]GoogleEvent{cafe, early, first, second})
	m := s.Model()
	if n := len(eventsOf(m, false)) - before; n != 4 {
		t.Fatalf("%d events added, want 4", n)
	}
	series := eventsOf(m, true)
	if len(series) != 1 {
		t.Fatalf("%d recurring events, want 1", len(series))
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
	if series[0]["parent"] != conference {
		t.Fatalf("the series is under %q, want its instances' category", series[0]["parent"])
	}
	cafeGroup := sourcesKeyed(m, cafe.Key)[0]["group"]
	rules := m.Table("RULE").Referencing("group", cafeGroup)
	if len(rules) != 1 || rules[0]["target"] != hummingbirds || rules[0]["replace_with"] != "parents" {
		t.Fatalf("the cafe's rules are %v", rules)
	}
	days := groupsOf(m, "day")
	if len(days) != 1 || days[0]["start"] != "2026-10-09" {
		t.Fatalf("days %v, want one on 2026-10-09", days)
	}
	if dayType, _ := m.Table("GROUP").Get(days[0]["parent"]); dayType["name"] != "Early Dismissal" || dayType["kind"] != "category" {
		t.Fatalf("the day is under %v, want its day type", dayType)
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
	if n := len(eventsOf(m, false)) - before; n != 2 {
		t.Fatalf("%d events left after two left the feed, want 2", n)
	}
	if n := len(groupsOf(m, "day")) + len(groupsOf(m, "day_part")); n != 0 {
		t.Fatalf("%d days and parts outlived their event", n)
	}
	if n := len(eventsOf(m, true)); n != 1 {
		t.Fatalf("the recurring event went with one instance left: %d", n)
	}

	sync([]GoogleEvent{cafe})
	if n := len(eventsOf(s.Model(), true)); n != 0 {
		t.Fatalf("a recurring event with no instances stayed: %d", n)
	}
}

func TestARemovedEventTakesItsAnswers(t *testing.T) {
	s, queue := calendarSample(t)
	from, to := feedWindow(testNow)
	cafe := GoogleEvent{CalendarItem: CalendarItem{Key: "a@google", Title: "Hummingbirds CAFE", Start: "2026-10-02 08:30", End: "2026-10-02 09:30"}}
	sync := func(feed []GoogleEvent) {
		t.Helper()
		rows := s.Model().calendarRows()
		v, err := NewVocabulary(rows)
		if err != nil {
			t.Fatal(err)
		}
		classified := map[string]Classification{}
		for _, it := range GoogleToClassify(rows, v, feed) {
			classified[it.Key] = Classification{Classrooms: []string{hummingbirds}, Who: "parents"}
		}
		apply(t, s, queue, GooglePlan(rows, v, feed, from, to, classified))
	}
	sync([]GoogleEvent{cafe})
	event := sourcesKeyed(s.Model(), cafe.Key)[0]["group"]
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000901", "kind": "group", "parent": event, "name": "Hummingbirds CAFE Going", "status": "open"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000902", "kind": "group", "parent": event, "name": "Hummingbirds CAFE Not Going", "status": "open"}),
		store.Update("GROUP", store.Row{"id": event}, store.Row{"rsvp_yes": "grp00000000901", "rsvp_no": "grp00000000902"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000901", "group": "grp00000000901", "person": "per00000000002", "member": "yes"}),
	); err != nil {
		t.Fatal(err)
	}
	sync(nil)
	m := s.Model()
	for _, id := range []string{event, "grp00000000901", "grp00000000902", "mem00000000901"} {
		if m.Has(id) {
			t.Errorf("%s outlived its event leaving the feed", id)
		}
	}
}

func TestOneDayPerDateTypeAndClassrooms(t *testing.T) {
	s, queue := calendarSample(t)
	if err := commit(s, DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000002", "kind": "calendar"})); err != nil {
		t.Fatal(err)
	}
	from, to := feedWindow(testNow)
	both := []string{hummingbirds, ospreys}
	early := GoogleEvent{CalendarItem: CalendarItem{Key: "early@google", Title: "Early dismissal", Start: "2026-10-07", End: "2026-10-07", AllDay: true}}
	google := func(feed []GoogleEvent) {
		t.Helper()
		rows := s.Model().calendarRows()
		v, err := NewVocabulary(rows)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, s, queue, GooglePlan(rows, v, feed, from, to, map[string]Classification{early.Key: {Classrooms: both, Who: "families", DayType: "Early Dismissal"}}))
	}
	pdf := func() {
		t.Helper()
		rows := s.Model().calendarRows()
		v, _ := NewVocabulary(rows)
		cal := YearCalendar{Document: "doc00000000002", Year: "2026-2027", Hash: "reading", Entries: []YearEntry{
			{Key: "first", Title: "First day of school", Start: "2026-10-05", End: "2026-10-05", Classrooms: both, Marker: "first_day"},
			{Key: "last", Title: "Last day of school", Start: "2026-10-09", End: "2026-10-09", Classrooms: both, Marker: "last_day"},
		}, Shaded: []ShadedDay{{Date: "2026-10-07", DayType: "Early Dismissal"}}}
		classified := map[string]Classification{}
		for _, it := range PDFToClassify(rows, v, cal, nil) {
			classified[it.Key] = Classification{Who: "families"}
		}
		edits, err := PDFPlan(rows, v, cal, classified, nil)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, s, queue, edits)
	}
	onTheDay := func() []store.Row {
		out := []store.Row{}
		for _, d := range groupsOf(s.Model(), "day") {
			if d["start"] == "2026-10-07" {
				out = append(out, d)
			}
		}
		return out
	}

	google([]GoogleEvent{early})
	pdf()
	days := onTheDay()
	if len(days) != 1 {
		t.Fatalf("%d days on the early dismissal, want the feed's and the pdf's to share one", len(days))
	}
	if n := len(s.Model().Table("GROUP_SOURCE").Referencing("group", days[0]["id"])); n != 2 {
		t.Fatalf("the shared day has %d sources, want the feed's and the pdf's", n)
	}
	if n := len(s.Model().Table("GROUP").Referencing("parent", days[0]["id"])); n != len(DayTemplates["Early Dismissal"]) {
		t.Fatalf("the shared day has %d parts", n)
	}

	google(nil)
	after := onTheDay()
	if len(after) != 1 || after[0]["id"] != days[0]["id"] {
		t.Fatalf("the day the pdf still states went with the feed's event: %v", after)
	}
	if n := len(s.Model().Table("GROUP_SOURCE").Referencing("group", days[0]["id"])); n != 1 {
		t.Fatalf("the day has %d sources once the feed dropped it, want the pdf's", n)
	}

	google([]GoogleEvent{early})
	if after := onTheDay(); len(after) != 1 || after[0]["id"] != days[0]["id"] {
		t.Fatalf("the feed's event back made another day: %v", after)
	}
}

func TestPDFPlan(t *testing.T) {
	s, queue := calendarSample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000002", "kind": "calendar"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000003", "kind": "calendar"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000004", "kind": "calendar"}),
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
	run := func(cal YearCalendar, matching bool) {
		t.Helper()
		rows := s.Model().calendarRows()
		v, _ := NewVocabulary(rows)
		matched := map[string][]string{}
		if matching {
			p := newCalendarPlan(rows, v)
			for _, e := range cal.Entries {
				for _, g := range p.googleOn(e) {
					if g["name"] == e.Title {
						matched[e.Key] = append(matched[e.Key], g["id"])
					}
				}
			}
		}
		classified := map[string]Classification{}
		for _, it := range PDFToClassify(rows, v, cal, matched) {
			classified[it.Key] = Classification{Who: "families"}
		}
		edits, err := PDFPlan(rows, v, cal, classified, matched)
		if err != nil {
			t.Fatal(err)
		}
		apply(t, s, queue, edits)
	}

	run(year("doc00000000002", btsn, picnic), false)
	btsnGroup := sourcesKeyed(s.Model(), btsnGoogle.Key)[0]["group"]
	twins := 0
	for _, g := range groupsOf(s.Model(), "event") {
		if g["name"] == btsn.Title {
			twins++
		}
	}
	if twins != 2 {
		t.Fatalf("%d Back to School Nights with no match, want the feed's and a pdf twin", twins)
	}
	run(year("doc00000000002", btsn, picnic), true)
	m := s.Model()
	days := groupsOf(m, "day")
	if len(days) != 9 {
		t.Fatalf("%d days, want 9: eight school days, one split for kindergarten", len(days))
	}
	byDate := map[string][]store.Row{}
	for _, d := range days {
		byDate[d["start"]] = append(byDate[d["start"]], d)
	}
	if len(byDate["2026-09-02"]) != 2 || len(byDate["2026-09-01"]) != 1 || byDate["2026-09-01"][0]["name"] != "No School" {
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
	if n := len(m.Table("GROUP_SOURCE").Referencing("group", btsnGroup)); n != 2 {
		t.Fatalf("Back to School Night has %d sources, want the feed's and the pdf's", n)
	}
	twins = 0
	for _, g := range groupsOf(m, "event") {
		if g["name"] == btsn.Title {
			twins++
		}
	}
	if twins != 1 {
		t.Fatalf("%d Back to School Nights once matched, want the twin gone", twins)
	}
	events := len(groupsOf(m, "event"))
	picnicGroup := ""
	for _, g := range groupsOf(m, "event") {
		if g["name"] == picnic.Title {
			picnicGroup = g["id"]
		}
	}
	if err := commit(s, ConfigSheet, store.Insert("ALIAS", store.Row{"id": "als00000000070", "alias": "oldpicnicid", "target": picnicGroup})); err != nil {
		t.Fatal(err)
	}
	dayIDs := func() map[string]string {
		out := map[string]string{}
		for _, d := range groupsOf(s.Model(), "day") {
			rules := []string{}
			for _, r := range s.Model().Table("RULE").Referencing("group", d["id"]) {
				rules = append(rules, r["target"])
			}
			out[d["start"]+" "+d["name"]+" "+strings.Join(rules, ",")] = d["id"]
		}
		return out
	}
	before := dayIDs()

	run(year("doc00000000003", btsn), true)
	if after := dayIDs(); !maps.Equal(after, before) {
		t.Fatalf("a new version with the same days remade them:\nbefore %v\nafter  %v", before, after)
	}
	m = s.Model()
	if n := len(groupsOf(m, "event")); n != events-1 {
		t.Fatalf("%d events after the picnic left the pdf, want %d", n, events-1)
	}
	if _, ok := m.Table("ALIAS").Get("als00000000070"); ok {
		t.Fatal("the old ID naming the picnic outlived it")
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

	shaded := year("doc00000000004", btsn)
	shaded.Shaded = append(shaded.Shaded, ShadedDay{Date: "2026-09-03", DayType: "No School"})
	run(shaded, true)
	after := dayIDs()
	for key, id := range before {
		if strings.HasPrefix(key, "2026-09-03 ") {
			if after[key] != "" {
				t.Fatalf("the regular day on a newly shaded date stayed: %s", key)
			}
			continue
		}
		if after[key] != id {
			t.Fatalf("the day %s changed from %s to %s when another date was shaded", key, id, after[key])
		}
	}
	if after["2026-09-03 No School "] == "" {
		t.Fatalf("no No School day on the newly shaded date: %v", after)
	}
	for _, src := range s.Model().Table("GROUP_SOURCE").All() {
		if src["document"] != "" && src["document"] != "doc00000000004" {
			t.Fatalf("a source still names an old version: %v", src)
		}
	}
}
