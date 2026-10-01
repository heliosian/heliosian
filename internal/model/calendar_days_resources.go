package model

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/api"
	"heliosian/internal/id"
)

const kindDayPlan = "day-plan"

type calendarTagResource struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	Role        string `json:"role,omitempty"`
	BuiltIn     bool   `json:"builtIn,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

type dayTypeResource struct {
	Name   string  `json:"name"`
	Role   string  `json:"role,omitempty"`
	Blocks []Block `json:"blocks"`
}

type dayPlanMe struct {
	Mine bool `json:"mine"`
}

type dayPlanResource struct {
	Date       string    `json:"date"`
	Weekday    string    `json:"weekday"`
	Classroom  string    `json:"classroom"`
	SchoolYear string    `json:"schoolYear"`
	YearStarts string    `json:"yearStarts,omitempty"`
	YearEnds   string    `json:"yearEnds,omitempty"`
	Me         dayPlanMe `json:"me"`
}

type dayPlanKey struct {
	date, classroom string
}

func (r calendarResources) calendarTags() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "calendar-tags",
		Shape: calendarTagResource{},
		Has:   func(m *Model, key string) bool { return m.Calendar.Tag(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			t := m.Calendar.Tag(key)
			if t == nil {
				return nil, false
			}
			return calendarTagResource{Name: t.Name, Description: t.Description, Group: t.Group, Role: t.Role, BuiltIn: t.BuiltIn, ImageURL: t.ImageURL}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, t := range m.Calendar.Tags {
				out = append(out, t.ID)
			}
			return out
		},
	}
}

func (r calendarResources) dayTypes() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "day-types",
		Shape: dayTypeResource{},
		Has:   func(m *Model, key string) bool { return m.Calendar.DayType(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			d := m.Calendar.DayType(key)
			if d == nil {
				return nil, false
			}
			return dayTypeResource{Name: d.Name, Role: d.Role, Blocks: d.Blocks}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, d := range m.Calendar.DayTypes {
				out = append(out, d.ID)
			}
			return out
		},
	}
}

func (a calendarApp) dayPlanIndex() ([]string, map[string]dayPlanKey) {
	s := a.pinned.scope
	s.dayPlansOnce.Do(func() {
		cal := a.model()
		dates := []string{}
		for date := range cal.Days {
			dates = append(dates, date)
		}
		slices.Sort(dates)
		s.dayPlanOrder, s.dayPlans = []string{}, map[string]dayPlanKey{}
		for _, date := range dates {
			for _, classroom := range cal.Roster.Names() {
				if _, ok := cal.Days[date][classroom]; !ok {
					continue
				}
				key := id.Of(a.idKey, kindDayPlan, date+"\x00"+classroom)
				s.dayPlanOrder = append(s.dayPlanOrder, key)
				s.dayPlans[key] = dayPlanKey{date: date, classroom: classroom}
			}
		}
	})
	return s.dayPlanOrder, s.dayPlans
}

func (a calendarApp) dayPlan(key string) (dayPlanKey, bool) {
	_, plans := a.dayPlanIndex()
	k, ok := plans[key]
	return k, ok
}

func (r calendarResources) dayPlanWhere(keep func(a calendarApp, q api.Query, k dayPlanKey) bool) func(*Model, api.Query) func(string) bool {
	return func(m *Model, q api.Query) func(string) bool {
		a := r.app.at(m)
		return func(key string) bool {
			k, ok := a.dayPlan(key)
			return ok && keep(a, q, k)
		}
	}
}

func (r calendarResources) dayPlans() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "day-plans",
		Shape: dayPlanResource{},
		Has: func(m *Model, key string) bool {
			_, ok := r.app.at(m).dayPlan(key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			k, ok := a.dayPlan(key)
			if !ok {
				return nil, false
			}
			day, err := time.ParseInLocation(DateFormat, k.date, Location)
			if err != nil {
				return nil, false
			}
			out := dayPlanResource{Date: k.date, Weekday: day.Weekday().String(), Classroom: k.classroom, SchoolYear: SchoolYear(day)}
			for _, y := range a.model().Years {
				if y.Label == out.SchoolYear {
					out.YearStarts, out.YearEnds = y.FirstDay, y.LastDay
				}
			}
			mine, _ := a.model().ViewOf(a.directory(), q.Actor.Email)
			out.Me.Mine = slices.Contains(mine, k.classroom)
			return out, true
		},
		List: func(m *Model, _ api.Query) []string {
			order, _ := r.app.at(m).dayPlanIndex()
			return order
		},
		Relations: map[string]api.Relation[*Model]{
			"day-type": {Type: "day-types", List: func(m *Model, _ api.Query, key string) []string {
				a := r.app.at(m)
				k, ok := a.dayPlan(key)
				if !ok {
					return nil
				}
				return []string{a.model().Days[k.date][k.classroom]}
			}},
			"set-by": {Type: "events", Many: true, List: func(m *Model, q api.Query, key string) []string {
				a := r.app.at(m)
				k, ok := a.dayPlan(key)
				if !ok {
					return nil
				}
				events, _ := a.viewerEvents(q)
				out := []string{}
				for _, e := range events {
					if e.AllDay && e.DayType != "" && slices.Contains(e.Dates, k.date) {
						out = append(out, e.ID)
					}
				}
				return out
			}},
		},
		Filters: map[string]api.Filter[*Model]{
			"date": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				date, err := dateParam("date", value)
				if err != nil {
					return nil, err
				}
				return r.dayPlanWhere(func(_ calendarApp, _ api.Query, k dayPlanKey) bool { return k.date == date })(m, q), nil
			},
			"classroom": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				return r.dayPlanWhere(func(_ calendarApp, _ api.Query, k dayPlanKey) bool {
					return strings.EqualFold(k.classroom, strings.TrimSpace(value))
				})(m, q), nil
			},
		},
	}
}
