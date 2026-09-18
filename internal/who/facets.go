package who

import "slices"

// A person's grade and classroom as every filter reads them - Who?'s own
// (web/who/filters.js, personFacets), Loop's rules and Heliosian's link and
// category audiences alike: a student's own, a parent's children's, and
// none for anyone else. A staff member is not in the classroom they teach
// for these purposes; a teacher wanting a room's things joins its list or
// is added by hand.

// Facets is a person's grades or classrooms, by that rule.
func (m *Model) Facets(p *Person, classroom bool) []string {
	pick := func(q *Person) string {
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
	for _, key := range m.FamilyKeysOf(p.Email) {
		for _, kid := range m.Families[key].KidEmails {
			if k := m.Person(kid); k != nil && pick(k) != "" {
				out = append(out, pick(k))
			}
		}
	}
	return out
}

// ClassroomsOf is a person's classrooms by the same rule, once each.
func (m *Model) ClassroomsOf(p *Person) []string {
	out := []string{}
	for _, c := range m.Facets(p, true) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
