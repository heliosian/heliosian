package team

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
)

type WidgetItem struct {
	Title    string `json:"title"`
	Under    string `json:"under,omitempty"`
	Start    string `json:"start,omitempty"`
	Timing   string `json:"timing,omitempty"`
	Position string `json:"position,omitempty"`
	Note     string `json:"note,omitempty"`
	Path     string `json:"path"`
	Image    string `json:"image,omitempty"`
}

type Widget struct {
	Mine     []WidgetItem `json:"mine"`
	Open     []WidgetItem `json:"open"`
	Priority []WidgetItem `json:"priority"`
}

const widgetOpen = 12

func (c *Cache) Widget(email string, at time.Time) Widget {
	m := c.Model()
	year, today := SchoolYear(at), at.Format(DateFormat)
	out := Widget{Mine: []WidgetItem{}, Open: []WidgetItem{}, Priority: []WidgetItem{}}
	on := func(a *Activity) (Volunteer, bool) {
		i := slices.IndexFunc(a.Volunteers, func(v Volunteer) bool { return strings.EqualFold(v.Email, email) })
		if i < 0 {
			return Volunteer{}, false
		}
		return a.Volunteers[i], true
	}
	var walk func([]*Activity)
	walk = func(list []*Activity) {
		for _, a := range list {
			walk(a.Children)
			if a.Priority && m.wanted(a, year, today) {
				if _, ok := on(a); !ok {
					item := m.widgetItem(a)
					item.Note = m.openNote(a)
					out.Priority = append(out.Priority, item)
				}
			}
			v, ok := on(a)
			if !ok || a.Year != year || a.Status == StatusDone || !m.VisibleTo(a, access.Actor{Email: email}) {
				continue
			}
			item := m.widgetItem(a)
			if last := lastDay(timed(m, a)); last != "" && last < today {
				continue
			}
			item.Position = v.Position
			out.Mine = append(out.Mine, item)
		}
	}
	walk(m.Activities)
	slices.SortStableFunc(out.Mine, byDay)
	slices.SortStableFunc(out.Priority, byDay)
	for _, a := range needs(m, at) {
		if len(out.Open) == widgetOpen {
			break
		}
		if _, ok := on(a); ok {
			continue
		}
		item := m.widgetItem(a)
		item.Note = m.openNote(a)
		out.Open = append(out.Open, item)
	}
	return out
}

func byDay(x, y WidgetItem) int {
	if (x.Start == "") != (y.Start == "") {
		if x.Start == "" {
			return 1
		}
		return -1
	}
	return strings.Compare(x.Start, y.Start)
}

func (m *Model) wanted(a *Activity, year, today string) bool {
	if a.Year != year || a.Status != StatusOpen || !m.VisibleTo(a, access.Actor{}) || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
		return false
	}
	last := lastDay(timed(m, a))
	return last == "" || last >= today
}

func (m *Model) widgetItem(a *Activity) WidgetItem {
	dated := timed(m, a)
	image := ""
	for n := a; n != nil && image == ""; n = m.byID[n.Parent] {
		image = n.ImageURL
	}
	return WidgetItem{Title: a.Title, Under: lineage(m, a), Start: dayOf(dated.Start), Timing: dated.Timing, Path: m.PathOf(a), Image: image}
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

func (m *Model) openNote(a *Activity) string {
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
