package model

import (
	"strings"
)

type DayKind struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

func (m *Calendar) calendarView(directory *Directory, f Feed) (classrooms, tags []string) {
	if f.Token == MyHeliosianToken {
		return m.myHeliosianView(directory, f.Email)
	}
	return m.feedView(&f)
}

func (m *Calendar) dayKinds(classrooms []string) map[string][]DayKind {
	out := map[string][]DayKind{}
	for date, byClassroom := range m.Days {
		byType := map[string][]string{}
		for _, c := range classrooms {
			if key := byClassroom[c]; key != "" && key != RegularDayType {
				byType[key] = append(byType[key], c)
			}
		}
		kinds := []DayKind{}
		for _, t := range m.DayTypes {
			rooms, ok := byType[t.ID]
			if !ok {
				continue
			}
			words := t.Name
			if len(rooms) != len(classrooms) {
				words += " · " + strings.Join(rooms, ", ")
			}
			kinds = append(kinds, DayKind{Name: t.Name, Words: words})
		}
		out[date] = kinds
	}
	return out
}
