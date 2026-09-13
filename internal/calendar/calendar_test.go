package calendar

import (
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/data"
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

func tables(t *testing.T) *Tables {
	t.Helper()
	tb, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatal(err)
	}
	return tb
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
	if m.Hidden != 1 || len(m.Events) != 22 {
		t.Errorf("visible %d hidden %d", len(m.Events), m.Hidden)
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
	tb.Overrides = append(tb.Overrides, map[string]string{"Event ID": "a7@sample", "Keywords": "Community, booths, hummingbirds, food"})
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
	tb.Events = append(tb.Events, map[string]string{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"})
	tb.Events = append(tb.Events, map[string]string{"Event ID": "X2", "Start": "2026-09-08", "Title": "Closed", "Tags": "Jays, Schedule", "Day Type": "No School"})
	tb.Events = append(tb.Events, map[string]string{"Event ID": "X3", "Start": "2026-09-08", "Title": "Still Short", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"})
	m, err := BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Plan("2026-09-08", "Jays"); got.Name != "No School" {
		t.Errorf("plan = %s, want No School", got.Name)
	}
}

func TestRefusals(t *testing.T) {
	broken := map[string]func(*Tables){
		"conflicting day types in one layer": func(tb *Tables) {
			tb.Events = append(tb.Events, map[string]string{"Event ID": "X1", "Start": "2026-09-08", "Title": "Short Day", "Tags": "Jays, Schedule", "Day Type": "Early Dismissal"})
			tb.Events = append(tb.Events, map[string]string{"Event ID": "X2", "Start": "2026-09-08", "Title": "No Care", "Tags": "Jays, Schedule", "Day Type": "No Aftercare"})
		},
		"timed event with a day type": func(tb *Tables) {
			tb.Overrides = append(tb.Overrides, map[string]string{"Event ID": "a7@sample", "Day Type": "Early Dismissal"})
		},
		"no tags": func(tb *Tables) {
			tb.Overrides = append(tb.Overrides, map[string]string{"Event ID": "a7@sample", "Tags": Clear})
		},
		"no regular day type": func(tb *Tables) {
			tb.DayTypes = tb.DayTypes[1:]
		},
		"no tags tab rows": func(tb *Tables) {
			tb.Tags = nil
		},
		"unknown tag": func(tb *Tables) {
			tb.Overrides = append(tb.Overrides, map[string]string{"Event ID": "a7@sample", "Tags": "Penguins"})
		},
		"unknown day type": func(tb *Tables) {
			tb.DayOverrides = append(tb.DayOverrides, map[string]string{"Date": "2026-09-08", "Day Type": "Snow Day"})
		},
		"unknown classroom in a day override": func(tb *Tables) {
			tb.DayOverrides = append(tb.DayOverrides, map[string]string{"Date": "2026-09-08", "Classrooms": "Penguins", "Day Type": "No School"})
		},
		"end before start": func(tb *Tables) {
			tb.Overrides = append(tb.Overrides, map[string]string{"Event ID": "a7@sample", "End": "2026-09-24 15:00"})
		},
		"duplicate id": func(tb *Tables) {
			tb.Events = append(tb.Events, map[string]string{"Event ID": "a7@sample", "Start": "2026-09-24", "Title": "Again"})
		},
		"pdf year mismatch": func(tb *Tables) {
			tb.PDF[0]["Year"] = "2025-2026"
		},
		"two first days": func(tb *Tables) {
			tb.PDF = append(tb.PDF, map[string]string{"Key": "pdf/x", "Year": "2026-2027", "Start": "2026-08-19", "Title": "Another first day", "Marker": MarkerFirstDay})
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

func TestSchoolYear(t *testing.T) {
	cases := map[string]string{"2026-07-15": "2026-2027", "2026-06-30": "2025-2026", "2027-01-05": "2026-2027"}
	for date, want := range cases {
		d, _ := time.ParseInLocation(DateFormat, date, Location)
		if got := SchoolYear(d); got != want {
			t.Errorf("%s: %s, want %s", date, got, want)
		}
	}
}
