package app

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/team"
)

type sampleImages struct{}

func (sampleImages) Has(key string) (bool, error) {
	return strings.HasPrefix(key, "sample/") || strings.HasPrefix(key, "brand/"), nil
}

func (sampleImages) Prefetch(context.Context, []string) error { return nil }

type sampleHousehold struct{}

func (sampleHousehold) Household(email string) (adults, kids []celebrate.Person) {
	switch email {
	case "jordan.whitfield@heliosschool.org":
		return []celebrate.Person{{Email: "robin.whitfield@heliosschool.org", Name: "Robin Whitfield"}}, []celebrate.Person{{Email: "sam.whitfield@heliosschool.org", Name: "Sam Whitfield"}, {Email: "ella.whitfield@heliosschool.org", Name: "Ella Whitfield"}}
	case "sam.whitfield@heliosschool.org":
		return []celebrate.Person{{Email: "jordan.whitfield@heliosschool.org", Name: "Jordan Whitfield"}, {Email: "robin.whitfield@heliosschool.org", Name: "Robin Whitfield"}}, []celebrate.Person{{Email: "ella.whitfield@heliosschool.org", Name: "Ella Whitfield"}}
	}
	return nil, nil
}

func (h sampleHousehold) Family(email string) map[string]bool {
	out := map[string]bool{}
	if email != "jordan.whitfield@heliosschool.org" {
		return out
	}
	adults, kids := h.Household(email)
	for _, p := range append(adults, kids...) {
		out[p.Email] = true
	}
	return out
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
	queue := store.NewQueue()
	parties, err := celebrate.NewCache(dir, dir, sampleImages{}, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	activities, err := team.NewCache(dir, dir, sampleImages{}, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	return calendarLinked{parties, activities, sampleHousehold{}}
}

func linkedFromSamples(t *testing.T) []calendar.Linked {
	t.Helper()
	return samplesLinked(t).list("nobody@x.org")
}

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
	others := 0
	for _, l := range linked.list("jordan.whitfield@heliosschool.org") {
		for _, p := range l.People {
			if !p.Mine {
				others++
			}
		}
	}
	if others == 0 {
		t.Fatal("the parent sees no one else's standing, so the student check proves nothing")
	}
	for _, l := range linked.list("sam.whitfield@heliosschool.org") {
		for _, p := range l.People {
			if !p.Mine {
				t.Errorf("a student sees %s's standing with %s", p.Name, l.ID)
			}
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
