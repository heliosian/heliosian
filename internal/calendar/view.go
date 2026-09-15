package calendar

import (
	"heliosian/internal/theme"
	"slices"
	"sort"
	"strings"
	"time"
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
	// Line is the word under a name on a contact card: a student's grade
	// and classroom, a parent's children, a staff member's place.
	Line string `json:"line,omitempty"`
}

type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	Children(email string) []Person
	Alerts(email string) (stale int, privacy bool)
	ClassroomColors() map[string]string
	GradeColors() map[string]string
}

// Responses are the people who answered an event, for an admin: each as
// the directory knows them, with their photo, for a contact card.
type Responses struct {
	Yes []Person `json:"yes,omitempty"`
	No  []Person `json:"no,omitempty"`
}

// contactLine is the word under a name on a contact card, as Who? has it:
// a student's grade and classroom, a parent's children with their grades,
// a staff member's place.
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
			name := k.Name
			if words := strings.Fields(name); len(words) > 0 {
				name = words[0]
			}
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

// thumb is a photo's address at thumbnail size, or nothing.
func thumb(url string) string {
	if url == "" {
		return ""
	}
	return url + "?thumb=1"
}

type Alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

type User struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
	// IsSuperAdmin is the platform's tier, for the Appearance tab alone.
	IsSuperAdmin bool     `json:"isSuperAdmin,omitempty"`
	IsStudent    bool     `json:"isStudent,omitempty"`
	IsParent     bool     `json:"isParent,omitempty"`
	IsStaff      bool     `json:"isStaff,omitempty"`
	Students     []Person `json:"students"`
	Classrooms   []string `json:"classrooms"`
	// Saved is the view this person kept, when they have: what the calendar
	// opens to for them in place of its own defaults.
	Saved *Setting `json:"saved,omitempty"`
	// Home is My Heliosian as this person has it: their name and mark for
	// it, and its place among their calendars - the first being their
	// default calendar.
	Home Feed `json:"home"`
	// Answers is this person's word on each event they have answered, by
	// event id: yes, no, or hidden.
	Answers map[string]string `json:"answers,omitempty"`
}

type View struct {
	User User `json:"user"`
	// ImageSources are the picture searches the picker can offer - for a
	// category's picture in Admin Tools, and for an event anyone shares.
	ImageSources []string                     `json:"imageSources,omitempty"`
	Today        string                       `json:"today"`
	Now          string                       `json:"now"`
	Classrooms   []Classroom                  `json:"classrooms"`
	Colors       map[string]string            `json:"colors"`
	Tags         []Tag                        `json:"tags"`
	DayTypes     []DayType                    `json:"dayTypes"`
	Years        []Year                       `json:"years"`
	Days         map[string]map[string]string `json:"days"`
	Events       []*Event                     `json:"events"`
	// Provenance is each event's admin-side story, for an admin alone; Names
	// puts a name to the addresses the events name.
	Provenance map[string]*Provenance `json:"provenance,omitempty"`
	// Responses is who said yes and who said no to each event, by event id,
	// for an admin alone; hidden is a person's own and named to nobody.
	Responses map[string]*Responses `json:"responses,omitempty"`
	// GradeColors are Who?'s colours per grade, for a student's badge.
	GradeColors map[string]string `json:"gradeColors,omitempty"`
	Names       map[string]string `json:"names,omitempty"`
	Feeds       []Feed            `json:"feeds"`
	Alerts      Alerts            `json:"alerts"`
	// Theme is the admin's colouring of the rail and the page.
	Theme theme.Theme `json:"theme"`
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

func Render(model *Model, directory Directory, email string, admin bool, now time.Time, linked []Linked) View {
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
	if saved, ok := model.Settings[normalizeEmail(email)]; ok && (len(saved.Classrooms) > 0 || len(saved.Tags) > 0) {
		user.Saved = &saved
	}
	user.Home = model.MyHeliosian(email)
	user.Answers = model.Answers[normalizeEmail(email)]
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
	// The events: the calendar's, with the pending ones the viewer shared -
	// every pending one for an admin - marked as such.
	events := model.eventsFor(email, linked)
	carried := map[string]bool{}
	for _, e := range events {
		carried[e.ID] = true
	}
	for _, e := range model.Pending {
		// One the viewer answered is there already, under Going, as a copy.
		if (admin || normalizeEmail(e.AddedBy) == normalizeEmail(email)) && !carried[e.ID] {
			events = append(events, e)
		}
	}
	// Who answered: every event for an admin, their own for whoever shared
	// one.
	mine := map[string]bool{}
	for _, e := range append(append([]*Event{}, model.Events...), model.Pending...) {
		if e.AddedBy != "" && normalizeEmail(e.AddedBy) == normalizeEmail(email) {
			mine[e.ID] = true
		}
	}
	var provenance map[string]*Provenance
	var responses map[string]*Responses
	if admin {
		provenance = model.Provenance
	}
	if admin || len(mine) > 0 {
		responses = map[string]*Responses{}
		for who, answers := range model.Answers {
			person, ok := directory.Person(who)
			if !ok {
				person = Person{Email: who, Name: displayName(who)}
			}
			person.PhotoURL = thumb(person.PhotoURL)
			person.Line = contactLine(directory, person)
			for id, answer := range answers {
				if !admin && !mine[id] {
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
			byName(r.No)
		}
	}
	return View{
		Provenance: provenance, Responses: responses, GradeColors: directory.GradeColors(), Names: names,
		User: user, Today: now.Format(DateFormat), Now: now.Format(DateTimeFormat),
		Classrooms: model.Roster.Classrooms, Colors: directory.ClassroomColors(), Tags: append(append([]Tag{}, model.Tags...), builtinTags...), DayTypes: model.DayTypes, Years: model.Years,
		Days: model.Days, Events: events, Feeds: feeds, Alerts: Alerts{Stale: stale, Privacy: privacy}, Theme: model.Theme,
	}
}
