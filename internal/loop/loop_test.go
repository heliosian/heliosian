package loop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

var tabNames = []string{groupsTab, managersTab, rulesTab, additionsTab, excludedTab, aliasesTab, messagesTab, deliveriesTab, adminsTab, archivedTab}

func sampleTables(t *testing.T) store.Tables {
	t.Helper()
	tabs, err := (&data.Dir{Root: "../../sampledata"}).Tabs(appName, tabNames, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := store.Tables{}
	for _, name := range tabNames {
		out[name] = tabs[name].Rows
	}
	return out
}

func sampleCache(t *testing.T) (*Cache, *data.Dir) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	cache, err := NewCache(dir, dir, func(string) bool { return false }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return cache, dir
}

func logRows(t *testing.T, dir *data.Dir, tab string) int {
	t.Helper()
	_, rows, err := dir.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range rows {
		if row["Tab"] == tab {
			n++
		}
	}
	return n
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
	if g.Posting != PostingManagers || model.Group("soccer-team").Posting != PostingEveryone {
		t.Fatal("a group's Posting cell is not read, or a blank one is not everyone")
	}
	if g.Replying != PostingMembers || model.Group("soccer-team").Replying != PostingEveryone {
		t.Fatal("a group's Replying cell is not read, or a blank one is not everyone")
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
	tables[additionsTab] = append(tables[additionsTab], store.Row{"Group": "nobody", "Email": "a@x.org"})
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
	tables[groupsTab][0][visibleColumn] = "on"
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a Visible cell reading on")
	}
}

func TestLoadRefusesAPostingItDoesNotKnow(t *testing.T) {
	tables := sampleTables(t)
	tables[groupsTab][0][postingColumn] = "staff"
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a Posting cell reading staff")
	}
	tables = sampleTables(t)
	tables[groupsTab][0][replyingColumn] = "staff"
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a Replying cell reading staff")
	}
}

func TestChecksRefuseBadGroups(t *testing.T) {
	good := Group{Name: "a-b", Title: "A", Visibility: VisibilityHidden, Posting: PostingEveryone, Replying: PostingEveryone, Managers: []string{"m@x.org"}, Rules: []Rule{{Kind: KindInclude, Roles: []string{"Staff"}}}}
	if err := CheckGroup(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(g *Group){
		"name":          func(g *Group) { g.Name = "Bad Name" },
		"two dots":      func(g *Group) { g.Name = "a..b" },
		"reserved":      func(g *Group) { g.Name = "postmaster" },
		"unsubscribe":   func(g *Group) { g.Name = "unsubscribe" },
		"unsub alias":   func(g *Group) { g.Aliases = []string{"unsubscribe"} },
		"visibility":    func(g *Group) { g.Visibility = "on" },
		"posting":       func(g *Group) { g.Posting = "staff" },
		"replying":      func(g *Group) { g.Replying = "staff" },
		"title":         func(g *Group) { g.Title = "" },
		"managers":      func(g *Group) { g.Managers = nil },
		"no include":    func(g *Group) { g.Rules[0].Kind = KindExclude },
		"empty rule":    func(g *Group) { g.Rules[0].Roles = nil },
		"bad role":      func(g *Group) { g.Rules[0].Roles = []string{"Alumni"} },
		"bad relation":  func(g *Group) { g.Rules[0].Family = []string{"Cousins"} },
		"comma tag":     func(g *Group) { g.Rules[0].Tags = []string{"m@x.org:a, b"} },
		"bare tag":      func(g *Group) { g.Rules[0].Tags = []string{"Carpool"} },
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

func TestSavingAGroupWritesOnlyWhatChanged(t *testing.T) {
	cache, dir := sampleCache(t)
	ctx := context.Background()
	additions := cache.Count(additionsTab, nil)
	g := Normalize(Group{Name: "chess.club", Title: " Chess Club ", Visibility: " Members ", Managers: []string{"M@X.org", "m@x.org"}, Rules: []Rule{{Kind: "Include", Search: "  Kim ", Tags: []string{" M@X.org:Chess "}}},
		Additions: []Addition{{Email: " Coach@Club.org ", Name: "  The  Coach "}, {Email: "coach@club.org", Name: "Again"}}})
	if err := cache.Commit(ctx, "m@x.org", groupOps(Group{}, g, true)...); err != nil {
		t.Fatal(err)
	}
	model := cache.Model()
	got := model.Group("chess.club")
	if len(model.Groups) != 4 || got.Title != "Chess Club" || got.Visibility != VisibilityMembers || len(got.Managers) != 1 || got.Rules[0].Kind != KindInclude || got.Rules[0].Search != "kim" || got.Rules[0].Tags[0] != "m@x.org:Chess" {
		t.Fatalf("%+v", got)
	}
	if len(got.Additions) != 1 || got.Additions[0] != (Addition{Email: "coach@club.org", Name: "The Coach"}) {
		t.Fatalf("additions: %+v", got.Additions)
	}
	next := *got
	next.Rules = append(slices.Clone(got.Rules), Rule{Kind: KindExclude, Roles: []string{"Staff"}})
	next.Additions = nil
	next.Visibility = ""
	if err := cache.Commit(ctx, "m@x.org", groupOps(*got, next, false)...); err != nil {
		t.Fatal(err)
	}
	model = cache.Model()
	if len(model.Group("chess.club").Rules) != 2 || len(model.Groups) != 4 || len(model.Group("chess.club").Additions) != 0 || model.Group("chess.club").Visibility != VisibilityHidden {
		t.Fatalf("second save: %+v", model.Group("chess.club"))
	}
	if cache.Count(additionsTab, nil) != additions || cache.Count(managersTab, store.Row{"Group": "chess.club"}) != 1 {
		t.Fatal("the additions or managers rows were not kept to the group")
	}
	if logRows(t, dir, managersTab) != 1 {
		t.Fatalf("an unchanged manager was written again: %d log rows", logRows(t, dir, managersTab))
	}
	if err := cache.Commit(ctx, "m@x.org", store.Delete(groupsTab, store.Row{"Name": "chess.club"})); err != nil {
		t.Fatal(err)
	}
	if model = cache.Model(); model.Group("chess.club") != nil || len(model.Groups) != 3 {
		t.Fatal("the group was not removed")
	}
	if cache.Count(managersTab, store.Row{"Group": "chess.club"}) != 0 || cache.Count(rulesTab, store.Row{"Group": "chess.club"}) != 0 {
		t.Fatal("the group's rows outlived it")
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
		tables[aliasesTab] = append(tables[aliasesTab], bad)
		if _, err := BuildModel(tables); err == nil {
			t.Errorf("accepted alias %v", bad)
		}
	}
	tables := sampleTables(t)
	tables[groupsTab] = append(tables[groupsTab], store.Row{"Name": "hummingbirds-families", "Title": "Clash"})
	tables[managersTab] = append(tables[managersTab], store.Row{"Group": "hummingbirds-families", "Email": "m@x.org"})
	tables[rulesTab] = append(tables[rulesTab], store.Row{"Group": "hummingbirds-families", "Kind": "include", "Roles": "Staff"})
	if _, err := BuildModel(tables); err == nil {
		t.Error("accepted a group named for another group's alias")
	}
}

func TestSuggestedIsEveryPartyAndActivityNoRuleNames(t *testing.T) {
	groups := []Group{{Rules: []Rule{{Tags: []string{"party:p1"}}, {Tags: []string{"gardeners", "activity:e2"}}}}}
	lists := []who.List{
		{Key: "party:p1", Kind: who.ListParty},
		{Key: "party:p2", Kind: who.ListParty},
		{Key: "activity:e1", Kind: who.ListActivity},
		{Key: "activity:e2", Kind: who.ListActivity},
		{Key: "room:K", Kind: who.ListRoom},
		{Key: "group:tech", Kind: who.ListGroup},
	}
	keys := []string{}
	for _, l := range Suggested(lists, groups) {
		keys = append(keys, l.Key)
	}
	if !slices.Equal(keys, []string{"party:p2", "activity:e1"}) {
		t.Fatalf("suggested %v", keys)
	}
}

func TestLoadRefusesAGroupWithoutManagers(t *testing.T) {
	tables := sampleTables(t)
	tables[groupsTab] = append(tables[groupsTab], store.Row{"Name": "lonely", "Title": "Lonely"})
	if _, err := BuildModel(tables); err == nil {
		t.Fatal("accepted a group with no managers")
	}
}

func TestArchivedIsOnePersonsAndFollowsTheGroup(t *testing.T) {
	cache, _ := sampleCache(t)
	ctx := context.Background()
	const jordan = "jordan.whitfield@heliosschool.org"
	match := store.Row{"Group": "soccer-team", "Email": jordan}
	for range 2 {
		if err := cache.Commit(ctx, jordan, store.Set(archivedTab, match, store.Row{})); err != nil {
			t.Fatal(err)
		}
	}
	model := cache.Model()
	if !model.Archived("soccer-team", jordan) || cache.Count(archivedTab, nil) != 1 {
		t.Fatalf("archiving twice left %d rows", cache.Count(archivedTab, nil))
	}
	if model.Archived("soccer-team", "abena.osei@heliosschool.org") || model.Archived("hummingbird-families", jordan) {
		t.Fatal("an archive reached another person or another group")
	}
	if err := cache.Commit(ctx, jordan, store.Delete(archivedTab, match)); err != nil {
		t.Fatal(err)
	}
	if cache.Count(archivedTab, nil) != 0 || cache.Model().Archived("soccer-team", jordan) {
		t.Fatal("unarchiving left the row")
	}
	if err := cache.Commit(ctx, jordan, store.Set(archivedTab, match, store.Row{})); err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(ctx, jordan, store.Delete(groupsTab, store.Row{"Name": "soccer-team"})); err != nil {
		t.Fatal(err)
	}
	if cache.Count(archivedTab, nil) != 0 {
		t.Fatal("deleting the group kept its archived row")
	}
	stray := sampleTables(t)
	stray[archivedTab] = append(stray[archivedTab], store.Row{"Group": "nowhere", "Email": jordan})
	if _, err := BuildModel(stray); err == nil {
		t.Fatal("accepted an archived row naming no group")
	}
}

func TestSuggestedTagsAreTheOwnersOwnNoRuleOfTheirsNames(t *testing.T) {
	groups := []Group{{Rules: []Rule{{Tags: []string{"jordan@x:Carpool"}}, {Tags: []string{"someone.else@x:Soccer Team"}}}}}
	tags := map[string][]string{"Carpool": {"a@x"}, "Soccer Team": {"b@x"}, "Book Club": {"c@x"}, "Empty": {}}
	got := SuggestedTags(tags, groups, "jordan@x")
	if !slices.Equal(got, []string{"Book Club", "Soccer Team"}) {
		t.Fatalf("suggested %v", got)
	}
}
