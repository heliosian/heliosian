package db

import (
	"testing"

	"heliosian/internal/store"
)

func TestLinkPointsAtEachAppsPage(t *testing.T) {
	origin := func(app string) string { return "https://" + app + ".example.org" }
	for _, c := range []struct {
		table string
		row   store.Row
		want  string
	}{
		{"PERSON", store.Row{"id": "per00000000002"}, "https://who.example.org/people/per00000000002"},
		{"GROUP", store.Row{"id": "grp00000000020", "kind": "family"}, "https://who.example.org/families/grp00000000020"},
		{"GROUP", store.Row{"id": "grp00000000010", "kind": "classroom", "slug": "hummingbirds"}, "https://who.example.org/classrooms/hummingbirds"},
		{"GROUP", store.Row{"id": "grp00000000011", "kind": "grade", "slug": "grade-k"}, "https://who.example.org/grades/grade-k"},
		{"GROUP", store.Row{"id": "grp00000000040", "kind": "event", "slug": "picnic"}, "https://when.example.org/e/picnic"},
		{"GROUP", store.Row{"id": "grp00000000041", "kind": "event"}, "https://when.example.org/e/grp00000000041"},
		{"GROUP", store.Row{"id": "grp00000000050", "kind": "activity", "slug": "book-fair"}, "https://team.example.org/v/book-fair"},
		{"GROUP", store.Row{"id": "grp00000000060", "kind": "party", "slug": "taco night"}, "https://celebrate.example.org/p/taco%20night"},
		{"GROUP", store.Row{"id": "grp00000000070", "kind": "group", "slug": "soccer", "mail": "Yes"}, "https://loop.example.org/groups/soccer"},
		{"GROUP", store.Row{"id": "grp00000000071", "kind": "group", "slug": "tag"}, ""},
		{"GROUP", store.Row{"id": "grp00000000072", "kind": "category"}, ""},
		{"DOCUMENT", store.Row{"id": "doc00000000102", "kind": "wiki", "slug": "getting-started"}, "https://wiki.example.org/p/getting-started"},
		{"DOCUMENT", store.Row{"id": "doc00000000200", "relation": "linked", "url": "https://docs.google.com/x"}, "https://docs.google.com/x"},
		{"DOCUMENT", store.Row{"id": "doc00000000001", "kind": "mail"}, ""},
		{"MEMBER", store.Row{"id": "mem00000000010"}, ""},
	} {
		if got := Link(c.table, c.row, origin); got != c.want {
			t.Errorf("%s %v links to %q, want %q", c.table, c.row, got, c.want)
		}
	}
}
