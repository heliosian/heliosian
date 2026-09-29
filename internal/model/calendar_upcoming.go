package model

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/mail"
)

type EventCard struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Path         string     `json:"path"`
	Start        string     `json:"start"`
	When         string     `json:"when"`
	StartAt      string     `json:"startAt"`
	EndAt        string     `json:"endAt,omitempty"`
	Location     string     `json:"location,omitempty"`
	Description  string     `json:"description,omitempty"`
	Dates        []string   `json:"dates"`
	Image        string     `json:"image,omitempty"`
	ImageApp     string     `json:"imageApp,omitempty"`
	Link         string     `json:"link,omitempty"`
	LinkApp      string     `json:"linkApp,omitempty"`
	Call         string     `json:"call,omitempty"`
	Mine         string     `json:"mine,omitempty"`
	Availability string     `json:"availability,omitempty"`
	Answer       string     `json:"answer,omitempty"`
	Invited      bool       `json:"invited,omitempty"`
	Hosted       bool       `json:"hosted,omitempty"`
	Cancelled    bool       `json:"cancelled,omitempty"`
	People       []Standing `json:"people,omitempty"`
}

const (
	appCalendar  = "when"
	appCelebrate = "celebrate"
	appTeam      = "team"
)

var callWords = map[string]string{"available": "Get tickets", "waitlist": "Join the waitlist", "sold-out": "Sold out", "open": "Join", "full": "Full"}

func standing(e *Event) {
	e.MineWords = mineWords(e)
	if e.Call = e.MineWords; e.Call == "" {
		e.Call = callWords[e.Availability]
	}
}

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

func linkedApp(e *Event) string {
	if e.Source == SourceCelebrate {
		return appCelebrate
	}
	return appTeam
}

func EventPath(e *Event) string {
	if e.Address != "" {
		return "/e/" + url.PathEscape(e.Address)
	}
	parts := strings.Split(e.ID, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/e/" + strings.Join(parts, "/")
}

func EventPage(e *Event) (app, path string) {
	if e.Link != "" {
		return linkedApp(e), e.Link
	}
	return appCalendar, EventPath(e)
}

func admits(model *Calendar, e *Event, classrooms, tags []string) bool {
	if len(e.Classrooms) > 0 && !overlaps(e.Classrooms, classrooms) {
		return false
	}
	categories := []string{}
	for _, t := range e.Tags {
		if model.Roster.byID(t) == nil {
			categories = append(categories, t)
		}
	}
	return len(categories) == 0 || overlaps(categories, tags)
}

func (m *Calendar) InView(e *Event, answer string, classrooms, tags []string) bool {
	if answer == AnswerHidden || answer == AnswerNo {
		return false
	}
	going := answer == AnswerYes && slices.Contains(tags, TagGoing)
	return going || e.Invited || admits(m, e, classrooms, tags)
}

func (m *Calendar) ViewOf(directory *Directory, email string) (classrooms, tags []string) {
	if chosen := m.DefaultCalendar(email); chosen != nil {
		return m.feedView(chosen)
	}
	return m.myHeliosianView(directory, email)
}

func (m *Calendar) viewUnder(directory *Directory, email, token string) (classrooms, tags []string) {
	if token == MyHeliosianToken {
		return m.myHeliosianView(directory, email)
	}
	if f := m.Feed(token); f != nil && token != "" && mail.Normalize(f.Email) == mail.Normalize(email) {
		return m.feedView(f)
	}
	return m.ViewOf(directory, email)
}

func (m *Calendar) myHeliosianView(directory *Directory, email string) (classrooms, tags []string) {
	classrooms = classroomsOf(m, directory.Person(email), students(directory, email))
	if len(classrooms) == 0 {
		classrooms = m.Roster.Names()
	}
	tags = []string{}
	for _, t := range m.Tags {
		if t.Default {
			tags = append(tags, t.ID)
		}
	}
	if saved, ok := m.Settings[mail.Normalize(email)]; ok && (len(saved.Classrooms) > 0 || len(saved.Tags) > 0) {
		if len(saved.Classrooms) > 0 {
			classrooms = saved.Classrooms
		}
		tags = saved.Tags
	}
	return classrooms, tags
}

func (m *Calendar) feedView(f *Feed) (classrooms, tags []string) {
	classrooms, tags = f.Classrooms, f.Tags
	if len(classrooms) == 0 {
		classrooms = m.Roster.Names()
	}
	if len(tags) == 0 {
		for _, t := range m.Tags {
			tags = append(tags, t.ID)
		}
	}
	return classrooms, tags
}

func (m *Calendar) MyCalendars(email string) []Feed {
	out := []Feed{}
	for _, f := range m.Feeds {
		if mail.Normalize(f.Email) == mail.Normalize(email) {
			out = append(out, f)
		}
	}
	home := m.MyHeliosian(email)
	at := min(max(home.Position, 0), len(out))
	return append(out[:at:at], append([]Feed{home}, out[at:]...)...)
}

func (m *Calendar) DefaultCalendar(email string) *Feed {
	mine := m.MyCalendars(email)
	if len(mine) == 0 || mine[0].Locked {
		return nil
	}
	return m.Feed(mine[0].Token)
}

func (m *Calendar) PartiesFor(directory *Directory, email string, linked []Linked, now time.Time) []EventCard {
	today := now.Format(DateFormat)
	out := []EventCard{}
	for _, e := range m.eventsFor(directory, email, linked) {
		if e.Source != SourceCelebrate || e.end.Format(DateFormat) < today {
			continue
		}
		out = append(out, m.card(directory, email, e))
	}
	return out
}

func (m *Calendar) card(directory *Directory, email string, e *Event) EventCard {
	u := EventCard{
		ID: e.ID, Title: e.Title, Path: EventPath(e), Start: e.start.Format(DateFormat), When: when(e),
		StartAt: e.Start, EndAt: e.End, Dates: e.Dates, Location: e.Location, Description: blurb(e),
		Image: "/" + m.pictureOf(e), ImageApp: appCalendar, Invited: e.Invited,
		Hosted: m.hostedBy(directory, email, e), Cancelled: e.Cancelled, Answer: m.AnswerOf(email, e.ID),
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

func (m *Calendar) UpcomingUnder(directory *Directory, email string, linked []Linked, now time.Time, limit int, token string) []EventCard {
	classrooms, tags := m.viewUnder(directory, email, token)
	today := now.Format(DateFormat)
	out := []EventCard{}
	for _, e := range m.eventsFor(directory, email, linked) {
		answer := m.AnswerOf(email, e.ID)
		if e.end.Format(DateFormat) < today || !m.InView(e, answer, classrooms, tags) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, m.card(directory, email, e))
	}
	return out
}
