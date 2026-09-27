package who

import (
	"slices"
	"strings"
)

func (m *Model) ResolveAll(emails []string) []string {
	out := []string{}
	for _, e := range emails {
		if r := m.Resolve(e); !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func (m *Model) GradeNames() []string {
	out := []string{}
	for _, g := range m.Grades {
		out = append(out, g.Name)
	}
	return out
}

func (m *Model) ClassroomNames() []string {
	out := []string{}
	for _, c := range m.Classrooms {
		out = append(out, c.Name)
	}
	return out
}

func (m *Model) Teaches(author string) []string {
	name, _, _ := strings.Cut(author, "<")
	name = strings.Trim(strings.TrimSpace(name), `"`)
	if name == "" {
		return nil
	}
	out := []string{}
	for _, c := range m.Crews {
		for _, email := range c.Teachers {
			if p := m.Person(email); p != nil && strings.EqualFold(p.FullName, name) && !slices.Contains(out, c.Classroom) {
				out = append(out, c.Classroom)
			}
		}
	}
	return out
}

func (c *Cache) Classrooms() []string {
	return c.Model().ClassroomNames()
}

func (c *Cache) Grades() []string {
	return c.Model().GradeNames()
}

func (c *Cache) Teaches(author string) []string {
	return c.Model().Teaches(author)
}
