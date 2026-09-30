package model

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
)

type activityWidget struct {
	mine     []*Activity
	needed   []*Activity
	priority []*Activity
}

const widgetNeeded = 12

func (m *Activities) volunteerOn(a *Activity, email string) (Volunteer, bool) {
	i := slices.IndexFunc(a.Volunteers, func(v Volunteer) bool { return strings.EqualFold(v.Email, email) })
	if i < 0 {
		return Volunteer{}, false
	}
	return a.Volunteers[i], true
}

func (m *Activities) widget(email string, at time.Time) activityWidget {
	year, today := ActivityYear(at), at.Format(DateFormat)
	out := activityWidget{mine: []*Activity{}, needed: []*Activity{}, priority: []*Activity{}}
	var walk func([]*Activity)
	walk = func(list []*Activity) {
		for _, a := range list {
			walk(a.Children)
			_, on := m.volunteerOn(a, email)
			if a.Priority && m.wanted(a, year, today) && !on {
				out.priority = append(out.priority, a)
			}
			if !on || a.Year != year || a.Status == StatusDone || !m.VisibleTo(a, access.Actor{Email: email}) {
				continue
			}
			if last := lastDay(timed(m, a)); last != "" && last < today {
				continue
			}
			out.mine = append(out.mine, a)
		}
	}
	walk(m.Activities)
	byDay := func(x, y *Activity) int {
		dx, dy := dayOf(timed(m, x).Start), dayOf(timed(m, y).Start)
		if (dx == "") != (dy == "") {
			if dx == "" {
				return 1
			}
			return -1
		}
		return strings.Compare(dx, dy)
	}
	slices.SortStableFunc(out.mine, byDay)
	slices.SortStableFunc(out.priority, byDay)
	for _, a := range needs(m, at) {
		if len(out.needed) == widgetNeeded {
			break
		}
		if _, on := m.volunteerOn(a, email); on {
			continue
		}
		out.needed = append(out.needed, a)
	}
	return out
}

func (m *Activities) wanted(a *Activity, year, today string) bool {
	if a.Year != year || a.Status != StatusOpen || !m.VisibleTo(a, access.Actor{}) || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
		return false
	}
	last := lastDay(timed(m, a))
	return last == "" || last >= today
}

func (m *Activities) picture(a *Activity) string {
	for n := a; n != nil; n = m.byID[n.Parent] {
		if n.ImageURL != "" {
			return n.ImageURL
		}
	}
	return ""
}

func dayOf(cell string) string {
	return cell[:min(len(cell), len(DateFormat))]
}

func lastDay(a *Activity) string {
	if a.End != "" {
		return dayOf(a.End)
	}
	return dayOf(a.Start)
}

func (m *Activities) openNote(a *Activity) string {
	label, needed := "", 0
	if a.CoLeaderNeeded {
		label = "Co-chair"
	}
	var count func(n *Activity)
	count = func(n *Activity) {
		if n.Status != StatusOpen || n.VolunteersComplete || !m.VisibleTo(n, access.Actor{}) {
			return
		}
		if left := n.Spots - len(n.Volunteers); n.Spots > 0 && left > 0 {
			needed += left
			if label == "" && n != a {
				label = n.Title
			}
		}
		for _, c := range n.Children {
			count(c)
		}
	}
	count(a)
	if needed == 0 && label != "" {
		return label + " wanted"
	}
	parts := []string{}
	if label != "" {
		parts = append(parts, label)
	}
	switch {
	case needed == 1:
		parts = append(parts, "1 volunteer needed")
	case needed > 1:
		parts = append(parts, strconv.Itoa(needed)+" volunteers needed")
	case label == "":
		parts = append(parts, "Volunteers wanted")
	}
	return strings.Join(parts, " · ")
}
