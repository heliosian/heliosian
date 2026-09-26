package team

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
)

// WidgetItem is one row of the home page's HCA-Team widget: the thing, the
// event it sits under, the day it happens (Start, YYYY-MM-DD, taken from the
// nearest dated ancestor for a role with no date of its own) or its free
// timing ("All Year"), and its page on the portal; for one of the viewer's
// sign-ups their position, for one needing people what it still wants
// (openNote).
type WidgetItem struct {
	Title    string `json:"title"`
	Under    string `json:"under,omitempty"`
	Start    string `json:"start,omitempty"`
	Timing   string `json:"timing,omitempty"`
	Position string `json:"position,omitempty"`
	Note     string `json:"note,omitempty"`
	Path     string `json:"path"`
	// Image is its picture, a path on the portal's host: its own, else the
	// nearest one above it - a role wears its event's.
	Image string `json:"image,omitempty"`
}

// Widget is what the home page's HCA-Team widget shows one person: what
// they are signed up for that is still ahead, what needs people, and what
// an admin has marked a priority.
type Widget struct {
	Mine     []WidgetItem `json:"mine"`
	Open     []WidgetItem `json:"open"`
	Priority []WidgetItem `json:"priority"`
}

// widgetOpen is how many things needing people the widget offers, its
// first few shown until More.
const widgetOpen = 12

// Widget reads this school year for one person: every activity, at any
// depth, they are on and that has not passed or been marked done, dated
// ones soonest first and the undated after; the things the portal's
// Volunteers needed list names (needs) that they are not already on, the
// first few; and every activity, at any depth, an admin marked a priority
// that still wants people and that they are not on, in the same order.
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
			if !ok || a.Year != year || a.Status == StatusDone || !m.VisibleTo(a, access.Viewer{Email: email}) {
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

// byDay puts dated rows soonest first and the undated after.
func byDay(x, y WidgetItem) int {
	if (x.Start == "") != (y.Start == "") {
		if x.Start == "" {
			return 1
		}
		return -1
	}
	return strings.Compare(x.Start, y.Start)
}

// wanted says an activity, at any depth, still wants people this school
// year: open, visible to everyone, not passed, not marked complete, and not
// full where it counts its spots - as the Volunteers needed list (needs)
// has it for the top level.
func (m *Model) wanted(a *Activity, year, today string) bool {
	if a.Year != year || a.Status != StatusOpen || !m.VisibleTo(a, access.Viewer{}) || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
		return false
	}
	last := lastDay(timed(m, a))
	return last == "" || last >= today
}

// widgetItem is an activity as a widget row, without the viewer's part.
func (m *Model) widgetItem(a *Activity) WidgetItem {
	dated := timed(m, a)
	image := ""
	for n := a; n != nil && image == ""; n = m.byID[n.Parent] {
		image = n.ImageURL
	}
	return WidgetItem{Title: a.Title, Under: lineage(m, a), Start: dayOf(dated.Start), Timing: dated.Timing, Path: m.PathOf(a), Image: image}
}

// dayOf is the date part of a start or end cell.
func dayOf(cell string) string {
	return cell[:min(len(cell), len(DateFormat))]
}

// lastDay is the last day an activity runs, its end's or else its start's.
func lastDay(a *Activity) string {
	if a.End != "" {
		return dayOf(a.End)
	}
	return dayOf(a.Start)
}

// openNote is what a thing needing people still wants, as the widget's
// second line says it: the co-chair when one is wanted, else the first of
// its open roles with spots to fill; and how many volunteers it still
// needs, over its own spots and every open role's under it - "Tech Setup ·
// 4 volunteers needed", "Co-chair · 2 volunteers needed" - or, counting no
// spots at all, Co-chair wanted or Volunteers wanted.
func (m *Model) openNote(a *Activity) string {
	label, needed := "", 0
	if a.CoLeaderNeeded {
		label = "Co-chair"
	}
	var count func(n *Activity)
	count = func(n *Activity) {
		if n.Status != StatusOpen || n.VolunteersComplete || !m.VisibleTo(n, access.Viewer{}) {
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
