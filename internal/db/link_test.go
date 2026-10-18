package db

import (
	"testing"

	"heliosian/internal/store"
)

func TestHrefPointsAtEachAppsPage(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct{ table, id, want string }{
		{"PERSON", "per00000000002", "https://who.test/people/per00000000002"},
		{"GROUP", "grp00000000020", "https://who.test/families/grp00000000020"},
		{"GROUP", "grp00000000010", "https://who.test/classrooms/hummingbirds"},
		{"GROUP", "grp00000000011", "https://who.test/grades/grade-3"},
		{"GROUP", "grp00000000040", "https://when.test/e/fall-picnic"},
		{"GROUP", "grp00000000512", "https://team.test/v/international-night"},
		{"GROUP", "grp00000000510", "https://team.test/v/grp00000000510"},
		{"GROUP", "grp00000000521", "https://celebrate.test/p/taco-night"},
		{"GROUP", "grp00000000041", "https://who.test/groups/grp00000000041"},
		{"GROUP", "grp00000000060", "https://who.test/groups/community"},
		{"DOCUMENT", "doc00000000102", "https://wiki.test/p/getting-started"},
		{"DOCUMENT", "doc00000000104", "https://wiki.test/p/getting-started/Drop-off-and-Pickup"},
		{"DOCUMENT", "doc00000000107", "https://wiki.test/p/Activities/Clubs"},
		{"DOCUMENT", "doc00000000001", ""},
	} {
		row, ok := m.Table(c.table).Get(c.id)
		if !ok {
			t.Fatalf("no %s %s", c.table, c.id)
		}
		if row["href"] != c.want {
			t.Errorf("%s %s links to %q, want %q", c.table, c.id, row["href"], c.want)
		}
	}
	rows := []store.Row{
		{"id": "doc00000000200", "relation": "linked", "url": "https://docs.google.com/x"},
		{"id": "doc00000000201", "kind": "wiki", "name": "Clubs & Cafés", "parent": "doc00000000202"},
		{"id": "doc00000000202", "kind": "wiki", "name": "Activities"},
	}
	documentGenerated(rows, testOrigin)
	for i, want := range []string{"https://docs.google.com/x", "https://wiki.test/p/Activities/Clubs-Caf%C3%A9s", "https://wiki.test/p/Activities"} {
		if rows[i]["href"] != want {
			t.Errorf("%s links to %q, want %q", rows[i]["id"], rows[i]["href"], want)
		}
	}
}

func TestHrefIsAColumnLikeAnyOther(t *testing.T) {
	m := sample(t).Model()
	for src, want := range map[string]string{
		`(from GROUP (where (= id "grp00000000040")) (columns name href))`: "https://when.test/e/fall-picnic",
		`(from GROUP (where (= id "grp00000000040")) (columns name))`:      "",
		`(from GROUP (where (= href "https://when.test/e/fall-picnic")))`:  "https://when.test/e/fall-picnic",
	} {
		res := m.Run(t.Context(), mustParse(t, src), Env{Viewer: parent, Now: testNow})
		if res.Err != nil || len(res.IDs) != 1 || res.IDs[0] != "grp00000000040" {
			t.Fatalf("%s answered %v, %v", src, res.IDs, res.Err)
		}
		if got := res.Rows()[0]["href"]; got != want {
			t.Errorf("%s gave href %q, want %q", src, got, want)
		}
	}
}
