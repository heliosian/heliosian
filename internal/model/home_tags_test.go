package model

import (
	"context"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestRenamingATagKeepsTheAudiencesThatNameIt(t *testing.T) {
	const (
		soccerTeamID = "dtg0000000002"
		asha         = "asha.chandra@heliosschool.org"
	)
	c, _, mux := homeServer(t)
	key := TagKey(soccerTeamID)
	rules := []Rule{{Kind: RuleInclude, Tags: []string{key}}}
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, audienceOps(thingLink+jaysChatID, c.Model().Home.link(jaysChatID).Rules, rules)...); err != nil {
		t.Fatal(err)
	}
	tagLabel := func() any {
		link := read(t, mux, homeAdmin, "/api/links/"+jaysChatID).one(t, "links", jaysChatID)
		got, _ := link["rules"].([]any)
		if len(got) != 1 {
			t.Fatalf("the link's rules = %v", link["rules"])
		}
		labels, _ := got[0].(map[string]any)["tagLabels"].([]any)
		if len(labels) != 1 {
			t.Fatalf("the rule's tag labels = %v", got[0])
		}
		return labels[0]
	}
	includes := func() bool {
		m := c.Model()
		return m.homeIncludes(m.Home.link(jaysChatID).Rules, asha)
	}
	if !includes() || tagLabel() != "Soccer Team" {
		t.Fatalf("the audience naming the tag leaves out %s or reads %q", asha, tagLabel())
	}
	if err := c.Commit(context.Background(), access.System("test"), DirectoryApp, store.Update(tagListTable, store.Row{tagID: soccerTeamID}, store.Row{tagName: "Football"})); err != nil {
		t.Fatal(err)
	}
	if !includes() {
		t.Fatalf("the renamed tag's audience leaves out %s", asha)
	}
	if got := tagLabel(); got != "Football" {
		t.Fatalf("the audience reads %q after the rename", got)
	}
}
