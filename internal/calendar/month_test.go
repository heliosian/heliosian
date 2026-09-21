package calendar

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// The rail's month on Heliosian: the school days with what kind of day each
// is for the viewer's classrooms, and the viewer's events that touch the
// month - past ones too, and one that runs in from the month before.
func TestMonth(t *testing.T) {
	m := load(t)
	sam := Person{Email: "sam@x.org", Name: "Sam", IsStudent: true, Grade: "Grade 3", Classroom: "Jays"}
	d := fakeDirectory{
		people: map[string]Person{"p@x.org": {Email: "p@x.org", Name: "Pat", IsParent: true}, "sam@x.org": sam},
		kids:   map[string][]Person{"p@x.org": {sam}},
	}
	at, _ := time.ParseInLocation(DateTimeFormat, "2026-09-13 08:00", Location)
	linked := []Linked{
		{Source: SourceCelebrate, ID: "P001", Title: "Fondue & Fort Night", Start: "2026-09-19 17:00", End: "2026-09-19 21:00", Path: "/p/fondue", Availability: "available"},
		{Source: SourceTeam, ID: "E005", Title: "Back to School Social", Start: "2026-08-27 15:00", End: "2026-08-27 17:00", Path: "/activities/E005", Availability: "done"},
	}
	got := m.Month(d, "p@x.org", linked, at, "2026-09")
	if got.Month != "2026-09" || got.Today != "2026-09-13" {
		t.Errorf("month %q today %q", got.Month, got.Today)
	}
	if _, weekend := got.Days["2026-09-13"]; weekend || got.Days["2026-09-14"].Kinds == nil || len(got.Days["2026-09-14"].Kinds) != 0 {
		t.Errorf("a Sunday is no school day and a regular Monday has no kinds: %v %v", got.Days["2026-09-13"], got.Days["2026-09-14"])
	}
	if k := got.Days["2026-09-07"].Kinds; len(k) != 1 || k[0].Name != NoSchoolDayType || k[0].Words != "No School" {
		t.Errorf("Labor Day = %+v", k)
	}
	if k := got.Days["2026-09-29"].Kinds; len(k) != 1 || k[0].Words != "Early Dismissal" {
		t.Errorf("conference day = %+v", k)
	}
	titles := []string{}
	for i, e := range got.Events {
		titles = append(titles, e.Title)
		if i > 0 && got.Events[i-1].Start > e.Start {
			t.Errorf("out of order: %q after %q", e.Title, got.Events[i-1].Title)
		}
		if e.Start > "2026-09-30" || strings.Compare(e.EndAt[:10], "2026-09-01") < 0 {
			t.Errorf("outside the month: %+v", e)
		}
		// The rail places an event by the days it sits on, so a card without
		// them lands on no day at all.
		if len(e.Dates) == 0 || e.Dates[0] != e.Start {
			t.Errorf("card carries no days to sit on: %+v", e)
		}
	}
	for _, want := range []string{"Labor Day - No School", "Jays and Ravens Camping", "Fondue & Fort Night", "Returning Grade ILP Conference, half days"} {
		if !slices.Contains(titles, want) {
			t.Errorf("month lacks %q: %v", want, titles)
		}
	}
	if slices.Contains(titles, "Hummingbird CAFE") || slices.Contains(titles, "MS Back to School Night") || slices.Contains(titles, "Back to School Social") {
		t.Errorf("month lists another classroom's, the middle school's, or last month's: %v", titles)
	}
	// The month before holds the social and the first days of school, whose
	// early dismissal is the kindergarten's alone.
	before := m.Month(d, "nobody@x.org", linked, at, "2026-08")
	if !slices.ContainsFunc(before.Events, func(u Card) bool { return u.Title == "Back to School Social" && u.LinkApp == "team" }) {
		t.Errorf("August lacks the social: %+v", before.Events)
	}
	if k := before.Days["2026-08-19"].Kinds; len(k) != 1 || k[0].Words != "Early Dismissal · Hummingbirds" {
		t.Errorf("kindergarten's short day for a stranger = %+v", k)
	}
	if k := before.Days["2026-08-19"]; len(m.Month(d, "p@x.org", nil, at, "2026-08").Days["2026-08-19"].Kinds) != 0 {
		t.Errorf("a Jays parent sees the kindergarten's short day: %+v", k)
	}
	// A month that does not parse is the month now is in; a summer month is
	// empty of school days.
	if fallback := m.Month(d, "p@x.org", nil, at, "next"); fallback.Month != "2026-09" {
		t.Errorf("fallback month = %q", fallback.Month)
	}
	if summer := m.Month(d, "p@x.org", nil, at, "2027-07"); len(summer.Days) != 0 || len(summer.Events) != 0 {
		t.Errorf("July = %d days %d events", len(summer.Days), len(summer.Events))
	}
}
