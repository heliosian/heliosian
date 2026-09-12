package events

import (
	"sort"
	"time"
)

// Upcoming is an event as Heliosian's front page lists it: enough to show a
// card and send the reader across to the portal.
type Upcoming struct {
	Title string `json:"title"`
	// Path is the event's page in the portal, relative to its origin.
	Path string `json:"path"`
	// Start is the day (YYYY-MM-DD) for the date stamp; When the fuller line
	// the share card uses ("Thursday, September 24 · 4:00 PM"). StartAt and
	// EndAt are the sheet's own cells (a day, or a day with a time) and
	// Location and Description the rest of what a calendar entry wants.
	Start       string `json:"start"`
	When        string `json:"when"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// Upcoming lists the open, dated root activities still ahead of today, soonest
// first, at most limit of them - what the front page calls Upcoming Events.
// Undated things are left out: without a day there is nothing to be upcoming.
func (m *Model) Upcoming(now time.Time, limit int) []Upcoming {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	type dated struct {
		at time.Time
		a  *Activity
	}
	var ahead []dated
	for _, a := range m.Activities {
		if a.Status != StatusOpen || a.Start == "" {
			continue
		}
		start, err := ParseWhen(a.Start)
		if err != nil {
			continue
		}
		last := start
		if end, err := ParseWhen(a.End); err == nil && end.After(start) {
			last = end
		}
		if time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC).Before(today) {
			continue
		}
		ahead = append(ahead, dated{start, a})
	}
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].at.Before(ahead[j].at) })
	out := []Upcoming{}
	for _, d := range ahead {
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, Upcoming{
			Title: d.a.Title, Path: m.PathOf(d.a), Start: d.at.Format(DateFormat), When: when(d.a),
			StartAt: d.a.Start, EndAt: d.a.End, Location: d.a.Location, Description: blurb(d.a), ImageURL: d.a.ImageURL,
		})
	}
	return out
}

// Upcoming is the model's list as of now, for the front page.
func (c *Cache) Upcoming(limit int) []Upcoming {
	return c.Model().Upcoming(time.Now().In(local), limit)
}
