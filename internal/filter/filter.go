// Package filter is the one reading of "these people" every app shares: a
// Rule as Helios Who?'s filters have it - roles, words, classrooms, grades,
// tags, and the relatives to add - evaluated against the directory. Loop's
// groups are lists of them, include and exclude; Heliosian's apps, sections
// and links each carry one to say who sees them. Who?'s own page evaluates
// the same choices in the browser (web/who/filters.js), and a grade or
// classroom is read as the directory reads it everywhere (who.Model.Facets).
package filter

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/who"
)

const (
	KindInclude = "include"
	KindExclude = "exclude"
)

// Roles are the role facet's values, and Relations the Family facet's: the
// relatives a rule's matches are widened by, as Who?'s Add family does.
var (
	Roles     = []string{"Student", "Parent", "Staff"}
	Relations = []string{"Parents", "Children", "Siblings"}
)

// Rule is one line of a definition: within it every choice must hold, so
// Parent and Grade 3 together are the parents of a Grade 3 student. Kind
// is include or exclude where a list of rules is read (Loop); a thing that
// carries one rule reads it as an include. Tags are read as Owner's.
type Rule struct {
	Kind       string   `json:"kind"`
	Roles      []string `json:"roles"`
	Search     string   `json:"search"`
	Classrooms []string `json:"classrooms"`
	Grades     []string `json:"grades"`
	Tags       []string `json:"tags"`
	Family     []string `json:"family"`
	Owner      string   `json:"owner"`
}

// Empty says the rule chooses nothing at all - which a list of rules
// refuses, and a thing carrying one rule reads as everyone.
func (r Rule) Empty() bool {
	return len(r.Roles)+len(r.Classrooms)+len(r.Grades)+len(r.Tags) == 0 && r.Search == ""
}

// CheckFacets refuses a role or relation the facets cannot mean, and a tag
// no rule could name.
func CheckFacets(r Rule) error {
	for _, role := range r.Roles {
		if !slices.Contains(Roles, role) {
			return fmt.Errorf("role %q is not one of %s", role, strings.Join(Roles, ", "))
		}
	}
	for _, relation := range r.Family {
		if !slices.Contains(Relations, relation) {
			return fmt.Errorf("family relation %q is not one of %s", relation, strings.Join(Relations, ", "))
		}
	}
	for _, tag := range r.Tags {
		if strings.Contains(tag, ",") {
			return fmt.Errorf("a tag with a comma in its name cannot be used in a rule")
		}
	}
	return nil
}

// Sources is what a rule is read against: the directory, and one person's
// tags, Magic Tags and the tags shared with them, by owner, since all three
// are that person's to see. A rule with no tags needs the directory alone.
type Sources struct {
	Directory *who.Model
	Tags      func(owner string) map[string][]string
	Lists     func(owner string) []who.List
	Shared    func(email string) []who.SharedTag
}

// SharedPrefix marks a shared tag in a rule's Tags: shared:<owner>:<name>,
// the owner's address having no colon of its own. It reads as the tag's
// owner's, and only while the rule's owner still manages it.
const SharedPrefix = "shared:"

// SharedKey is how a rule names a tag shared with its owner.
func SharedKey(owner, name string) string {
	return SharedPrefix + owner + ":" + name
}

// Tagged is everyone under one of the owner's tags, Magic Tags or shared
// tags, by the name a rule holds: a tag's name, a Magic Tag's key, or a
// shared tag's key.
func (s Sources) Tagged(owner string) map[string][]string {
	out := map[string][]string{}
	if s.Tags != nil {
		for name, people := range s.Tags(owner) {
			out[name] = people
		}
	}
	if s.Lists != nil {
		for _, list := range s.Lists(owner) {
			out[list.Key] = list.People
		}
	}
	if s.Shared != nil {
		for _, shared := range s.Shared(owner) {
			out[SharedKey(shared.Owner, shared.Name)] = shared.People
		}
	}
	return out
}

func anyIn(have, want []string) bool {
	for _, h := range have {
		if slices.Contains(want, h) {
			return true
		}
	}
	return false
}

// Reason is why a rule reached someone: the relation it reached them
// through and whom - a parent of Mia, say, when the rule matched Mia - or
// nothing for someone it matched itself. Rule and Added are a list's to
// fill: the rule's place among its rules, or that someone was put on by
// hand.
type Reason struct {
	Rule    int    `json:"rule"`
	Through string `json:"through,omitempty"`
	Via     string `json:"via,omitempty"`
	ViaName string `json:"viaName,omitempty"`
	Added   bool   `json:"added,omitempty"`
}

// InRole reports whether a person is one of the roles a rule keeps.
func InRole(p *who.Person, roles []string) bool {
	return len(roles) == 0 || (p.IsStudent && slices.Contains(roles, "Student")) || (p.IsParent && slices.Contains(roles, "Parent")) || (p.IsStaff && slices.Contains(roles, "Staff"))
}

// Matches is everyone one rule picks out, each with the relation it reached
// them through, as Who?'s tag page reads the same choices: the words, the
// classrooms, the grades and the tags pick people out, Family adds the
// relatives asked for, and the roles then keep only those of that kind -
// so "Parents tagged in Carpool, plus their parents" is the parents of the
// tagged children as well as the tagged parents, never the children.
// Someone the rule matches itself stays a direct match whatever relations
// also reach them. tagged is Tagged for the rule's owner, or nil for a
// rule with no tags.
func Matches(r Rule, s Sources, tagged map[string][]string) map[string]Reason {
	model := s.Directory
	out := map[string]Reason{}
	for i := range model.People {
		p := &model.People[i]
		if r.Search != "" && !strings.Contains(strings.ToLower(p.FullName), r.Search) && !strings.Contains(strings.ToLower(p.Email), r.Search) {
			continue
		}
		// A grade or classroom is read as the directory reads it for every
		// filter (who.Model.Facets): a student's own, a parent's children's.
		if len(r.Grades) > 0 && !anyIn(model.Facets(p, false), r.Grades) {
			continue
		}
		if len(r.Classrooms) > 0 && !anyIn(model.Facets(p, true), r.Classrooms) {
			continue
		}
		if len(r.Tags) > 0 && !slices.ContainsFunc(r.Tags, func(tag string) bool { return slices.Contains(tagged[tag], p.Email) }) {
			continue
		}
		out[p.Email] = Reason{}
	}
	direct := map[string]bool{}
	for email := range out {
		direct[email] = true
	}
	reach := func(email, through, via string) {
		if _, ok := out[email]; !ok && email != via {
			out[email] = Reason{Through: through, Via: via, ViaName: model.DisplayName(via)}
		}
	}
	for _, email := range SortedKeys(direct) {
		p := model.Person(email)
		for _, key := range model.FamilyKeysOf(email) {
			family := model.Families[key]
			if p.IsStudent && slices.Contains(r.Family, "Parents") {
				for _, adult := range family.AdultEmails {
					reach(adult, "Parents", email)
				}
			}
			if p.IsParent && slices.Contains(r.Family, "Children") {
				for _, kid := range family.KidEmails {
					reach(kid, "Children", email)
				}
			}
			if p.IsStudent && slices.Contains(r.Family, "Siblings") {
				for _, kid := range family.KidEmails {
					reach(kid, "Siblings", email)
				}
			}
		}
	}
	for email := range out {
		if !InRole(model.Person(email), r.Roles) {
			delete(out, email)
		}
	}
	return out
}

// Includes says whether one rule, read as an include, picks this person
// out - the same answer Matches gives, asked of one address. An empty rule
// picks out nobody; what an empty rule means is the caller's to say.
func Includes(r Rule, s Sources, email string) bool {
	if r.Empty() || s.Directory == nil {
		return false
	}
	var tagged map[string][]string
	if len(r.Tags) > 0 {
		tagged = s.Tagged(r.Owner)
	}
	_, ok := Matches(r, s, tagged)[s.Directory.Resolve(email)]
	return ok
}

func SortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// List is several rules read together, as a group is: the include rules'
// matches less the exclude rules', then Additions - addresses on by hand
// whatever the rules say - and Excluded - addresses off whatever they say.
type List struct {
	Rules     []Rule
	Additions []string
	Excluded  []string
}

// tagged is each owner's tags, fetched once, for the rules that name any.
func (s Sources) taggedFor(rules []Rule) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for _, r := range rules {
		if _, ok := out[r.Owner]; !ok && len(r.Tags) > 0 {
			out[r.Owner] = s.Tagged(r.Owner)
		}
	}
	return out
}

// Reasons is everyone a list picks out and why: the include rules' matches
// less the exclude rules', as addresses the directory keys them by,
// leaving out anyone whose address is a placeholder nothing can reach, each
// with every include rule that reached them; then the additions, on by
// hand whatever the rules say, once each; less the excluded.
func Reasons(l List, s Sources) map[string][]Reason {
	tagged := s.taggedFor(l.Rules)
	in, out := map[string][]Reason{}, map[string]bool{}
	for i, r := range l.Rules {
		for email, reason := range Matches(r, s, tagged[r.Owner]) {
			if r.Kind == KindExclude {
				out[email] = true
				continue
			}
			reason.Rule = i
			in[email] = append(in[email], reason)
		}
	}
	for _, reasons := range in {
		slices.SortFunc(reasons, func(a, b Reason) int { return a.Rule - b.Rule })
	}
	for email := range in {
		if p := s.Directory.Person(email); out[email] || p == nil || p.EmailMasked {
			delete(in, email)
		}
	}
	for _, email := range l.Additions {
		if _, ok := in[email]; !ok {
			in[email] = []Reason{{Added: true}}
		}
	}
	for _, email := range l.Excluded {
		delete(in, email)
	}
	return in
}

// RuleCounts is how many people each rule touches, by its place in the
// list: for an include rule, everyone it matches, relatives and all; for
// an exclude rule, everyone it takes out - those it matches whom an include
// rule had placed on the list. Placeholders and people the directory does
// not hold count for neither.
func RuleCounts(l List, s Sources) []int {
	tagged := s.taggedFor(l.Rules)
	real := func(email string) bool {
		p := s.Directory.Person(email)
		return p != nil && !p.EmailMasked
	}
	in := map[string]bool{}
	matched := make([]map[string]Reason, len(l.Rules))
	for i, r := range l.Rules {
		matched[i] = Matches(r, s, tagged[r.Owner])
		if r.Kind == KindInclude {
			for email := range matched[i] {
				if real(email) {
					in[email] = true
				}
			}
		}
	}
	counts := make([]int, len(l.Rules))
	for i, r := range l.Rules {
		for email := range matched[i] {
			if !real(email) {
				continue
			}
			if r.Kind == KindInclude || in[email] {
				counts[i]++
			}
		}
	}
	return counts
}

// Members is Reasons' people alone, sorted.
func Members(l List, s Sources) []string {
	members := []string{}
	for email := range Reasons(l, s) {
		members = append(members, email)
	}
	sort.Strings(members)
	return members
}

// OnList says whether a list picks this person out, by any address the
// directory knows them by.
func OnList(l List, s Sources, email string) bool {
	_, ok := Reasons(l, s)[s.Directory.Resolve(email)]
	return ok
}

// ListOption and SharedOption are a Magic Tag and a shared tag as an
// editor's Tags dropdown offers them.
type ListOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type SharedOption struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	OwnerName string `json:"ownerName"`
}

// Options is what a rule editor offers a viewer: the classrooms, the grades
// with a student in them, the viewer's own tags, their Magic Tags and the
// tags shared with them, and the roles and relations.
type Options struct {
	Classrooms []string       `json:"classrooms"`
	Grades     []string       `json:"grades"`
	Tags       []string       `json:"tags"`
	Lists      []ListOption   `json:"lists"`
	Shared     []SharedOption `json:"shared"`
	Roles      []string       `json:"roles"`
	Relations  []string       `json:"relations"`
}

// OptionsFor is the choices a rule editor offers this viewer.
func OptionsFor(s Sources, viewer string) Options {
	model := s.Directory
	classrooms := []string{}
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, c.Name)
	}
	present := map[string]bool{}
	for _, p := range model.People {
		if p.IsStudent && p.Grade != "" {
			present[p.Grade] = true
		}
	}
	grades := []string{}
	for _, g := range model.Grades {
		if present[g.Name] {
			grades = append(grades, g.Name)
		}
	}
	tags := []string{}
	if s.Tags != nil {
		for name := range s.Tags(viewer) {
			tags = append(tags, name)
		}
	}
	slices.Sort(tags)
	lists := []ListOption{}
	if s.Lists != nil {
		for _, l := range s.Lists(viewer) {
			lists = append(lists, ListOption{Key: l.Key, Name: l.Name, Kind: l.Kind})
		}
	}
	slices.SortFunc(lists, func(x, y ListOption) int { return strings.Compare(x.Name, y.Name) })
	shared := []SharedOption{}
	if s.Shared != nil {
		for _, t := range s.Shared(viewer) {
			shared = append(shared, SharedOption{Key: SharedKey(t.Owner, t.Name), Name: t.Name, OwnerName: t.OwnerName})
		}
	}
	return Options{Classrooms: classrooms, Grades: grades, Tags: tags, Lists: lists, Shared: shared, Roles: Roles, Relations: Relations}
}

// RuleColumns are a rule's cells as a sheet holds one, whichever tab keys
// it (a group's name, a thing's key) in a column of its own before them.
var RuleColumns = []string{"Kind", "Roles", "Search", "Classrooms", "Grades", "Tags", "Family", "Owner"}

// SplitList reads a list cell: comma-separated, trimmed, without repeats.
func SplitList(cell string) []string {
	out := []string{}
	for _, item := range strings.Split(cell, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func JoinList(items []string) string {
	return strings.Join(items, ", ")
}

// RuleFromRow reads a rule's cells.
func RuleFromRow(row map[string]string) Rule {
	return Rule{
		Kind: row["Kind"], Roles: SplitList(row["Roles"]), Search: row["Search"],
		Classrooms: SplitList(row["Classrooms"]), Grades: SplitList(row["Grades"]),
		Tags: SplitList(row["Tags"]), Family: SplitList(row["Family"]), Owner: row["Owner"],
	}
}

// RuleCells is a rule as a sheet holds it.
func RuleCells(r Rule) map[string]string {
	return map[string]string{
		"Kind": r.Kind, "Roles": JoinList(r.Roles), "Search": r.Search,
		"Classrooms": JoinList(r.Classrooms), "Grades": JoinList(r.Grades),
		"Tags": JoinList(r.Tags), "Family": JoinList(r.Family), "Owner": r.Owner,
	}
}

// Clean is a rule as an editor sent it, tidied: the kind lowercased, the
// lists trimmed and without repeats, the words lowercased and single-spaced,
// the owner lowercased.
func Clean(r Rule) Rule {
	tags := []string{}
	for _, t := range r.Tags {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return Rule{
		Kind:       strings.ToLower(strings.TrimSpace(r.Kind)),
		Roles:      SplitList(JoinList(r.Roles)),
		Search:     strings.Join(strings.Fields(strings.ToLower(r.Search)), " "),
		Classrooms: SplitList(JoinList(r.Classrooms)),
		Grades:     SplitList(JoinList(r.Grades)),
		Tags:       tags,
		Family:     SplitList(JoinList(r.Family)),
		Owner:      strings.ToLower(strings.TrimSpace(r.Owner)),
	}
}

// Check refuses a rule a list cannot read: a kind that is neither, a facet
// the filter has no such thing as, or nothing chosen at all.
func Check(r Rule) error {
	if r.Kind != KindInclude && r.Kind != KindExclude {
		return fmt.Errorf("kind %q is not %s or %s", r.Kind, KindInclude, KindExclude)
	}
	if err := CheckFacets(r); err != nil {
		return err
	}
	if r.Empty() {
		return fmt.Errorf("a rule needs a role, some words, a classroom, a grade or a tag")
	}
	return nil
}
