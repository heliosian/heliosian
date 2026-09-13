package calendar

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	if m.Hidden != 1 || m.Duplicates != 2 || len(m.Events) != 20 {
		t.Errorf("visible %d hidden %d duplicates %d", len(m.Events), m.Hidden, m.Duplicates)
	}
	// The feed's Labor Day and the PDF's say the same thing; the feed's is
	// kept. The PDF's two kindergarten half days are inside the feed's four.
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

func TestDedupe(t *testing.T) {
	tb := tables(t)
	// A hand-added twin of a feed event wins over it; a differing span or
	// tag is not a twin.
	tb.Events = append(tb.Events,
		map[string]string{"Event ID": "D1", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Intl Night", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community"},
		map[string]string{"Event ID": "D2", "Start": "2026-09-24 16:00", "End": "2026-09-24 19:00", "Title": "Intl Night, longer", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community"},
		map[string]string{"Event ID": "D3", "Start": "2026-09-24 16:00", "End": "2026-09-24 18:00", "Title": "Intl Night, parents", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Community, Parents"},
	)
	m, err := BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Duplicates != 3 || m.Event("a7@sample") != nil || m.Event("D1") == nil || m.Event("D2") == nil || m.Event("D3") == nil {
		t.Errorf("duplicates %d, a7 %v, D1 %v, D2 %v, D3 %v", m.Duplicates, m.Event("a7@sample"), m.Event("D1"), m.Event("D2"), m.Event("D3"))
	}
	// Four one-day feed entries cover the PDF's four-day conference span;
	// two do not.
	everyone := "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule"
	tb = tables(t)
	for _, date := range []string{"2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"} {
		tb.Events = append(tb.Events, map[string]string{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": "Early Dismissal"})
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
		tb.Events = append(tb.Events, map[string]string{"Event ID": "C" + date, "Start": date, "Title": "Conferences", "Tags": everyone, "Day Type": "Early Dismissal"})
	}
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-09-29/returning-grade-ilp-conference-half-days") == nil {
		t.Errorf("a half-covered span folded")
	}
	// Two feed weeks of a title-only span cover the PDF's one written across
	// the weekend between them; the feed's pair is the living source.
	assessment := "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Assessment"
	tb = tables(t)
	tb.Google = append(tb.Google,
		map[string]string{"Key": "m1", "Start": "2026-08-24", "End": "2026-08-28", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
		map[string]string{"Key": "m2", "Start": "2026-08-31", "End": "2026-09-02", "Title": "MAP Assessment", "Updated": "2026-08-01 09:00", "Sequence": "0"},
	)
	tb.PDF = append(tb.PDF, map[string]string{"Key": "pdf/2026-2027/2026-08-24/map-assessment", "Year": "2026-2027", "Start": "2026-08-24", "End": "2026-09-02", "Title": "MAP Assessment", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons"})
	for _, id := range []string{"m1", "m2", "pdf/2026-2027/2026-08-24/map-assessment"} {
		tb.Enrichment = append(tb.Enrichment, map[string]string{"Event ID": id, "Tags": assessment})
	}
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("pdf/2026-2027/2026-08-24/map-assessment") != nil || m.Event("m1") == nil || m.Event("m2") == nil {
		t.Errorf("a span across a weekend stayed: pdf %v", m.Event("pdf/2026-2027/2026-08-24/map-assessment"))
	}
	// A weekend-only event has no claims: it folds against an exact twin,
	// same dates and tags, and stands beside one a day longer.
	tb = tables(t)
	tb.Events = append(tb.Events,
		map[string]string{"Event ID": "W1", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Family Camping", "Tags": "Jays, Ravens, Trip"},
		map[string]string{"Event ID": "W2", "Start": "2026-09-12", "End": "2026-09-13", "Title": "Camping weekend", "Tags": "Jays, Ravens, Trip"},
		map[string]string{"Event ID": "W3", "Start": "2026-09-12", "End": "2026-09-14", "Title": "Camping, longer", "Tags": "Jays, Ravens, Trip"},
	)
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if (m.Event("W1") == nil) == (m.Event("W2") == nil) || m.Event("W3") == nil {
		t.Errorf("weekend twins: W1 %v, W2 %v, W3 %v", m.Event("W1"), m.Event("W2"), m.Event("W3"))
	}
	// A PDF row carrying a year marker is kept over a feed twin without one.
	tb = tables(t)
	tb.Google = append(tb.Google, map[string]string{"Key": "g1", "Start": "2026-08-18", "End": "2026-08-18", "Title": "first  day of SCHOOL", "Updated": "2026-08-01 09:00", "Sequence": "0"})
	tb.Enrichment = append(tb.Enrichment, map[string]string{"Event ID": "g1", "Tags": "Hummingbirds, Hawks, Falcons, Jays, Ravens, Condors, Ospreys, Egrets, Herons, Schedule"})
	m, err = BuildModel(tb, roster)
	if err != nil {
		t.Fatal(err)
	}
	if m.Event("g1") != nil || m.Event("pdf/2026-2027/2026-08-18/first-day-of-school") == nil || len(m.Years) != 1 {
		t.Errorf("marker twin lost: g1 %v, years %d", m.Event("g1"), len(m.Years))
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
		"feed naming an unknown classroom": func(tb *Tables) {
			tb.Feeds = append(tb.Feeds, map[string]string{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Classrooms": "Penguins"})
		},
		"feed naming an unknown tag": func(tb *Tables) {
			tb.Feeds = append(tb.Feeds, map[string]string{"Token": "t2", "Email": "a@x.org", "Name": "Mine", "Tags": "Bake Sales"})
		},
		"feed with no name": func(tb *Tables) {
			tb.Feeds = append(tb.Feeds, map[string]string{"Token": "t2", "Email": "a@x.org"})
		},
		"feed with no token": func(tb *Tables) {
			tb.Feeds = append(tb.Feeds, map[string]string{"Email": "a@x.org", "Name": "Mine"})
		},
		"two feeds with one token": func(tb *Tables) {
			tb.Feeds = append(tb.Feeds, map[string]string{"Token": tb.Feeds[0]["Token"], "Email": "a@x.org", "Name": "Mine"})
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
	tables := tables(t)
	next := tables.WithFeed(map[string]string{"Token": "t2", "Email": "a@x.org", "Name": "Mine"})
	if len(next.Feeds) != 2 || len(tables.Feeds) != 1 {
		t.Errorf("with feed: %d then %d rows", len(tables.Feeds), len(next.Feeds))
	}
	if gone := next.WithoutFeed("t2"); len(gone.Feeds) != 1 || len(next.Feeds) != 2 {
		t.Errorf("without feed: %d rows", len(gone.Feeds))
	}
	if m2, err := BuildModel(next, roster); err != nil || len(m2.Feeds) != 2 {
		t.Errorf("model over the added feed: %v, %d feeds", err, len(m2.Feeds))
	}
}

func TestICS(t *testing.T) {
	m := load(t)
	f := m.Feed("sample7feedtoken4jordan2whitfield")
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-01 08:00", Location)
	out := string(ICS(m, f, "https://calendar.local.heliosian.com:8080", at))
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n", "X-WR-CALNAME:Whitfield school days\r\n", "END:VCALENDAR\r\n",
		"UID:a4@sample\r\n", "DTSTART;VALUE=DATE:20260907\r\n", "DTEND;VALUE=DATE:20260908\r\n",
		"SUMMARY:Jays and Ravens Camping\r\n", "DTSTART;VALUE=DATE:20260909\r\n", "DTEND;VALUE=DATE:20260912\r\n",
		"DTSTAMP:20260901T150000Z\r\n", "URL:https://calendar.local.heliosian.com:8080/events/a5@sample\r\n",
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
	people map[string]Person
	kids   map[string][]Person
}

func (d fakeDirectory) Resolve(email string) string { return email }

func (d fakeDirectory) Person(email string) (Person, bool) {
	p, ok := d.people[email]
	return p, ok
}

func (d fakeDirectory) Children(email string) []Person { return d.kids[email] }

func (d fakeDirectory) Alerts(string) (int, bool) { return 2, true }

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
	v := Render(m, d, "jordan.whitfield@heliosschool.org", false, at)
	if strings.Join(v.User.Classrooms, ",") != "Jays,Ospreys" || len(v.User.Students) != 2 || v.User.Initial != "J" {
		t.Errorf("parent = %+v", v.User)
	}
	if len(v.Feeds) != 1 || v.Today != "2026-09-08" || len(v.Events) != 20 || v.Alerts.Stale != 2 || !v.Alerts.Privacy {
		t.Errorf("view = feeds %d today %s events %d alerts %+v", len(v.Feeds), v.Today, len(v.Events), v.Alerts)
	}
	if v.Days["2026-09-08"]["Jays"] != "Regular" || len(v.Classrooms) != 9 || len(v.Tags) != 15 || v.Colors["Jays"] != "#fec502" {
		t.Errorf("plan, vocabulary, or colors missing")
	}
	cases := map[string]string{"sam@x.org": "Jays", "teacher@x.org": "Hawks", "office@x.org": "", "nobody@x.org": ""}
	for email, want := range cases {
		v := Render(m, d, email, false, at)
		if got := strings.Join(v.User.Classrooms, ","); got != want || len(v.Feeds) != 0 {
			t.Errorf("%s: classrooms %q, want %q; feeds %d", email, got, want, len(v.Feeds))
		}
	}
	if v := Render(m, d, "nobody@x.org", true, at); v.User.Name != "Nobody" || !v.User.IsAdmin {
		t.Errorf("stranger = %+v", v.User)
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
