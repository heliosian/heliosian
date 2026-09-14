package calendar

import (
	"strings"
	"time"
)

// MonthFormat is a month as the front page's rail asks for one: "2026-09".
const MonthFormat = "2006-01"

// Month is a month as Heliosian's rail shows it to one viewer: today, the
// school days in the month with what kind of day each is for their
// classrooms, and every event of theirs that touches the month - what the
// small month's dots and the day card under it are drawn from.
type Month struct {
	Month string `json:"month"`
	Today string `json:"today"`
	// Days holds every school day in the month, by date, with the day types
	// in force for the viewer's classrooms other than Regular; a date that
	// is not here is a weekend, a holiday the year calendar leaves out, or
	// outside the school year.
	Days   map[string]Day `json:"days"`
	Events []Upcoming     `json:"events"`
}

// Day is one school day: the day types in force for the viewer's
// classrooms other than Regular, each naming its classrooms when they are
// not all of them - "Early Dismissal", "No Aftercare · Hummingbirds" - the
// way the calendar's own day words read.
type Day struct {
	Kinds []Kind `json:"kinds"`
}

// Kind is one day type in force: its name, and the words for it.
type Kind struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

// Month reckons a month for one viewer under the filter the calendar page
// first shows them (viewOf): the plan for their classrooms day by day, and
// the events the view admits that fall in the month, in date order, each
// with the viewer's answer and the ones they hid left out. A month that
// does not parse is the month now is in.
func (m *Model) Month(directory Directory, email string, linked []Linked, now time.Time, month string) Month {
	return m.MonthUnder(directory, email, linked, now, month, "")
}

// MonthUnder is Month read under one of the person's saved calendars by
// token, or My Heliosian by its token, rather than their default - the
// rail's picker - or under the default for a blank or unknown token.
func (m *Model) MonthUnder(directory Directory, email string, linked []Linked, now time.Time, month, token string) Month {
	first, err := time.ParseInLocation(MonthFormat, month, Location)
	if err != nil {
		first = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, Location)
	}
	last := first.AddDate(0, 1, -1)
	from, to := first.Format(DateFormat), last.Format(DateFormat)
	classrooms, tags := m.viewUnder(directory, email, token)
	out := Month{Month: first.Format(MonthFormat), Today: now.Format(DateFormat), Days: map[string]Day{}, Events: []Upcoming{}}
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
	for _, e := range m.eventsFor(email, linked) {
		answer := m.AnswerOf(email, e.ID)
		if e.start.Format(DateFormat) > to || e.end.Format(DateFormat) < from || answer == AnswerHidden || !admits(m, e, classrooms, tags) {
			continue
		}
		u := m.card(e)
		u.Answer = answer
		out.Events = append(out.Events, u)
	}
	return out
}
