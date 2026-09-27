package calendar

import (
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/config"
)

type Person struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	PhotoURL    string `json:"photoUrl,omitempty"`
	IsStudent   bool   `json:"isStudent,omitempty"`
	IsParent    bool   `json:"isParent,omitempty"`
	IsStaff     bool   `json:"isStaff,omitempty"`
	Grade       string `json:"grade,omitempty"`
	Classroom   string `json:"classroom,omitempty"`
	Line        string `json:"line,omitempty"`
	EmailMasked bool   `json:"-"`
}

type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	Children(email string) []Person
	Household(email string) []string
	Family(email string) map[string]bool
	Parents(email string) []string
	Alerts(email string) (stale []string, privacy []string)
	ClassroomColors() map[string]string
	GradeColors() map[string]string
	People() []Person
	Lists(email string) []List
}

type Responses struct {
	Yes   []Person `json:"yes,omitempty"`
	Maybe []Person `json:"maybe,omitempty"`
	No    []Person `json:"no,omitempty"`
}

func contactLine(directory Directory, p Person) string {
	switch {
	case p.IsStudent:
		parts := []string{}
		if p.Grade != "" {
			parts = append(parts, p.Grade)
		}
		if p.Classroom != "" {
			parts = append(parts, p.Classroom)
		}
		return strings.Join(parts, " · ")
	case p.IsParent:
		kids := []string{}
		for _, k := range directory.Children(p.Email) {
			name := FirstWord(k.Name)
			if k.Grade != "" {
				name += " (" + k.Grade + ")"
			}
			kids = append(kids, name)
		}
		if len(kids) > 0 {
			return "Parent to " + strings.Join(kids, ", ")
		}
		return "Parent"
	case p.IsStaff:
		return "Staff"
	}
	return ""
}

func thumb(url string) string {
	if url == "" {
		return ""
	}
	return url + "?thumb=1"
}

type Alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

type User struct {
	Email        string            `json:"email"`
	Name         string            `json:"name"`
	Initial      string            `json:"initial"`
	PhotoURL     string            `json:"photoUrl,omitempty"`
	IsAdmin      bool              `json:"isAdmin"`
	IsSuperAdmin bool              `json:"isSuperAdmin,omitempty"`
	IsStudent    bool              `json:"isStudent,omitempty"`
	IsParent     bool              `json:"isParent,omitempty"`
	IsStaff      bool              `json:"isStaff,omitempty"`
	Students     []Person          `json:"students"`
	Classrooms   []string          `json:"classrooms"`
	Saved        *Setting          `json:"saved,omitempty"`
	Home         Feed              `json:"home"`
	Answers      map[string]string `json:"answers,omitempty"`
}

type View struct {
	User        User                         `json:"user"`
	ImageSearch bool                         `json:"imageSearch"`
	Today       string                       `json:"today"`
	Now         string                       `json:"now"`
	Classrooms  []Classroom                  `json:"classrooms"`
	Colors      map[string]string            `json:"colors"`
	Tags        []Tag                        `json:"tags"`
	DayTypes    []DayType                    `json:"dayTypes"`
	Years       []Year                       `json:"years"`
	Days        map[string]map[string]string `json:"days"`
	Events      []*Event                     `json:"events"`
	Provenance  map[string]*Provenance       `json:"provenance,omitempty"`
	Responses   map[string]*Responses        `json:"responses,omitempty"`
	GradeColors map[string]string            `json:"gradeColors,omitempty"`
	Names       map[string]string            `json:"names,omitempty"`
	Feeds       []Feed                       `json:"feeds"`
	Alerts      Alerts                       `json:"alerts"`
}

func displayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func students(directory Directory, me Person) []Person {
	out := []Person{}
	if me.IsStudent {
		out = append(out, me)
	}
	for _, kid := range directory.Children(me.Email) {
		if !slices.ContainsFunc(out, func(p Person) bool { return p.Email == kid.Email }) {
			out = append(out, kid)
		}
	}
	return out
}

func classroomsOf(model *Model, me Person, kids []Person) []string {
	wanted := map[string]bool{}
	for _, kid := range kids {
		wanted[kid.Classroom] = true
	}
	if me.IsStaff {
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

func (m *Model) EventsFor(v access.Actor, directory Directory, linked []Linked) []*Event {
	events := m.eventsFor(directory, v.Email, linked)
	carried := map[string]bool{}
	for _, e := range events {
		carried[e.ID] = true
	}
	for _, e := range m.Pending {
		if !e.Cancelled && (v.Admin || config.NormalizeEmail(e.AddedBy) == config.NormalizeEmail(v.Email)) && !carried[e.ID] {
			events = append(events, m.withInvitation(e))
		}
	}
	return events
}

func (m *Model) ResponsesFor(v access.Actor, directory Directory) map[string]*Responses {
	mine := map[string]bool{}
	for _, e := range append(append([]*Event{}, m.Events...), m.Pending...) {
		if e.AddedBy != "" && !e.PosterLeft && config.NormalizeEmail(e.AddedBy) == config.NormalizeEmail(v.Email) {
			mine[e.ID] = true
		}
	}
	if !v.Admin && len(mine) == 0 {
		return nil
	}
	responses := map[string]*Responses{}
	for who, answers := range m.Answers {
		person, ok := directory.Person(who)
		if !ok {
			person = Person{Email: who, Name: displayName(who)}
		}
		person.PhotoURL = thumb(person.PhotoURL)
		person.Line = contactLine(directory, person)
		for id, answer := range answers {
			if !v.Admin && !mine[id] {
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
	byName := func(list []Person) {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	for _, r := range responses {
		byName(r.Yes)
		byName(r.Maybe)
		byName(r.No)
	}
	return responses
}

func Render(model *Model, directory Directory, as access.Actor, now time.Time, linked []Linked) View {
	email, admin := as.Email, as.Admin
	me, known := directory.Person(email)
	if !known {
		me = Person{Email: email, Name: displayName(email)}
	}
	initial := ""
	if me.Name != "" {
		initial = strings.ToUpper(me.Name[:1])
	}
	kids := students(directory, me)
	user := User{
		Email: email, Name: me.Name, Initial: initial, PhotoURL: me.PhotoURL, IsAdmin: admin,
		IsStudent: me.IsStudent, IsParent: me.IsParent, IsStaff: me.IsStaff,
		Students: kids, Classrooms: classroomsOf(model, me, kids),
	}
	if saved, ok := model.Settings[config.NormalizeEmail(email)]; ok && (len(saved.Classrooms) > 0 || len(saved.Tags) > 0) {
		user.Saved = &saved
	}
	user.Home = model.MyHeliosian(email)
	user.Answers = model.Answers[config.NormalizeEmail(email)]
	feeds := []Feed{}
	for _, f := range model.Feeds {
		if f.Email == email {
			feeds = append(feeds, f)
		}
	}
	stale, privacy := directory.Alerts(email)
	names := map[string]string{}
	for _, e := range append(append([]*Event{}, model.Events...), model.Pending...) {
		if e.AddedBy != "" {
			if p, ok := directory.Person(e.AddedBy); ok && p.Name != "" {
				names[e.AddedBy] = p.Name
			}
		}
	}
	events := model.EventsFor(as, directory, linked)
	var provenance map[string]*Provenance
	if admin {
		provenance = model.Provenance
	}
	return View{
		Provenance: provenance, Responses: model.ResponsesFor(as, directory), GradeColors: directory.GradeColors(), Names: names,
		User: user, Today: now.Format(DateFormat), Now: now.Format(DateTimeFormat),
		Classrooms: model.Roster.Classrooms, Colors: directory.ClassroomColors(), Tags: append(append([]Tag{}, model.Tags...), builtinTags...), DayTypes: model.DayTypes, Years: model.Years,
		Days: model.Days, Events: events, Feeds: feeds, Alerts: Alerts{Stale: stale, Privacy: privacy},
	}
}
