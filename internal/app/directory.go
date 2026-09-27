package app

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

type directory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d directory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func (d directory) Person(email string) (string, string, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return "", "", false
	}
	return p.FullName, model.HeroPhoto(p.Email), true
}

func (d directory) Grade(email string) string {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsStudent {
		return ""
	}
	return p.Grade
}

func (d directory) Parents(email string) []string {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsStudent {
		return nil
	}
	return p.ParentContactEmails
}

func (d directory) GradeColors() map[string]string {
	return d.settings.Settings().GradeColors
}

func (d directory) HomePeople() []home.Person {
	model := d.cache.Model()
	out := []home.Person{}
	for _, p := range model.People {
		if p.Email != "" {
			out = append(out, home.Person{Name: p.FullName, Email: p.Email})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type audienceSources struct{ loopDirectory }

func (s audienceSources) Sources() filter.Sources {
	return loop.SourcesOf(s.loopDirectory)
}

func (d directory) Alerts(email string) ([]string, []string) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

func (d directory) SpoofPerson(email string) (auth.Person, bool) {
	model := d.cache.Model()
	p := model.Person(model.Resolve(strings.ToLower(strings.TrimSpace(email))))
	if p == nil {
		return auth.Person{}, false
	}
	return auth.Person{Email: p.Email, Name: p.FullName, Words: placeWords(*p)}, true
}

func (d directory) SpoofPeople() []auth.Person {
	model := d.cache.Model()
	out := []auth.Person{}
	for _, p := range model.People {
		out = append(out, auth.Person{Email: p.Email, Name: p.FullName, Words: placeWords(p)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func placeWords(p who.Person) string {
	switch {
	case p.IsStaff:
		if p.JobTitle == "" {
			return "Staff"
		}
		return p.JobTitle
	case p.IsStudent:
		if p.Grade == "" {
			return "Student"
		}
		return p.Grade
	case p.IsParent:
		return "Parent"
	}
	return ""
}

func (d directory) People() []team.DirectoryPerson {
	model := d.cache.Model()
	out := []team.DirectoryPerson{}
	for _, p := range model.People {
		title := placeWords(p)
		person := team.DirectoryPerson{
			Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Title: title,
			IsStudent: p.IsStudent, ParentEmails: p.ParentContactEmails,
			Pronouns: p.Pronouns, Phone: p.Phone, Grade: p.Grade, Classroom: p.Classroom,
			JobTitle: p.JobTitle, Department: p.Department,
		}
		if p.IsParent {
			person.Spouses, person.Children = teamHousehold(model, p.Email)
		}
		out = append(out, person)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func teamHousehold(model *who.Model, email string) (adults, kids []team.Child) {
	grown, young := model.Household(email)
	for _, a := range grown {
		adults = append(adults, team.Child{Email: a.Email, Name: a.FullName})
	}
	for _, k := range young {
		kids = append(kids, team.Child{Email: k.Email, Name: k.FullName, Grade: k.Grade})
	}
	return adults, kids
}

func (d directory) Family(email string) map[string]bool {
	return d.cache.Model().Family(email)
}

func (d directory) Household(email string) (adults, kids []team.Child) {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsParent {
		return nil, nil
	}
	return teamHousehold(d.cache.Model(), p.Email)
}

type upcomingEvents struct {
	cache     *calendar.Cache
	directory calendar.Directory
	linked    func(email string) []calendar.Linked
}

func (u upcomingEvents) list(email, token string) home.Upcoming {
	model := u.cache.Model()
	out := home.Upcoming{Events: model.UpcomingUnder(u.directory, email, u.linked(email), time.Now().In(calendar.Location), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(model, email, token)
	return out
}

func savedCalendars(model *calendar.Model, email, token string) (list []home.SavedCalendar, def, current string) {
	for _, f := range model.MyCalendars(email) {
		list = append(list, home.SavedCalendar{Token: f.Token, Name: f.Name, Emoji: f.Emoji, Locked: f.Locked})
	}
	def = list[0].Token
	current = def
	if slices.ContainsFunc(list, func(c home.SavedCalendar) bool { return c.Token == token }) {
		current = token
	}
	return list, def, current
}

func (u upcomingEvents) month(email, month, token string) home.Month {
	model := u.cache.Model()
	m := model.MonthUnder(u.directory, email, u.linked(email), time.Now().In(calendar.Location), month, token)
	_, _, current := savedCalendars(model, email, token)
	return home.Month{Month: m.Month, Today: m.Today, Days: m.Days, Events: m.Events, Calendar: current}
}

type birthdayDirectory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d birthdayDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func birthdayPerson(p *who.Person) birthday.Person {
	return birthday.Person{Email: p.Email, Name: p.FullName, PhotoURL: p.PhotoURL, JobTitle: p.JobTitle, Department: p.Department}
}

func (d birthdayDirectory) Person(email string) (birthday.Person, bool) {
	p := d.cache.Model().Person(email)
	if p == nil {
		return birthday.Person{}, false
	}
	return birthdayPerson(p), true
}

func (d birthdayDirectory) Staff() []birthday.Person {
	model := d.cache.Model()
	out := []birthday.Person{}
	for i := range model.People {
		if model.People[i].IsStaff {
			out = append(out, birthdayPerson(&model.People[i]))
		}
	}
	return out
}

func (d birthdayDirectory) People() []birthday.Person {
	model := d.cache.Model()
	out := []birthday.Person{}
	for i := range model.People {
		if model.People[i].Email != "" {
			out = append(out, birthdayPerson(&model.People[i]))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (d birthdayDirectory) Departments() []string {
	return d.cache.Model().Departments
}

func (d birthdayDirectory) Alerts(email string) ([]string, []string) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

type celebrateDirectory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d celebrateDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func celebratePerson(model *who.Model, p *who.Person) celebrate.Person {
	title := ""
	switch {
	case p.IsStudent:
		title = p.Grade
		if title == "" {
			title = "Student"
		}
	case p.IsStaff:
		title = p.JobTitle
		if title == "" {
			title = "Staff"
		}
	case p.IsParent:
		title = "Parent"
	}
	return celebrate.Person{
		Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), IsStudent: p.IsStudent, IsParent: p.IsParent,
		IsStaff: p.IsStaff, Grade: p.Grade, JobTitle: p.JobTitle, Title: title,
		Pronouns: p.Pronouns, Phone: p.Phone, Classroom: p.Classroom, Department: p.Department, ParentEmails: p.ParentContactEmails,
	}
}

func (d celebrateDirectory) Person(email string) (celebrate.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return celebrate.Person{}, false
	}
	return celebratePerson(model, p), true
}

func (d celebrateDirectory) Household(email string) (adults, kids []celebrate.Person) {
	model := d.cache.Model()
	grown, young := model.Household(email)
	for _, a := range grown {
		adults = append(adults, celebratePerson(model, a))
	}
	for _, k := range young {
		kids = append(kids, celebratePerson(model, k))
	}
	return adults, kids
}

func (d celebrateDirectory) Family(email string) map[string]bool {
	return d.cache.Model().Family(email)
}

func (d celebrateDirectory) People() []celebrate.Person {
	model := d.cache.Model()
	out := []celebrate.Person{}
	for i := range model.People {
		person := celebratePerson(model, &model.People[i])
		if person.IsParent {
			person.Spouses, person.Children = d.Household(person.Email)
		}
		out = append(out, person)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (d celebrateDirectory) Alerts(email string) ([]string, []string) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

type calendarDirectory struct {
	cache    *who.Cache
	settings *config.Cache
	lists    func(email string) []who.List
}

func (d calendarDirectory) People() []calendar.Person {
	model := d.cache.Model()
	out := []calendar.Person{}
	for i := range model.People {
		out = append(out, calendarPerson(model, &model.People[i]))
	}
	return out
}

func (d calendarDirectory) Lists(email string) []calendar.List {
	out := []calendar.List{}
	tags := d.cache.Tags(email)
	for _, name := range slices.Sorted(maps.Keys(tags)) {
		out = append(out, calendar.List{Key: "tag:" + name, Name: name, Kind: "tag", People: tags[name]})
	}
	for _, shared := range d.cache.SharedTags(email) {
		out = append(out, calendar.List{Key: "shared:" + shared.Owner + ":" + shared.Name, Name: shared.Name + " (" + shared.OwnerName + "'s)", Kind: "tag", People: shared.People})
	}
	if d.lists != nil {
		for _, list := range d.lists(email) {
			if list.Archived {
				continue
			}
			out = append(out, calendar.List{Key: list.Key, Name: list.Name, Kind: list.Kind, People: list.People})
		}
	}
	return out
}

func (d calendarDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func calendarPerson(model *who.Model, p *who.Person) calendar.Person {
	return calendar.Person{
		Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), IsStudent: p.IsStudent, IsParent: p.IsParent,
		IsStaff: p.IsStaff, Grade: p.Grade, Classroom: p.Classroom, EmailMasked: p.EmailMasked,
	}
}

func (d calendarDirectory) Person(email string) (calendar.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return calendar.Person{}, false
	}
	return calendarPerson(model, p), true
}

func (d calendarDirectory) GradeColors() map[string]string {
	return d.settings.Settings().GradeColors
}

func (d calendarDirectory) Children(email string) []calendar.Person {
	model := d.cache.Model()
	out := []calendar.Person{}
	for _, k := range model.Children(email) {
		out = append(out, calendarPerson(model, k))
	}
	return out
}

func (d calendarDirectory) Household(email string) []string {
	adults, kids := d.cache.Model().Household(email)
	return emailsOf(append(adults, kids...))
}

func (d calendarDirectory) Family(email string) map[string]bool {
	return d.cache.Model().Family(email)
}

func (d calendarDirectory) Parents(email string) []string {
	return emailsOf(d.cache.Model().Parents(email))
}

func emailsOf(people []*who.Person) []string {
	out := []string{}
	for _, p := range people {
		out = append(out, p.Email)
	}
	return out
}

func (d calendarDirectory) Alerts(email string) ([]string, []string) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

func (d calendarDirectory) ClassroomColors() map[string]string {
	return d.settings.Settings().ClassroomColors
}
