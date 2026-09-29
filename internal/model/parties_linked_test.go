package model

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func partiesLinkedFor(cache *PartiesCache, email string) []Linked {
	return cache.Model().Linked(partiesSampleDirectory.HouseholdOf(email), partiesTestNow())
}

func TestLinkedParties(t *testing.T) {
	cache, _ := partiesServer(t)
	parties := partiesLinkedFor(cache, "nobody@x.org")
	if len(parties) != 14 {
		t.Fatalf("parties = %d", len(parties))
	}
	for i := 1; i < len(parties); i++ {
		if parties[i].Start < parties[i-1].Start {
			t.Errorf("parties out of order: %s before %s", parties[i-1].Start, parties[i].Start)
		}
	}
	for _, p := range parties {
		if p.ID == "pty0000000013" || p.ID == "pty0000000014" {
			t.Errorf("party %s is not open", p.ID)
		}
		if p.Start == "" || p.Title == "" || p.Path == "" || p.Availability == "" {
			t.Errorf("party incomplete: %+v", p)
		}
		if p.ID == "pty0000000001" && (p.Path != "/p/fondue" || p.Location != "The Parks' House in Los Altos" || p.Start != "2026-09-19 17:00" || p.End != "2026-09-19 21:00") {
			t.Errorf("fondue = %+v", p)
		}
		if strings.Contains(p.Description, "Alder Court") || strings.Contains(p.Location, "Alder Court") {
			t.Errorf("street address leaked: %+v", p)
		}
		if p.Mine != "" {
			t.Errorf("stranger stands with %s: %q", p.ID, p.Mine)
		}
	}
}

func TestLinkedPartiesMine(t *testing.T) {
	cache, _ := partiesServer(t)
	byID := map[string]Linked{}
	others := 0
	for _, l := range partiesLinkedFor(cache, jordan) {
		byID[l.ID] = l
		for _, p := range l.People {
			if !p.Mine {
				others++
			}
		}
	}
	want := map[string]string{
		"pty0000000001": MineGoing, "pty0000000002": MineGoing, "pty0000000003": MineGoing, "pty0000000005": MineGoing, "pty0000000012": MineGoing,
		"pty0000000006": MineWaitlisted, "pty0000000007": MineWaitlisted, "pty0000000004": "",
	}
	for id, mine := range want {
		if l, ok := byID[id]; !ok || l.Mine != mine {
			t.Errorf("%s: listed %v, mine %q, want %q", id, ok, l.Mine, mine)
		}
	}
	if others == 0 {
		t.Fatal("the parent sees no one else's standing, so the student check proves nothing")
	}
	for _, l := range partiesLinkedFor(cache, partiesKid) {
		for _, p := range l.People {
			if !p.Mine {
				t.Errorf("a student sees %s's standing with %s", p.Name, l.ID)
			}
		}
	}
}

func TestPartyListsCarryTheirHosts(t *testing.T) {
	cache, _ := partiesServer(t)
	lists := cache.Model().MagicTags(partiesSampleDirectory, jordan, time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	i := slices.IndexFunc(lists, func(l MagicTag) bool { return l.Key == "party:pty0000000001" })
	if i < 0 {
		t.Fatalf("no Fondue & Fort Night list: %+v", lists)
	}
	if !slices.Contains(lists[i].Hosts, jordan) {
		t.Fatalf("hosts: %v", lists[i].Hosts)
	}
	if !slices.Contains(lists[i].People, jordan) {
		t.Fatalf("Fondue & Fort Night's list leaves off its host: %v", lists[i].People)
	}
}

func TestPastPartyListsStayArchived(t *testing.T) {
	cache, _ := partiesServer(t)
	party := func(now time.Time) MagicTag {
		lists := cache.Model().MagicTags(partiesSampleDirectory, jordan, now)
		i := slices.IndexFunc(lists, func(l MagicTag) bool { return l.Key == "party:pty0000000001" })
		if i < 0 {
			t.Fatalf("no Fondue & Fort Night list at %s: %+v", now, lists)
		}
		return lists[i]
	}
	before := party(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	if before.Archived {
		t.Fatalf("archived before it happens: %+v", before)
	}
	after := party(time.Date(2028, 9, 1, 12, 0, 0, 0, time.UTC))
	if !after.Archived || !slices.Equal(after.People, before.People) || len(after.Guests) != len(before.Guests) {
		t.Errorf("after: archived %v, people %v guests %d, want %v guests %d", after.Archived, after.People, len(after.Guests), before.People, len(before.Guests))
	}
}
