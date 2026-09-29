package model

import (
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

type CalendarPerson struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	PhotoURL  string `json:"photoUrl,omitempty"`
	IsStudent bool   `json:"isStudent,omitempty"`
	IsParent  bool   `json:"isParent,omitempty"`
	IsStaff   bool   `json:"isStaff,omitempty"`
	Grade     string `json:"grade,omitempty"`
	Classroom string `json:"classroom,omitempty"`
	Line      string `json:"line,omitempty"`
}

func personView(directory *Directory, p *Person) CalendarPerson {
	return CalendarPerson{
		Email: p.Email, Name: p.FullName, PhotoURL: thumb(directory.HeroPhoto(p.Email)),
		IsStudent: p.IsStudent, IsParent: p.IsParent, IsStaff: p.IsStaff,
		Grade: p.Grade, Classroom: p.Classroom, Line: contactLine(directory, p),
	}
}

type Responses struct {
	Yes   []CalendarPerson `json:"yes,omitempty"`
	Maybe []CalendarPerson `json:"maybe,omitempty"`
	No    []CalendarPerson `json:"no,omitempty"`
}

func contactLine(directory *Directory, p *Person) string {
	if p.IsStudent {
		parts := []string{}
		if p.Grade != "" {
			parts = append(parts, p.Grade)
		}
		if p.Classroom != "" {
			parts = append(parts, p.Classroom)
		}
		return strings.Join(parts, " · ")
	}
	if p.IsParent {
		kids := []string{}
		for _, k := range directory.Children(p.Email) {
			name := FirstWord(k.FullName)
			if k.Grade != "" {
				name += " (" + k.Grade + ")"
			}
			kids = append(kids, name)
		}
		if len(kids) > 0 {
			return "Parent to " + strings.Join(kids, ", ")
		}
	}
	return p.Words()
}

func thumb(url string) string {
	if url == "" {
		return ""
	}
	return url + "?thumb=1"
}

type CalendarUser struct {
	Email      string            `json:"email"`
	Name       string            `json:"name"`
	Initial    string            `json:"initial"`
	PhotoURL   string            `json:"photoUrl,omitempty"`
	IsAdmin    bool              `json:"isAdmin"`
	IsStudent  bool              `json:"isStudent,omitempty"`
	IsParent   bool              `json:"isParent,omitempty"`
	IsStaff    bool              `json:"isStaff,omitempty"`
	Students   []CalendarPerson  `json:"students"`
	Classrooms []string          `json:"classrooms"`
	Saved      *CalendarSetting  `json:"saved,omitempty"`
	Home       Feed              `json:"home"`
	Answers    map[string]string `json:"answers,omitempty"`
}

type CalendarView struct {
	User        CalendarUser                 `json:"user"`
	ImageSearch bool                         `json:"imageSearch"`
	Today       string                       `json:"today"`
	Now         string                       `json:"now"`
	Classrooms  []RosterClassroom            `json:"classrooms"`
	Colors      map[string]string            `json:"colors"`
	Tags        []CalendarTag                `json:"tags"`
	DayTypes    []DayType                    `json:"dayTypes"`
	Years       []CalendarYear               `json:"years"`
	Days        map[string]map[string]string `json:"days"`
	Events      []*Event                     `json:"events"`
	Provenance  map[string]*Provenance       `json:"provenance,omitempty"`
	Responses   map[string]*Responses        `json:"responses,omitempty"`
	GradeColors map[string]string            `json:"gradeColors,omitempty"`
	Names       map[string]string            `json:"names,omitempty"`
	Feeds       []Feed                       `json:"feeds"`
}

func students(directory *Directory, email string) []*Person {
	out := []*Person{}
	if me := directory.Person(email); me != nil && me.IsStudent {
		out = append(out, me)
	}
	for _, kid := range directory.Children(email) {
		if !slices.ContainsFunc(out, func(p *Person) bool { return p.Email == kid.Email }) {
			out = append(out, kid)
		}
	}
	return out
}

func classroomsOf(model *Calendar, me *Person, kids []*Person) []string {
	wanted := map[string]bool{}
	for _, kid := range kids {
		wanted[kid.Classroom] = true
	}
	if me != nil && me.IsStaff {
		wanted[me.Classroom] = true
	}
	out := []string{}
	for _, name := range model.Roster.Names() {
		if wanted[name] {
			out = append(out, name)
		}
	}
	return out
}

func (m *Calendar) EventsFor(v access.Actor, directory *Directory, linked []Linked) []*Event {
	events := m.eventsFor(directory, v.Email, linked)
	carried := map[string]bool{}
	for _, e := range events {
		carried[e.ID] = true
	}
	for _, e := range m.Pending {
		if !e.Cancelled && (v.May(SeeAllEvents) || mail.Normalize(e.AddedBy) == mail.Normalize(v.Email)) && !carried[e.ID] {
			events = append(events, m.withInvitation(e))
		}
	}
	return events
}

func (m *Calendar) ResponsesFor(v access.Actor, directory *Directory) map[string]*Responses {
	mine := map[string]bool{}
	for _, e := range append(append([]*Event{}, m.Events...), m.Pending...) {
		if e.AddedBy != "" && !e.PosterLeft && mail.Normalize(e.AddedBy) == mail.Normalize(v.Email) {
			mine[e.ID] = true
		}
	}
	if !v.May(SeeAllEvents) && len(mine) == 0 {
		return nil
	}
	responses := map[string]*Responses{}
	for who, answers := range m.Answers {
		person := CalendarPerson{Email: who, Name: cells.DisplayName(who)}
		if p := directory.Person(who); p != nil {
			person = personView(directory, p)
		}
		for id, answer := range answers {
			if !v.May(SeeAllEvents) && !mine[id] {
				continue
			}
			r := responses[id]
			if r == nil {
				r = &Responses{}
				responses[id] = r
			}
			switch answer {
			case AnswerYes:
				r.Yes = append(r.Yes, person)
			case AnswerMaybe:
				r.Maybe = append(r.Maybe, person)
			case AnswerNo:
				r.No = append(r.No, person)
			}
		}
	}
	byName := func(list []CalendarPerson) {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	for _, r := range responses {
		byName(r.Yes)
		byName(r.Maybe)
		byName(r.No)
	}
	return responses
}

func RenderCalendar(model *Calendar, directory *Directory, settings *Config, as access.Actor, now time.Time, linked []Linked) CalendarView {
	email, admin := as.Email, as.May(SeeAllEvents)
	me := directory.Person(email)
	kids := students(directory, email)
	user := CalendarUser{Email: email, Name: cells.DisplayName(email), IsAdmin: admin, Students: []CalendarPerson{}, Classrooms: classroomsOf(model, me, kids)}
	if me != nil {
		user.Name, user.PhotoURL = me.FullName, directory.HeroPhoto(email)
		user.IsStudent, user.IsParent, user.IsStaff = me.IsStudent, me.IsParent, me.IsStaff
	}
	if user.Name != "" {
		user.Initial = strings.ToUpper(user.Name[:1])
	}
	for _, kid := range kids {
		user.Students = append(user.Students, personView(directory, kid))
	}
	if saved, ok := model.Settings[mail.Normalize(email)]; ok && (len(saved.Classrooms) > 0 || len(saved.Tags) > 0) {
		user.Saved = &saved
	}
	user.Home = model.MyHeliosian(email)
	user.Answers = model.Answers[mail.Normalize(email)]
	feeds := []Feed{}
	for _, f := range model.Feeds {
		if f.Email == email {
			feeds = append(feeds, f)
		}
	}
	names := map[string]string{}
	for _, e := range append(append([]*Event{}, model.Events...), model.Pending...) {
		if e.AddedBy != "" {
			if p := directory.Person(e.AddedBy); p != nil && p.FullName != "" {
				names[e.AddedBy] = p.FullName
			}
		}
	}
	events := model.EventsFor(as, directory, linked)
	var provenance map[string]*Provenance
	if admin {
		provenance = model.Provenance
	}
	return CalendarView{
		Provenance: provenance, Responses: model.ResponsesFor(as, directory), GradeColors: settings.GradeColors, Names: names,
		User: user, Today: now.Format(DateFormat), Now: now.Format(DateTimeFormat),
		Classrooms: model.Roster.Classrooms, Colors: settings.ClassroomColors, Tags: model.Tags, DayTypes: model.DayTypes, Years: model.Years,
		Days: model.Days, Events: events, Feeds: feeds,
	}
}
