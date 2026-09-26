package calendar

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

var roster = Roster{Classrooms: []Classroom{
	{Name: "Hummingbirds", Band: "Hummingbirds", Grades: []string{"Kindergarten"}},
	{Name: "Hawks", Band: "Halcons", Grades: []string{"Grade 1"}},
	{Name: "Falcons", Band: "Halcons", Grades: []string{"Grade 2"}},
	{Name: "Jays", Band: "Jayvens", Grades: []string{"Grade 3"}},
	{Name: "Ravens", Band: "Jayvens", Grades: []string{"Grade 4"}},
	{Name: "Condors", Band: "Cospreys", Grades: []string{"Grade 5"}, Crews: []string{"Big Sur", "Pinnacles"}},
	{Name: "Ospreys", Band: "Cospreys", Grades: []string{"Grade 6"}, Crews: []string{"River", "Sea"}},
	{Name: "Egrets", Band: "Hegrets", Grades: []string{"Grade 7"}, Crews: []string{"Great", "Snowy"}},
	{Name: "Herons", Band: "Hegrets", Grades: []string{"Grade 8"}, Crews: []string{"Great Blue", "Green"}},
}}

func readTables(t *testing.T, dir *data.Dir) store.Tables {
	t.Helper()
	out := store.Tables{}
	for _, tab := range spec(nil, nil).Tabs {
		_, rows, err := dir.Table(appName, tab.Name)
		if err != nil {
			t.Fatal(err)
		}
		out[tab.Name] = rows
	}
	return out
}

func tables(t *testing.T) store.Tables {
	t.Helper()
	return readTables(t, &data.Dir{Root: "../../sampledata"})
}

func load(t *testing.T) *Model {
	t.Helper()
	model, err := BuildModel(tables(t), roster)
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
		if e.ID == "a12@sample" {
			t.Errorf("hidden event listed")
		}
	}
	if len(m.Tags) != 15 || m.Tags[0].Name != "Schedule" {
		t.Errorf("tags = %+v", m.Tags)
	}
	camping := m.Event("a5@sample")
	if camping.Title != "Jays and Ravens Camping" || strings.Join(camping.Classrooms, ",") != "Jays,Ravens" || strings.Join(camping.Tags, ",") != "Jays,Ravens,Trip" {
		t.Errorf("override not applied: %+v", camping)
	}
	if !slices.Contains(camping.Keywords, "overnight") {
		t.Errorf("enrichment keywords lost: %v", camping.Keywords)
	}
	bts := m.Event("a2@sample")
	if strings.Join(bts.Classrooms, ",") != "Hummingbirds,Hawks,Falcons,Jays,Ravens" || bts.AllDay || !slices.Contains(bts.Tags, "Orientation") {
		t.Errorf("lower school event: %+v", bts)
	}
	hca := m.Event("3PL9W2ZC")
	if hca.Source != SourceSheet || !slices.Contains(hca.Tags, "Community") || len(hca.Classrooms) != 9 {
		t.Errorf("sheet event: %+v", hca)
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
	tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "a7@sample", "Keywords": "Community, booths, hummingbirds, food"})
	m, err := BuildModel(tb, roster)
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
	regular := m.DayType("Regular")
	if len(regular.Blocks) != 4 || regular.Blocks[0].Name != "Dropoff" || regular.Blocks[3].End != "18:00" {
		t.Errorf("regular blocks = %+v", regular.Blocks)
	}
	if none := m.DayType("No School"); len(none.Blocks) != 0 {
		t.Errorf("no school has blocks: %+v", none.Blocks)
	}
}

func TestNoSchoolWins(t *testing.T) {
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "Closed", "Tags": "Jays, Schedule", "Day Type": "No School"},
		store.Row{"Event ID": "X3", "Start": "2026-09-08", "Title": "Still Short", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"})
	m, err := BuildModel(tb, roster)
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
		store.Row{"Event ID": "W1", "Start": "2026-09-11", "End": "2026-09-14", "Title": "Conferences", "Tags": "Jays, Conference", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "W2", "Start": "2026-10-10", "End": "2026-10-13", "Title": "Fall Retreat", "Tags": "Jays, Conference", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "W3", "Start": "2026-11-08", "Title": "Open House", "Tags": "Jays, Conference", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "W4", "Start": "2026-12-18", "End": "2026-12-22", "Title": "Break Conferences", "Tags": "Jays, Conference", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "W5", "Start": "2026-09-11", "End": "2026-09-14", "Title": "Eighth Grade Trip", "Tags": "Jays, Trip"})
	m, err := BuildModel(tb, roster)
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
		store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "First Day", "Tags": "Jays, Schedule", "Day Type": "Regular"},
		store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "Short Day", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"},
		store.Row{"Event ID": "X3", "Start": "2026-09-08", "Title": "Still First Day", "Tags": "Jays, Schedule", "Day Type": "Regular"})
	m, err := BuildModel(tb, roster)
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
		store.Row{"Event ID": "D1", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "international  night", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community"},
		store.Row{"Event ID": "D2", "Start": "2026-09-24 16:00", "End": "2026-09-24 19:00", "Title": "International Night", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community"},
		store.Row{"Event ID": "D3", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "International Night", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community, Parents"},
		store.Row{"Event ID": "D4", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Cross Country", "Tags": "Condors, Ospreys, Egrets, Herons, Clubs"},
		store.Row{"Event ID": "D5", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Science Olympiad", "Tags": "Condors, Ospreys, Egrets, Herons, Clubs"},
	)
	m, err := BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Duplicates != 3 || m.Event("a7@sample") != nil || m.Event("D1") == nil || m.Event("D2") == nil || m.Event("D3") == nil || m.Event("D4") == nil || m.Event("D5") == nil {
		t.Errorf("duplicates %d, a7 %v, D1 %v, D2 %v, D3 %v, D4 %v, D5 %v", m.Duplicates, m.Event("a7@sample"), m.Event("D1"), m.Event("D2"), m.Event("D3"), m.Event("D4"), m.Event("D5"))
	}
	everyone := "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule"
	tb = tables(t)
	for _, date := range []string{"2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"} {
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": "Early Dismissal"})
	}
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-09-29/returning-grade-ilp-conference-half-days") != nil || m.Event("C2026-10-02") == nil {
		t.Errorf("a covered span stayed")
	}
	tb = tables(t)
	for _, date := range []string{"2026-09-29", "2026-09-30"} {
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": "Early Dismissal"})
	}
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-09-29/returning-grade-ilp-conference-half-days") == nil {
		t.Errorf("a half-covered span folded")
	}
	assessment := "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Assessment"
	tb = tables(t)
	tb[GoogleTab] = append(tb[GoogleTab],
		store.Row{"Key": "m1", "Start": "2026-08-24", "End": "2026-08-28", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
		store.Row{"Key": "m2", "Start": "2026-08-31", "End": "2026-09-02", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
	)
	tb[PDFTab] = append(tb[PDFTab], store.Row{"Key": "pdf/2026-2027/2026-08-24/map-assessment", "Year": "2026-2027", "Start": "2026-08-24", "End": "2026-09-02", "Title": "MAP Assessment", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons"})
	for _, id := range []string{"m1", "m2", "pdf/2026-2027/2026-08-24/map-assessment"} {
		tb[EnrichmentTab] = append(tb[EnrichmentTab], store.Row{"Event ID": id, "Tags": assessment})
	}
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-08-24/map-assessment") != nil || m.Event("m1") == nil || m.Event("m2") == nil {
		t.Errorf("a span across a weekend stayed: pdf %v", m.Event("pdf/2026-2027/2026-08-24/map-assessment"))
	}
	tb = tables(t)
	tb[EventsTab] = append(tb[EventsTab],
		store.Row{"Event ID": "W1", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Family Camping", "Tags": "Jays, Ravens, Trip"},
		store.Row{"Event ID": "W2", "Start": "2026-09-12", "End": "2026-09-13", "Title": "family  camping", "Tags": "Jays, Ravens, Trip"},
		store.Row{"Event ID": "W3", "Start": "2026-09-12", "End": "2026-09-14", "Title": "Family Camping", "Tags": "Jays, Ravens, Trip"},
		store.Row{"Event ID": "W4", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Camping Weekend", "Tags": "Jays, Ravens, Trip"},
	)
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if (m.Event("W1") == nil) == (m.Event("W2") == nil) || m.Event("W3") == nil || m.Event("W4") == nil {
		t.Errorf("weekend twins: W1 %v, W2 %v, W3 %v, W4 %v", m.Event("W1"), m.Event("W2"), m.Event("W3"), m.Event("W4"))
	}
	tb = tables(t)
	tb[GoogleTab] = append(tb[GoogleTab], store.Row{"Key": "g1", "Start": "2026-08-18", "End": "2026-08-18", "Title": "first  day of SCHOOL", "Updated": "2026-08-01 09:00", "Sequence": "0"})
	tb[EnrichmentTab] = append(tb[EnrichmentTab], store.Row{"Event ID": "g1", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule"})
	m, err = BuildModel(tb, roster)
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
				store.Row{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"},
				store.Row{"Event ID": "X2", "Start": "2026-09-08", "Title": "No Care", "Tags": "Jays, Schedule", "Day Type": "No Aftercare"})
		},
		"timed event with a day type": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "a7@sample", "Day Type": "Early Dismissal"})
		},
		"no tags": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "a7@sample", "Tags": Clear})
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
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "a7@sample", "Tags": "Penguins"})
		},
		"unknown day type": func(tb store.Tables) {
			tb[DayOverridesTab] = append(tb[DayOverridesTab], store.Row{"Date": "2026-09-08", "Day Type": "Snow Day"})
		},
		"unknown classroom in a day override": func(tb store.Tables) {
			tb[DayOverridesTab] = append(tb[DayOverridesTab], store.Row{"Date": "2026-09-08", "Classrooms": "Penguins", "Day Type": "No School"})
		},
		"end before start": func(tb store.Tables) {
			tb[OverridesTab] = append(tb[OverridesTab], store.Row{"Event ID": "a7@sample", "End": "2026-09-24 15:00"})
		},
		"duplicate id": func(tb store.Tables) {
			tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "a7@sample", "Start": "2026-09-24", "Title": "Again"})
		},
		"pdf year mismatch": func(tb store.Tables) {
			tb[PDFTab][0]["Year"] = "2025-2026"
		},
		"two first days": func(tb store.Tables) {
			tb[PDFTab] = append(tb[PDFTab], store.Row{"Key": "pdf/x", "Year": "2026-2027", "Start": "2026-08-19", "Title": "Another first day", "Marker": MarkerFirstDay})
		},
		"feed naming an unknown classroom": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Classrooms": "Penguins"})
		},
		"feed naming an unknown tag": func(tb store.Tables) {
			tb[FeedsTab] = append(tb[FeedsTab], store.Row{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Tags": "Bake Sales"})
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
		if _, err := BuildModel(tb, roster); err == nil {
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
	m2, err := BuildModel(next, roster)
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
	out := string(ICS(m, fakeDirectory{}, f, nil, "https://calendar.heliosiandev.com:8080", at))
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "X-WR-CALNAME:Whitfield school days\r\n", "END:VCALENDAR\r\n",
		"UID:a4@sample\r\n", "DTSTART;VALUE=DATE:20260907\r\n", "DTEND;VALUE=DATE:20260908\r\n",
		"SUMMARY:Jays and Ravens Camping\r\n", "DTSTART;VALUE=DATE:20260909\r\n", "DTEND;VALUE=DATE:20260912\r\n",
		"DTSTAMP:20260901T150000Z\r\n", "URL:https://calendar.heliosiandev.com:8080/e/a5@sample\r\n",
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
		if len(line) > icsLineMax {
			t.Errorf("line over %d octets: %q", icsLineMax, line)
		}
	}
	long := strings.Repeat("ünïcödé ", 30)
	for _, piece := range fold("DESCRIPTION:" + long) {
		if len(piece) > icsLineMax || !utf8.ValidString(piece) {
			t.Errorf("fold broke a line: %q", piece)
		}
	}
	if got := strings.Join(fold("DESCRIPTION:"+long), ""); strings.ReplaceAll(got, " ", "") != strings.ReplaceAll("DESCRIPTION:"+long, " ", "") {
		t.Errorf("fold lost text")
	}
	if got := icsText("a;b,c\\d\nline"); got != `a\;b\,c\\d\nline` {
		t.Errorf("escape = %q", got)
	}
}

type fakeDirectory struct {
	people     map[string]Person
	kids       map[string][]Person
	households map[string][]string
	parents    map[string][]string
	lists      []List
}

func (d fakeDirectory) Household(email string) []string { return d.households[email] }

func (d fakeDirectory) Parents(email string) []string { return d.parents[email] }

func (d fakeDirectory) Resolve(email string) string { return email }

func (d fakeDirectory) Person(email string) (Person, bool) {
	p, ok := d.people[email]
	return p, ok
}

func (d fakeDirectory) Children(email string) []Person { return d.kids[email] }

func (d fakeDirectory) People() []Person {
	out := []Person{}
	for _, p := range d.people {
		out = append(out, p)
	}
	return out
}

func (d fakeDirectory) Lists(string) []List { return d.lists }

func (d fakeDirectory) Alerts(string) ([]string, []string) {
	return []string{"Sam's photo", "Family photo"}, []string{"address"}
}

func (d fakeDirectory) GradeColors() map[string]string { return nil }

func (d fakeDirectory) ClassroomColors() map[string]string {
	return map[string]string{"Jays": "#fec502", "Ravens": "#fec502"}
}

func TestRender(t *testing.T) {
	m := load(t)
	sam := Person{Email: "sam@x.org", Name: "Sam", IsStudent: true, Grade: "Grade 3", Classroom: "Jays"}
	ella := Person{Email: "ella@x.org", Name: "Ella", IsStudent: true, Grade: "Grade 6", Classroom: "Ospreys"}
	d := fakeDirectory{
		people: map[string]Person{
			"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true},
			"sam@x.org":                         sam,
			"ella@x.org":                        ella,
			"teacher@x.org":                     {Email: "teacher@x.org", Name: "Ms Finch", IsStaff: true, Classroom: "Hawks"},
			"office@x.org":                      {Email: "office@x.org", Name: "Pat", IsStaff: true},
		},
		kids: map[string][]Person{"jordan.whitfield@heliosschool.org": {ella, sam}},
	}
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-08 08:00", Location)
	v := Render(m, d, "jordan.whitfield@heliosschool.org", false, at, nil)
	if strings.Join(v.User.Classrooms, ",") != "Jays,Ospreys" || len(v.User.Students) != 2 || v.User.Initial != "J" {
		t.Errorf("parent = %+v", v.User)
	}
	if len(v.Feeds) != 1 || v.Today != "2026-09-08" || len(v.Events) != 20 || len(v.Alerts.Stale) != 2 || len(v.Alerts.Privacy) != 1 {
		t.Errorf("view = feeds %d today %s events %d alerts %+v", len(v.Feeds), v.Today, len(v.Events), v.Alerts)
	}
	if v.Days["2026-09-08"]["Jays"] != "Regular" || len(v.Classrooms) != 9 || len(v.Tags) != 20 || v.Colors["Jays"] != "#fec502" {
		t.Errorf("plan, vocabulary, or colors missing")
	}
	cases := map[string]string{"sam@x.org": "Jays", "teacher@x.org": "Hawks", "office@x.org": "", "nobody@x.org": ""}
	for email, want := range cases {
		v := Render(m, d, email, false, at, nil)
		if got := strings.Join(v.User.Classrooms, ","); got != want || len(v.Feeds) != 0 {
			t.Errorf("%s: classrooms %q, want %q; feeds %d", email, got, want, len(v.Feeds))
		}
	}
	if v := Render(m, d, "nobody@x.org", true, at, nil); v.User.Name != "Nobody" || !v.User.IsAdmin {
		t.Errorf("stranger = %+v", v.User)
	}
}

func TestRenderLinked(t *testing.T) {
	m := load(t)
	d := fakeDirectory{people: map[string]Person{}, kids: map[string][]Person{}}
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-08 08:00", Location)
	linked := []Linked{
		{Source: SourceCelebrate, ID: "P001", Title: "Fondue & Fort Night", Summary: "A cozy evening of fondue", Description: "Join us.", Location: "The Parks' House", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineWaitlisted},
		{Source: SourceTeam, ID: "E006", Title: "Book Fair", Start: "2027-03-30", End: "2027-04-02", Path: "/activities/E006", Availability: "open"},
		{Source: SourceTeam, ID: "E001", Title: "HCA International Night 2026", Description: "Booths wanted.", Start: "2026-09-24 15:30", End: "2026-09-24 18:30", Path: "/v/international-night", Availability: "open", Mine: MineGoing},
		{Source: SourceTeam, ID: "E005", Title: "Back to School Social", Start: "2026-08-27 15:00", End: "2026-08-27 17:00", Path: "/activities/E005", Availability: "done"},
	}
	v := Render(m, d, "nobody@x.org", false, at, linked)
	if len(v.Events) != 23 {
		t.Fatalf("events = %d", len(v.Events))
	}
	if len(v.Tags) != 20 || v.Tags[15].Name != TagCelebrate || v.Tags[16].Name != TagHCA || v.Tags[17].Name != TagMisc || v.Tags[18].Name != TagGoing || v.Tags[19].Name != TagWaitlisted || len(m.Tags) != 15 {
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
		case "celebrate/P001":
			fondue = e
		case "team/E006":
			fair = e
		case "a7@sample":
			night = e
		case "team/E005":
			social = e
		case "team/E001":
			t.Errorf("international night listed twice")
		}
	}
	if fondue == nil || fair == nil || night == nil || social == nil {
		t.Fatal("linked events missing")
	}
	if night.Link != "/v/international-night" || night.Availability != "open" || night.Mine != MineGoing || night.Source != SourceGoogle || !slices.Contains(night.Tags, TagHCA) || !slices.Contains(night.Tags, TagGoing) || !slices.Contains(night.Tags, "Community") || night.Description != "Booths from every classroom." {
		t.Errorf("folded event = %+v", night)
	}
	if orig := m.Event("a7@sample"); orig.Link != "" || orig.Mine != "" || slices.Contains(orig.Tags, TagHCA) || slices.Contains(orig.Tags, TagGoing) {
		t.Errorf("model event changed by folding: %+v", orig)
	}
	if social.Link != "/activities/E005" || social.Mine != "" || strings.Join(social.Tags, ",") != TagHCA {
		t.Errorf("social folded into back to school night: %+v", social)
	}
	if fondue.Source != SourceCelebrate || fondue.Link != "/p/fondue" || fondue.Availability != "available" || fondue.Mine != MineWaitlisted || fondue.AllDay || strings.Join(fondue.Tags, ",") != TagCelebrate+","+TagWaitlisted || len(fondue.Classrooms) != 0 {
		t.Errorf("party = %+v", fondue)
	}
	if fondue.Description != "A cozy evening of fondue\n\nJoin us." || fondue.End != "2026-09-19 21:00" || strings.Join(fondue.Dates, ",") != "2026-09-19" {
		t.Errorf("party text or dates = %+v", fondue)
	}
	if !fair.AllDay || strings.Join(fair.Tags, ",") != TagHCA || fair.Link != "/activities/E006" || len(fair.Dates) != 4 {
		t.Errorf("hca event = %+v", fair)
	}
	if len(m.Events) != 20 {
		t.Errorf("model events changed: %d", len(m.Events))
	}
}

func TestUpcoming(t *testing.T) {
	m := load(t)
	sam := Person{Email: "sam@x.org", Name: "Sam", IsStudent: true, Grade: "Grade 3", Classroom: "Jays"}
	ella := Person{Email: "ella@x.org", Name: "Ella", IsStudent: true, Grade: "Grade 6", Classroom: "Ospreys"}
	d := fakeDirectory{
		people: map[string]Person{
			"jordan.whitfield@heliosschool.org": {Email: "jordan.whitfield@heliosschool.org", Name: "Jordan", IsParent: true},
			"sam@x.org":                         sam,
			"ella@x.org":                        ella,
		},
		kids: map[string][]Person{"jordan.whitfield@heliosschool.org": {ella, sam}},
	}
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-10 08:00", Location)
	linked := []Linked{
		{Source: SourceCelebrate, ID: "P001", Title: "Fondue & Fort Night", Summary: "A cozy evening of fondue", Location: "The Parks' House", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineWaitlisted, Image: "/party-images/fondue.jpg"},
		{Source: SourceTeam, ID: "E001", Title: "HCA International Night 2026", Start: "2026-09-24 15:30", End: "2026-09-24 18:30", Path: "/v/international-night", Availability: "open"},
		{Source: SourceTeam, ID: "E005", Title: "Back to School Social", Start: "2026-08-27 15:00", End: "2026-08-27 17:00", Path: "/activities/E005", Availability: "done"},
	}
	got := m.Upcoming(d, "jordan.whitfield@heliosschool.org", linked, at, 0)
	titles := []string{}
	for i, u := range got {
		titles = append(titles, u.Title)
		if i > 0 && got[i-1].Start > u.Start {
			t.Errorf("out of order: %q (%s) after %q (%s)", u.Title, u.Start, got[i-1].Title, got[i-1].Start)
		}
		if u.EndAt < "2026-09-10" || u.When == "" || u.Path == "" || u.Image == "" || u.ImageApp == "" {
			t.Errorf("upcoming %+v", u)
		}
	}
	if slices.Contains(titles, "Hummingbird CAFE") || slices.Contains(titles, "Hawks and Falcons CAFE") || !slices.Contains(titles, "Condors and Ospreys CAFE") || slices.Contains(titles, "Back to School Social") {
		t.Errorf("a parent in Jays and Ospreys sees %v", titles)
	}
	if len(got) == 0 || got[0].Title != "Jays and Ravens Camping" || got[0].Path != "/e/a5@sample" || got[0].Image != "/brand/default-header.jpg" || got[0].ImageApp != "calendar" || got[0].Link != "" || got[0].Call != "" {
		t.Errorf("first = %+v", got[0])
	}
	party, night := got[1], got[2]
	if party.Title != "Fondue & Fort Night" || party.Path != "/e/celebrate/P001" || party.Link != "/p/fondue" || party.LinkApp != "celebrate" || party.Call != "Waitlisted" || party.Mine != MineWaitlisted || party.Availability != "available" || party.Image != "/party-images/fondue.jpg" || party.ImageApp != "celebrate" || party.When != "Saturday, September 19 · 5:00 – 9:00 PM" || party.Description != "A cozy evening of fondue" {
		t.Errorf("party = %+v", party)
	}
	if night.Title != "International Night" || night.Path != "/e/a7@sample" || night.Link != "/v/international-night" || night.LinkApp != "team" || night.Call != "Join" || night.Mine != "" || night.ImageApp != "calendar" || night.StartAt != "2026-09-24 16:00" || night.EndAt != "2026-09-24 18:00" {
		t.Errorf("folded hca event = %+v", night)
	}
	if all := m.Upcoming(d, "nobody@x.org", linked, at, 0); len(all) != len(got)+2 {
		t.Errorf("a stranger sees %d, a parent %d", len(all), len(got))
	}
	if two := m.Upcoming(d, "jordan.whitfield@heliosschool.org", linked, at, 2); len(two) != 2 || two[1].Title != party.Title {
		t.Errorf("limit 2 = %+v", two)
	}
	if later := m.Upcoming(d, "nobody@x.org", nil, at.AddDate(1, 0, 0), 0); len(later) != 0 {
		t.Errorf("a year on = %+v", later)
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
		"Tags": "Hummingbirds, Hawks", "Added By": "office@x.org", "Added": "2026-09-01",
	})
	m, err := BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if e := m.Event("MISC0001"); strings.Join(e.Tags, ",") != "Hummingbirds,Hawks,Misc" || strings.Join(e.Classrooms, ",") != "Hummingbirds,Hawks" {
		t.Errorf("untagged event = %+v", e)
	}
	if e := m.Event("a5@sample"); slices.Contains(e.Tags, TagMisc) {
		t.Errorf("categorized event got Misc: %+v", e)
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
		tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "shared", "Start": "2026-10-01", "End": "2026-10-01", "Title": "Shared", "Tags": "Jays", "Sharing": cell})
		m, err := BuildModel(tb, roster)
		if err != nil {
			t.Fatalf("%q: %v", cell, err)
		}
		if e := m.Event("shared"); e == nil || e.Sharing != want {
			t.Errorf("%q: %+v", cell, e)
		}
	}
	tb := tables(t)
	tb[EventsTab] = append(tb[EventsTab], store.Row{"Event ID": "shared", "Start": "2026-10-01", "End": "2026-10-01", "Title": "Shared", "Tags": "Jays", "Sharing": "Private"})
	if _, err := BuildModel(tb, roster); err == nil || !strings.Contains(err.Error(), `sharing "Private"`) {
		t.Errorf("Private read: %v", err)
	}
}

// TestPartiesFor checks Heliosian's Celebrate widget: every party still
// ahead, whatever the viewer's filters, with its standing and way in, and
// nothing else the calendar holds.
func TestPartiesFor(t *testing.T) {
	m := load(t)
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-10 08:00", Location)
	linked := []Linked{
		{Source: SourceCelebrate, ID: "P001", Title: "Fondue & Fort Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available", Mine: MineGoing, Who: []string{"Ella"}},
		{Source: SourceCelebrate, ID: "P002", Title: "Bagels", Start: "2026-09-05 10:00", End: "2026-09-05 12:00", Path: "/p/bagels", Availability: "past"},
		{Source: SourceCelebrate, ID: "P003", Title: "Wurst", Start: "2026-10-03 15:30", End: "2026-10-03 18:30", Path: "/p/wurst", Availability: "available"},
		{Source: SourceTeam, ID: "E001", Title: "HCA International Night 2026", Start: "2026-09-24 15:30", End: "2026-09-24 18:30", Path: "/v/international-night", Availability: "open"},
	}
	got := m.PartiesFor(fakeDirectory{}, "nobody@heliosschool.org", linked, at)
	if len(got) != 2 || got[0].Title != "Fondue & Fort Night" || got[1].Title != "Wurst" {
		t.Fatalf("parties: %+v", got)
	}
	if f := got[0]; f.Mine != MineGoing || f.Call != "Ella has a ticket" || f.Link != "/p/fondue" || f.LinkApp != "celebrate" {
		t.Errorf("fondue: %+v", f)
	}
	if w := got[1]; w.Mine != "" || w.Call != "Get tickets" || w.StartAt != "2026-10-03 15:30" {
		t.Errorf("wurst: %+v", w)
	}
}
