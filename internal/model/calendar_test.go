package model

import (
	"encoding/base64"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

var sampleKey = []byte("sample")

var roster = Roster{Classrooms: []RosterClassroom{
	{ID: ClassroomID(sampleKey, "Hummingbirds"), Name: "Hummingbirds", Band: "Hummingbirds", Grades: []string{"Kindergarten"}},
	{ID: ClassroomID(sampleKey, "Hawks"), Name: "Hawks", Band: "Halcons", Grades: []string{"Grade 1"}},
	{ID: ClassroomID(sampleKey, "Falcons"), Name: "Falcons", Band: "Halcons", Grades: []string{"Grade 2"}},
	{ID: ClassroomID(sampleKey, "Jays"), Name: "Jays", Band: "Jayvens", Grades: []string{"Grade 3"}},
	{ID: ClassroomID(sampleKey, "Ravens"), Name: "Ravens", Band: "Jayvens", Grades: []string{"Grade 4"}},
	{ID: ClassroomID(sampleKey, "Condors"), Name: "Condors", Band: "Cospreys", Grades: []string{"Grade 5"}, Crews: []string{"Big Sur", "Pinnacles"}},
	{ID: ClassroomID(sampleKey, "Ospreys"), Name: "Ospreys", Band: "Cospreys", Grades: []string{"Grade 6"}, Crews: []string{"River", "Sea"}},
	{ID: ClassroomID(sampleKey, "Egrets"), Name: "Egrets", Band: "Hegrets", Grades: []string{"Grade 7"}, Crews: []string{"Great", "Snowy"}},
	{ID: ClassroomID(sampleKey, "Herons"), Name: "Herons", Band: "Hegrets", Grades: []string{"Grade 8"}, Crews: []string{"Great Blue", "Green"}},
}}

func sampleIDs(t *testing.T) map[string]string {
	t.Helper()
	tb := tables(t)
	out := map[string]string{}
	for _, row := range tb[TagsTab] {
		out[row["Tag"]] = row["Tag ID"]
	}
	for _, row := range tb[DayTypesTab] {
		out[row["Day Type"]] = row["Day Type ID"]
	}
	for _, c := range roster.Classrooms {
		out[c.Name] = c.ID
	}
	return out
}

func calendarIDs(t *testing.T, names string) string {
	t.Helper()
	known := sampleIDs(t)
	out := []string{}
	for _, name := range cells.SplitList(names) {
		key, ok := known[name]
		if !ok {
			t.Fatalf("the sample calendar has no id for %q", name)
		}
		out = append(out, key)
	}
	return cells.JoinList(out)
}

func readTables(t *testing.T, dir *data.Dir) store.Tables {
	t.Helper()
	out := store.Tables{}
	for _, tab := range calendarTabs {
		_, rows, err := dir.Table(CalendarApp, tab.Name)
		if err != nil {
			t.Fatal(err)
		}
		out[tab.Name] = rows
	}
	return out
}

func tables(t *testing.T) store.Tables {
	t.Helper()
	return readTables(t, &data.Dir{Root: sampleRoot})
}

func quoted(t *testing.T, names string) string {
	t.Helper()
	out := []string{}
	for _, key := range cells.SplitList(calendarIDs(t, names)) {
		out = append(out, strconv.Quote(key))
	}
	return strings.Join(out, ",")
}

func load(t *testing.T) *Calendar {
	t.Helper()
	model, err := BuildCalendar(tables(t), roster)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func TestSampleModel(t *testing.T) {
	m := load(t)
	if m.Hidden != 1 || m.Duplicates != 2 || len(m.Events) != 20 {
		t.Errorf("visible %d hidden %d duplicates %d", len(m.Events), m.Hidden, m.Duplicates)
	}
	if m.Event("a4@sample") == nil || m.Event("pdf/2026-2027/2026-09-07/labor-day") != nil || m.Event("pdf/2026-2027/2026-08-18/12-30p-dismissals-k-only") != nil {
		t.Errorf("dedupe kept the wrong one")
	}
	for _, e := range m.Events {
		if e.ID == "gev0000000012" {
			t.Errorf("hidden event listed")
		}
	}
	if len(m.Tags) != 20 || m.Tags[0].Name != "Schedule" || m.Tags[0].ID != TagSchedule || m.Tags[0].Role != "schedule" || m.Tags[0].BuiltIn || !m.Tags[15].BuiltIn || m.Tags[15].ID != TagCelebrate {
		t.Errorf("tags = %+v", m.Tags)
	}
	camping := m.Event("a5@sample")
	if camping.Title != "Jays and Ravens Camping" || strings.Join(camping.Classrooms, ",") != "Jays,Ravens" || cells.JoinList(camping.Tags) != calendarIDs(t, "Jays, Ravens, Trip") {
		t.Errorf("override not applied: %+v", camping)
	}
	if !slices.Contains(camping.Keywords, "overnight") {
		t.Errorf("enrichment keywords lost: %v", camping.Keywords)
	}
	bts := m.Event("a2@sample")
	if strings.Join(bts.Classrooms, ",") != "Hummingbirds,Hawks,Falcons,Jays,Ravens" || bts.AllDay || !slices.Contains(bts.Tags, calendarIDs(t, "Orientation")) {
		t.Errorf("lower school event: %+v", bts)
	}
	hca := m.Event("evt0000000002")
	if hca.Source != SourceSheet || !slices.Contains(hca.Tags, calendarIDs(t, "Community")) || len(hca.Classrooms) != 9 {
		t.Errorf("sheet event: %+v", hca)
	}
	if m.Event("3PL9W2ZC") != hca || m.Event("3pl9w2zc") != hca {
		t.Errorf("an old ID does not reach its event")
	}
	kinder := m.Event("a6@sample")
	if slices.Contains(kinder.Keywords, "kindergarten") == false || slices.Contains(kinder.Keywords, "hummingbirds") {
		t.Errorf("keywords = %v", kinder.Keywords)
	}
	if len(m.Years) != 1 || m.Years[0].Label != "2026-2027" || m.Years[0].FirstDay != "2026-08-18" || m.Years[0].LastDay != "2027-06-04" {
		t.Errorf("years = %+v", m.Years)
	}
	for i := 1; i < len(m.Events); i++ {
		if m.Events[i-1].Start > m.Events[i].Start && m.Events[i-1].AllDay == m.Events[i].AllDay {
			t.Errorf("events out of order at %d: %s then %s", i, m.Events[i-1].Start, m.Events[i].Start)
		}
	}
}

func TestKeywordsNeverRepeatTags(t *testing.T) {
	tb := tables(t)
	tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "Keywords": "Community, booths, hummingbirds, food"})
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Event("a7@sample").Keywords, ","); got != "booths,food" {
		t.Errorf("keywords = %s", got)
	}
}

func TestDays(t *testing.T) {
	m := load(t)
	cases := []struct{ date, classroom, want string }{
		{"2026-08-18", "Hummingbirds", "Early Dismissal"},
		{"2026-08-18", "Hawks", "Regular"},
		{"2026-08-19", "Hummingbirds", "Early Dismissal"},
		{"2026-08-20", "Hummingbirds", "Regular"},
		{"2026-08-21", "Hummingbirds", "Early Dismissal"},
		{"2026-08-21", "Herons", "Early Dismissal"},
		{"2026-09-07", "Jays", "No School"},
		{"2026-09-30", "Condors", "Early Dismissal"},
		{"2026-12-17", "Ospreys", "No Aftercare"},
		{"2026-12-18", "Ospreys", "No School"},
		{"2026-12-25", "Egrets", "No School"},
		{"2027-06-04", "Egrets", "Early Dismissal"},
		{"2026-08-22", "Hawks", ""},
		{"2027-07-04", "Hawks", ""},
		{"2026-08-17", "Hawks", ""},
	}
	for _, c := range cases {
		got, ok := m.Plan(c.date, c.classroom)
		if c.want == "" {
			if ok {
				t.Errorf("%s %s: got %s, want nothing", c.date, c.classroom, got.Name)
			}
			continue
		}
		if !ok || got.Name != c.want {
			t.Errorf("%s %s: got %q, want %q", c.date, c.classroom, got.Name, c.want)
		}
	}
	regular := m.DayType(RegularDayType)
	if regular.Name != "Regular" || regular.Role != "regular" || len(regular.Blocks) != 4 || regular.Blocks[0].Name != "Dropoff" || regular.Blocks[3].End != "18:00" {
		t.Errorf("regular = %+v", regular)
	}
	if none := m.DayType(NoSchoolDayType); none.Name != "No School" || len(none.Blocks) != 0 {
		t.Errorf("no school has blocks: %+v", none.Blocks)
	}
}

func TestNoSchoolWins(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "Closed", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "No School")},
		store.Row{"Event ID": "X3", "Start": "2026-09-08", "Title": "Still Short", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Early Dismissal")})
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Plan("2026-09-08", "Jays"); got.Name != "No School" {
		t.Errorf("plan = %s, want No School", got.Name)
	}
}

func TestWeekendInsideASpan(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "W1", "Start": "2026-09-11", "End": "2026-09-14", "Title": "Conferences", "Tags": calendarIDs(t, "Jays, Conference"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "W2", "Start": "2026-10-10", "End": "2026-10-13", "Title": "Fall Retreat", "Tags": calendarIDs(t, "Jays, Conference"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "W3", "Start": "2026-11-08", "Title": "Open House", "Tags": calendarIDs(t, "Jays, Conference"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "W4", "Start": "2026-12-18", "End": "2026-12-22", "Title": "Break Conferences", "Tags": calendarIDs(t, "Jays, Conference"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "W5", "Start": "2026-09-11", "End": "2026-09-14", "Title": "Eighth Grade Trip", "Tags": calendarIDs(t, "Jays, Trip")})
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Event("W1").Dates, ","); got != "2026-09-11,2026-09-14" {
		t.Errorf("conference span sits on %s", got)
	}
	if got := strings.Join(m.Event("W5").Dates, ","); got != "2026-09-11,2026-09-12,2026-09-13,2026-09-14" {
		t.Errorf("trip across a weekend sits on %s", got)
	}
	if got := m.Event("W1"); got.Start != "2026-09-11" || got.End != "2026-09-14" {
		t.Errorf("conference span was rewritten: %s to %s", got.Start, got.End)
	}
	cases := []struct{ date, want string }{
		{"2026-09-11", "Early Dismissal"},
		{"2026-09-12", ""},
		{"2026-09-13", ""},
		{"2026-09-14", "Early Dismissal"},
		{"2026-10-10", "Early Dismissal"},
		{"2026-10-11", "Early Dismissal"},
		{"2026-11-08", "Early Dismissal"},
		{"2026-12-19", "Early Dismissal"},
		{"2026-12-20", "Early Dismissal"},
	}
	for _, c := range cases {
		got, ok := m.Plan(c.date, "Jays")
		if c.want == "" {
			if ok {
				t.Errorf("%s: got %s, want nothing", c.date, got.Name)
			}
			continue
		}
		if !ok || got.Name != c.want {
			t.Errorf("%s: got %q, want %q", c.date, got.Name, c.want)
		}
	}
	if _, ok := m.Plan("2026-12-26", "Jays"); !ok {
		t.Errorf("winter break lost the weekend it covers")
	}
}

func TestRegularYields(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "First Day", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Regular")},
		store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "Short Day", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Early Dismissal")},
		store.Row{"Event ID": "X3", "Start": "2026-09-08", "Title": "Still First Day", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Regular")})
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Plan("2026-09-08", "Jays"); got.Name != "Early Dismissal" {
		t.Errorf("plan = %s, want Early Dismissal", got.Name)
	}
}

func TestDedupe(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "D1", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "international  night", "Tags": calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community")},
		store.Row{"Event ID": "D2", "Start": "2026-09-24 16:00", "End": "2026-09-24 19:00", "Title": "International Night", "Tags": calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community")},
		store.Row{"Event ID": "D3", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "International Night", "Tags": calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community, Parents")},
		store.Row{"Event ID": "D4", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Cross Country", "Tags": calendarIDs(t, "Condors, Ospreys, Egrets, Herons, Clubs")},
		store.Row{"Event ID": "D5", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Science Olympiad", "Tags": calendarIDs(t, "Condors, Ospreys, Egrets, Herons, Clubs")},
	)
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Duplicates != 3 || m.Event("a7@sample") != nil || m.Event("D1") == nil || m.Event("D2") == nil || m.Event("D3") == nil || m.Event("D4") == nil || m.Event("D5") == nil {
		t.Errorf("duplicates %d, a7 %v, D1 %v, D2 %v, D3 %v, D4 %v, D5 %v", m.Duplicates, m.Event("a7@sample"), m.Event("D1"), m.Event("D2"), m.Event("D3"), m.Event("D4"), m.Event("D5"))
	}
	everyone := calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule")
	early := calendarIDs(t, "Early Dismissal")
	tb = tables(t)
	for _, date := range []string{"2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"} {
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": early})
	}
	m, err = BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-09-29/returning-grade-ilp-conference-half-days") != nil || m.Event("C2026-10-02") == nil {
		t.Errorf("a covered span stayed")
	}
	tb = tables(t)
	for _, date := range []string{"2026-09-29", "2026-09-30"} {
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": early})
	}
	m, err = BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-09-29/returning-grade-ilp-conference-half-days") == nil {
		t.Errorf("a half-covered span folded")
	}
	assessment := calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Assessment")
	tb = tables(t)
	tb[GoogleTab] = append(tb[GoogleTab],
		store.Row{"Key": "m1", "Event ID": "gev0000000101", "Start": "2026-08-24", "End": "2026-08-28", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
		store.Row{"Key": "m2", "Event ID": "gev0000000102", "Start": "2026-08-31", "End": "2026-09-02", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
	)
	tb[PDFTab] = append(tb[PDFTab], store.Row{"Key": "pdf/2026-2027/2026-08-24/map-assessment", "Event ID": "pev0000000101", "Year": "2026-2027", "Start": "2026-08-24", "End": "2026-09-02", "Title": "MAP Assessment", "Tags": calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons")})
	for _, id := range []string{"gev0000000101", "gev0000000102", "pev0000000101"} {
		tb[EnrichmentTab] = append(tb[EnrichmentTab], store.Row{"Event ID": id, "Tags": assessment})
	}
	m, err = BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-08-24/map-assessment") != nil || m.Event("m1") == nil || m.Event("m2") == nil {
		t.Errorf("a span across a weekend stayed: pdf %v", m.Event("pdf/2026-2027/2026-08-24/map-assessment"))
	}
	tb = tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "W1", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Family Camping", "Tags": calendarIDs(t, "Jays, Ravens, Trip")},
		store.Row{"Event ID": "W2", "Start": "2026-09-12", "End": "2026-09-13", "Title": "family  camping", "Tags": calendarIDs(t, "Jays, Ravens, Trip")},
		store.Row{"Event ID": "W3", "Start": "2026-09-12", "End": "2026-09-14", "Title": "Family Camping", "Tags": calendarIDs(t, "Jays, Ravens, Trip")},
		store.Row{"Event ID": "W4", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Camping Weekend", "Tags": calendarIDs(t, "Jays, Ravens, Trip")},
	)
	m, err = BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if (m.Event("W1") == nil) == (m.Event("W2") == nil) || m.Event("W3") == nil || m.Event("W4") == nil {
		t.Errorf("weekend twins: W1 %v, W2 %v, W3 %v, W4 %v", m.Event("W1"), m.Event("W2"), m.Event("W3"), m.Event("W4"))
	}
	tb = tables(t)
	tb[GoogleTab] = append(tb[GoogleTab], store.Row{"Key": "g1", "Event ID": "gev0000000103", "Start": "2026-08-18", "End": "2026-08-18", "Title": "first  day of SCHOOL", "Updated": "2026-08-01 09:00", "Sequence": "0"})
	tb[EnrichmentTab] = append(tb[EnrichmentTab], store.Row{"Event ID": "gev0000000103", "Tags": calendarIDs(t, "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule")})
	m, err = BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("g1") != nil || m.Event("pdf/2026-2027/2026-08-18/first-day-of-school") == nil || len(m.Years) != 1 {
		t.Errorf("marker twin lost: g1 %v, years %d", m.Event("g1"), len(m.Years))
	}
}

func TestRefusals(t *testing.T) {
	broken := map[string]func(store.Tables){
		"conflicting day types in one layer": func(tb store.Tables) {
			tb[EventsTab] = append(tb[EventsTab],
				store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "Early Dismissal")},
				store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "No Care", "Tags": calendarIDs(t, "Jays, Schedule"), "Day Type": calendarIDs(t, "No Aftercare")})
		},
		"timed event with a day type": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "Day Type": calendarIDs(t, "Early Dismissal")})
		},
		"no tags": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "Tags": Clear})
		},
		"no regular day type": func(tb store.Tables) {
			tb[DayTypesTab] = tb[DayTypesTab][1:]
		},
		"no tags tab rows": func(tb store.Tables) {
			tb[TagsTab] = nil
		},
		"a tag's order ending in 0": func(tb store.Tables) {
			tb[TagsTab][0][store.OrderColumn] = "a0"
		},
		"a feed's order in capitals": func(tb store.Tables) {
			tb[FeedsTab][0][store.OrderColumn] = "B"
		},
		"unknown tag": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "Tags": "Penguins"})
		},
		"a tags cell naming an unknown id": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "Tags": calendarIDs(t, "Jays") + ", tag0000000999"})
		},
		"a tags cell naming a tag by its name": func(tb store.Tables) {
			tb[EnrichmentTab][0]["Tags"] = calendarIDs(t, "Hummingbirds") + ", Schedule"
		},
		"a tags cell naming a classroom by its name": func(tb store.Tables) {
			tb[EnrichmentTab][0]["Tags"] = "Hummingbirds, " + calendarIDs(t, "Schedule")
		},
		"a tags row with no id": func(tb store.Tables) {
			tb[TagsTab][1]["Tag ID"] = ""
		},
		"two tags with one id": func(tb store.Tables) {
			tb[TagsTab][1]["Tag ID"] = tb[TagsTab][2]["Tag ID"]
		},
		"no row for a built-in tag": func(tb store.Tables) {
			tb[TagsTab] = slices.DeleteFunc(tb[TagsTab], func(r store.Row) bool { return r["Tag ID"] == TagGoing })
		},
		"no row for a built-in day type": func(tb store.Tables) {
			tb[DayTypesTab] = slices.DeleteFunc(tb[DayTypesTab], func(r store.Row) bool { return r["Day Type ID"] == NoSchoolDayType })
		},
		"a day types row with no id": func(tb store.Tables) {
			tb[DayTypesTab][1]["Day Type ID"] = ""
		},
		"unknown day type": func(tb store.Tables) {
			tb[DayOverridesTab] = append(tb[DayOverridesTab], store.Row{"Date": "2026-09-08", "Day Type": "Snow Day"})
		},
		"a day type named by its name": func(tb store.Tables) {
			tb[DayOverridesTab] = append(tb[DayOverridesTab], store.Row{"Date": "2026-09-08", "Day Type": "No School"})
		},
		"unknown classroom in a day override": func(tb store.Tables) {
			tb[DayOverridesTab] = append(tb[DayOverridesTab], store.Row{"Date": "2026-09-08", "Classrooms": "Penguins", "Day Type": calendarIDs(t, "No School")})
		},
		"end before start": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "gev0000000007", "End": "2026-09-24 15:00"})
		},
		"duplicate id": func(tb store.Tables) {
			tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "gev0000000007", "Start": "2026-09-24", "Title": "Again"})
		},
		"pdf year mismatch": func(tb store.Tables) {
			tb[PDFTab][0]["Year"] = "2025-2026"
		},
		"two first days": func(tb store.Tables) {
			tb[PDFTab] = append(tb[PDFTab], store.Row{"Key": "pdf/x", "Event ID": "pev0000000102", "Year": "2026-2027", "Start": "2026-08-19", "Title": "Another first day", "Marker": MarkerFirstDay})
		},
		"feed naming an unknown classroom": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Classrooms": "Penguins"})
		},
		"feed naming an unknown tag": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Tags": "Bake Sales"})
		},
		"feed naming a tag by its name": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Tags": "Trip"})
		},
		"feed with no name": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org"})
		},
		"feed with no token": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Email": "a@x.org", "Name": "Mine"})
		},
		"two feeds with one token": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": tb[FeedsTab][0]["Token"], "Email": "a@x.org", "Name": "Mine"})
		},
	}
	for name, breaker := range broken {
		tb := tables(t)
		breaker(tb)
		if _, err := BuildCalendar(tb, roster); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestFeeds(t *testing.T) {
	m := load(t)
	f := m.Feed("sample7feedtoken4jordan2whitfield")
	if f == nil || f.Email != "jordan.whitfield@heliosschool.org" || strings.Join(f.Classrooms, ",") != "Jays,Ospreys" {
		t.Fatalf("feed = %+v", f)
	}
	cases := map[string]bool{
		"a4@sample":  true,
		"a5@sample":  true,
		"a6@sample":  false,
		"a7@sample":  false,
		"a11@sample": false,
	}
	for id, want := range cases {
		if got := f.Carries(m.Event(id)); got != want {
			t.Errorf("%s carried = %v, want %v", id, got, want)
		}
	}
	everything := Feed{}
	if !everything.Carries(m.Event("a6@sample")) || !everything.Carries(m.Event("a7@sample")) {
		t.Errorf("a feed with no filter left something out")
	}
	next := tables(t)
	next[FeedsTab] = append(next[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", store.OrderColumn: "a"}, store.Row{"Token": "t3", "Email": "a@x.org", "Name": "Yours"})
	next[FeedsTab][0][store.OrderColumn] = "m"
	m2, err := BuildCalendar(next, roster)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	for _, f := range m2.Feeds {
		order = append(order, f.Token)
	}
	if strings.Join(order, ",") != "t2,sample7feedtoken4jordan2whitfield,t3" || m2.Feed("t3").Name != "Yours" {
		t.Errorf("feeds by order: %v", order)
	}
}

func TestICS(t *testing.T) {
	m := load(t)
	f := m.Feed("sample7feedtoken4jordan2whitfield")
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-01 08:00", Location)
	out := string(ICS(m, calendarDirectory(t, "../../sampledata"), f, nil, "https://calendar.heliosiandev.com:8080", at))
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "X-WR-CALNAME:Whitfield school days\r\n", "END:VCALENDAR\r\n",
		"UID:a4@sample\r\n", "DTSTART;VALUE=DATE:20260907\r\n", "DTEND;VALUE=DATE:20260908\r\n",
		"SUMMARY:Jays and Ravens Camping\r\n", "DTSTART;VALUE=DATE:20260909\r\n", "DTEND;VALUE=DATE:20260912\r\n",
		"DTSTAMP:20260901T150000Z\r\n", "URL:https://calendar.heliosiandev.com:8080/e/gev0000000005\r\n",
		"CATEGORIES:Jays\\, Ravens\\, Trip\r\n", "DESCRIPTION:No School\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("feed lacks %q", want)
		}
	}
	for _, absent := range []string{"Hummingbird CAFE", "International Night"} {
		if strings.Contains(out, absent) {
			t.Errorf("feed carries %q", absent)
		}
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line over 75 octets: %q", line)
		}
	}
}

func calendarDirectory(t *testing.T, root string) *Directory {
	t.Helper()
	return loadDirectory(t, &data.Dir{Root: root}, nil, testkit.None, sampleKey)
}

func TestRender(t *testing.T) {
	m := load(t)
	d := calendarDirectory(t, "../../sampledata")
	settings := &Config{ClassroomColors: map[string]string{"Jays": "#fec502", "Ravens": "#fec502"}}
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-08 08:00", Location)
	v := RenderCalendar(m, d, settings, access.Actor{Email: "jordan.whitfield@heliosschool.org"}, at, nil)
	if strings.Join(v.User.Classrooms, ",") != "Jays,Ospreys" || len(v.User.Students) != 2 || v.User.Initial != "J" {
		t.Errorf("parent = %+v", v.User)
	}
	if len(v.Feeds) != 1 || v.Today != "2026-09-08" || len(v.Events) != 22 {
		t.Errorf("view = feeds %d today %s events %d", len(v.Feeds), v.Today, len(v.Events))
	}
	if v.Days["2026-09-08"]["Jays"] != RegularDayType || len(v.Classrooms) != 9 || v.Classrooms[3].ID != calendarIDs(t, "Jays") || len(v.Tags) != 20 || v.Colors["Jays"] != "#fec502" {
		t.Errorf("plan, vocabulary, or colors missing")
	}
	cases := map[string]string{"sam.whitfield@heliosschool.org": "Jays", "miguel.santos@heliosschool.org": "Hawks", "bill.ryder@heliosschool.org": "", "nobody@x.org": ""}
	for email, want := range cases {
		v := RenderCalendar(m, d, settings, access.Actor{Email: email}, at, nil)
		if got := strings.Join(v.User.Classrooms, ","); got != want || len(v.Feeds) != 0 {
			t.Errorf("%s: classrooms %q, want %q; feeds %d", email, got, want, len(v.Feeds))
		}
	}
	if v := RenderCalendar(m, d, settings, d.ActorOf("nobody@x.org", CalendarAdminAllowances), at, nil); v.User.Name != "Nobody" || !v.User.IsAdmin {
		t.Errorf("stranger = %+v", v.User)
	}
}

func TestRenderLinked(t *testing.T) {
	m := load(t)
	d := calendarDirectory(t, "../../sampledata")
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-08 08:00", Location)
	linked := []Linked{
		{Source: SourceCelebrate, ID: "pty0000000001", EventID: "pty0000000001", Title: "Fondue & Fort Night", Summary: "A cozy evening of fondue", Description: "Join us.", Location: "The Parks' House", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineWaitlisted},
		{Source: SourceTeam, ID: "act0000000006", EventID: "tev0000000006", Title: "Book Fair", Start: "2027-03-30", End: "2027-04-02", Path: "/activities/act0000000006", Availability: "open"},
		{Source: SourceTeam, ID: "act0000000001", EventID: "tev0000000001", Title: "HCA International Night 2026", Description: "Booths wanted.", Start: "2026-09-24 15:30", End: "2026-09-24 18:30", Path: "/v/international-night", Availability: "open", Mine: MineGoing},
		{Source: SourceTeam, ID: "act0000000005", EventID: "tev0000000005", Title: "Back to School Social", Start: "2026-08-27 15:00", End: "2026-08-27 17:00", Path: "/activities/act0000000005", Availability: "done"},
	}
	v := RenderCalendar(m, d, &Config{}, access.Actor{Email: "nobody@x.org"}, at, linked)
	if len(v.Events) != 23 {
		t.Fatalf("events = %d", len(v.Events))
	}
	if len(v.Tags) != 20 || v.Tags[15].ID != TagCelebrate || v.Tags[16].ID != TagHCA || v.Tags[17].ID != TagMisc || v.Tags[18].ID != TagGoing || v.Tags[19].ID != TagWaitlisted || v.Tags[18].Name != "Going" || v.Tags[18].Role != "going" || len(m.Tags) != 20 {
		t.Errorf("tags = %+v", v.Tags)
	}
	var fondue, fair, night, social *Event
	for i, e := range v.Events {
		if e.Source == SourceCelebrate || e.Source == SourceTeam {
			if i == 0 || v.Events[i-1].start.After(e.start) {
				t.Errorf("linked event out of order at %d", i)
			}
		}
		switch e.ID {
		case "pty0000000001":
			fondue = e
		case "tev0000000006":
			fair = e
		case "gev0000000007":
			night = e
		case "tev0000000005":
			social = e
		case "tev0000000001":
			t.Errorf("international night listed twice")
		}
	}
	if fondue == nil || fair == nil || night == nil || social == nil {
		t.Fatal("linked events missing")
	}
	if night.Link != "/v/international-night" || night.Availability != "open" || night.Mine != MineGoing || night.Source != SourceGoogle || !slices.Contains(night.Tags, TagHCA) || !slices.Contains(night.Tags, TagGoing) || !slices.Contains(night.Tags, calendarIDs(t, "Community")) || night.Description != "Booths from every classroom." {
		t.Errorf("folded event = %+v", night)
	}
	if orig := m.Event("a7@sample"); orig.Link != "" || orig.Mine != "" || slices.Contains(orig.Tags, TagHCA) || slices.Contains(orig.Tags, TagGoing) {
		t.Errorf("model event changed by folding: %+v", orig)
	}
	if social.Link != "/activities/act0000000005" || social.Mine != "" || strings.Join(social.Tags, ",") != TagHCA {
		t.Errorf("social folded into back to school night: %+v", social)
	}
	if fondue.Source != SourceCelebrate || fondue.Link != "/p/fondue" || fondue.Availability != "available" || fondue.Mine != MineWaitlisted || fondue.AllDay || strings.Join(fondue.Tags, ",") != TagCelebrate+","+TagWaitlisted || len(fondue.Classrooms) != 0 {
		t.Errorf("party = %+v", fondue)
	}
	if fondue.Description != "A cozy evening of fondue\n\nJoin us." || fondue.End != "2026-09-19 21:00" || strings.Join(fondue.Dates, ",") != "2026-09-19" {
		t.Errorf("party text or dates = %+v", fondue)
	}
	if !fair.AllDay || strings.Join(fair.Tags, ",") != TagHCA || fair.Link != "/activities/act0000000006" || len(fair.Dates) != 4 {
		t.Errorf("hca event = %+v", fair)
	}
	if len(m.Events) != 20 {
		t.Errorf("model events changed: %d", len(m.Events))
	}
}

func sampleEventsServer(t *testing.T) (*Store, http.Handler) {
	t.Helper()
	sampleSheet(t)
	outsideSuperAdmin(t, sheet)
	listRows(t, nil)
	s := sampleStore(t, sheet, queue, sampleDeps(sampleKey))
	mux := http.NewServeMux()
	hooks := RegisterCalendar(mux, calendarDeps(s, CalendarMail{Sender: mailtest.Discard()}))
	return s, served(mux, s, hooks)
}

func titlesOf(events []map[string]any) []string {
	out := []string{}
	for _, e := range events {
		title, _ := e["title"].(string)
		out = append(out, title)
	}
	return out
}

func eventTitled(events []map[string]any, title string) map[string]any {
	for _, e := range events {
		if e["title"] == title {
			return e
		}
	}
	return nil
}

func TestUpcoming(t *testing.T) {
	_, h := sampleEventsServer(t)
	clockAt(t, time.Date(2026, 9, 10, 8, 0, 0, 0, Location))
	const parentOf = "jordan.whitfield@heliosschool.org"
	got := upcomingOf(t, h, parentOf, feedKey(parentOf, MyHeliosianToken))
	titles := titlesOf(got)
	for i, u := range got {
		if i > 0 && got[i-1]["start"].(string) > u["start"].(string) {
			t.Errorf("out of order: %q (%s) after %q (%s)", u["title"], u["start"], got[i-1]["title"], got[i-1]["start"])
		}
		if u["end"].(string) < "2026-09-10" || u["path"] == "" || u["app"] == "" {
			t.Errorf("upcoming %+v", u)
		}
	}
	if slices.Contains(titles, "Hummingbird CAFE") || slices.Contains(titles, "Hawks and Falcons CAFE") || !slices.Contains(titles, "Condors and Ospreys CAFE") || slices.Contains(titles, "Back to School Social") {
		t.Errorf("a parent in Jays and Ospreys sees %v", titles)
	}
	if len(got) == 0 || got[0]["title"] != "Jays and Ravens Camping" || got[0]["path"] != "/e/gev0000000005" || got[0]["app"] != "when" || got[0]["link"] != nil || got[0]["call"] != nil {
		t.Fatalf("first = %+v", got)
	}
	party := eventTitled(got, "Fondue & Fort Night")
	if party == nil || party["id"] != "pty0000000001" || party["path"] != "/p/fondue" || party["link"] != "/p/fondue" || party["app"] != "celebrate" || party["mine"] != MineGoing || party["availability"] != "available" || party["image"] == nil || party["start"] != "2026-09-19 17:00" {
		t.Errorf("party = %+v", party)
	}
	if call, _ := party["call"].(string); call != "You, Robin, Sam and Ella have tickets" {
		t.Errorf("the family's call on the party = %q", call)
	}
	night := eventTitled(got, "International Night")
	if night == nil || night["id"] != "gev0000000007" || night["path"] != "/v/international-night" || night["link"] != "/v/international-night" || night["app"] != "team" || night["start"] != "2026-09-24 16:00" || night["end"] != "2026-09-24 18:00" {
		t.Errorf("folded hca event = %+v", night)
	}
	if later := listOf(t, h, parentOf, "/api/events?from=2027-09-10&calendar="+feedKey(parentOf, MyHeliosianToken)); len(later) != 0 {
		t.Errorf("a year on = %v", titlesOf(later))
	}
	if rec := testkit.Call(t, h, parentOf, "GET", "/api/events?from=next+week", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a from that is no date: %d", rec.Code)
	}
	if rec := testkit.Call(t, h, parentOf, "GET", "/api/events?to=2026-9-1", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a to that is no date: %d", rec.Code)
	}
}

func TestEventsFromAndTo(t *testing.T) {
	_, h := sampleEventsServer(t)
	const viewer = "jordan.whitfield@heliosschool.org"
	all := listOf(t, h, viewer, "/api/events")
	window := listOf(t, h, viewer, "/api/events?from=2026-09-20&to=2026-10-05")
	if len(window) == 0 || len(window) >= len(all) {
		t.Fatalf("a window of %d out of %d", len(window), len(all))
	}
	inWindow := func(e map[string]any) bool {
		start, end := e["start"].(string)[:10], e["end"].(string)[:10]
		return end >= "2026-09-20" && start <= "2026-10-05"
	}
	for _, e := range window {
		if !inWindow(e) {
			t.Errorf("%s (%s to %s) is outside the window", e["title"], e["start"], e["end"])
		}
	}
	want := 0
	for _, e := range all {
		if inWindow(e) {
			want++
		}
	}
	if len(window) != want {
		t.Errorf("the window holds %d of the %d the calendar has in it", len(window), want)
	}
}

func TestSameEvent(t *testing.T) {
	at := func(start, end string) *Event {
		s, e, allDay, err := parseWhen(start, end)
		if err != nil {
			t.Fatal(err)
		}
		return &Event{start: s, end: e, AllDay: allDay}
	}
	cases := []struct {
		a, b   string
		ea, eb *Event
		want   bool
	}{
		{"International Night", "HCA International Night 2026", at("2026-09-24 16:00", "2026-09-24 18:00"), at("2026-09-24 15:30", "2026-09-24 18:30"), true},
		{"Movie Night", "All School Movie Night", at("2026-11-06 18:00", "2026-11-06 20:00"), at("2026-11-06", ""), true},
		{"Spring Celebration", "Helios Spring Celebration 2027", at("2027-03-06 17:30", "2027-03-06 22:00"), at("2027-03-06 17:30", "2027-03-06 22:00"), true},
		{"LS Back to School Night", "Back to School Social", at("2026-08-27 18:00", "2026-08-27 19:30"), at("2026-08-27 15:00", "2026-08-27 17:00"), false},
		{"International Night", "International Night", at("2026-09-24 16:00", "2026-09-24 18:00"), at("2026-09-25 16:00", "2026-09-25 18:00"), false},
		{"Coffee Cart", "Coffee Cart", at("2026-10-02 08:00", "2026-10-02 09:00"), at("2026-10-02 15:00", "2026-10-02 16:00"), false},
		{"Cocoa & Cookies", "Cocoa and Cookies", at("2026-12-16 11:45", "2026-12-16 13:00"), at("2026-12-16 11:45", "2026-12-16 13:00"), true},
	}
	for _, c := range cases {
		c.ea.Title, c.eb.Title = c.a, c.b
		if got := sameEvent(c.ea, c.eb); got != c.want {
			t.Errorf("%q vs %q: %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMiscTag(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab], store.Row{
		"Event ID": "MISC0001", "Start": "2026-10-14 08:30", "End": "2026-10-14 10:00", "Title": "Vision Screening",
		"Tags": calendarIDs(t, "Hummingbirds, Hawks"), "Added By": "office@x.org", "Added": "2026-09-01",
	})
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if e := m.Event("MISC0001"); cells.JoinList(e.Tags) != calendarIDs(t, "Hummingbirds, Hawks, Misc") || strings.Join(e.Classrooms, ",") != "Hummingbirds,Hawks" {
		t.Errorf("untagged event = %+v", e)
	}
	if e := m.Event("a5@sample"); slices.Contains(e.Tags, TagMisc) {
		t.Errorf("categorized event got Misc: %+v", e)
	}
}

func TestRenamingKeepsEvents(t *testing.T) {
	trip, early := calendarIDs(t, "Trip"), calendarIDs(t, "Early Dismissal")
	tb := tables(t)
	for _, row := range tb[TagsTab] {
		switch row["Tag ID"] {
		case trip:
			row["Tag"] = "Outing"
		case TagGoing:
			row["Tag"] = "Attending"
		}
	}
	for _, row := range tb[DayTypesTab] {
		if row["Day Type ID"] == early {
			row["Day Type"] = "Half Day"
		}
	}
	m, err := BuildCalendar(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	camping := m.Event("a5@sample")
	if !slices.Contains(camping.Tags, trip) || m.TagName(trip) != "Outing" || !slices.Contains(m.TagNames(camping.Tags), "Outing") {
		t.Errorf("a renamed tag lost its event: %v, %v", camping.Tags, m.TagNames(camping.Tags))
	}
	if got, _ := m.Plan("2026-08-18", "Hummingbirds"); got.ID != early || got.Name != "Half Day" {
		t.Errorf("a renamed day type left the plan: %+v", got)
	}
	if e := m.Event("pdf/2026-2027/2026-08-21/professional-development-half-day"); e == nil || e.DayType != early || m.DayTypeName(e.DayType) != "Half Day" {
		t.Errorf("a renamed day type left its event: %+v", e)
	}
	if g := m.Tag(TagGoing); g == nil || g.Name != "Attending" || !g.BuiltIn || g.Role != "going" {
		t.Errorf("a renamed built-in = %+v", g)
	}
}

func TestBuiltInRowsAreRequired(t *testing.T) {
	tb := tables(t)
	tb[TagsTab] = slices.DeleteFunc(tb[TagsTab], func(r store.Row) bool { return r["Tag ID"] == TagWaitlisted })
	if _, err := BuildCalendar(tb, roster); err == nil || !strings.Contains(err.Error(), "built-in tag "+TagWaitlisted) {
		t.Errorf("a missing built-in tag row: %v", err)
	}
	tb = tables(t)
	tb[DayTypesTab] = slices.DeleteFunc(tb[DayTypesTab], func(r store.Row) bool { return r["Day Type ID"] == EarlyDismissalDayType })
	if _, err := BuildCalendar(tb, roster); err == nil || !strings.Contains(err.Error(), "built-in day type "+EarlyDismissalDayType) {
		t.Errorf("a missing built-in day type row: %v", err)
	}
	tb = tables(t)
	tb[EventsTab][0]["Tags"] = "tag0000000999"
	if _, err := BuildCalendar(tb, roster); err == nil || !strings.Contains(err.Error(), `tag "tag0000000999" is neither an id`) {
		t.Errorf("a tags cell naming an unknown id: %v", err)
	}
}

func TestSchoolYear(t *testing.T) {
	cases := map[string]string{"2026-07-15": "2026-2027", "2026-06-30": "2025-2026", "2027-01-05": "2026-2027"}
	for date, want := range cases {
		d, _ := time.ParseInLocation(DateFormat, date, Location)
		if got := SchoolYear(d); got != want {
			t.Errorf("%s: %s, want %s", date, got, want)
		}
	}
}

func TestGoogleEventURL(t *testing.T) {
	cases := map[string]string{
		"abc123@google.com":                 "abc123 heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com",
		"abc123@google.com/20260901":        "abc123_20260901 heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com",
		"abc123@google.com/20260901T160000": "abc123_20260901T230000Z heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com",
	}
	for key, pair := range cases {
		want := "https://www.google.com/calendar/event?eid=" + base64.RawStdEncoding.EncodeToString([]byte(pair))
		if got := GoogleEventURL(key); got != want {
			t.Errorf("%s: %s, want %s", key, got, want)
		}
	}
	if got := GoogleEventURL("a1@sample"); got != "" {
		t.Errorf("sample key linked: %s", got)
	}
}

func TestSharingWords(t *testing.T) {
	for cell, want := range map[string]string{"": SharingPublic, "public": SharingPublic, "LINK": SharingLink, "invite only": SharingInvited} {
		tb := tables(t)
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "shared", "Start": "2026-10-01", "End": "2026-10-01", "Title": "Shared", "Tags": calendarIDs(t, "Jays"), "Sharing": cell})
		m, err := BuildCalendar(tb, roster)
		if err != nil {
			t.Fatalf("%q: %v", cell, err)
		}
		if e := m.Event("shared"); e == nil || e.Sharing != want {
			t.Errorf("%q: %+v", cell, e)
		}
	}
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "shared", "Start": "2026-10-01", "End": "2026-10-01", "Title": "Shared", "Tags": calendarIDs(t, "Jays"), "Sharing": "Private"})
	if _, err := BuildCalendar(tb, roster); err == nil || !strings.Contains(err.Error(), `sharing "Private"`) {
		t.Errorf("Private read: %v", err)
	}
}

func TestPartiesFor(t *testing.T) {
	_, h := sampleEventsServer(t)
	parties := func(email string) []map[string]any {
		return listOf(t, h, email, "/api/events?app=celebrate&from="+now().Format(DateFormat))
	}
	got := parties(sam)
	titles := titlesOf(got)
	if len(got) == 0 || got[0]["title"] != "Fondue & Fort Night" || !slices.Contains(titles, "Wurst Helios Party") {
		t.Fatalf("parties: %v", titles)
	}
	for i, p := range got {
		if p["app"] != "celebrate" || p["source"] != SourceCelebrate || p["end"].(string)[:10] < now().Format(DateFormat) {
			t.Errorf("not an upcoming party: %+v", p)
		}
		if i > 0 && got[i-1]["start"].(string) > p["start"].(string) {
			t.Errorf("out of order: %s after %s", p["title"], got[i-1]["title"])
		}
	}
	if slices.Contains(titles, "Baegels and Meimosas") || slices.Contains(titles, "International Night") || slices.Contains(titles, "Jays and Ravens Camping") {
		t.Errorf("the parties list a past party, an HCA event or a school event: %v", titles)
	}
	if f := got[0]; f["mine"] != MineGoing || f["call"] == nil || f["link"] != "/p/fondue" {
		t.Errorf("fondue: %+v", f)
	}
	if w := eventTitled(got, "Wurst Helios Party"); w["mine"] != nil || w["call"] != "Get tickets" || w["start"] != "2026-10-03 15:30" {
		t.Errorf("wurst: %+v", w)
	}
	for _, p := range got {
		if p["hosted"] == true {
			t.Errorf("a party sam does not host is marked hosted: %+v", p)
		}
	}
	for _, other := range []string{"when", "team"} {
		for _, e := range listOf(t, h, sam, "/api/events?app="+other) {
			if e["app"] != other {
				t.Errorf("app=%s lists %s from %s", other, e["title"], e["app"])
			}
		}
	}
}

func TestCardsMarkHosted(t *testing.T) {
	_, h := sampleEventsServer(t)
	hosted := func(email string) any {
		return eventTitled(listOf(t, h, email, "/api/events?app=celebrate"), "Tees & Teas")["hosted"]
	}
	if got := hosted("jordan.whitfield@heliosschool.org"); got != true {
		t.Errorf("the host's own party: hosted %v", got)
	}
	if got := hosted(mia); got != nil {
		t.Errorf("someone else's party: hosted %v", got)
	}
}
