package ask

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"heliosian/internal/model"
)

func TestGroupMailReadsForMembersOfGroupsTheySee(t *testing.T) {
	sources := sampleSources(t)
	docs := &model.Documents{Documents: []*model.Document{
		{Key: "humming", Title: "Snack rota", Kind: model.DocumentKindGroup, Channel: "hummingbird-families", Date: "2026-09-10", Markdown: "Snacks."},
		{Key: "list", Title: "Picnic", Kind: model.DocumentKindList, Channel: "parents", Date: "2026-09-10", Markdown: "Picnic."},
		{Key: "middle", Title: "Dance", Kind: model.DocumentKindGroup, Channel: "middle-school-parents", Date: "2026-09-10", Markdown: "Dance."},
		{Key: "soccer", Title: "Saturday", Kind: model.DocumentKindGroup, Channel: "soccer-team", Date: "2026-09-10", Markdown: "Game."},
	}}
	sources.Documents = func() *model.Documents { return docs }
	groups := sources.Loop()
	members := func(name string) []string { return groups.Named(name).Members(sources.LoopSources()) }
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
