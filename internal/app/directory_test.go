package app

import (
	"slices"
	"testing"
	"time"

	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/loop"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/testkit"
)

func TestATeamAdminMakesAnEmailListFromAnyActivity(t *testing.T) {
	const admin, parent = "jordan.whitfield@heliosschool.org", "robin.whitfield@heliosschool.org"
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	directory, err := model.LoadDirectory(dir, nil, testkit.None, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	none := func() []string { return nil }
	images := testkit.Images(func(string) bool { return true })
	activities, err := team.NewCache(dir, dir, images, func() []string { return []string{admin} }, queue)
	if err != nil {
		t.Fatal(err)
	}
	parties, err := celebrate.NewCache(dir, dir, images, none, queue)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	tags := loopMagicTags(directory, parties.Model(), activities.Model(), activities.IsAdmin)
	shared := magicTags(directory, parties.Model(), activities.Model())
	var theirs model.MagicTag
	for _, l := range tags(admin, now) {
		if l.Kind == model.MagicTagActivity && l.Parent == "" && !slices.Contains(l.Hosts, admin) && len(l.People) > len(l.Hosts) {
			theirs = l
			break
		}
	}
	if theirs.Key == "" {
		t.Fatal("the admin reads no activity they do not co-chair")
	}
	if slices.ContainsFunc(shared(admin, now), func(l model.MagicTag) bool { return l.Key == theirs.Key }) {
		t.Fatalf("outside Loop the admin reads %s, which they do not co-chair", theirs.Key)
	}
	if slices.ContainsFunc(tags(parent, now), func(l model.MagicTag) bool { return l.Key == theirs.Key }) {
		t.Fatalf("a parent who is no admin reads %s", theirs.Key)
	}
	sources := loop.Sources{Directory: directory, MagicTags: func(owner string) []model.MagicTag { return tags(owner, now) }}
	rules := []model.Rule{{Kind: model.RuleInclude, Tags: []string{theirs.Key}}}
	managers := append([]string{admin}, theirs.Hosts...)
	if err := sources.Writable(admin, managers, nil, rules); err != nil {
		t.Fatalf("the admin cannot save %s: %v", theirs.Key, err)
	}
	if err := sources.Writable(parent, []string{parent}, nil, rules); err == nil {
		t.Fatalf("a parent saved %s", theirs.Key)
	}
	members := loop.Reasons(loop.Group{Managers: theirs.Hosts, Rules: rules}, sources)
	for _, p := range theirs.People {
		if _, ok := members[p]; !ok {
			t.Errorf("%s is off the list its co-chairs manage", p)
		}
	}
}
