package model

import (
	"time"

	"heliosian/internal/cells"
)

type BirthdayPerson struct {
	Email      string
	Name       string
	PhotoURL   string
	JobTitle   string
	Department string
}

func birthdayPersonOf(p *Person) BirthdayPerson {
	return BirthdayPerson{Email: p.Email, Name: p.FullName, PhotoURL: p.PhotoURL, JobTitle: p.JobTitle, Department: p.Department}
}

type birthdayViewer struct {
	directory *Directory
}

func (v birthdayViewer) person(email string) (BirthdayPerson, bool) {
	if p := v.directory.Person(v.directory.Resolve(email)); p != nil {
		return birthdayPersonOf(p), true
	}
	return BirthdayPerson{Email: email, Name: cells.DisplayName(email)}, false
}

type StaffView struct {
	BirthdayPerson
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
	Notes            []BirthdayNote
}

func dateCell(t time.Time) string {
	return t.Format(DateFormat)
}

func (v birthdayViewer) staff(m *Birthdays, b *Birthday, year BirthdayYear, today time.Time) StaffView {
	person, listed := v.person(b.Email)
	person.Email = b.Email
	sv := StaffView{BirthdayPerson: person, InDirectory: listed, Year: year.Label, Birthday: b.Birthday, Override: b.Override, Level: b.Level, LevelNote: b.Note, Notes: []BirthdayNote{}}
	if b.Birthday == "" {
		return sv
	}
	month, day, _ := ParseMonthDay(b.Birthday)
	occurrence := year.Occurrence(month, day)
	sv.BirthdayThisYear = dateCell(occurrence)
	newsletter, hasNewsletter := year.Newsletter(occurrence, m.issueDates())
	if b.Override != "" {
		newsletter, _ = ParseDate(m.NewsletterDate(b.Override).Date)
		hasNewsletter = true
	}
	if hasNewsletter {
		sv.NewsletterDate = dateCell(newsletter)
		sv.RequestBy = dateCell(RequestBy(newsletter, m.Settings.RequestLeadDays))
		sv.DueBy = dateCell(RequestBy(newsletter, m.Settings.DueByLeadDays))
	}
	if a, ok := m.Assignment(b.Email, year.Label); ok {
		to, _ := v.person(a.AssignedTo)
		sv.AssignedTo, sv.AssignedToName, sv.AssignedOn = a.AssignedTo, to.Name, a.AssignedOn
	}
	contacted := false
	if o, ok := m.OutreachFor(b.Email, year.Label); ok {
		contacted = true
		sv.ContactedOn, sv.ContactedBy = o.ContactedOn, o.ContactedBy
	}
	donated, used := false, false
	if d, ok := m.Donation(b.Email, year.Label); ok {
		donated, used = true, d.UsedOn != ""
		sv.Donation = &d
	}
	if d, ok := m.Donation(b.Email, ShiftYearSpan(year.Label, -1)); ok {
		sv.LastDonation = &d
	}
	sv.Stage = Stage(sv.Level, contacted, donated, used, RequestBy(newsletter, m.Settings.RequestLeadDays), hasNewsletter, today)
	for _, n := range m.Notes {
		if n.Email == b.Email {
			sv.Notes = append(sv.Notes, n)
		}
	}
	return sv
}
