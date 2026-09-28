package when

import (
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/who"
)

type Person struct {
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

func personView(directory *who.Model, p *who.Person) Person {
	return Person{
		Email: p.Email, Name: p.FullName, PhotoURL: thumb(directory.HeroPhoto(p.Email)),
		IsStudent: p.IsStudent, IsParent: p.IsParent, IsStaff: p.IsStaff,
		Grade: p.Grade, Classroom: p.Classroom, Line: contactLine(directory, p),
	}
}

type Responses struct {
	Yes   []Person `json:"yes,omitempty"`
	Maybe []Person `json:"maybe,omitempty"`
	No    []Person `json:"no,omitempty"`
}

func contactLine(directory *who.Model, p *who.Person) string {
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

type User struct {
	Email      string            `json:"email"`
	Name       string            `json:"name"`
	Initial    string            `json:"initial"`
	PhotoURL   string            `json:"photoUrl,omitempty"`
	IsAdmin    bool              `json:"isAdmin"`
	IsStudent  bool              `json:"isStudent,omitempty"`
	IsParent   bool              `json:"isParent,omitempty"`
	IsStaff    bool              `json:"isStaff,omitempty"`
	Students   []Person          `json:"students"`
	Classrooms []string          `json:"classrooms"`
	Saved      *Setting          `json:"saved,omitempty"`
	Home       Feed              `json:"home"`
	Answers    map[string]string `json:"answers,omitempty"`
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
}

func students(directory *who.Model, email string) []*who.Person {
	out := []*who.Person{}
	if me := directory.Person(email); me != nil && me.IsStudent {
		out = append(out, me)
	}
	for _, kid := range directory.Children(email) {
		if !slices.ContainsFunc(out, func(p *who.Person) bool { return p.Email == kid.Email }) {
			out = append(out, kid)
		}
	}
	return out
}

func classroomsOf(model *Model, me *who.Person, kids []*who.Person) []string {
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

func (m *Model) EventsFor(v access.Actor, directory *who.Model, linked []Linked) []*Event {
	events := m.eventsFor(directory, v.Email, linked)
	carried := map[string]bool{}
	for _, e := range events {
		carried[e.ID] = true
	}
	for _, e := range m.Pending {
		if !e.Cancelled && (v.May(SeeAll) || config.NormalizeEmail(e.AddedBy) == config.NormalizeEmail(v.Email)) && !carried[e.ID] {
			events = append(events, m.withInvitation(e))
		}
	}
	return events
}

func (m *Model) ResponsesFor(v access.Actor, directory *who.Model) map[string]*Responses {
	mine := map[string]bool{}
	for _, e := range append(append([]*Event{}, m.Events...), m.Pending...) {
		if e.AddedBy != "" && !e.PosterLeft && config.NormalizeEmail(e.AddedBy) == config.NormalizeEmail(v.Email) {
			mine[e.ID] = true
		}
	}
	if !v.May(SeeAll) && len(mine) == 0 {
		return nil
	}
	responses := map[string]*Responses{}
	for who, answers := range m.Answers {
		person := Person{Email: who, Name: cells.DisplayName(who)}
		if p := directory.Person(who); p != nil {
			person = personView(directory, p)
		}
		for id, answer := range answers {
			if !v.May(SeeAll) && !mine[id] {
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

func Render(model *Model, directory *who.Model, settings *config.Settings, as access.Actor, now time.Time, linked []Linked) View {
	email, admin := as.Email, as.May(SeeAll)
	me := directory.Person(email)
	kids := students(directory, email)
	user := User{Email: email, Name: cells.DisplayName(email), IsAdmin: admin, Students: []Person{}, Classrooms: classroomsOf(model, me, kids)}
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
	return View{
		Provenance: provenance, Responses: model.ResponsesFor(as, directory), GradeColors: settings.GradeColors, Names: names,
		User: user, Today: now.Format(DateFormat), Now: now.Format(DateTimeFormat),
		Classrooms: model.Roster.Classrooms, Colors: settings.ClassroomColors, Tags: model.Tags, DayTypes: model.DayTypes, Years: model.Years,
		Days: model.Days, Events: events, Feeds: feeds,
	}
}
