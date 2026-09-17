package loop

import (
	"fmt"
	"slices"
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
	if g == nil || len(g.Managers) != 2 || len(g.Rules) != 2 || len(g.Additions) != 0 || g.Visibility != VisibilityEveryone {
		t.Fatalf("middle-school-parents: %+v", g)
	}
	if model.Group("soccer-team").Visibility != VisibilityHidden || model.Group("hummingbird-families").Visibility != VisibilityHidden {
		t.Fatal("a group with a blank Visible cell is not hidden")
	}
	if g.Rules[1].Kind != KindExclude || g.Rules[1].Search != "haddad" {
		t.Fatalf("exclude rule: %+v", g.Rules[1])
	}
	soccer := model.Group("soccer-team")
	if len(soccer.Additions) != 2 || soccer.Addition("coach.rivera@coastsidesoccer.example.org").Name != "Coach Rivera" {
		t.Fatalf("soccer-team additions: %+v", soccer.Additions)
	}
}

func TestLoadRefusesAnAdditionOnNoGroup(t *testing.T) {
	tables := sampleTables(t)
	tables.Additions = append(tables.Additions, map[string]string{"Group": "nobody", "Email": "a@x.org"})
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted an addition naming no group")
	}
}

func TestNamesTakeDots(t *testing.T) {
	for _, name := range []string{"soccer.team", "grade.5-parents", "a.b"} {
		if err := CheckName(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"soccer..team", ".soccer", "soccer.", "a"} {
		if err := CheckName(name); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadRefusesAVisibilityItDoesNotKnow(t *testing.T) {
	tables := sampleTables(t)
	tables.Groups[0][visibleColumn] = "on"
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a Visible cell reading on")
	}
}

func TestChecksRefuseBadGroups(t *testing.T) {
	good := Group{Name: "a-b", Title: "A", Visibility: VisibilityHidden, Managers: []string{"m@x.org"}, Rules: []Rule{{Kind: KindInclude, Roles: []string{"Staff"}, Owner: "m@x.org"}}}
	if err := CheckGroup(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(g *Group){
		"name":          func(g *Group) { g.Name = "Bad Name" },
		"two dots":      func(g *Group) { g.Name = "a..b" },
		"reserved":      func(g *Group) { g.Name = "postmaster" },
		"visibility":    func(g *Group) { g.Visibility = "on" },
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
		"bad addition":  func(g *Group) { g.Additions = []Addition{{Email: "not an address", Name: "Nobody"}} },
		"long name":     func(g *Group) { g.Additions = []Addition{{Email: "a@x.org", Name: strings.Repeat("x", 81)}} },
		"too many added": func(g *Group) {
			for i := 0; i <= maxAdditions; i++ {
				g.Additions = append(g.Additions, Addition{Email: fmt.Sprintf("a%d@x.org", i)})
			}
		},
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
	g := Normalize(Group{Name: "chess.club", Title: " Chess Club ", Visibility: " Members ", Managers: []string{"M@X.org", "m@x.org"}, Rules: []Rule{{Kind: "Include", Search: "  Kim ", Owner: "M@X.org"}},
		Additions: []Addition{{Email: " Coach@Club.org ", Name: "  The  Coach "}, {Email: "coach@club.org", Name: "Again"}}})
	next := tables.withGroup(g)
	if len(tables.Groups) != 3 || len(next.Groups) != 4 {
		t.Fatalf("groups %d -> %d", len(tables.Groups), len(next.Groups))
	}
	model, err := BuildModel(next)
	if err != nil {
		t.Fatal(err)
	}
	got := model.Group("chess.club")
	if got.Title != "Chess Club" || got.Visibility != VisibilityMembers || len(got.Managers) != 1 || got.Rules[0].Kind != KindInclude || got.Rules[0].Search != "kim" || got.Rules[0].Owner != "m@x.org" {
		t.Fatalf("%+v", got)
	}
	if len(got.Additions) != 1 || got.Additions[0] != (Addition{Email: "coach@club.org", Name: "The Coach"}) {
		t.Fatalf("additions: %+v", got.Additions)
	}
	g.Rules = append(g.Rules, Rule{Kind: KindExclude, Roles: []string{"Staff"}, Owner: "m@x.org"})
	g.Additions = nil
	g.Visibility = ""
	again := next.withGroup(g)
	model, err = BuildModel(again)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Group("chess.club").Rules) != 2 || len(again.Groups) != 4 || len(model.Group("chess.club").Additions) != 0 || model.Group("chess.club").Visibility != VisibilityHidden {
		t.Fatalf("second save: %+v", model.Group("chess.club"))
	}
	if len(again.Additions) != len(tables.Additions) {
		t.Fatalf("the additions rows were not replaced: %d", len(again.Additions))
	}
	model, err = BuildModel(again.withoutGroup("chess.club"))
	if err != nil {
		t.Fatal(err)
	}
	if model.Group("chess.club") != nil || len(model.Groups) != 3 {
		t.Fatal("the group was not removed")
	}
	if len(next.withoutGroup("chess.club").Additions) != len(tables.Additions) {
		t.Fatal("the additions rows were not removed")
	}
}

func TestAliasesReachTheirGroupAndStayUnique(t *testing.T) {
	model, err := BuildModel(sampleTables(t))
	if err != nil {
		t.Fatal(err)
	}
	g := model.Group("hummingbird-families")
	if !slices.Equal(g.Aliases, []string{"hummingbirds-families"}) || !slices.Equal(g.Names(), []string{"hummingbird-families", "hummingbirds-families"}) {
		t.Fatalf("aliases %v", g.Aliases)
	}
	if model.Resolve("hummingbirds-families") != g || model.Resolve("hummingbird-families") != g || model.Resolve("nobody") != nil {
		t.Fatal("an alias does not resolve to its group")
	}
	for _, bad := range []map[string]string{
		{"Group": "soccer-team", "Alias": "hummingbirds-families"},
		{"Group": "soccer-team", "Alias": "middle-school-parents"},
		{"Group": "soccer-team", "Alias": "soccer-team"},
		{"Group": "soccer-team", "Alias": "postmaster"},
		{"Group": "soccer-team", "Alias": "Not An Address"},
	} {
		tables := sampleTables(t)
		tables.Aliases = append(tables.Aliases, bad)
		if _, err := BuildModel(tables); err == nil {
			t.Errorf("accepted alias %v", bad)
		}
	}
	tables := sampleTables(t)
	tables.Groups = append(tables.Groups, map[string]string{"Name": "hummingbirds-families", "Title": "Clash"})
	tables.Managers = append(tables.Managers, map[string]string{"Group": "hummingbirds-families", "Email": "m@x.org"})
	tables.Rules = append(tables.Rules, map[string]string{"Group": "hummingbirds-families", "Kind": "include", "Roles": "Staff", "Owner": "m@x.org"})
	if _, err := BuildModel(tables); err == nil {
		t.Error("accepted a group named for another group's alias")
	}
}

func TestLoadRefusesAGroupWithoutManagers(t *testing.T) {
	tables := sampleTables(t)
	tables.Groups = append(tables.Groups, map[string]string{"Name": "lonely", "Title": "Lonely"})
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a group with no managers")
	}
}
