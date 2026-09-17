package app

import (
	"slices"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/loop"
	"heliosian/internal/who"
)

func TestGroupListsCarryAdditionsAsGuests(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	tables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := who.BuildModel(tables, nil, staticFiles{})
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
}
