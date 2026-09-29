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
	outsideSuperAdmin(t, dir)
	c := sampleStore(t, dir, queue, sampleDeps(sampleKey))
	key := TagKey(soccerTeamID)
	rules := []Rule{{Kind: RuleInclude, Tags: []string{key}}}
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, audienceOps(thingLink+jaysChatID, c.Model().Home.link(jaysChatID).Rules, rules)...); err != nil {
		t.Fatal(err)
	}
	tagLabels := func() map[string]string {
		m := c.Model()
		return homeTagLabels(m, m.Home.Categories, nil, homeAdmin)
	}
	includes := func() bool {
		m := c.Model()
		return m.homeIncludes(m.Home.link(jaysChatID).Rules, asha)
	}
	if !includes() || tagLabels()[key] != "Soccer Team" {
		t.Fatalf("the audience naming the tag leaves out %s or reads %q", asha, tagLabels()[key])
	}
	mux := http.NewServeMux()
	RegisterDirectory(mux, DirectoryRoutes{Store: c})
	if rec := testkit.Form(t, mux, homeAdmin, "/api/directory/tag-rename", url.Values{"tag": {soccerTeamID}, "name": {"Football"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if !includes() {
		t.Fatalf("the renamed tag's audience leaves out %s", asha)
	}
	if got := tagLabels()[key]; got != "Football" {
		t.Fatalf("the audience reads %q after the rename", got)
	}
}
