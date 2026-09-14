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
	// ID is the event's, for an answer to name it.
	ID    string `json:"id"`
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
	// Answer is the viewer's word on it: yes, no, or nothing yet.
	Answer string `json:"answer,omitempty"`
	// People is everyone in the household with a part in a linked event.
	People []Standing `json:"people,omitempty"`
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

// mineWords is the household's standing in words: the viewer's own as
// "you", another member's by name - "Sam is going", "Sam and Alex are
// waitlisted", "Sam signed up".
func mineWords(e *Event) string {
	names := joinNames(e.MineWho)
	switch e.Mine {
	case MineWaitlisted:
		if names != "" {
			return names + " " + isAre(e.MineWho) + " waitlisted"
		}
		return "Waitlisted"
	case MineGoing:
		if e.Source == SourceCelebrate {
			if names != "" {
				return names + " " + isAre(e.MineWho) + " going"
			}
			return "You're going"
		}
		if names != "" {
			return names + " signed up"
		}
		return "Signed up"
	}
	return ""
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func isAre(names []string) string {
	if len(names) > 1 {
		return "are"
	}
	return "is"
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
// default calendar - the first in their rail - where it is a saved one;
// else My Heliosian - the view they saved in place of
// both, if any, else their own classrooms - their children's, their own as
// a teacher, every classroom for someone with none - and the categories
// on by default.
func (m *Model) viewOf(directory Directory, email string) (classrooms, tags []string) {
	if chosen := m.DefaultCalendar(email); chosen != nil {
		return m.feedView(chosen)
	}
	return m.myHeliosianView(directory, email)
}

// viewUnder is the view one of the person's saved calendars gives, by
// token - or My Heliosian by its token - and their default view (viewOf)
// for a blank token or one that is not theirs.
func (m *Model) viewUnder(directory Directory, email, token string) (classrooms, tags []string) {
	if token == MyHeliosianToken {
		return m.myHeliosianView(directory, email)
	}
	if f := m.Feed(token); f != nil && token != "" && normalizeEmail(f.Email) == normalizeEmail(email) {
		return m.feedView(f)
	}
	return m.viewOf(directory, email)
}

// myHeliosianView is My Heliosian for one person: the calendar's own
// defaults, or the view they saved in their place.
func (m *Model) myHeliosianView(directory Directory, email string) (classrooms, tags []string) {
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
	// A saved view stands in for both - a Settings row that holds one,
	// rather than only a default calendar.
	if saved, ok := m.Settings[normalizeEmail(email)]; ok && (len(saved.Classrooms) > 0 || len(saved.Tags) > 0) {
		if len(saved.Classrooms) > 0 {
			classrooms = saved.Classrooms
		}
		tags = saved.Tags
	}
	return classrooms, tags
}

// feedView is a saved calendar's filter as a view: every classroom or
// tag where it carries no filter.
func (m *Model) feedView(f *Feed) (classrooms, tags []string) {
	classrooms, tags = f.Classrooms, f.Tags
	if len(classrooms) == 0 {
		classrooms = m.Roster.Names()
	}
	if len(tags) == 0 {
		for _, t := range append(append([]Tag{}, m.Tags...), builtinTags...) {
			tags = append(tags, t.Name)
		}
	}
	return classrooms, tags
}

// MyCalendars are a person's saved calendars in the rail's order, with
// My Heliosian among them at its position.
func (m *Model) MyCalendars(email string) []Feed {
	out := []Feed{}
	for _, f := range m.Feeds {
		if normalizeEmail(f.Email) == normalizeEmail(email) {
			out = append(out, f)
		}
	}
	home := m.MyHeliosian(email)
	at := min(max(home.Position, 0), len(out))
	return append(out[:at:at], append([]Feed{home}, out[at:]...)...)
}

// DefaultCalendar is a person's default calendar - the first in their
// rail, the one the page opens to and Upcoming is read under - as a saved
// calendar, or nil when it is My Heliosian.
func (m *Model) DefaultCalendar(email string) *Feed {
	mine := m.MyCalendars(email)
	if len(mine) == 0 || mine[0].Locked {
		return nil
	}
	return m.Feed(mine[0].Token)
}

// card is one event as the front page takes it.
func (m *Model) card(e *Event) Upcoming {
	u := Upcoming{
		ID: e.ID, Title: e.Title, Path: eventPath(e), Start: e.start.Format(DateFormat), When: when(e),
		StartAt: e.Start, EndAt: e.End, Location: e.Location, Description: blurb(e),
		Image: "/" + m.pictureOf(e), ImageApp: appCalendar,
	}
	if e.Link != "" {
		u.Link, u.LinkApp = e.Link, linkedApp(e)
		u.Mine, u.Availability, u.People = e.Mine, e.Availability, e.MinePeople
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
	return m.UpcomingUnder(directory, email, linked, now, limit, "")
}

// UpcomingUnder is Upcoming read under one of the person's saved calendars
// by token, or My Heliosian by its token, rather than their default - the
// front page's picker - or under the default for a blank or unknown token.
func (m *Model) UpcomingUnder(directory Directory, email string, linked []Linked, now time.Time, limit int, token string) []Upcoming {
	classrooms, tags := m.viewUnder(directory, email, token)
	today := now.Format(DateFormat)
	out := []Upcoming{}
	for _, e := range m.eventsFor(email, linked) {
		answer := m.AnswerOf(email, e.ID)
		if e.end.Format(DateFormat) < today || answer == AnswerHidden || !admits(m, e, classrooms, tags) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		u := m.card(e)
		u.Answer = answer
		out = append(out, u)
	}
	return out
}
