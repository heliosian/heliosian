package model

import (
	"strings"
	"time"
)

const MonthFormat = "2006-01"

type CalendarMonth struct {
	Month  string                 `json:"month"`
	Today  string                 `json:"today"`
	Days   map[string]CalendarDay `json:"days"`
	Events []EventCard            `json:"events"`
}

type CalendarDay struct {
	Kinds []DayKind `json:"kinds"`
}

type DayKind struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

func (m *Calendar) MonthUnder(directory *Directory, email string, linked []Linked, now time.Time, month, token string) CalendarMonth {
	first, err := time.ParseInLocation(MonthFormat, month, Location)
	if err != nil {
		first = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, Location)
	}
	last := first.AddDate(0, 1, -1)
	from, to := first.Format(DateFormat), last.Format(DateFormat)
	classrooms, tags := m.viewUnder(directory, email, token)
	out := CalendarMonth{Month: first.Format(MonthFormat), Today: now.Format(DateFormat), Days: map[string]CalendarDay{}, Events: []EventCard{}}
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		date := d.Format(DateFormat)
		byClassroom, ok := m.Days[date]
		if !ok {
			continue
		}
		byType := map[string][]string{}
		for _, c := range classrooms {
			if key := byClassroom[c]; key != "" && key != RegularDayType {
				byType[key] = append(byType[key], c)
			}
		}
		day := CalendarDay{Kinds: []DayKind{}}
		for _, t := range m.DayTypes {
			rooms, ok := byType[t.ID]
			if !ok {
				continue
			}
			words := t.Name
			if len(rooms) != len(classrooms) {
				words += " · " + strings.Join(rooms, ", ")
			}
			day.Kinds = append(day.Kinds, DayKind{Name: t.Name, Words: words})
		}
		out.Days[date] = day
	}
	for _, e := range m.eventsFor(directory, email, linked) {
		answer := m.AnswerOf(email, e.ID)
		if e.start.Format(DateFormat) > to || e.end.Format(DateFormat) < from || !m.InView(e, answer, classrooms, tags) {
			continue
		}
		out.Events = append(out.Events, m.card(directory, email, e))
	}
	return out
}
