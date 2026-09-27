package when

import (
	"strings"
	"time"
)

const MonthFormat = "2006-01"

type Month struct {
	Month  string         `json:"month"`
	Today  string         `json:"today"`
	Days   map[string]Day `json:"days"`
	Events []Card         `json:"events"`
}

type Day struct {
	Kinds []Kind `json:"kinds"`
}

type Kind struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

func (m *Model) MonthUnder(directory Directory, email string, linked []Linked, now time.Time, month, token string) Month {
	first, err := time.ParseInLocation(MonthFormat, month, Location)
	if err != nil {
		first = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, Location)
	}
	last := first.AddDate(0, 1, -1)
	from, to := first.Format(DateFormat), last.Format(DateFormat)
	classrooms, tags := m.viewUnder(directory, email, token)
	out := Month{Month: first.Format(MonthFormat), Today: now.Format(DateFormat), Days: map[string]Day{}, Events: []Card{}}
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		date := d.Format(DateFormat)
		byClassroom, ok := m.Days[date]
		if !ok {
			continue
		}
		byType := map[string][]string{}
		for _, c := range classrooms {
			if name := byClassroom[c]; name != "" && name != RegularDayType {
				byType[name] = append(byType[name], c)
			}
		}
		day := Day{Kinds: []Kind{}}
		for _, t := range m.DayTypes {
			rooms, ok := byType[t.Name]
			if !ok {
				continue
			}
			words := t.Name
			if len(rooms) != len(classrooms) {
				words += " · " + strings.Join(rooms, ", ")
			}
			day.Kinds = append(day.Kinds, Kind{Name: t.Name, Words: words})
		}
		out.Days[date] = day
	}
	for _, e := range m.eventsFor(directory, email, linked) {
		answer := m.AnswerOf(email, e.ID)
		if e.start.Format(DateFormat) > to || e.end.Format(DateFormat) < from || !m.InView(e, answer, classrooms, tags) {
			continue
		}
		u := m.card(e)
		u.Answer = answer
		out.Events = append(out.Events, u)
	}
	return out
}
