package loop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

var tabNames = []string{groupsTab, managersTab, rulesTab, additionsTab, excludedTab, id.AliasesTab, messagesTab, deliveriesTab, admins.Tab, archivedTab}

const (
	soccerID  = "grp0000000001"
	middleID  = "grp0000000002"
	hummingID = "grp0000000003"
)

func sampleTables(t *testing.T) store.Tables {
	t.Helper()
	tabs, err := (&data.Dir{Root: "../../sampledata"}).Tabs(context.Background(), appName, tabNames, nil)
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
	cache, err := NewCache(dir, dir, func() []string { return nil }, store.NewQueue(), []byte("test"))
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
	model, err := BuildModel(sampleTables(t), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	g := model.Group(middleID)
	if g == nil || g.Name != "middle-school-parents" || model.Named("middle-school-parents") != g || len(g.Managers) != 2 || len(g.Rules) != 2 || len(g.Additions) != 0 || g.Visibility != VisibilityEveryone {
		t.Fatalf("middle-school-parents: %+v", g)
	}
	if model.Group(soccerID).Visibility != VisibilityHidden || model.Group(hummingID).Visibility != VisibilityHidden {
		t.Fatal("a group with a blank Visible cell is not hidden")
	}
	if g.Posting != PostingManagers || model.Group(soccerID).Posting != PostingEveryone {
		t.Fatal("a group's Posting cell is not read, or a blank one is not everyone")
	}
	if g.Replying != PostingMembers || model.Group(soccerID).Replying != PostingEveryone {
		t.Fatal("a group's Replying cell is not read, or a blank one is not everyone")
	}
	if g.Rules[1].Kind != KindExclude || g.Rules[1].Search != "haddad" {
		t.Fatalf("exclude rule: %+v", g.Rules[1])
	}
	soccer := model.Group(soccerID)
	if len(soccer.Additions) != 2 || soccer.Addition("coach.rivera@coastsidesoccer.example.org").Name != "Coach Rivera" {
		t.Fatalf("soccer-team additions: %+v", soccer.Additions)
	}
	if model.Group("soccer-team") != nil || model.Named(soccerID) != nil || model.Group(strings.ToUpper(soccerID)) != soccer {
		t.Fatal("a group's name and ID are not kept apart, or an ID in capitals is not the ID")
	}
}

func TestLoadRefusesAnAdditionOnNoGroup(t *testing.T) {
	for _, group := range []string{"nobody", "soccer-team"} {
		tables := sampleTables(t)
		tables[additionsTab] = append(tables[additionsTab], store.Row{"Group": group, "Email": "a@x.org"})
		if _, err := BuildModel(tables, []byte("test")); err == nil {
			t.Fatalf("accepted an addition naming %s", group)
		}
	}
}

func TestLoadRefusesAGroupWithoutAGoodID(t *testing.T) {
	for _, bad := range []string{"", "soccer-team", "grp000000000i", middleID} {
		tables := sampleTables(t)
		tables[groupsTab][0][idColumn] = bad
		if _, err := BuildModel(tables, []byte("test")); err == nil || !strings.Contains(err.Error(), idColumn) {
			t.Errorf("Group ID %q: %v", bad, err)
		}
	}
}

func TestNamesTakeDots(t *testing.T) {
	for _, name := range []string{"soccer.team", "grade.5-parents", "a.b"} {
		if err := CheckName(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"soccer..team", ".soccer", "soccer.", "a", "grp0000000009"} {
		if err := CheckName(name); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadRefusesAVisibilityItDoesNotKnow(t *testing.T) {
	tables := sampleTables(t)
	tables[groupsTab][0][visibleColumn] = "on"
	if _, err := BuildModel(tables, []byte("test")); err == nil {
		t.Fatal("accepted a Visible cell reading on")
	}
}

func TestLoadRefusesAPostingItDoesNotKnow(t *testing.T) {
	tables := sampleTables(t)
	tables[groupsTab][0][postingColumn] = "staff"
	if _, err := BuildModel(tables, []byte("test")); err == nil {
		t.Fatal("accepted a Posting cell reading staff")
	}
	tables = sampleTables(t)
	tables[groupsTab][0][replyingColumn] = "staff"
	if _, err := BuildModel(tables, []byte("test")); err == nil {
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
		"tag by name":   func(g *Group) { g.Rules[0].Tags = []string{"tag:Carpool"} },
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
	chess := id.New(func(string) bool { return false })
	g := Normalize(Group{ID: chess, Name: "chess.club", Aliases: []string{"Chess"}, Title: " Chess Club ", Visibility: " Members ", Managers: []string{"M@X.org", "m@x.org"}, Rules: []Rule{{Kind: "Include", Search: "  Kim ", Tags: []string{" tag:DTG0000000009 "}}},
		Additions: []Addition{{Email: " Coach@Club.org ", Name: "  The  Coach "}, {Email: "coach@club.org", Name: "Again"}}})
	manager := access.Actor{Email: "m@x.org"}
	if err := cache.CommitAndWait(ctx, manager, groupOps(Group{}, g, true)...); err != nil {
		t.Fatal(err)
	}
	model := cache.Model()
	got := model.Group(chess)
	if len(model.Groups) != 4 || got.Name != "chess.club" || got.Title != "Chess Club" || got.Visibility != VisibilityMembers || len(got.Managers) != 1 || got.Rules[0].Kind != KindInclude || got.Rules[0].Search != "kim" || got.Rules[0].Tags[0] != "tag:dtg0000000009" {
		t.Fatalf("%+v", got)
	}
	if len(got.Additions) != 1 || got.Additions[0] != (Addition{Email: "coach@club.org", Name: "The Coach"}) {
		t.Fatalf("additions: %+v", got.Additions)
	}
	if model.Resolve("chess") != got || cache.Count(id.AliasesTab, store.Row{id.AliasColumn: "chess", id.IDColumn: chess}) != 1 {
		t.Fatal("the alias was not written against the group's ID")
	}
	for _, tab := range []string{managersTab, rulesTab, additionsTab} {
		if cache.Count(tab, store.Row{"Group": chess}) != 1 || cache.Count(tab, store.Row{"Group": "chess.club"}) != 0 {
			t.Fatalf("%s does not name the group by its ID", tab)
		}
	}
	next := *got
	next.Rules = append(slices.Clone(got.Rules), Rule{Kind: KindExclude, Roles: []string{"Staff"}})
	next.Additions = nil
	next.Visibility = ""
	if err := cache.CommitAndWait(ctx, manager, groupOps(*got, next, false)...); err != nil {
		t.Fatal(err)
	}
	model = cache.Model()
	if len(model.Group(chess).Rules) != 2 || len(model.Groups) != 4 || len(model.Group(chess).Additions) != 0 || model.Group(chess).Visibility != VisibilityHidden {
		t.Fatalf("second save: %+v", model.Group(chess))
	}
	if cache.Count(additionsTab, nil) != additions || cache.Count(managersTab, store.Row{"Group": chess}) != 1 {
		t.Fatal("the additions or managers rows were not kept to the group")
	}
	if logRows(t, dir, managersTab) != 1 {
		t.Fatalf("an unchanged manager was written again: %d log rows", logRows(t, dir, managersTab))
	}
	if err := cache.Commit(ctx, manager, store.Delete(groupsTab, store.Row{idColumn: chess})); err != nil {
		t.Fatal(err)
	}
	if model = cache.Model(); model.Group(chess) != nil || model.Resolve("chess") != nil || len(model.Groups) != 3 {
		t.Fatal("the group was not removed")
	}
	if cache.Count(managersTab, store.Row{"Group": chess}) != 0 || cache.Count(rulesTab, store.Row{"Group": chess}) != 0 || cache.Count(id.AliasesTab, store.Row{id.IDColumn: chess}) != 0 {
		t.Fatal("the group's rows outlived it")
	}
}

func TestSavingANewGroupMintsItsID(t *testing.T) {
	cache, _ := sampleCache(t)
	model := cache.Model()
	actor := access.Actor{Email: "m@x.org"}
	sources := Sources{Directory: &who.Model{}}
	taken := func(key string) bool { return model.Group(key) != nil }
	ops, g, action, err := model.SaveGroup(actor, sources, Group{Name: "chess.club", Title: "Chess Club", Rules: []Rule{{Kind: KindInclude, Roles: []string{"Staff"}}}}, taken)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, ok := id.Parse(g.ID); !ok || parsed != g.ID || taken(g.ID) || action != "add" {
		t.Fatalf("minted %q, action %s", g.ID, action)
	}
	if _, _, _, err := model.SaveGroup(actor, sources, Group{ID: g.ID, Name: "chess.club", Title: "Chess Club", Managers: []string{actor.Email}, Rules: g.Rules}, taken); err == nil {
		t.Fatal("an edit to a group the sheet does not have was taken")
	}
	if err := cache.CommitAndWait(context.Background(), actor, ops...); err != nil {
		t.Fatal(err)
	}
	if saved := cache.Model().Named("chess.club"); saved == nil || saved.ID != g.ID || cache.Model().Group(g.ID) != saved {
		t.Fatalf("the saved group is %+v", saved)
	}
	current := *model.Group(soccerID)
	current.Name = "soccer-team-2"
	if _, _, _, err := model.SaveGroup(access.Actor{Email: current.Managers[0]}, sources, current, taken); err == nil {
		t.Fatal("a group was renamed")
	}
}

func TestAliasesReachTheirGroupAndStayUnique(t *testing.T) {
	model, err := BuildModel(sampleTables(t), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	g := model.Group(hummingID)
	if !slices.Equal(g.Aliases, []string{"hummingbirds-families"}) || !slices.Equal(g.Names(), []string{"hummingbird-families", "hummingbirds-families"}) {
		t.Fatalf("aliases %v", g.Aliases)
	}
	if model.Resolve("hummingbirds-families") != g || model.Resolve("hummingbird-families") != g || model.Resolve("nobody") != nil || model.Resolve(hummingID) != nil {
		t.Fatal("an alias does not resolve to its group")
	}
	for _, bad := range []map[string]string{
		{"Alias": "hummingbirds-families", "ID": soccerID},
		{"Alias": "middle-school-parents", "ID": soccerID},
		{"Alias": "soccer-team", "ID": soccerID},
		{"Alias": "postmaster", "ID": soccerID},
		{"Alias": "Not An Address", "ID": soccerID},
		{"Alias": "soccer", "ID": "soccer-team"},
		{"Alias": "soccer", "ID": "grp0000000009"},
		{"Alias": "", "ID": soccerID},
	} {
		tables := sampleTables(t)
		tables[id.AliasesTab] = append(tables[id.AliasesTab], bad)
		if _, err := BuildModel(tables, []byte("test")); err == nil {
			t.Errorf("accepted alias %v", bad)
		}
	}
	tables := sampleTables(t)
	tables[id.AliasesTab] = append(tables[id.AliasesTab], map[string]string{"Alias": " Soccer ", "ID": strings.ToUpper(soccerID)})
	model, err = BuildModel(tables, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if model.Resolve("soccer") != model.Group(soccerID) {
		t.Fatal("an alias written loosely does not reach its group")
	}
	tables = sampleTables(t)
	tables[groupsTab] = append(tables[groupsTab], store.Row{idColumn: "grp0000000009", "Name": "hummingbirds-families", "Title": "Clash"})
	tables[managersTab] = append(tables[managersTab], store.Row{"Group": "grp0000000009", "Email": "m@x.org"})
	tables[rulesTab] = append(tables[rulesTab], store.Row{"Group": "grp0000000009", "Kind": "include", "Roles": "Staff"})
	if _, err := BuildModel(tables, []byte("test")); err == nil {
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
	tables[groupsTab] = append(tables[groupsTab], store.Row{idColumn: "grp0000000009", "Name": "lonely", "Title": "Lonely"})
	if _, err := BuildModel(tables, []byte("test")); err == nil || !strings.Contains(err.Error(), "manager") {
		t.Fatalf("a group with no managers: %v", err)
	}
}

func TestArchivedIsOnePersonsAndFollowsTheGroup(t *testing.T) {
	cache, _ := sampleCache(t)
	ctx := context.Background()
	const jordan = "jordan.whitfield@heliosschool.org"
	match := store.Row{"Group": soccerID, "Email": jordan}
	as := access.Actor{Email: jordan}
	for range 2 {
		if err := cache.Commit(ctx, as, store.Upsert(archivedTab, match, store.Row{})); err != nil {
			t.Fatal(err)
		}
	}
	model := cache.Model()
	if !model.Archived(soccerID, jordan) || cache.Count(archivedTab, nil) != 1 {
		t.Fatalf("archiving twice left %d rows", cache.Count(archivedTab, nil))
	}
	if model.Archived(soccerID, "abena.osei@heliosschool.org") || model.Archived(hummingID, jordan) {
		t.Fatal("an archive reached another person or another group")
	}
	if err := cache.Commit(ctx, as, store.Delete(archivedTab, match)); err != nil {
		t.Fatal(err)
	}
	if cache.Count(archivedTab, nil) != 0 || cache.Model().Archived(soccerID, jordan) {
		t.Fatal("unarchiving left the row")
	}
	if err := cache.Commit(ctx, as, store.Upsert(archivedTab, match, store.Row{})); err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(ctx, as, store.Delete(groupsTab, store.Row{idColumn: soccerID})); err != nil {
		t.Fatal(err)
	}
	if cache.Count(archivedTab, nil) != 0 {
		t.Fatal("deleting the group kept its archived row")
	}
	for _, group := range []string{"nowhere", "soccer-team"} {
		stray := sampleTables(t)
		stray[archivedTab] = append(stray[archivedTab], store.Row{"Group": group, "Email": jordan})
		if _, err := BuildModel(stray, []byte("test")); err == nil {
			t.Fatalf("accepted an archived row naming %s", group)
		}
	}
}

func TestSuggestedTagsAreTheOwnersOwnNoRuleNames(t *testing.T) {
	groups := []Group{{Rules: []Rule{{Tags: []string{filter.TagKey("dtg0000000001")}}, {Tags: []string{filter.TagKey("dtg0000000009")}}}}}
	tags := []who.Tag{
		{ID: "dtg0000000003", Name: "Book Club", People: []string{"c@x"}},
		{ID: "dtg0000000001", Name: "Carpool", People: []string{"a@x"}},
		{ID: "dtg0000000004", Name: "Empty", People: []string{}},
		{ID: "dtg0000000002", Name: "Soccer Team", People: []string{"b@x"}},
	}
	got := []string{}
	for _, tag := range SuggestedTags(tags, groups) {
		got = append(got, tag.Name)
	}
	if !slices.Equal(got, []string{"Book Club", "Soccer Team"}) {
		t.Fatalf("suggested %v", got)
	}
}
