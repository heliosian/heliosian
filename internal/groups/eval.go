package groups

import (
	"slices"
	"sort"
	"strings"

	"heliosian/internal/who"
)

// Sources is what a rule is read against: the directory, and one person's
// tags and Magic Tags by owner, since both are private to that person.
type Sources struct {
	Directory *who.Model
	Tags      func(owner string) map[string][]string
	Lists     func(owner string) []who.List
}

// tagged is everyone under one of the owner's tags or Magic Tags, by the
// name the rule holds: a tag's name, or a Magic Tag's key.
func (s Sources) tagged(owner string) map[string][]string {
	out := map[string][]string{}
	for name, people := range s.Tags(owner) {
		out[name] = people
	}
	for _, list := range s.Lists(owner) {
		out[list.Key] = list.People
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

// matches is everyone one rule picks out: every facet set must hold, then
// Family adds the relatives asked for.
func matches(r Rule, s Sources, tagged map[string][]string) map[string]bool {
	model := s.Directory
	out := map[string]bool{}
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
		out[p.Email] = true
	}
	for _, email := range sortedKeys(out) {
		p := model.Person(email)
		for _, key := range model.FamilyKeysOf(email) {
			family := model.Families[key]
			if p.IsStudent && slices.Contains(r.Family, "Parents") {
				for _, adult := range family.AdultEmails {
					out[adult] = true
				}
			}
			if p.IsParent && slices.Contains(r.Family, "Children") {
				for _, kid := range family.KidEmails {
					out[kid] = true
				}
			}
			if p.IsStudent && slices.Contains(r.Family, "Siblings") {
				for _, kid := range family.KidEmails {
					out[kid] = true
				}
			}
		}
	}
	return out
}

// Members is everyone a group's rules put on it: the include rules' matches
// less the exclude rules', as addresses the directory keys them by, leaving
// out anyone whose address is a placeholder nothing can reach.
func Members(g Group, s Sources) []string {
	tagged := map[string]map[string][]string{}
	for _, r := range g.Rules {
		if _, ok := tagged[r.Owner]; !ok && len(r.Tags) > 0 {
			tagged[r.Owner] = s.tagged(r.Owner)
		}
	}
	in, out := map[string]bool{}, map[string]bool{}
	for _, r := range g.Rules {
		into := in
		if r.Kind == KindExclude {
			into = out
		}
		for email := range matches(r, s, tagged[r.Owner]) {
			into[email] = true
		}
	}
	members := []string{}
	for email := range in {
		if out[email] {
			continue
		}
		if p := s.Directory.Person(email); p == nil || p.EmailMasked {
			continue
		}
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
