package ask

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"heliosian/internal/calendar"
)

type calendarClassroom = calendar.Classroom

// eventCard is an event as the tools answer it: when and what, its
// audience, the way in when another app runs it, and where the viewer's
// household stands with it.
type eventCard struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Start        string   `json:"start"`
	End          string   `json:"end,omitempty"`
	When         string   `json:"whenAgainstToday,omitempty"`
	Past         bool     `json:"past"`
	AllDay       bool     `json:"allDay,omitempty"`
	Location     string   `json:"location,omitempty"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags"`
	Classrooms   []string `json:"classrooms,omitempty"`
	DayType      string   `json:"dayType,omitempty"`
	Link         string   `json:"link"`
	Availability string   `json:"availability,omitempty"`
	MyAnswer     string   `json:"myAnswer,omitempty"`
	Household    []string `json:"household,omitempty"`
}

func (v *viewer) eventCard(e *calendar.Event) eventCard {
	c := eventCard{
		ID: e.ID, Title: e.Title, Start: e.Start, AllDay: e.AllDay, Location: e.Location, Description: clip(e.Description, 400),
		Tags: e.Tags, Classrooms: e.Classrooms, DayType: e.DayType, Availability: e.Availability, MyAnswer: v.calendar.AnswerOf(v.email, e.ID),
	}
	if e.End != e.Start {
		c.End = e.End
	}
	c.When = v.timing(e.Start, e.End)
	if until, ok := v.daysAway(e.End); ok {
		c.Past = until < 0
	}
	c.Link = eventLink(e)
	for _, s := range e.MinePeople {
		words := s.Name
		if s.Mine {
			words = "you"
		}
		if s.Note != "" {
			words += " (" + s.Note + ")"
		}
		c.Household = append(c.Household, words)
	}
	if e.Mine != "" && len(c.Household) == 0 {
		c.Household = []string{e.Mine}
	}
	return c
}

func eventLink(e *calendar.Event) string {
	app, path := calendar.Page(e)
	return appBases[app] + path
}

var calendarEvents = tool{
	name:        "calendar_events",
	description: "Events from the school calendar (Helios When) in a date range, the other apps' parties and HCA events folded in, each with the viewer's own standing. Without dates, the next two weeks; with a query, the whole calendar. mine narrows to the viewer's default view: their classrooms and the categories on by default, the way the calendar opens for them.",
	words:       "Looking at the calendar",
	properties: map[string]any{
		"from":      str("First day, like 2026-09-24. Today unless said."),
		"to":        str("Last day, like 2026-10-08. Two weeks after from unless said."),
		"query":     str("Words to find in a title, description or keywords."),
		"classroom": str("Only events for this classroom (and events for everyone)."),
		"tag":       str("Only events filed under this category or classroom tag."),
		"mine":      boolean("Only what the viewer's default view of the calendar shows."),
		"limit":     integer("How many to return, 40 unless said, 80 at most."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			From, To, Query, Classroom, Tag string
			Mine                            bool
			Limit                           int
		}](input)
		if err != nil {
			return nil, err
		}
		today := time.Date(v.now.Year(), v.now.Month(), v.now.Day(), 0, 0, 0, 0, calendar.Location)
		from, to := today, today.AddDate(0, 0, 14)
		if in.From != "" {
			if from, err = date(in.From); err != nil {
				return nil, err
			}
			to = from.AddDate(0, 0, 14)
		}
		if in.To != "" {
			if to, err = date(in.To); err != nil {
				return nil, err
			}
		}
		if in.Query != "" && in.From == "" && in.To == "" {
			from, to = time.Time{}, today.AddDate(10, 0, 0)
		}
		if to.Before(from) {
			return nil, fmt.Errorf("the range ends before it starts")
		}
		var classrooms, tags []string
		if in.Mine {
			classrooms, tags = v.calendar.ViewOf(v.sources.CalendarDirectory, v.email)
		}
		limit := limitOf(in.Limit, 40, 80)
		out := []eventCard{}
		total := 0
		for _, e := range v.calendar.EventsFor(v.sources.CalendarDirectory, v.email, v.sources.Linked(v.email)) {
			start, end := eventSpan(e)
			if end.Before(from) || start.After(to.AddDate(0, 0, 1).Add(-time.Second)) {
				continue
			}
			if in.Query != "" && !contains(e.Title, in.Query) && !contains(e.Description, in.Query) && !contains(strings.Join(e.Keywords, " "), in.Query) {
				continue
			}
			if in.Classroom != "" && len(e.Classrooms) > 0 && !slices.ContainsFunc(e.Classrooms, func(c string) bool { return strings.EqualFold(c, in.Classroom) }) {
				continue
			}
			if in.Tag != "" && !slices.ContainsFunc(e.Tags, func(t string) bool { return strings.EqualFold(t, in.Tag) }) {
				continue
			}
			if in.Mine && !admits(v.calendar, e, classrooms, tags) {
				continue
			}
			total++
			if len(out) < limit {
				out = append(out, v.eventCard(e))
			}
		}
		return map[string]any{"today": today.Format(calendar.DateFormat), "from": from.Format(calendar.DateFormat), "to": to.Format(calendar.DateFormat), "events": out, "matched": total, "shown": len(out)}, nil
	},
}

// eventSpan is an event's first and last moment, from the cells.
func eventSpan(e *calendar.Event) (time.Time, time.Time) {
	parse := func(cell string) time.Time {
		if t, err := time.ParseInLocation(calendar.DateTimeFormat, cell, calendar.Location); err == nil {
			return t
		}
		t, _ := time.ParseInLocation(calendar.DateFormat, cell, calendar.Location)
		return t
	}
	start, end := parse(e.Start), parse(e.End)
	if e.AllDay {
		end = end.AddDate(0, 0, 1).Add(-time.Second)
	}
	return start, end
}

// admits is the calendar's own first-view rule: an event reaches a view
// when one of its classrooms is among the view's (or it has none) and one
// of its categories is on (or it has none).
func admits(m *calendar.Model, e *calendar.Event, classrooms, tags []string) bool {
	if len(e.Classrooms) > 0 && !overlaps(e.Classrooms, classrooms) {
		return false
	}
	categories := []string{}
	for _, t := range e.Tags {
		if !slices.Contains(m.Roster.Names(), t) {
			categories = append(categories, t)
		}
	}
	return len(categories) == 0 || overlaps(categories, tags)
}

func overlaps(a, b []string) bool {
	for _, item := range a {
		if slices.Contains(b, item) {
			return true
		}
	}
	return false
}

var dayPlan = tool{
	name:        "day_plan",
	description: "What kind of school day a date is, classroom by classroom: Regular, Early Dismissal, No School and so on with the day's hours, and the all-day events that set it. Today unless a date is given; the viewer's own classrooms unless one is named, every classroom for someone with none.",
	words:       "Checking the day plan",
	properties: map[string]any{
		"date":      str("The day, like 2026-09-24. Today unless said."),
		"classroom": str("One classroom, else the viewer's own."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Date, Classroom string }](input)
		if err != nil {
			return nil, err
		}
		day := time.Date(v.now.Year(), v.now.Month(), v.now.Day(), 0, 0, 0, 0, calendar.Location)
		if in.Date != "" {
			if day, err = date(in.Date); err != nil {
				return nil, err
			}
		}
		classrooms := []string{}
		if in.Classroom != "" {
			for _, name := range v.calendar.Roster.Names() {
				if contains(name, in.Classroom) {
					classrooms = append(classrooms, name)
				}
			}
			if len(classrooms) == 0 {
				return nil, fmt.Errorf("there is no classroom called %q", in.Classroom)
			}
		} else {
			classrooms, _ = v.calendar.ViewOf(v.sources.CalendarDirectory, v.email)
		}
		key := day.Format(calendar.DateFormat)
		plans := []map[string]any{}
		for _, c := range classrooms {
			dt, ok := v.calendar.Plan(key, c)
			if !ok {
				plans = append(plans, map[string]any{"classroom": c, "dayType": "outside the school year, or a weekend"})
				continue
			}
			plans = append(plans, map[string]any{"classroom": c, "dayType": dt.Name, "blocks": dt.Blocks})
		}
		setting := []eventCard{}
		for _, e := range v.calendar.Events {
			if e.AllDay && e.DayType != "" && slices.Contains(e.Dates, key) {
				setting = append(setting, v.eventCard(e))
			}
		}
		year := calendar.SchoolYear(day)
		var span any
		for _, y := range v.calendar.Years {
			if y.Label == year {
				span = y
			}
		}
		return map[string]any{"date": key, "weekday": day.Weekday().String(), "schoolYear": year, "yearSpan": span, "classrooms": plans, "setBy": setting}, nil
	},
}
