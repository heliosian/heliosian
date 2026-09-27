package who

import "slices"

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
	out := []string{}
	for _, k := range m.Children(p.Email) {
		if pick(k) != "" {
			out = append(out, pick(k))
		}
	}
	return out
}

func (m *Model) ClassroomsOf(p *Person) []string {
	out := []string{}
	for _, c := range m.Facets(p, true) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}
