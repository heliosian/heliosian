package team

import (
	"slices"
	"testing"
	"time"

	"heliosian/internal/when"
	"heliosian/internal/who"
)

func linkedFor(cache *Cache, email string) map[string]when.Linked {
	byID := map[string]when.Linked{}
	for _, l := range cache.Model().Linked(when.FamilyOf(directory, email)) {
		byID[l.ID] = l
	}
	return byID
}

func TestLinkedActivities(t *testing.T) {
	cache, _ := newServer(t)
	byID := linkedFor(cache, "nobody@x.org")
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
	for _, l := range byID {
		if l.Mine != "" {
			t.Errorf("stranger stands with %s: %q", l.ID, l.Mine)
		}
	}
}

func TestLinkedActivitiesMine(t *testing.T) {
	cache, _ := newServer(t)
	byID := linkedFor(cache, admin)
	for id, mine := range map[string]string{"E001": when.MineGoing, "E002": ""} {
		if l, ok := byID[id]; !ok || l.Mine != mine {
			t.Errorf("%s: listed %v, mine %q, want %q", id, ok, l.Mine, mine)
		}
	}
	for _, l := range linkedFor(cache, student) {
		for _, p := range l.People {
			if !p.Mine {
				t.Errorf("a student sees %s's standing with %s", p.Name, l.ID)
			}
		}
	}
}

func TestActivityListsCarryTheirHosts(t *testing.T) {
	cache, _ := newServer(t)
	lists := cache.Model().Lists(directory, admin, time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	i := slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "activity:E001" })
	if i < 0 {
		t.Fatalf("no International Night list: %+v", lists)
	}
	if !slices.Equal(lists[i].Hosts, []string{admin, chair}) {
		t.Fatalf("hosts: %v", lists[i].Hosts)
	}
	for _, host := range lists[i].Hosts {
		if !slices.Contains(lists[i].People, host) {
			t.Fatalf("International Night's list leaves off its co-chair %s: %v", host, lists[i].People)
		}
	}
}

func TestCommitteesAreListsToo(t *testing.T) {
	cache, _ := newServer(t)
	mina := directory.Resolve(chair)
	lists := cache.Model().Lists(directory, mina, time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	names := map[string]string{}
	for _, l := range lists {
		names[l.Key] = l.Name
	}
	if names["activity:E002"] != "Spring Celebration" || names["activity:E023"] != "Spring Celebration: Decor" {
		t.Errorf("mina's lists: %v", names)
	}
	for _, l := range lists {
		if l.Key == "activity:E023" && l.Parent != "activity:E002" {
			t.Errorf("decor sits under %q", l.Parent)
		}
		if l.Key == "activity:E002" && l.Parent != "" {
			t.Errorf("the event sits under %q", l.Parent)
		}
	}
}
