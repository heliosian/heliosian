package app

import (
	"sort"

	"heliosian/internal/calendar"
	"heliosian/internal/who"
)

// CalendarRoster is the directory's classrooms as the calendar resolves
// audiences against them: each with its band, the grades its students are
// in, and its crews.
func CalendarRoster(m *who.Model) calendar.Roster {
	bandOf := map[string]string{}
	order := map[string]int{}
	for i, g := range m.Grades {
		bandOf[g.Name] = g.Band
		order[g.Name] = i
	}
	grades := map[string]map[string]bool{}
	for _, p := range m.People {
		if !p.IsStudent || p.Classroom == "" || p.Grade == "" {
			continue
		}
		if grades[p.Classroom] == nil {
			grades[p.Classroom] = map[string]bool{}
		}
		grades[p.Classroom][p.Grade] = true
	}
	crews := map[string][]string{}
	for _, c := range m.Crews {
		if c.Name != "" {
			crews[c.Classroom] = append(crews[c.Classroom], c.Name)
		}
	}
	roster := calendar.Roster{Classrooms: []calendar.Classroom{}}
	for _, c := range m.Classrooms {
		names := []string{}
		for g := range grades[c.Name] {
			names = append(names, g)
		}
		sort.Slice(names, func(i, j int) bool { return order[names[i]] < order[names[j]] })
		band := ""
		if len(names) > 0 {
			band = bandOf[names[0]]
		}
		roster.Classrooms = append(roster.Classrooms, calendar.Classroom{Name: c.Name, Band: band, Grades: names, Crews: crews[c.Name]})
	}
	return roster
}
