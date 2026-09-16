package groups

import (
	"slices"
	"sort"
	"strings"

	"heliosian/internal/who"
)

// Sources is what a rule is read against: the directory, and one person's
// tags, Magic Tags and the tags shared with them, by owner, since all three
// are that person's to see.
type Sources struct {
	Directory *who.Model
	Tags      func(owner string) map[string][]string
	Lists     func(owner string) []who.List
	Shared    func(email string) []who.SharedTag
}

// sharedPrefix marks a shared tag in a rule's Tags: shared:<owner>:<name>,
// the owner's address having no colon of its own. It reads as the tag's
// owner's, and only while the rule's owner still manages it.
const sharedPrefix = "shared:"

// SharedKey is how a rule names a tag shared with its owner.
func SharedKey(owner, name string) string {
	return sharedPrefix + owner + ":" + name
}

// tagged is everyone under one of the owner's tags, Magic Tags or shared
// tags, by the name the rule holds: a tag's name, a Magic Tag's key, or a
// shared tag's key.
func (s Sources) tagged(owner string) map[string][]string {
	out := map[string][]string{}
	for name, people := range s.Tags(owner) {
		out[name] = people
	}
	for _, list := range s.Lists(owner) {
		out[list.Key] = list.People
	}
	for _, shared := range s.Shared(owner) {
		out[SharedKey(shared.Owner, shared.Name)] = shared.People
	}
	return out
}

// facets is a person's grade or classroom as Who?'s filters read it: a
// student's own, a parent's children's, and none for anyone else.
func facets(model *who.Model, p *who.Person, classroom bool) []string {
	pick := func(q *who.Person) string {
		if classroom {
			return q.Classroom
		}
		return q.Grade
	}
	if p.IsStudent {
		if v := pick(p); v != "" {
			return []string{v}
		}
		return nil
	}
	if !p.IsParent {
		return nil
	}
	out := []string{}
	for _, key := range model.FamilyKeysOf(p.Email) {
		for _, kid := range model.Families[key].KidEmails {
			if k := model.Person(kid); k != nil && pick(k) != "" {
				out = append(out, pick(k))
			}
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

// Reason is why a member is on a group: the rule, by its place among the
// group's rules, and the relation it reached them through and whom - a
// parent of Mia, say, when the rule matched Mia - or nothing for someone
// it matched itself; or Added, for someone a manager put on by hand.
type Reason struct {
	Rule    int    `json:"rule"`
	Through string `json:"through,omitempty"`
	Via     string `json:"via,omitempty"`
	ViaName string `json:"viaName,omitempty"`
	Added   bool   `json:"added,omitempty"`
}

// matches is everyone one rule picks out, each with the relation it reached
// them through: every facet set must hold, then Family adds the relatives
// asked for. Someone the rule matches itself stays a direct match whatever
// relations also reach them.
func matches(r Rule, s Sources, tagged map[string][]string) map[string]Reason {
	model := s.Directory
	out := map[string]Reason{}
	for i := range model.People {
		p := &model.People[i]
		if len(r.Roles) > 0 && !((p.IsStudent && slices.Contains(r.Roles, "Student")) || (p.IsParent && slices.Contains(r.Roles, "Parent")) || (p.IsStaff && slices.Contains(r.Roles, "Staff"))) {
			continue
		}
		if r.Search != "" && !strings.Contains(strings.ToLower(p.FullName), r.Search) && !strings.Contains(strings.ToLower(p.Email), r.Search) {
			continue
		}
		if len(r.Grades) > 0 && !anyIn(facets(model, p, false), r.Grades) {
			continue
		}
		if len(r.Classrooms) > 0 && !anyIn(facets(model, p, true), r.Classrooms) {
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
	for _, email := range sortedKeys(direct) {
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
	return out
}

// Reasons is everyone on a group and why: the include rules' matches less
// the exclude rules', as addresses the directory keys them by, leaving out
// anyone whose address is a placeholder nothing can reach, each with every
// include rule that reached them; then the additions, on by hand whatever
// the rules say, once each.
func Reasons(g Group, s Sources) map[string][]Reason {
	tagged := map[string]map[string][]string{}
	for _, r := range g.Rules {
		if _, ok := tagged[r.Owner]; !ok && len(r.Tags) > 0 {
			tagged[r.Owner] = s.tagged(r.Owner)
		}
	}
	in, out := map[string][]Reason{}, map[string]bool{}
	for i, r := range g.Rules {
		for email, reason := range matches(r, s, tagged[r.Owner]) {
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
	for _, a := range g.Additions {
		if _, ok := in[a.Email]; !ok {
			in[a.Email] = []Reason{{Added: true}}
		}
	}
	return in
}

// Members is Reasons' people alone, sorted.
func Members(g Group, s Sources) []string {
	members := []string{}
	for email := range Reasons(g, s) {
		members = append(members, email)
	}
	sort.Strings(members)
	return members
}

// Plan is every group as Google should hold it.
func Plan(model *Model, s Sources) []Desired {
	out := make([]Desired, 0, len(model.Groups))
	for _, g := range model.Groups {
		out = append(out, Desired{Name: g.Name, Title: g.Title, Description: g.Description, Members: Members(g, s)})
	}
	return out
}
