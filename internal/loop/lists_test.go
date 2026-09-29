package loop

import (
	"context"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestGroupListsCarryAdditionsAsGuests(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	directory, err := model.LoadDirectory(dir, nil, testkit.None, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	groups, err := NewCache(dir, dir, func() []string { return nil }, store.NewQueue(), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	sources := Sources{
		Directory: directory,
		Tags:      directory.Tags,
		MagicTags: directory.RoomParentTags,
		Shared:    directory.SharedTags,
	}
	jordan := "jordan.whitfield@heliosschool.org"
	lists := groups.Model().Lists(sources, jordan)
	i := slices.IndexFunc(lists, func(l model.MagicTag) bool { return l.Key == "group:"+soccerID })
	if i < 0 {
		t.Fatalf("no soccer team list: %+v", lists)
	}
	list := lists[i]
	if list.Slug != "soccer-team" {
		t.Fatalf("the list's slug is %q, not the group's name", list.Slug)
	}
	coach := "coach.rivera@coastsidesoccer.example.org"
	if slices.Contains(list.People, coach) || slices.Contains(list.People, jordan) {
		t.Fatalf("people: %v", list.People)
	}
	want := []model.Guest{
		{ID: soccerID + ":" + coach, Name: "Coach Rivera", Email: coach},
		{ID: soccerID + ":office@coastsidesoccer.example.org", Name: "Coastside League Office", Email: "office@coastsidesoccer.example.org"},
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
	if err := groups.Commit(context.Background(), access.Actor{Email: jordan}, store.Upsert("Archived", store.Row{"Group": soccerID, "Email": jordan}, store.Row{})); err != nil {
		t.Fatal(err)
	}
	lists = groups.Model().Lists(sources, jordan)
	i = slices.IndexFunc(lists, func(l model.MagicTag) bool { return l.Key == "group:"+soccerID })
	if i < 0 || !lists[i].Archived {
		t.Fatalf("the archived group's list is missing or unmarked: %+v", lists)
	}
}

func TestTaggedFindsTheGroupThatCarriesATag(t *testing.T) {
	m := &Model{Groups: []Group{{Name: "soccer", Rules: []model.Rule{{Kind: model.RuleInclude, Tags: []string{"room:jays"}}}}}}
	if got := m.Tagged("activity:act0000000023"); got != "" {
		t.Errorf("decor has a list already: %q", got)
	}
	m.Groups = append(m.Groups, Group{Name: "decor", Rules: []model.Rule{{Kind: model.RuleInclude, Tags: []string{"activity:act0000000023"}}}})
	if got := m.Tagged("activity:act0000000023"); got != "decor" {
		t.Errorf("decor's list: %q", got)
	}
}
