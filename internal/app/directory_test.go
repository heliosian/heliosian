package app

import (
	"slices"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestATeamAdminMakesAnEmailListFromAnyActivity(t *testing.T) {
	const admin, parent = "jordan.whitfield@heliosschool.org", "robin.whitfield@heliosschool.org"
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	images := testkit.Images(func(string) bool { return true })
	s, err := model.NewStore(dir, dir, queue, model.Deps{IDKey: []byte("sample"), Static: testkit.None, Activities: images, Parties: images, Home: images})
	if err != nil {
		t.Fatal(err)
	}
	m := s.Model()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	sources := m.Audience(now)
	shared := m.MagicTagsOf
	var theirs model.MagicTag
	for _, l := range sources.MagicTags(admin) {
		if l.Kind == model.MagicTagActivity && l.Parent == "" && !slices.Contains(l.Hosts, admin) && len(l.People) > len(l.Hosts) {
			theirs = l
			break
		}
	}
	if theirs.Key == "" {
		t.Fatal("the admin reads no activity they do not co-chair")
	}
	if !slices.ContainsFunc(shared(admin, now), func(l model.MagicTag) bool { return l.Key == theirs.Key }) {
		t.Fatalf("outside Loop the admin does not hold %s, as a Team admin holds every activity's", theirs.Key)
	}
	if slices.ContainsFunc(sources.MagicTags(parent), func(l model.MagicTag) bool { return l.Key == theirs.Key }) {
		t.Fatalf("a parent who is no admin reads %s", theirs.Key)
	}
	rules := []model.Rule{{Kind: model.RuleInclude, Tags: []string{theirs.Key}}}
	managers := append([]string{admin}, theirs.Hosts...)
	if err := sources.Writable(admin, managers, nil, rules); err != nil {
		t.Fatalf("the admin cannot save %s: %v", theirs.Key, err)
	}
	if err := sources.Writable(parent, []string{parent}, nil, rules); err == nil {
		t.Fatalf("a parent saved %s", theirs.Key)
	}
	members := model.EmailList{Managers: theirs.Hosts, Rules: rules}.Reasons(sources)
	for _, p := range theirs.People {
		if _, ok := members[p]; !ok {
			t.Errorf("%s is off the list its co-chairs manage", p)
		}
	}
}
