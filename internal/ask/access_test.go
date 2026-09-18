package ask

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"heliosian/internal/artifacts"
	"heliosian/internal/loop"
)

func TestGroupMailReadsForMembersOfGroupsTheySee(t *testing.T) {
	sources := sampleSources(t)
	docs := &artifacts.Model{Documents: []*artifacts.Document{
		{Key: "humming", Title: "Snack rota", Kind: artifacts.KindGroup, Channel: "hummingbird-families", Date: "2026-09-10", Markdown: "Snacks."},
		{Key: "list", Title: "Picnic", Kind: artifacts.KindList, Channel: "parents", Date: "2026-09-10", Markdown: "Picnic."},
		{Key: "middle", Title: "Dance", Kind: artifacts.KindGroup, Channel: "middle-school-parents", Date: "2026-09-10", Markdown: "Dance."},
		{Key: "soccer", Title: "Saturday", Kind: artifacts.KindGroup, Channel: "soccer-team", Date: "2026-09-10", Markdown: "Game."},
	}}
	sources.Artifacts = func() *artifacts.Model { return docs }
	groups := sources.Loop()
	members := func(name string) []string { return loop.Members(*groups.Group(name), sources.LoopSources()) }
	pick := func(from []string, not ...[]string) string {
		for _, email := range from {
			if email == jordan || email == "dana.hawkins@heliosschool.org" || email == "ruth.amari@heliosschool.org" {
				continue
			}
			if !slices.ContainsFunc(not, func(other []string) bool { return slices.Contains(other, email) }) {
				return email
			}
		}
		t.Fatal("no one to pick")
		return ""
	}
	middle := pick(members("middle-school-parents"), members("soccer-team"))
	humming := pick(members("hummingbird-families"), members("middle-school-parents"), members("soccer-team"))
	for email, want := range map[string][]string{
		jordan:  {"list", "middle", "soccer"},
		middle:  {"list", "middle"},
		humming: {"list"},
	} {
		v := app{sources: sources}.viewer(email)
		got := []string{}
		for _, d := range v.documents().Documents {
			got = append(got, d.Key)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s reads %v, not %v", email, got, want)
		}
		for _, key := range []string{"humming", "middle", "soccer"} {
			_, err := v.run(context.Background(), "read_document", json.RawMessage(`{"key":"`+key+`"}`))
			if (err == nil) != slices.Contains(want, key) {
				t.Errorf("%s reading %s: %v", email, key, err)
			}
		}
	}
}
