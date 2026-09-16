package groups

import (
	"strings"
	"testing"

	"heliosian/internal/data"
)

func sampleTables(t *testing.T) *Tables {
	t.Helper()
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func TestSampleSheetLoads(t *testing.T) {
	model, err := BuildModel(sampleTables(t))
	if err != nil {
		t.Fatal(err)
	}
	g := model.Group("middle-school-parents")
	if g == nil || len(g.Managers) != 2 || len(g.Rules) != 2 {
		t.Fatalf("middle-school-parents: %+v", g)
	}
	if g.Rules[1].Kind != KindExclude || g.Rules[1].Search != "haddad" {
		t.Fatalf("exclude rule: %+v", g.Rules[1])
	}
}

func TestChecksRefuseBadGroups(t *testing.T) {
	good := Group{Name: "a-b", Title: "A", Managers: []string{"m@x.org"}, Rules: []Rule{{Kind: KindInclude, Roles: []string{"Staff"}, Owner: "m@x.org"}}}
	if err := CheckGroup(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(g *Group){
		"name":          func(g *Group) { g.Name = "Bad Name" },
		"reserved":      func(g *Group) { g.Name = "postmaster" },
		"title":         func(g *Group) { g.Title = "" },
		"managers":      func(g *Group) { g.Managers = nil },
		"no include":    func(g *Group) { g.Rules[0].Kind = KindExclude },
		"empty rule":    func(g *Group) { g.Rules[0].Roles = nil },
		"bad role":      func(g *Group) { g.Rules[0].Roles = []string{"Alumni"} },
		"bad relation":  func(g *Group) { g.Rules[0].Family = []string{"Cousins"} },
		"comma tag":     func(g *Group) { g.Rules[0].Tags = []string{"a, b"} },
		"no owner":      func(g *Group) { g.Rules[0].Owner = "" },
		"long title":    func(g *Group) { g.Title = strings.Repeat("x", 81) },
		"too many rule": func(g *Group) { g.Rules = append(g.Rules, make([]Rule, maxRules)...) },
	}
	for name, edit := range cases {
		g := good
		g.Managers = append([]string{}, good.Managers...)
		g.Rules = append([]Rule{}, good.Rules...)
		edit(&g)
		if err := CheckGroup(g); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestWithGroupRoundTrips(t *testing.T) {
	tables := sampleTables(t)
	g := Normalize(Group{Name: "chess-club", Title: " Chess Club ", Managers: []string{"M@X.org", "m@x.org"}, Rules: []Rule{{Kind: "Include", Search: "  Kim ", Owner: "M@X.org"}}})
	next := tables.withGroup(g)
	if len(tables.Groups) != 3 || len(next.Groups) != 4 {
		t.Fatalf("groups %d -> %d", len(tables.Groups), len(next.Groups))
	}
	model, err := BuildModel(next)
	if err != nil {
		t.Fatal(err)
	}
	got := model.Group("chess-club")
	if got.Title != "Chess Club" || len(got.Managers) != 1 || got.Rules[0].Kind != KindInclude || got.Rules[0].Search != "kim" || got.Rules[0].Owner != "m@x.org" {
		t.Fatalf("%+v", got)
	}
	g.Rules = append(g.Rules, Rule{Kind: KindExclude, Roles: []string{"Staff"}, Owner: "m@x.org"})
	again := next.withGroup(g)
	model, err = BuildModel(again)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Group("chess-club").Rules) != 2 || len(again.Groups) != 4 {
		t.Fatalf("second save: %+v", model.Group("chess-club"))
	}
	model, err = BuildModel(again.withoutGroup("chess-club"))
	if err != nil {
		t.Fatal(err)
	}
	if model.Group("chess-club") != nil || len(model.Groups) != 3 {
		t.Fatal("the group was not removed")
	}
}

func TestLoadRefusesAGroupWithoutManagers(t *testing.T) {
	tables := sampleTables(t)
	tables.Groups = append(tables.Groups, map[string]string{"Name": "lonely", "Title": "Lonely"})
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a group with no managers")
	}
}
