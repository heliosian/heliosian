package calendar

import (
	"net/url"
	"slices"
	"strings"
	"time"
)

// Card is an event as another app lists it - Heliosian's Upcoming Events,
// the rail's month: enough to show a card, add it to a calendar, and send
// the reader across, to the event's page here and, for an event another app
// runs, to that app. It is the one card shape; Heliosian keeps no copy of
// it, so a field added here reaches the front page without a second struct
// learning about it.
type Card struct {
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
	// Dates are the days the rail's month shows it under (Event.Dates), which
	// for a day type's span written across a weekend is not every day between
	// StartAt and EndAt.
	Dates []string `json:"dates"`
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
	// Answer is the viewer's word on it: yes, no, or nothing yet; Invited,
	// that it was sent to them, so a card with no answer asks for one.
	Answer  string `json:"answer,omitempty"`
	Invited bool   `json:"invited,omitempty"`
	// People is everyone in the household with a part in a linked event.
	People []Standing `json:"people,omitempty"`
}

// The app keys the front page reaches origins by, as the toolbar names them.
const (
	appCalendar  = "calendar"
	appCelebrate = "celebrate"
	appTeam      = "team"
)

// callWords is the way into a linked event, in words, for a household with
// no standing of its own yet.
var callWords = map[string]string{"available": "Get tickets", "waitlist": "Join the waitlist", "sold-out": "Sold out", "open": "Join", "full": "Full"}

// standing writes what a linked event offers into the event itself, once, so
// every reader says the same words (Event.MineWords, Event.Call).
func standing(e *Event) {
	e.MineWords = mineWords(e)
	if e.Call = e.MineWords; e.Call == "" {
		e.Call = callWords[e.Availability]
	}
}

// mineWords is the household's standing in words: the viewer's own as
// "you", another member's by name - "Sam has a ticket", "Sam and Alex have
// tickets", "Sam and Alex are waitlisted", "Sam signed up". A party says
// who holds a ticket rather than who is going: a ticket is what Celebrate
// knows.
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
			switch {
			case len(e.MineWho) > 1:
				return names + " have tickets"
			case names != "":
				return names + " has a ticket"
			}
			return "You have a ticket"
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

// EventPath is the event's page here, each segment of a linked event's id
// escaped on its own, as the page itself builds it.
func EventPath(e *Event) string {
	// A friendly address an admin gave it wins over the import key.
	if e.Address != "" {
		return "/e/" + url.PathEscape(e.Address)
	}
	parts := strings.Split(e.ID, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/e/" + strings.Join(parts, "/")
}

// Page is where an event is looked at: the app that runs a linked one and
// its path there, else the calendar and the event's own page.
func Page(e *Event) (app, path string) {
	if e.Link != "" {
		return linkedApp(e), e.Link
	}
	return appCalendar, EventPath(e)
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

// ViewOf is viewOf for another app reading the calendar as one person: the
// classrooms and the categories their default view admits.
func (m *Model) ViewOf(directory Directory, email string) (classrooms, tags []string) {
	return m.viewOf(directory, email)
}

// EventsFor is eventsFor for another app: every event as one viewer stands
// with them, the linked ones folded in.
func (m *Model) EventsFor(directory Directory, email string, linked []Linked) []*Event {
	return m.eventsFor(directory, email, linked)
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

// PartiesFor is every Helios Celebrate party still ahead for one viewer,
// soonest first, each as the card the front page takes - its household's
// standing and the way in as the calendar words them - for Heliosian's
// Celebrate widget. It is every party, not only those the viewer's calendar
// filters admit.
func (m *Model) PartiesFor(directory Directory, email string, linked []Linked, now time.Time) []Card {
	today := now.Format(DateFormat)
	out := []Card{}
	for _, e := range m.eventsFor(directory, email, linked) {
		if e.Source != SourceCelebrate || e.end.Format(DateFormat) < today {
			continue
		}
		u := m.card(e)
		u.Answer = m.AnswerOf(email, e.ID)
		out = append(out, u)
	}
	return out
}

// card is one event as the front page takes it.
func (m *Model) card(e *Event) Card {
	u := Card{
		ID: e.ID, Title: e.Title, Path: EventPath(e), Start: e.start.Format(DateFormat), When: when(e),
		StartAt: e.Start, EndAt: e.End, Dates: e.Dates, Location: e.Location, Description: blurb(e),
		Image: "/" + m.pictureOf(e), ImageApp: appCalendar, Invited: e.Invited,
	}
	if e.Link != "" {
		u.Link, u.LinkApp = e.Link, linkedApp(e)
		u.Mine, u.Availability, u.People, u.Call = e.Mine, e.Availability, e.MinePeople, e.Call
		if e.Image != "" {
			u.ImageApp = linkedApp(e)
		}
	}
	return u
}

// Upcoming lists what is ahead for one viewer, as the calendar page first
// shows it (viewOf): every event from today on that the view admits, the
// other apps' events folded in, soonest first, at most limit of them.
func (m *Model) Upcoming(directory Directory, email string, linked []Linked, now time.Time, limit int) []Card {
	return m.UpcomingUnder(directory, email, linked, now, limit, "")
}

// UpcomingUnder is Upcoming read under one of the person's saved calendars
// by token, or My Heliosian by its token, rather than their default - the
// front page's picker - or under the default for a blank or unknown token.
func (m *Model) UpcomingUnder(directory Directory, email string, linked []Linked, now time.Time, limit int, token string) []Card {
	classrooms, tags := m.viewUnder(directory, email, token)
	today := now.Format(DateFormat)
	out := []Card{}
	for _, e := range m.eventsFor(directory, email, linked) {
		answer := m.AnswerOf(email, e.ID)
		// A yes reaches across classrooms, so long as Going is in view, and
		// an invitation does whatever is in view; a no leaves the list, as
		// hiding does - the month still shows it, in gray.
		going := answer == AnswerYes && slices.Contains(tags, TagGoing)
		if e.end.Format(DateFormat) < today || answer == AnswerHidden || answer == AnswerNo || !(going || e.Invited || admits(m, e, classrooms, tags)) {
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
