package db

import (
	"testing"

	"heliosian/internal/store"
)

func TestHrefPointsAtEachAppsPage(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct{ table, id, want string }{
		{"PERSON", "per00000000002", "https://who.test/p/rowan.ashdown"},
		{"PERSON", "per00000000004", "https://who.test/p/per00000000004"},
		{"GROUP", "grp00000000020", "https://who.test/g/grp00000000020"},
		{"GROUP", "grp00000000010", "https://who.test/g/hummingbirds"},
		{"GROUP", "grp00000000011", "https://who.test/g/grade-3"},
		{"GROUP", "grp00000000040", "https://when.test/g/fall-picnic"},
		{"GROUP", "grp00000000512", "https://team.test/g/international-night"},
		{"GROUP", "grp00000000510", "https://team.test/g/grp00000000510"},
		{"GROUP", "grp00000000521", "https://celebrate.test/g/taco-night"},
		{"GROUP", "grp00000000041", "https://loop.test/g/grp00000000041"},
		{"GROUP", "grp00000000060", ""},
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

func TestEachAppsHrefIsBlankWhereTheRowIsNotItsThing(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct {
		id   string
		want map[string]string
	}{
		{"grp00000000020", map[string]string{"path": "/g/grp00000000020", "who_href": "https://who.test/g/grp00000000020"}},
		{"grp00000000040", map[string]string{"path": "/g/fall-picnic", "loop_href": "https://loop.test/g/fall-picnic", "when_href": "https://when.test/g/fall-picnic"}},
		{"grp00000000512", map[string]string{"path": "/g/international-night", "loop_href": "https://loop.test/g/international-night", "when_href": "https://when.test/g/international-night", "team_href": "https://team.test/g/international-night"}},
		{"grp00000000521", map[string]string{"path": "/g/taco-night", "loop_href": "https://loop.test/g/taco-night", "when_href": "https://when.test/g/taco-night", "celebrate_href": "https://celebrate.test/g/taco-night"}},
		{"grp00000000060", map[string]string{"path": "/g/community"}},
		{"grp00000000041", map[string]string{"path": "/g/grp00000000041", "loop_href": "https://loop.test/g/grp00000000041"}},
	} {
		row, ok := m.Table("GROUP").Get(c.id)
		if !ok {
			t.Fatalf("no GROUP %s", c.id)
		}
		for _, column := range []string{"path", "who_href", "loop_href", "when_href", "team_href", "celebrate_href"} {
			if row[column] != c.want[column] {
				t.Errorf("%s %s is %q, want %q", c.id, column, row[column], c.want[column])
			}
		}
	}
}

func TestHrefIsAColumnLikeAnyOther(t *testing.T) {
	m := sample(t).Model()
	for src, want := range map[string]string{
		`(from GROUP (where (= id "grp00000000040")) (columns name href))`: "https://when.test/g/fall-picnic",
		`(from GROUP (where (= id "grp00000000040")) (columns name))`:      "",
		`(from GROUP (where (= href "https://when.test/g/fall-picnic")))`:  "https://when.test/g/fall-picnic",
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
