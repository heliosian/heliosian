package app

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type sampleImages struct{}

func (sampleImages) Has(key string) (bool, error) {
	return strings.HasPrefix(key, "sample/") || strings.HasPrefix(key, "brand/"), nil
}

func (sampleImages) Prefetch(context.Context, []string) error { return nil }

func samplesLinked(t *testing.T) calendarLinked {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	queue := store.NewQueue()
	parties, err := celebrate.NewCache(dir, dir, sampleImages{}, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	activities, err := team.NewCache(dir, dir, sampleImages{}, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := who.LoadModel(dir, nil, StaticFiles{Root: "web/who"}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return calendarLinked{parties, activities, func() *who.Model { return directory }}
}

func linkedFromSamples(t *testing.T) []when.Linked {
	t.Helper()
	return samplesLinked(t).list("nobody@x.org")
}

func TestCalendarLinkedMine(t *testing.T) {
	linked := samplesLinked(t)
	byID := map[string]when.Linked{}
	for _, l := range linked.list("jordan.whitfield@heliosschool.org") {
		byID[l.ID] = l
	}
	want := map[string]string{
		"P001": when.MineGoing, "P002": when.MineGoing, "P003": when.MineGoing, "P005": when.MineGoing, "P012": when.MineGoing,
		"P006": when.MineWaitlisted, "P007": when.MineWaitlisted, "P004": "",
		"E001": when.MineGoing, "E002": "",
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
	parties := []when.Linked{}
	for _, l := range linkedFromSamples(t) {
		if l.Source == when.SourceCelebrate {
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
	byID := map[string]when.Linked{}
	for _, l := range linkedFromSamples(t) {
		if l.Source == when.SourceTeam {
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
