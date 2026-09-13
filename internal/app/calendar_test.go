package app

import (
	"strings"
	"testing"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/events"
)

type sampleImages struct{}

func (sampleImages) Has(key string) (bool, error) {
	return strings.HasPrefix(key, "sample/") || strings.HasPrefix(key, "brand/"), nil
}

func (sampleImages) Prefetch([]string) error { return nil }

type directQueue struct{}

func (directQueue) Add(f func()) { f() }

func linkedFromSamples(t *testing.T) []calendar.Linked {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	parties, err := celebrate.NewCache(dir, sampleImages{}, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	activities, err := events.NewCache(dir, sampleImages{}, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return calendarLinked{parties, activities}.list()
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
