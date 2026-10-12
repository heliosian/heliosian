package model

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/sharecard"
)

func activityPreviewable(m *Activities, a *Activity) bool {
	return a != nil && (a.Status == StatusOpen || a.Status == StatusDone) && m.VisibleTo(a, access.Actor{})
}

func timed(m *Activities, a *Activity) *Activity {
	for n := a; n != nil; n = m.byID[n.Parent] {
		if n.Start != "" || n.Timing != "" {
			return n
		}
	}
	return a
}

func lineage(m *Activities, a *Activity) string {
	names := []string{}
	for n := m.byID[a.Parent]; n != nil; n = m.byID[n.Parent] {
		names = append([]string{n.Title}, names...)
	}
	return strings.Join(names, " › ")
}

func whenText(a *Activity) string {
	day, hours := activityWhenLines(a)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

func needs(m *Activities, at time.Time) []*Activity {
	year, today := ActivityYear(at), at.Format(DateFormat)
	dated, undated := []*Activity{}, []*Activity{}
	for _, a := range m.Activities {
		if a.Year != year || a.Status != StatusOpen || !m.VisibleTo(a, access.Actor{}) || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
			continue
		}
		if a.Start == "" {
			undated = append(undated, a)
		} else if a.Start[:min(len(a.Start), len(DateFormat))] >= today {
			dated = append(dated, a)
		}
	}
	slices.SortStableFunc(dated, func(x, y *Activity) int { return strings.Compare(x.Start, y.Start) })
	return append(dated, undated...)
}

func activityWhenLines(a *Activity) (string, string) {
	if a.Timing != "" {
		return a.Timing, ""
	}
	if days, hours := spanLines(a); days != "" {
		return days, hours
	}
	start, err := time.ParseInLocation(DateTimeFormat, a.Start, Location)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, a.Start, Location); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, a.End, Location)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func spanLines(a *Activity) (string, string) {
	start, err := cells.When(a.Start)
	if err != nil {
		return "", ""
	}
	end, err := cells.When(a.End)
	if err != nil || (end.Year() == start.Year() && end.YearDay() == start.YearDay()) {
		return "", ""
	}
	days := start.Format("Monday, January 2") + " – " + end.Format("Monday, January 2")
	if len(a.Start) > len(DateFormat) && len(a.End) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM") + " – " + end.Format("Mon 3:04 PM")
	}
	if len(a.Start) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM")
	}
	return days, ""
}
