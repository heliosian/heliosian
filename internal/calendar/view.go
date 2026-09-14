package calendar

import (
	"slices"
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
}

type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	Children(email string) []Person
	Alerts(email string) (stale int, privacy bool)
	ClassroomColors() map[string]string
}

type Alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

type User struct {
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	Initial    string   `json:"initial"`
	PhotoURL   string   `json:"photoUrl,omitempty"`
	IsAdmin    bool     `json:"isAdmin"`
	IsStudent  bool     `json:"isStudent,omitempty"`
	IsParent   bool     `json:"isParent,omitempty"`
	IsStaff    bool     `json:"isStaff,omitempty"`
	Students   []Person `json:"students"`
	Classrooms []string `json:"classrooms"`
}

type View struct {
	User User `json:"user"`
	// ImageSources are the picture searches Admin Tools can offer, for an
	// admin alone.
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
	Names      map[string]string      `json:"names,omitempty"`
	Feeds      []Feed                 `json:"feeds"`
	Alerts     Alerts                 `json:"alerts"`
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
	feeds := []Feed{}
	for _, f := range model.Feeds {
		if f.Email == email {
			feeds = append(feeds, f)
		}
	}
	stale, privacy := directory.Alerts(email)
	names := map[string]string{}
	for _, e := range model.Events {
		if e.AddedBy != "" {
			if p, ok := directory.Person(e.AddedBy); ok && p.Name != "" {
				names[e.AddedBy] = p.Name
			}
		}
	}
	var provenance map[string]*Provenance
	if admin {
		provenance = model.Provenance
	}
	return View{
		Provenance: provenance, Names: names,
		User: user, Today: now.Format(DateFormat), Now: now.Format(DateTimeFormat),
		Classrooms: model.Roster.Classrooms, Colors: directory.ClassroomColors(), Tags: append(append([]Tag{}, model.Tags...), builtinTags...), DayTypes: model.DayTypes, Years: model.Years,
		Days: model.Days, Events: withLinked(model.Events, linked), Feeds: feeds, Alerts: Alerts{Stale: stale, Privacy: privacy},
	}
}
