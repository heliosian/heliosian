package model

import (
	"context"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func sample(t *testing.T) (AudienceSources, *Directory) {
	t.Helper()
	directory := loadDirectory(t, &data.Dir{Root: "../../sampledata"}, nil, testkit.Files("../../web/who"), sampleKey)
	return AudienceSources{
		Directory: directory,
		MagicTags: func(owner string) []MagicTag {
			lists := directory.RoomParentTags(owner)
			if owner == jordan {
				lists = append(lists, MagicTag{Key: "party:p1", Name: "Pizza Night", Kind: MagicTagParty, People: []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org"}})
			}
			return lists
		},
		EmailLists: &EmailLists{},
	}, directory
}

const (
	carpoolID    = "dtg0000000001"
	soccerTeamID = "dtg0000000002"
	bookClubID   = "dtg0000000003"
	soccerGroup  = "grp0000000001"
)

func tagged(directory *Directory, key string) []string {
	tag, _ := directory.Tag(key)
	return tag.People
}

func sourcesOf(directory *Directory) AudienceSources {
	return AudienceSources{Directory: directory, MagicTags: directory.RoomParentTags, EmailLists: &EmailLists{}}
}

func TestRenamingATagKeepsTheGroupsThatNameIt(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	q := store.NewQueue()
	s := sampleStore(t, dir, q, sampleDeps(sampleKey))
	g := *s.Model().EmailLists.Group(soccerGroup)
	if !slices.Contains(g.Rules[0].Tags, TagKey(soccerTeamID)) {
		t.Fatalf("the sample group's rules do not name the soccer team: %+v", g.Rules)
	}
	before := g.Members(sourcesOf(s.Model().Directory))
	if len(before) <= len(tagged(s.Model().Directory, soccerTeamID)) {
		t.Fatalf("the group's members %v do not reach past the tag to its parents", before)
	}
	if err := s.Commit(context.Background(), access.System("test"), DirectoryApp, store.Update(tagListTable, store.Row{tagID: soccerTeamID}, store.Row{tagName: "Football"})); err != nil {
		t.Fatal(err)
	}
	renamed := sourcesOf(s.Model().Directory)
	if after := g.Members(renamed); !slices.Equal(after, before) {
		t.Fatalf("the renamed tag's group: %v, before the rename %v", after, before)
	}
	if labels := renamed.TagLabels(g.Rules[0], g.Managers, jordan); !slices.Equal(labels, []string{"Football"}) {
		t.Fatalf("the rule reads %q after the rename", labels)
	}
}

func TestSharedTagsReadForTheManagers(t *testing.T) {
	s, directory := sample(t)
	key := TagKey(bookClubID)
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Tags = []string{key} }))
	want := tagged(directory, bookClubID)
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("book club: got %v, want %v", got, want)
	}
	unshared := membersOf(t, s, []string{"colin.quinn@heliosschool.org"}, rule(RuleInclude, func(r *Rule) { r.Tags = []string{key} }))
	if len(unshared) != 0 {
		t.Fatalf("a group whose managers the tag is not shared with read it: %v", unshared)
	}
}

func rule(kind string, edit func(r *Rule)) Rule {
	r := Rule{Kind: kind}
	edit(&r)
	return r
}

func listMembers(t *testing.T, s AudienceSources, rules ...Rule) []string {
	t.Helper()
	return membersOf(t, s, []string{jordan}, rules...)
}

func membersOf(t *testing.T, s AudienceSources, managers []string, rules ...Rule) []string {
	t.Helper()
	g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: managers, Rules: rules})
	if err := CheckList(g); err != nil {
		t.Fatal(err)
	}
	return g.Members(s)
}

func TestRoleRuleIsEveryoneInTheRole(t *testing.T) {
	s, _ := sample(t)
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Roles = []string{"Student"} }))
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
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Roles = []string{"Parent"}; r.Grades = []string{"Grade 3"} }))
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
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Search = "Whitfield" }))
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
	base := func(r *Rule) { r.Roles = []string{"Student"}; r.Classrooms = []string{"Hummingbirds"} }
	alone := listMembers(t, s, rule(RuleInclude, base))
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
	widened := listMembers(t, s, rule(RuleInclude, func(r *Rule) { base(r); r.Family = []string{"Parents", "Siblings"} }))
	if !slices.Contains(widened, "nico.torres@heliosschool.org") {
		t.Fatalf("siblings: %v", widened)
	}
	for _, email := range widened {
		if !s.Directory.Person(email).IsStudent {
			t.Fatalf("%s is not a student, yet the rule keeps students", email)
		}
	}
	parents := listMembers(t, s, rule(RuleInclude, func(r *Rule) {
		r.Roles = []string{"Parent"}
		r.Classrooms = []string{"Hummingbirds"}
		r.Family = []string{"Parents"}
	}))
	if !slices.Contains(parents, "elena.torres@heliosschool.org") || slices.Contains(parents, "mia.torres@heliosschool.org") {
		t.Fatalf("parents of hummingbirds: %v", parents)
	}
	all := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Classrooms = []string{"Hummingbirds"}; r.Family = []string{"Parents"} }))
	if !slices.Contains(all, "elena.torres@heliosschool.org") || !slices.Contains(all, "mia.torres@heliosschool.org") {
		t.Fatalf("hummingbirds and parents: %v", all)
	}
}

func TestReasonsSayWhichRuleAndRelation(t *testing.T) {
	s, _ := sample(t)
	g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan}, Rules: []Rule{
		rule(RuleExclude, func(r *Rule) { r.Search = "marco" }),
		rule(RuleInclude, func(r *Rule) {
			r.Roles = []string{"Student"}
			r.Classrooms = []string{"Hummingbirds"}
			r.Family = []string{"Parents", "Siblings"}
		}),
		rule(RuleInclude, func(r *Rule) { r.Search = "nico" }),
	}})
	reasons := g.Reasons(s)
	if got := reasons["mia.torres@heliosschool.org"]; !slices.Equal(got, []Reason{{Rule: 1}}) {
		t.Fatalf("mia: %+v", got)
	}
	mia := "mia.torres@heliosschool.org"
	if got := reasons["nico.torres@heliosschool.org"]; !slices.Equal(got, []Reason{{Rule: 1, Through: "Siblings", Via: mia, ViaName: "Mia Torres"}, {Rule: 2}}) {
		t.Fatalf("nico: %+v", got)
	}
	if _, in := reasons["elena.torres@heliosschool.org"]; in {
		t.Fatal("a parent is in through a rule that keeps students")
	}
}

func TestExcludeRulesSubtract(t *testing.T) {
	s, _ := sample(t)
	got := listMembers(t, s,
		rule(RuleInclude, func(r *Rule) { r.Roles = []string{"Student"} }),
		rule(RuleExclude, func(r *Rule) { r.Search = "torres" }),
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
	s, directory := sample(t)
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Tags = []string{TagKey(carpoolID)} }))
	want := tagged(directory, carpoolID)
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("carpool: got %v, want %v", got, want)
	}
	other := membersOf(t, s, []string{"colin.quinn@heliosschool.org"}, rule(RuleInclude, func(r *Rule) { r.Tags = []string{TagKey(carpoolID)} }))
	if len(other) != 0 {
		t.Fatalf("a manager the tag is not theirs found %v", other)
	}
}

func TestMagicTagsMatchByKey(t *testing.T) {
	s, _ := sample(t)
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Tags = []string{"party:p1"} }))
	if !slices.Equal(got, []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org"}) {
		t.Fatalf("party: %v", got)
	}
}

func TestAdditionsJoinTheMembersOnce(t *testing.T) {
	s, _ := sample(t)
	mia := "mia.torres@heliosschool.org"
	g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan},
		Rules: []Rule{
			rule(RuleInclude, func(r *Rule) { r.Search = "mia torres" }),
			rule(RuleExclude, func(r *Rule) { r.Search = "coach" }),
		},
		Additions: []Addition{{Email: "coach@club.example.org", Name: "Coach"}, {Email: mia, Name: "Mia by hand"}},
	})
	reasons := g.Reasons(s)
	if got := reasons["coach@club.example.org"]; !slices.Equal(got, []Reason{{Added: true}}) {
		t.Fatalf("coach: %+v", got)
	}
	if got := reasons[mia]; !slices.Equal(got, []Reason{{Rule: 0}}) {
		t.Fatalf("mia is on by rule, not by hand: %+v", got)
	}
	if got := g.Members(s); !slices.Equal(got, []string{"coach@club.example.org", mia}) {
		t.Fatalf("members: %v", got)
	}
}

func TestSampleGroupsLoadAndHaveMembers(t *testing.T) {
	s, _ := sample(t)
	dir := &data.Dir{Root: "../../sampledata"}
	m := sampleStore(t, dir, store.NewQueue(), sampleDeps(sampleKey)).Model().EmailLists
	if len(m.Groups) != 3 {
		t.Fatalf("%d groups loaded", len(m.Groups))
	}
	for _, g := range m.Groups {
		members := g.Members(s)
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
	g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan},
		Rules:     []Rule{rule(RuleInclude, func(r *Rule) { r.Search = "torres" })},
		Additions: []Addition{{Email: "coach@club.example.org", Name: "Coach"}},
		Excluded:  []Excluded{{Email: " Mia.Torres@heliosschool.org ", Note: "  Asked  to be left off ", When: "2026-09-16T10:00:00Z"}, {Email: "coach@club.example.org"}},
	})
	if len(g.Excluded) != 2 || g.Excluded[0].Email != mia || g.Excluded[0].Note != "Asked to be left off" || !g.HasExcluded(mia) {
		t.Fatalf("excluded: %+v", g.Excluded)
	}
	members := g.Members(s)
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
		{VisibilityHidden, map[string]bool{jordan: true}, map[string]bool{jordan: true}},
		{VisibilityMembers, map[string]bool{jordan: true, nico: true, mia: true}, map[string]bool{jordan: true, nico: true}},
		{VisibilityEveryone, map[string]bool{jordan: true, nico: true, mia: true, outsider: true}, map[string]bool{jordan: true, nico: true}},
	} {
		g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan}, Visibility: c.visibility,
			Rules:    []Rule{rule(RuleInclude, func(r *Rule) { r.Search = "torres" })},
			Excluded: []Excluded{{Email: mia}},
		})
		for _, email := range []string{jordan, nico, mia, outsider} {
			as := access.Actor{Email: email}
			if got := g.VisibleTo(as, s); got != c.sees[email] {
				t.Errorf("%s: %s sees %v", c.visibility, email, got)
			}
			if got := g.MailReadableBy(email, s); got != c.reads[email] {
				t.Errorf("%s: %s reads the mail %v", c.visibility, email, got)
			}
			shown := g.For(as, s)
			if (shown != nil) != c.sees[email] {
				t.Errorf("%s: %s got %v from For", c.visibility, email, shown)
			}
			if shown != nil && (len(shown.Rules) > 0) != (email == jordan) {
				t.Errorf("%s: %s sees rules %v", c.visibility, email, shown.Rules)
			}
		}
		if !g.VisibleTo(access.Actor{Email: outsider, Allowances: access.Grant(EmailListsAdminAllowances)}, s) {
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
		{PostingEveryone, map[string]bool{jordan: true, nico: true, mia: true, outsider: true}},
		{PostingMembers, map[string]bool{jordan: true, nico: true, mia: true}},
		{PostingManagers, map[string]bool{jordan: true}},
	} {
		posting := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan}, Posting: c.posting, Replying: PostingManagers,
			Rules:    []Rule{rule(RuleInclude, func(r *Rule) { r.Search = "torres" })},
			Excluded: []Excluded{{Email: mia}},
		})
		replying := posting
		replying.Posting, replying.Replying = PostingManagers, c.posting
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
	lists := s.MagicTags
	s.MagicTags = func(owner string) []MagicTag {
		return append(lists(owner), MagicTag{Key: "party:p2", Name: "Movie Night", Kind: MagicTagParty, People: []string{"abena.osei@heliosschool.org", "harper.quinn@heliosschool.org"}})
	}
	got := listMembers(t, s, rule(RuleInclude, func(r *Rule) { r.Tags = []string{"party:p2"}; r.Family = []string{"Parents"} }))
	want := []string{"abena.osei@heliosschool.org", "colin.quinn@heliosschool.org", "dana.hawkins@heliosschool.org", "harper.quinn@heliosschool.org"}
	if !slices.Equal(got, want) {
		t.Fatalf("movie night: got %v, want %v", got, want)
	}
}

func TestRuleCountsSayWhoEachRuleTouches(t *testing.T) {
	s, _ := sample(t)
	g := NormalizeList(EmailList{Name: "test", Title: "Test", Managers: []string{jordan}, Rules: []Rule{
		rule(RuleInclude, func(r *Rule) { r.Tags = []string{"party:p1"} }),
		rule(RuleExclude, func(r *Rule) { r.Search = "osei" }),
		rule(RuleExclude, func(r *Rule) { r.Search = "torres" }),
	}})
	counts := g.RuleCounts(s)
	if !slices.Equal(counts, []int{2, 1, 0}) {
		t.Fatalf("counts %v", counts)
	}
}
