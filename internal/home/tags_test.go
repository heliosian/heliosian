package home

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/model"
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
	directory, err := model.NewDirectoryCache(dir, dir, nil, testkit.None, queue, []byte("test"), func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	sources := func() model.AudienceSources {
		d := directory.Model()
		return model.AudienceSources{Directory: d}
	}
	c, err := NewCache(dir, dir, testkit.All, func() []string { return nil }, sources, queue)
	if err != nil {
		t.Fatal(err)
	}
	key := model.TagKey(soccerTeamID)
	rules := []model.Rule{{Kind: model.RuleInclude, Tags: []string{key}}}
	if err := c.Commit(context.Background(), access.System("test"), audience(thingLink+jaysChatID, c.Model().link(jaysChatID).Rules, rules)...); err != nil {
		t.Fatal(err)
	}
	a := app{cache: c, sources: c.sources}
	if !c.includes(c.Model().link(jaysChatID).Rules, asha) || a.tagLabels(c.Model().Categories, nil, admin)[key] != "Soccer Team" {
		t.Fatalf("the audience naming the tag leaves out %s or reads %q", asha, a.tagLabels(c.Model().Categories, nil, admin)[key])
	}
	mux := http.NewServeMux()
	model.RegisterDirectory(mux, model.DirectoryRoutes{Cache: directory})
	if rec := testkit.Form(t, mux, admin, "/api/directory/tag-rename", url.Values{"tag": {soccerTeamID}, "name": {"Football"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if !c.includes(c.Model().link(jaysChatID).Rules, asha) {
		t.Fatalf("the renamed tag's audience leaves out %s", asha)
	}
	if got := a.tagLabels(c.Model().Categories, nil, admin)[key]; got != "Football" {
		t.Fatalf("the audience reads %q after the rename", got)
	}
}
