package loop_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/loop"
	"heliosian/internal/who"
)

type staticFiles struct{}

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("../../web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

const jordan = "jordan.whitfield@heliosschool.org"

func sample(t *testing.T) (loop.Sources, *who.Tables) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	tables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(tables, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return loop.Sources{
		Directory: model,
		Tags:      func(owner string) map[string][]string { return who.TagsOf(tables.Tags, model, owner) },
		Lists: func(owner string) []who.List {
			lists := model.RoomParentLists(owner)
			if owner == jordan {
				lists = append(lists, who.List{Key: "party:p1", Name: "Pizza Night", Kind: who.ListParty, People: []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org"}})
			}
			return lists
		},
		Shared: func(email string) []who.SharedTag {
			return who.SharedTagsOf(tables.Tags, tables.Managers, model, email)
		},
	}, tables
}

func TestSharedTagsReadForTheManagers(t *testing.T) {
	s, tables := sample(t)
	key := filter.TagKey("abena.osei@heliosschool.org", "Book Club")
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{key} }))
	want := who.TagsOf(tables.Tags, s.Directory, "abena.osei@heliosschool.org")["Book Club"]
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("book club: got %v, want %v", got, want)
	}
	unshared := membersOf(t, s, []string{"colin.quinn@heliosschool.org"}, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{key} }))
	if len(unshared) != 0 {
		t.Fatalf("a group whose managers the tag is not shared with read it: %v", unshared)
	}
}

func rule(kind string, edit func(r *loop.Rule)) loop.Rule {
	r := loop.Rule{Kind: kind}
	edit(&r)
	return r
}

func members(t *testing.T, s loop.Sources, rules ...loop.Rule) []string {
	t.Helper()
	return membersOf(t, s, []string{jordan}, rules...)
}

func membersOf(t *testing.T, s loop.Sources, managers []string, rules ...loop.Rule) []string {
	t.Helper()
	g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: managers, Rules: rules})
	if err := loop.CheckGroup(g); err != nil {
		t.Fatal(err)
	}
	return loop.Members(g, s)
}

func TestRoleRuleIsEveryoneInTheRole(t *testing.T) {
	s, _ := sample(t)
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Roles = []string{"Student"} }))
	want := []string{}
	for _, p := range s.Directory.People {
		if p.IsStudent && !p.EmailMasked {
			want = append(want, p.Email)
		}
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("students: got %v, want %v", got, want)
	}
}

func TestParentsMatchThroughTheirChildren(t *testing.T) {
	s, _ := sample(t)
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Roles = []string{"Parent"}; r.Grades = []string{"Grade 3"} }))
	if !slices.Contains(got, jordan) {
		t.Fatalf("a Grade 3 parent is missing: %v", got)
	}
	for _, email := range got {
		if p := s.Directory.Person(email); !p.IsParent {
			t.Fatalf("%s is not a parent", email)
		}
	}
}

func TestSearchMatchesNameOrAddress(t *testing.T) {
	s, _ := sample(t)
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "Whitfield" }))
	if len(got) < 3 {
		t.Fatalf("whitfield: got %v", got)
	}
	for _, email := range got {
		p := s.Directory.Person(email)
		if !strings.Contains(strings.ToLower(p.FullName), "whitfield") && !strings.Contains(email, "whitfield") {
			t.Fatalf("%s does not match", email)
		}
	}
}

func TestFamilyWidensAClassroom(t *testing.T) {
	s, _ := sample(t)
	base := func(r *loop.Rule) { r.Roles = []string{"Student"}; r.Classrooms = []string{"Hummingbirds"} }
	alone := members(t, s, rule(loop.KindInclude, base))
	if !slices.Contains(alone, "mia.torres@heliosschool.org") {
		t.Fatalf("hummingbirds: %v", alone)
	}
	if slices.Contains(alone, "nico.torres@heliosschool.org") {
		t.Fatal("a sibling is in without Siblings")
	}
	for _, email := range alone {
		if !s.Directory.Person(email).IsStudent {
			t.Fatalf("%s is not a student", email)
		}
	}
	widened := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { base(r); r.Family = []string{"Parents", "Siblings"} }))
	if !slices.Contains(widened, "nico.torres@heliosschool.org") {
		t.Fatalf("siblings: %v", widened)
	}
	for _, email := range widened {
		if !s.Directory.Person(email).IsStudent {
			t.Fatalf("%s is not a student, yet the rule keeps students", email)
		}
	}
	parents := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) {
		r.Roles = []string{"Parent"}
		r.Classrooms = []string{"Hummingbirds"}
		r.Family = []string{"Parents"}
	}))
	if !slices.Contains(parents, "elena.torres@heliosschool.org") || slices.Contains(parents, "mia.torres@heliosschool.org") {
		t.Fatalf("parents of hummingbirds: %v", parents)
	}
	all := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Classrooms = []string{"Hummingbirds"}; r.Family = []string{"Parents"} }))
	if !slices.Contains(all, "elena.torres@heliosschool.org") || !slices.Contains(all, "mia.torres@heliosschool.org") {
		t.Fatalf("hummingbirds and parents: %v", all)
	}
}

func TestReasonsSayWhichRuleAndRelation(t *testing.T) {
	s, _ := sample(t)
	g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan}, Rules: []loop.Rule{
		rule(loop.KindExclude, func(r *loop.Rule) { r.Search = "marco" }),
		rule(loop.KindInclude, func(r *loop.Rule) {
			r.Roles = []string{"Student"}
			r.Classrooms = []string{"Hummingbirds"}
			r.Family = []string{"Parents", "Siblings"}
		}),
		rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "nico" }),
	}})
	reasons := loop.Reasons(g, s)
	if got := reasons["mia.torres@heliosschool.org"]; !slices.Equal(got, []loop.Reason{{Rule: 1}}) {
		t.Fatalf("mia: %+v", got)
	}
	mia := "mia.torres@heliosschool.org"
	if got := reasons["nico.torres@heliosschool.org"]; !slices.Equal(got, []loop.Reason{{Rule: 1, Through: "Siblings", Via: mia, ViaName: "Mia Torres"}, {Rule: 2}}) {
		t.Fatalf("nico: %+v", got)
	}
	if _, in := reasons["elena.torres@heliosschool.org"]; in {
		t.Fatal("a parent is in through a rule that keeps students")
	}
}

func TestExcludeRulesSubtract(t *testing.T) {
	s, _ := sample(t)
	got := members(t, s,
		rule(loop.KindInclude, func(r *loop.Rule) { r.Roles = []string{"Student"} }),
		rule(loop.KindExclude, func(r *loop.Rule) { r.Search = "torres" }),
	)
	for _, email := range got {
		if strings.Contains(email, "torres") {
			t.Fatalf("%s was not excluded", email)
		}
	}
	if len(got) == 0 {
		t.Fatal("everyone was excluded")
	}
}

func TestTagsReadTheOwnersOwn(t *testing.T) {
	s, tables := sample(t)
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{filter.TagKey(jordan, "Carpool")} }))
	want := who.TagsOf(tables.Tags, s.Directory, jordan)["Carpool"]
	if !slices.Equal(got, want) {
		t.Fatalf("carpool: got %v, want %v", got, want)
	}
	other := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{filter.TagKey("asha.chandra@heliosschool.org", "Carpool")} }))
	if len(other) != 0 {
		t.Fatalf("another owner's tag of the same name found %v", other)
	}
}

func TestMagicTagsMatchByKey(t *testing.T) {
	s, _ := sample(t)
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{"party:p1"} }))
	if !slices.Equal(got, []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org"}) {
		t.Fatalf("party: %v", got)
	}
}

func TestAdditionsJoinTheMembersOnce(t *testing.T) {
	s, _ := sample(t)
	mia := "mia.torres@heliosschool.org"
	g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan},
		Rules: []loop.Rule{
			rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "mia torres" }),
			rule(loop.KindExclude, func(r *loop.Rule) { r.Search = "coach" }),
		},
		Additions: []loop.Addition{{Email: "coach@club.example.org", Name: "Coach"}, {Email: mia, Name: "Mia by hand"}},
	})
	reasons := loop.Reasons(g, s)
	if got := reasons["coach@club.example.org"]; !slices.Equal(got, []loop.Reason{{Added: true}}) {
		t.Fatalf("coach: %+v", got)
	}
	if got := reasons[mia]; !slices.Equal(got, []loop.Reason{{Rule: 0}}) {
		t.Fatalf("mia is on by rule, not by hand: %+v", got)
	}
	if got := loop.Members(g, s); !slices.Equal(got, []string{"coach@club.example.org", mia}) {
		t.Fatalf("members: %v", got)
	}
}

func TestSampleGroupsLoadAndHaveMembers(t *testing.T) {
	s, _ := sample(t)
	dir := &data.Dir{Root: "../../sampledata"}
	cache, err := loop.NewCache(dir, dir, func(string) bool { return false }, who.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	model := cache.Model()
	if len(model.Groups) != 3 {
		t.Fatalf("%d groups loaded", len(model.Groups))
	}
	for _, g := range model.Groups {
		members := loop.Members(g, s)
		if len(members) == 0 {
			t.Errorf("%s has nobody", g.Address())
		}
		if g.Address() != g.Name+"@loop.heliosian.com" {
			t.Errorf("address %s", g.Address())
		}
		if !g.Prefix {
			t.Errorf("%s does not prefix its subjects", g.Name)
		}
		if g.Name == "soccer-team" && !slices.Contains(members, "coach.rivera@coastsidesoccer.example.org") {
			t.Errorf("the coach is not on the soccer team: %v", members)
		}
	}
}

func TestExcludedAreLeftOff(t *testing.T) {
	s, _ := sample(t)
	mia := "mia.torres@heliosschool.org"
	g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan},
		Rules:     []loop.Rule{rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "torres" })},
		Additions: []loop.Addition{{Email: "coach@club.example.org", Name: "Coach"}},
		Excluded:  []loop.Excluded{{Email: " Mia.Torres@heliosschool.org ", Note: "  Asked  to be left off ", When: "2026-09-16T10:00:00Z"}, {Email: "coach@club.example.org"}},
	})
	if len(g.Excluded) != 2 || g.Excluded[0].Email != mia || g.Excluded[0].Note != "Asked to be left off" || !g.HasExcluded(mia) {
		t.Fatalf("excluded: %+v", g.Excluded)
	}
	members := loop.Members(g, s)
	if slices.Contains(members, mia) || slices.Contains(members, "coach@club.example.org") {
		t.Fatalf("an excluded person is on the group: %v", members)
	}
	if !slices.Contains(members, "nico.torres@heliosschool.org") {
		t.Fatalf("the rest of the family is gone too: %v", members)
	}
}

func TestWhoSeesAGroupAndWhoReadsItsMail(t *testing.T) {
	s, _ := sample(t)
	mia, nico, outsider := "mia.torres@heliosschool.org", "nico.torres@heliosschool.org", "sam.whitfield@heliosschool.org"
	for _, c := range []struct {
		visibility  string
		sees, reads map[string]bool
	}{
		{loop.VisibilityHidden, map[string]bool{jordan: true}, map[string]bool{jordan: true}},
		{loop.VisibilityMembers, map[string]bool{jordan: true, nico: true, mia: true}, map[string]bool{jordan: true, nico: true}},
		{loop.VisibilityEveryone, map[string]bool{jordan: true, nico: true, mia: true, outsider: true}, map[string]bool{jordan: true, nico: true}},
	} {
		g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan}, Visibility: c.visibility,
			Rules:    []loop.Rule{rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "torres" })},
			Excluded: []loop.Excluded{{Email: mia}},
		})
		for _, email := range []string{jordan, nico, mia, outsider} {
			if got := g.VisibleTo(email, false, s); got != c.sees[email] {
				t.Errorf("%s: %s sees %v", c.visibility, email, got)
			}
			if got := g.MailReadableBy(email, s); got != c.reads[email] {
				t.Errorf("%s: %s reads the mail %v", c.visibility, email, got)
			}
		}
		if !g.VisibleTo(outsider, true, s) {
			t.Errorf("%s: an admin does not see it", c.visibility)
		}
	}
}

func TestWhoMayPost(t *testing.T) {
	s, _ := sample(t)
	mia, nico, outsider := "mia.torres@heliosschool.org", "nico.torres@heliosschool.org", "sam.whitfield@heliosschool.org"
	for _, c := range []struct {
		posting string
		posts   map[string]bool
	}{
		{loop.PostingEveryone, map[string]bool{jordan: true, nico: true, mia: true, outsider: true}},
		{loop.PostingMembers, map[string]bool{jordan: true, nico: true, mia: true}},
		{loop.PostingManagers, map[string]bool{jordan: true}},
	} {
		posting := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan}, Posting: c.posting, Replying: loop.PostingManagers,
			Rules:    []loop.Rule{rule(loop.KindInclude, func(r *loop.Rule) { r.Search = "torres" })},
			Excluded: []loop.Excluded{{Email: mia}},
		})
		replying := posting
		replying.Posting, replying.Replying = loop.PostingManagers, c.posting
		for _, email := range []string{jordan, nico, mia, outsider} {
			if got := posting.PostableBy(email, false, s); got != c.posts[email] {
				t.Errorf("%s: %s posts %v", c.posting, email, got)
			}
			if got := replying.PostableBy(email, true, s); got != c.posts[email] {
				t.Errorf("%s: %s replies %v", c.posting, email, got)
			}
		}
	}
}

func TestSuggestedGroupRuleIsTheListPlusStudentsParents(t *testing.T) {
	s, _ := sample(t)
	lists := s.Lists
	s.Lists = func(owner string) []who.List {
		return append(lists(owner), who.List{Key: "party:p2", Name: "Movie Night", Kind: who.ListParty, People: []string{"abena.osei@heliosschool.org", "harper.quinn@heliosschool.org"}})
	}
	got := members(t, s, rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{"party:p2"}; r.Family = []string{"Parents"} }))
	want := []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org", "dana.hawkins@heliosschool.org", "harper.quinn@heliosschool.org"}
	if !slices.Equal(got, want) {
		t.Fatalf("movie night: got %v, want %v", got, want)
	}
}

func TestRuleCountsSayWhoEachRuleTouches(t *testing.T) {
	s, _ := sample(t)
	g := loop.Normalize(loop.Group{Name: "test", Title: "Test", Managers: []string{jordan}, Rules: []loop.Rule{
		rule(loop.KindInclude, func(r *loop.Rule) { r.Tags = []string{"party:p1"} }),
		rule(loop.KindExclude, func(r *loop.Rule) { r.Search = "osei" }),
		rule(loop.KindExclude, func(r *loop.Rule) { r.Search = "torres" }),
	}})
	counts := loop.RuleCounts(g, s)
	if !slices.Equal(counts, []int{2, 1, 0}) {
		t.Fatalf("counts %v", counts)
	}
}
