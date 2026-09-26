package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/loop"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

func TestGroupListsCarryAdditionsAsGuests(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	directory, err := who.LoadModel(dir, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	groups, err := loop.NewCache(dir, dir, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	model := groups.Model()
	sources := loop.Sources{
		Directory: directory,
		Tags:      directory.Tags,
		Lists:     directory.RoomParentLists,
		Shared:    directory.SharedTags,
	}
	jordan := "jordan.whitfield@heliosschool.org"
	lists := GroupLists(model, sources, jordan)
	i := slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "group:soccer-team" })
	if i < 0 {
		t.Fatalf("no soccer team list: %+v", lists)
	}
	list := lists[i]
	coach := "coach.rivera@coastsidesoccer.example.org"
	if slices.Contains(list.People, coach) || slices.Contains(list.People, jordan) {
		t.Fatalf("people: %v", list.People)
	}
	want := []who.Guest{
		{ID: "soccer-team:" + coach, Name: "Coach Rivera", Email: coach},
		{ID: "soccer-team:office@coastsidesoccer.example.org", Name: "Coastside League Office", Email: "office@coastsidesoccer.example.org"},
	}
	if !slices.Equal(list.Guests, want) {
		t.Fatalf("guests: %+v", list.Guests)
	}
	for _, p := range list.People {
		if directory.Person(p) == nil {
			t.Errorf("%s is listed as a person but is not in the directory", p)
		}
	}
	if list.Archived {
		t.Fatal("a group nobody archived came marked archived")
	}
	if err := groups.Commit(context.Background(), jordan, store.Set("Archived", store.Row{"Group": "soccer-team", "Email": jordan}, store.Row{})); err != nil {
		t.Fatal(err)
	}
	lists = GroupLists(groups.Model(), sources, jordan)
	i = slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "group:soccer-team" })
	if i < 0 || !lists[i].Archived {
		t.Fatalf("the archived group's list is missing or unmarked: %+v", lists)
	}
}

type anyImage struct{}

func (anyImage) Has(string) (bool, error) { return true, nil }

func (anyImage) Prefetch(context.Context, []string) error { return nil }

func TestMagicTagsCarryTheirHosts(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	directory, err := who.LoadModel(dir, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	queue := store.NewQueue()
	portalCache, err := team.NewCache(dir, dir, anyImage{}, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	portal := portalCache.Model()
	siteCache, err := celebrate.NewCache(dir, dir, anyImage{}, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	site := siteCache.Model()
	jordan := "jordan.whitfield@heliosschool.org"
	lists := SmartLists(directory, portal, site, jordan, time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	i := slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "activity:E001" })
	if i < 0 {
		t.Fatalf("no International Night list: %+v", lists)
	}
	if !slices.Equal(lists[i].Hosts, []string{jordan, "mina.park@heliosschool.org"}) {
		t.Fatalf("hosts: %v", lists[i].Hosts)
	}
	for _, host := range lists[i].Hosts {
		if !slices.Contains(lists[i].People, host) {
			t.Fatalf("International Night's list leaves off its co-chair %s: %v", host, lists[i].People)
		}
	}
	i = slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "party:P001" })
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
