package calendar

import (
	"net/url"
	"strings"
	"time"
)

// Upcoming is an event as Heliosian's front page lists it: enough to show a
// card, add it to a calendar, and send the reader across - to the event's
// page here, and for an event another app runs, to that app.
type Upcoming struct {
	Title string `json:"title"`
	// Path is the event's page on the calendar, relative to its origin.
	Path string `json:"path"`
	// Start is the day (YYYY-MM-DD) for the date stamp; When the fuller line
	// the share card uses ("Thursday, September 24 · 4:00 – 6:00 PM"). StartAt
	// and EndAt are the sheet's own cells (a day, or a day with a time) and
	// Location and Description the rest of what a calendar entry wants.
	Start       string `json:"start"`
	When        string `json:"when"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	// Image is the picture the event's page here wears, as a path on
	// ImageApp's host: the calendar's own, or the app that runs a linked
	// event, whose picture is fetched from there.
	Image    string `json:"image,omitempty"`
	ImageApp string `json:"imageApp,omitempty"`
	// Link is a linked event's page on LinkApp, the app that runs it; Call
	// what the reader can do there now, as the calendar's own rows say it -
	// the household's standing first, else the way in, nothing once it has
	// passed or closed; Mine and Availability the standing and the way in
	// themselves, for the card to dress the call by.
	Link         string `json:"link,omitempty"`
	LinkApp      string `json:"linkApp,omitempty"`
	Call         string `json:"call,omitempty"`
	Mine         string `json:"mine,omitempty"`
	Availability string `json:"availability,omitempty"`
}

// The app keys the front page reaches origins by, as the toolbar names them.
const (
	appCalendar  = "calendar"
	appCelebrate = "celebrate"
	appTeam      = "team"
)

// callWords and mineWords are what a linked event's pill says on the
// calendar's own rows (call in web/calendar/state.js), kept in step so the
// front page's button says the same.
var callWords = map[string]string{"available": "Get tickets", "waitlist": "Join the waitlist", "sold-out": "Sold out", "open": "Join", "full": "Full"}

func mineWords(e *Event) string {
	switch e.Mine {
	case MineWaitlisted:
		return "Waitlisted"
	case MineGoing:
		if e.Source == SourceCelebrate {
			return "You're going"
		}
		return "Signed up"
	}
	return ""
}

// linkedApp is the app that runs a linked event: Celebrate for a party,
// HCA-Team for everything else with a way in - the school's own listing of
// an HCA event included, which keeps the school as its source once folded.
func linkedApp(e *Event) string {
	if e.Source == SourceCelebrate {
		return appCelebrate
	}
	return appTeam
}

// eventPath is the event's page here, each segment of a linked event's id
// escaped on its own, as the page itself builds it.
func eventPath(e *Event) string {
	parts := strings.Split(e.ID, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/events/" + strings.Join(parts, "/")
}

// admits says whether the calendar page first shows an event to a viewer:
// one of its classrooms is among theirs (an event with none is everyone's),
// and one of its categories is on by default (an event with none shows) -
// the filters as they stand before the viewer has touched them.
func admits(model *Model, e *Event, classrooms, tags []string) bool {
	if len(e.Classrooms) > 0 && !overlaps(e.Classrooms, classrooms) {
		return false
	}
	categories := []string{}
	for _, t := range e.Tags {
		if !model.Roster.has(t) {
			categories = append(categories, t)
		}
	}
	return len(categories) == 0 || overlaps(categories, tags)
}

// viewOf is the filter the calendar page first shows a viewer under: their
// own classrooms - their children's, their own as a teacher, every classroom
// for someone with none - and the categories on by default.
func (m *Model) viewOf(directory Directory, email string) (classrooms, tags []string) {
	me, known := directory.Person(email)
	if !known {
		me = Person{Email: email}
	}
	classrooms = classroomsOf(m, me, students(directory, me))
	if len(classrooms) == 0 {
		classrooms = m.Roster.Names()
	}
	tags = []string{}
	for _, t := range append(append([]Tag{}, m.Tags...), builtinTags...) {
		if t.Default {
			tags = append(tags, t.Name)
		}
	}
	return classrooms, tags
}

// card is one event as the front page takes it.
func (m *Model) card(e *Event) Upcoming {
	u := Upcoming{
		Title: e.Title, Path: eventPath(e), Start: e.start.Format(DateFormat), When: when(e),
		StartAt: e.Start, EndAt: e.End, Location: e.Location, Description: blurb(e),
		Image: "/" + m.pictureOf(e), ImageApp: appCalendar,
	}
	if e.Link != "" {
		u.Link, u.LinkApp = e.Link, linkedApp(e)
		u.Mine, u.Availability = e.Mine, e.Availability
		if u.Call = mineWords(e); u.Call == "" {
			u.Call = callWords[e.Availability]
		}
		if e.Image != "" {
			u.ImageApp = linkedApp(e)
		}
	}
	return u
}

// Upcoming lists what is ahead for one viewer, as the calendar page first
// shows it (viewOf): every event from today on that the view admits, the
// other apps' events folded in, soonest first, at most limit of them.
func (m *Model) Upcoming(directory Directory, email string, linked []Linked, now time.Time, limit int) []Upcoming {
	classrooms, tags := m.viewOf(directory, email)
	today := now.Format(DateFormat)
	out := []Upcoming{}
	for _, e := range withLinked(m.Events, linked) {
		if e.end.Format(DateFormat) < today || !admits(m, e, classrooms, tags) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, m.card(e))
	}
	return out
}
