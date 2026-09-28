package birthday

import (
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/who"
)

type Person struct {
	Email      string
	Name       string
	PhotoURL   string
	JobTitle   string
	Department string
}

func personView(p *who.Person) Person {
	return Person{Email: p.Email, Name: p.FullName, PhotoURL: p.PhotoURL, JobTitle: p.JobTitle, Department: p.Department}
}

type viewer struct {
	directory *who.Model
}

func (v viewer) person(email string) (Person, bool) {
	if p := v.directory.Person(v.directory.Resolve(email)); p != nil {
		return personView(p), true
	}
	return Person{Email: email, Name: cells.DisplayName(email)}, false
}

type StaffView struct {
	Person
	InDirectory      bool
	Year             string
	Birthday         string
	BirthdayThisYear string
	NewsletterDate   string
	RequestBy        string
	DueBy            string
	Override         string
	Level            string
	LevelNote        string
	Stage            string
	AssignedTo       string
	AssignedToName   string
	AssignedOn       string
	ContactedOn      string
	ContactedBy      string
	Donation         *Donation
	LastDonation     *Donation
	Notes            []Note
}

func dateCell(t time.Time) string {
	return t.Format(DateFormat)
}

func (v viewer) staff(model *Model, b *Birthday, year Year, today time.Time) StaffView {
	person, listed := v.person(b.Email)
	person.Email = b.Email
	sv := StaffView{Person: person, InDirectory: listed, Year: year.Label, Birthday: b.Birthday, Override: b.Override, Level: b.Level, LevelNote: b.Note, Notes: []Note{}}
	if b.Birthday == "" {
		return sv
	}
	month, day, _ := ParseMonthDay(b.Birthday)
	occurrence := year.Occurrence(month, day)
	sv.BirthdayThisYear = dateCell(occurrence)
	newsletter, hasNewsletter := year.Newsletter(occurrence, model.issueDates())
	if b.Override != "" {
		newsletter, _ = ParseDate(model.NewsletterDate(b.Override).Date)
		hasNewsletter = true
	}
	if hasNewsletter {
		sv.NewsletterDate = dateCell(newsletter)
		sv.RequestBy = dateCell(RequestBy(newsletter, model.Settings.RequestLeadDays))
		sv.DueBy = dateCell(RequestBy(newsletter, model.Settings.DueByLeadDays))
	}
	if a, ok := model.Assignment(b.Email, year.Label); ok {
		to, _ := v.person(a.AssignedTo)
		sv.AssignedTo, sv.AssignedToName, sv.AssignedOn = a.AssignedTo, to.Name, a.AssignedOn
	}
	contacted := false
	if o, ok := model.OutreachFor(b.Email, year.Label); ok {
		contacted = true
		sv.ContactedOn, sv.ContactedBy = o.ContactedOn, o.ContactedBy
	}
	donated, used := false, false
	if d, ok := model.Donation(b.Email, year.Label); ok {
		donated, used = true, d.UsedOn != ""
		sv.Donation = &d
	}
	if d, ok := model.Donation(b.Email, ShiftYear(year.Label, -1)); ok {
		sv.LastDonation = &d
	}
	sv.Stage = Stage(sv.Level, contacted, donated, used, RequestBy(newsletter, model.Settings.RequestLeadDays), hasNewsletter, today)
	for _, n := range model.Notes {
		if n.Email == b.Email {
			sv.Notes = append(sv.Notes, n)
		}
	}
	return sv
}
