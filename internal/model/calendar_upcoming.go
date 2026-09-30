package model

import (
	"net/url"
	"slices"
	"strings"

	"heliosian/internal/mail"
)

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
	if names == "You" {
		names = ""
	}
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

func (m *Calendar) goingFor(directory *Directory, email string, c *Event) {
	family := m.familyGoing(directory, email, c.ID)
	going := family
	if m.AnswerOf(email, c.ID) == AnswerYes {
		going = append([]string{"You"}, family...)
	}
	if len(family) > 0 {
		c.Going = joinNames(going) + " " + isAre(going) + " going"
	}
	rest := slices.DeleteFunc(slices.Clone(c.MineWho), func(who string) bool { return slices.Contains(going, who) })
	if c.Mine != MineGoing || len(rest) == len(c.MineWho) {
		return
	}
	c.MineWho = rest
	c.Call = ""
	if len(rest) > 0 {
		c.Call = mineWords(c)
	}
}

func (m *Calendar) familyGoing(directory *Directory, email, id string) []string {
	family := directory.Family(email)
	adults, kids := directory.Household(email)
	names := []string{}
	for _, p := range append(adults, kids...) {
		if family[p.Email] && m.AnswerOf(p.Email, id) == AnswerYes {
			names = append(names, firstNameOf(p.FullName, p.Email))
		}
	}
	return names
}
