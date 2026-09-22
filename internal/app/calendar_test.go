package app

import (
	"strings"
	"testing"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

type sampleImages struct{}

func (sampleImages) Has(key string) (bool, error) {
	return strings.HasPrefix(key, "sample/") || strings.HasPrefix(key, "brand/"), nil
}

func (sampleImages) Prefetch([]string) error { return nil }

type directQueue struct{}

func (directQueue) Add(f func()) { f() }

// sampleHousehold is the Whitfields as the directory would list them for
// Jordan, and nobody for anyone else.
type sampleHousehold struct{}

func (sampleHousehold) Household(email string) (adults, kids []celebrate.Person) {
	if email != "jordan.whitfield@heliosschool.org" {
		return nil, nil
	}
	return []celebrate.Person{{Email: "robin.whitfield@heliosschool.org", Name: "Robin Whitfield"}}, []celebrate.Person{{Email: "sam.whitfield@heliosschool.org", Name: "Sam Whitfield"}, {Email: "ella.whitfield@heliosschool.org", Name: "Ella Whitfield"}}
}

func (sampleHousehold) Person(email string) (celebrate.Person, bool) {
	if email == "jordan.whitfield@heliosschool.org" {
		return celebrate.Person{Email: email, Name: "Jordan Whitfield"}, true
	}
	return celebrate.Person{}, false
}

func samplesLinked(t *testing.T) calendarLinked {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	parties, err := celebrate.NewCache(dir, sampleImages{}, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	activities, err := team.NewCache(dir, sampleImages{}, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return calendarLinked{parties, activities, sampleHousehold{}}
}

func linkedFromSamples(t *testing.T) []calendar.Linked {
	t.Helper()
	return samplesLinked(t).list("nobody@x.org")
}

// The viewer's standing: a ticket bought or held by anyone in the household
// is Going, a waitlist request alone is Waitlisted, a sign-up on an HCA event
// is Going, and a stranger stands nowhere.
func TestCalendarLinkedMine(t *testing.T) {
	linked := samplesLinked(t)
	byID := map[string]calendar.Linked{}
	for _, l := range linked.list("jordan.whitfield@heliosschool.org") {
		byID[l.ID] = l
	}
	want := map[string]string{
		"P001": calendar.MineGoing, "P002": calendar.MineGoing, "P003": calendar.MineGoing, "P005": calendar.MineGoing, "P012": calendar.MineGoing,
		"P006": calendar.MineWaitlisted, "P007": calendar.MineWaitlisted, "P004": "",
		"E001": calendar.MineGoing, "E002": "",
	}
	for id, mine := range want {
		l, ok := byID[id]
		if !ok || l.Mine != mine {
			t.Errorf("%s: listed %v, mine %q, want %q", id, ok, l.Mine, mine)
		}
	}
	for _, l := range linked.list("nobody@x.org") {
		if l.Mine != "" {
			t.Errorf("stranger stands with %s: %q", l.ID, l.Mine)
		}
	}
}

func TestCalendarLinkedParties(t *testing.T) {
	parties := []calendar.Linked{}
	for _, l := range linkedFromSamples(t) {
		if l.Source == calendar.SourceCelebrate {
			parties = append(parties, l)
		}
	}
	if len(parties) != 14 {
		t.Fatalf("parties = %d", len(parties))
	}
	for i := 1; i < len(parties); i++ {
		if parties[i].Start < parties[i-1].Start {
			t.Errorf("parties out of order: %s before %s", parties[i-1].Start, parties[i].Start)
		}
	}
	for _, p := range parties {
		if p.ID == "P013" || p.ID == "P014" {
			t.Errorf("party %s is not open", p.ID)
		}
		if p.Start == "" || p.Title == "" || p.Path == "" || p.Availability == "" {
			t.Errorf("party incomplete: %+v", p)
		}
		if p.ID == "P001" && (p.Path != "/p/fondue" || p.Location != "The Parks' House in Los Altos" || p.Start != "2026-09-19 17:00" || p.End != "2026-09-19 21:00") {
			t.Errorf("fondue = %+v", p)
		}
		if strings.Contains(p.Description, "Alder Court") || strings.Contains(p.Location, "Alder Court") {
			t.Errorf("street address leaked: %+v", p)
		}
	}
}

func TestCalendarLinkedActivities(t *testing.T) {
	byID := map[string]calendar.Linked{}
	for _, l := range linkedFromSamples(t) {
		if l.Source == calendar.SourceTeam {
			byID[l.ID] = l
		}
	}
	if len(byID) != 7 {
		t.Fatalf("activities = %d: %v", len(byID), byID)
	}
	for _, id := range []string{"E006", "E007", "E012", "E016"} {
		if _, ok := byID[id]; ok {
			t.Errorf("%s listed: hidden, undated, pending, or a thing under an event", id)
		}
	}
	night := byID["E001"]
	if night.Title != "International Night" || night.Path != "/v/international-night" || night.Start != "2026-09-24 16:00" || night.End != "2026-09-24 18:00" || night.Location != "Helios Blacktop" || night.Availability != "open" {
		t.Errorf("international night = %+v", night)
	}
	if byID["E005"].Availability != "done" || byID["E013"].Availability != "done" {
		t.Errorf("done events = %+v %+v", byID["E005"], byID["E013"])
	}
}

// The roster carries each person's household - the adults, then the kids
// of every family they are in, themselves left out - so an invitation
// sent to one of them is on the calendar of all of them.
func TestCalendarRosterHouseholds(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	tables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := who.BuildModel(tables, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	roster := CalendarRoster(directory)
	if got := strings.Join(roster.Households["jordan.whitfield@heliosschool.org"], ","); got != "robin.whitfield@heliosschool.org,sam.whitfield@heliosschool.org,ella.whitfield@heliosschool.org" {
		t.Errorf("jordan's household = %q", got)
	}
	if got := strings.Join(roster.Households["sam.whitfield@heliosschool.org"], ","); got != "jordan.whitfield@heliosschool.org,robin.whitfield@heliosschool.org,ella.whitfield@heliosschool.org" {
		t.Errorf("sam's household = %q", got)
	}
	if _, has := roster.Households["office@heliosschool.org"]; has {
		t.Errorf("someone with no family has a household")
	}
}
