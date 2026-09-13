package app

import (
	"sort"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/events"
	"heliosian/internal/who"
)

// calendarLinked hands the calendar what the other apps run: every open,
// dated party on Helios Celebrate and every dated event HCA-Team lists, open
// or done, each with the page it is served at there and what a reader can do
// now. An app whose sheet has not loaded contributes nothing.
type calendarLinked struct {
	celebrate *celebrate.Cache
	events    *events.Cache
}

func (c calendarLinked) list() []calendar.Linked {
	now := time.Now().In(calendar.Location)
	return append(c.parties(now), c.activities()...)
}

func (c calendarLinked) parties(now time.Time) []calendar.Linked {
	out := []calendar.Linked{}
	model := c.celebrate.Model()
	if model == nil {
		return out
	}
	for _, p := range model.SortedParties("") {
		if p.Status != celebrate.StatusOpen || p.Start == "" {
			continue
		}
		out = append(out, calendar.Linked{
			Source: calendar.SourceCelebrate, ID: p.ID, Title: p.Title, Summary: p.Summary, Description: p.Description, Location: p.Location,
			Start: p.Start, End: p.End, Path: model.PathOf(p), Availability: p.Availability(now),
		})
	}
	return out
}

// An HCA-Team event is open to join until it is done or every spot is taken;
// the things under it stay off the calendar, since the event stands for them.
func (c calendarLinked) activities() []calendar.Linked {
	out := []calendar.Linked{}
	model := c.events.Model()
	if model == nil {
		return out
	}
	for _, a := range model.Activities {
		if (a.Status != events.StatusOpen && a.Status != events.StatusDone) || a.Start == "" {
			continue
		}
		availability := "open"
		switch {
		case a.Status == events.StatusDone:
			availability = "done"
		case a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots):
			availability = "full"
		}
		out = append(out, calendar.Linked{
			Source: calendar.SourceTeam, ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location,
			Start: a.Start, End: a.End, Path: model.PathOf(a), Availability: availability,
		})
	}
	return out
}

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
