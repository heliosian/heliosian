package model

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestRenamingATagKeepsTheAudiencesThatNameIt(t *testing.T) {
	const (
		soccerTeamID = "dtg0000000002"
		asha         = "asha.chandra@heliosschool.org"
	)
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	none := func() []string { return nil }
	directory, err := NewDirectoryCache(dir, dir, nil, testkit.None, queue, []byte("test"), none)
	if err != nil {
		t.Fatal(err)
	}
	parties, err := NewPartiesCache(dir, dir, testkit.All, none, queue)
	if err != nil {
		t.Fatal(err)
	}
	activities, err := NewActivitiesCache(dir, dir, testkit.All, none, queue)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewHomeCache(dir, dir, testkit.All, none, directory, parties, activities, queue)
	if err != nil {
		t.Fatal(err)
	}
	key := TagKey(soccerTeamID)
	rules := []Rule{{Kind: RuleInclude, Tags: []string{key}}}
	if err := c.Commit(context.Background(), access.System("test"), audienceOps(thingLink+jaysChatID, c.Model().link(jaysChatID).Rules, rules)...); err != nil {
		t.Fatal(err)
	}
	a := homeApp{cache: c}
	if !c.includes(c.Model().link(jaysChatID).Rules, asha) || a.tagLabels(c.Model().Categories, nil, homeAdmin)[key] != "Soccer Team" {
		t.Fatalf("the audience naming the tag leaves out %s or reads %q", asha, a.tagLabels(c.Model().Categories, nil, homeAdmin)[key])
	}
	mux := http.NewServeMux()
	RegisterDirectory(mux, DirectoryRoutes{Cache: directory})
	if rec := testkit.Form(t, mux, homeAdmin, "/api/directory/tag-rename", url.Values{"tag": {soccerTeamID}, "name": {"Football"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if !c.includes(c.Model().link(jaysChatID).Rules, asha) {
		t.Fatalf("the renamed tag's audience leaves out %s", asha)
	}
	if got := a.tagLabels(c.Model().Categories, nil, homeAdmin)[key]; got != "Football" {
		t.Fatalf("the audience reads %q after the rename", got)
	}
}
