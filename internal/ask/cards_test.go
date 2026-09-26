package ask

import (
	"strings"
	"testing"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

type coloredDirectory struct {
	calendar.Directory
	colors map[string]string
}

func (d coloredDirectory) ClassroomColors() map[string]string { return d.colors }

func TestClassroomChipsWearTheirColor(t *testing.T) {
	sources := sampleSources(t)
	sources.CalendarDirectory = coloredDirectory{sources.CalendarDirectory, map[string]string{"Jays": "#1f6fb2"}}
	v := app{sources: sources}.viewer(jordan)
	jays, _ := v.linkCard(whoBase + who.ClassroomPath("Jays"))
	if jays.Color != "#1f6fb2" || jays.Image == "" {
		t.Errorf("Jays: %+v", jays)
	}
	if hawks, _ := v.linkCard(whoBase + who.ClassroomPath("Hawks")); hawks.Color != "" {
		t.Errorf("Hawks has no color set, but its chip has %q", hawks.Color)
	}
}

func TestLinkExamplesShowWhatTheChipsShow(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 18, 9, 0, 0, 0, calendar.Location)
	out := linkExamples(v)
	t.Log(out)
	kinds := map[string]bool{}
	for _, address := range v.exampleLinks() {
		card, ok := v.linkCard(address)
		if !ok {
			t.Errorf("an example with no chip: %s", address)
			continue
		}
		kinds[card.Kind] = true
		if !strings.Contains(out, "["+card.Name+"]("+address+")") {
			t.Errorf("the examples lack %s", address)
		}
	}
	for _, kind := range []string{"person", "family", "classroom", "event", "activity", "party", "group"} {
		if !kinds[kind] {
			t.Errorf("no %s among the examples", kind)
		}
	}
	if !strings.Contains(out, `a badge reading "`) {
		t.Errorf("the event example does not show its day:\n%s", out)
	}
}

func TestLinkCardsKeepEachAppsVisibility(t *testing.T) {
	for _, email := range []string{jordan, "ruth.amari@heliosschool.org"} {
		v := sampleViewer(t, email)
		if card, ok := v.linkCard(whoLink("sam.whitfield@heliosschool.org")); !ok || card.Kind != "person" || card.Name != "Sam Whitfield" {
			t.Errorf("%s: Sam's card %+v %v", email, card, ok)
		}
		if _, ok := v.linkCard(whoLink("nobody.here@heliosschool.org")); ok {
			t.Errorf("%s: a card for someone the directory does not list", email)
		}
		for key, family := range v.directory.Families {
			if card, ok := v.linkCard(whoBase + who.FamilyPath(key)); !ok || card.Kind != "family" || card.Name != family.Name || card.Image != family.PhotoURL {
				t.Errorf("%s: family %s card %+v %v", email, key, card, ok)
			}
		}
		if _, ok := v.linkCard(whoBase + who.FamilyPath("no-such-family")); ok {
			t.Errorf("%s: a card for a family the directory does not list", email)
		}
		for _, c := range v.directory.Classrooms {
			if card, ok := v.linkCard(whoBase + who.ClassroomPath(c.Name)); !ok || card.Kind != "classroom" || card.Name != c.Name {
				t.Errorf("%s: classroom %s card %+v %v", email, c.Name, card, ok)
			}
		}
		seen := map[string]bool{}
		for _, e := range v.calendar.EventsFor(v.whenAs, v.sources.CalendarDirectory, v.sources.Linked(email)) {
			seen[e.ID] = true
			if !strings.HasPrefix(eventLink(e), whenBase) {
				continue
			}
			if card, ok := v.linkCard(eventLink(e)); !ok || card.Kind != "event" || card.Name != e.Title || card.Badge == "" {
				t.Errorf("%s: event %q card %+v %v", email, e.Title, card, ok)
			}
		}
		for _, e := range append(append([]*calendar.Event{}, v.calendar.Events...), v.calendar.Pending...) {
			if _, ok := v.linkCard(eventLink(e)); !seen[e.ID] && strings.HasPrefix(eventLink(e), whenBase) && ok {
				t.Errorf("%s: a card for %q, which the calendar does not show them", email, e.Title)
			}
		}
		for _, g := range v.loop.Groups {
			_, ok := v.linkCard(loopBase + g.Path())
			if ok != g.VisibleTo(v.loopAs, v.sources.LoopSources()) {
				t.Errorf("%s: group %s card %v", email, g.Name, ok)
			}
		}
		hidden, shown := 0, 0
		for _, p := range v.celebrate.Parties {
			_, ok := v.linkCard(celebrateBase + v.celebrate.PathOf(p))
			if ok != p.VisibleTo(v.partyAs) {
				t.Errorf("%s: party %q (%s) card %v", email, p.Title, p.Status, ok)
			}
			if ok {
				shown++
			} else {
				hidden++
			}
		}
		for _, root := range v.team.Activities {
			for _, a := range append([]*team.Activity{root}, root.Descendants()...) {
				_, ok := v.linkCard(teamBase + v.team.PathOf(a))
				if ok != v.team.VisibleTo(a, v.teamAs) {
					t.Errorf("%s: activity %q (%s) card %v", email, a.Title, a.Status, ok)
				}
			}
		}
		if shown == 0 {
			t.Errorf("%s: no party has a card", email)
		}
		t.Logf("%s: %d parties with cards, %d without", email, shown, hidden)
	}
}
