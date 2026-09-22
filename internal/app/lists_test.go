package app

import (
	"slices"
	"testing"
	"time"

	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

func TestGroupListsCarryAdditionsAsGuests(t *testing.T) {
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
	groupTables, err := loop.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	model, err := loop.BuildModel(groupTables)
	if err != nil {
		t.Fatal(err)
	}
	sources := loop.Sources{
		Directory: directory,
		Tags:      func(owner string) map[string][]string { return who.TagsOf(tables.Tags, directory, owner) },
		Lists:     directory.RoomParentLists,
		Shared: func(email string) []who.SharedTag {
			return who.SharedTagsOf(tables.Tags, tables.Managers, directory, email)
		},
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
	// A group archived by its manager keeps its list, marked so, for them
	// alone.
	groupTables.Archived = append(groupTables.Archived, map[string]string{"Group": "soccer-team", "Email": jordan})
	if model, err = loop.BuildModel(groupTables); err != nil {
		t.Fatal(err)
	}
	lists = GroupLists(model, sources, jordan)
	i = slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "group:soccer-team" })
	if i < 0 || !lists[i].Archived {
		t.Fatalf("the archived group's list is missing or unmarked: %+v", lists)
	}
}

type anyImage struct{}

func (anyImage) Has(string) (bool, error) { return true, nil }

func (anyImage) Prefetch([]string) error { return nil }

func TestMagicTagsCarryTheirHosts(t *testing.T) {
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
	portalTables, err := team.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	portal, err := team.BuildModel(portalTables, anyImage{})
	if err != nil {
		t.Fatal(err)
	}
	siteTables, err := celebrate.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	site, err := celebrate.BuildModel(siteTables, anyImage{})
	if err != nil {
		t.Fatal(err)
	}
	jordan := "jordan.whitfield@heliosschool.org"
	lists := SmartLists(directory, portal, site, jordan, time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC))
	i := slices.IndexFunc(lists, func(l who.List) bool { return l.Key == "activity:E001" })
	if i < 0 {
		t.Fatalf("no International Night list: %+v", lists)
	}
	if !slices.Equal(lists[i].Hosts, []string{jordan, "mina.park@heliosschool.org"}) {
		t.Fatalf("hosts: %v", lists[i].Hosts)
	}
	// The hosts are on the list itself, the viewer among them, so a group
	// made of it reaches whoever is running the event.
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
