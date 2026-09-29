package celebrate

import (
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/model"
)

func linkedFor(cache *Cache, email string) []model.Linked {
	return cache.Model().Linked(sampleDirectory.HouseholdOf(email), testNow())
}

func TestLinkedParties(t *testing.T) {
	cache, _ := newServer(t)
	parties := linkedFor(cache, "nobody@x.org")
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
	cache, _ := newServer(t)
	byID := map[string]model.Linked{}
	others := 0
	for _, l := range linkedFor(cache, parent) {
		byID[l.ID] = l
		for _, p := range l.People {
			if !p.Mine {
				others++
			}
		}
	}
	want := map[string]string{
		"pty0000000001": model.MineGoing, "pty0000000002": model.MineGoing, "pty0000000003": model.MineGoing, "pty0000000005": model.MineGoing, "pty0000000012": model.MineGoing,
		"pty0000000006": model.MineWaitlisted, "pty0000000007": model.MineWaitlisted, "pty0000000004": "",
	}
	for id, mine := range want {
		if l, ok := byID[id]; !ok || l.Mine != mine {
			t.Errorf("%s: listed %v, mine %q, want %q", id, ok, l.Mine, mine)
		}
	}
	if others == 0 {
		t.Fatal("the parent sees no one else's standing, so the student check proves nothing")
	}
	for _, l := range linkedFor(cache, kid) {
		for _, p := range l.People {
			if !p.Mine {
				t.Errorf("a student sees %s's standing with %s", p.Name, l.ID)
			}
		}
	}
}

func TestPartyListsCarryTheirHosts(t *testing.T) {
	cache, _ := newServer(t)
	lists := cache.Model().Lists(sampleDirectory, parent, time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	i := slices.IndexFunc(lists, func(l model.MagicTag) bool { return l.Key == "party:pty0000000001" })
	if i < 0 {
		t.Fatalf("no Fondue & Fort Night list: %+v", lists)
	}
	if !slices.Contains(lists[i].Hosts, parent) {
		t.Fatalf("hosts: %v", lists[i].Hosts)
	}
	if !slices.Contains(lists[i].People, parent) {
		t.Fatalf("Fondue & Fort Night's list leaves off its host: %v", lists[i].People)
	}
}
