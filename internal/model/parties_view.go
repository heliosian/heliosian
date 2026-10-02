package model

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
)

type PartyPerson struct {
	Email        string        `json:"email"`
	Name         string        `json:"name"`
	PhotoURL     string        `json:"photoUrl,omitempty"`
	IsStudent    bool          `json:"isStudent,omitempty"`
	IsParent     bool          `json:"isParent,omitempty"`
	IsStaff      bool          `json:"isStaff,omitempty"`
	Grade        string        `json:"grade,omitempty"`
	JobTitle     string        `json:"jobTitle,omitempty"`
	Title        string        `json:"title,omitempty"`
	Pronouns     string        `json:"pronouns,omitempty"`
	Phone        string        `json:"phone,omitempty"`
	Classroom    string        `json:"classroom,omitempty"`
	Department   string        `json:"department,omitempty"`
	ParentEmails []string      `json:"parentEmails,omitempty"`
	Spouses      []PartyPerson `json:"spouses,omitempty"`
	Children     []PartyPerson `json:"children,omitempty"`
}

func partyPersonOf(directory *Directory, p *Person) PartyPerson {
	flat := func(p *Person) PartyPerson {
		return PartyPerson{
			Email: p.Email, Name: p.FullName, PhotoURL: directory.HeroPhoto(p.Email), IsStudent: p.IsStudent, IsParent: p.IsParent,
			IsStaff: p.IsStaff, Grade: p.Grade, JobTitle: p.JobTitle, Title: p.Words(),
			Pronouns: p.Pronouns, Phone: p.Phone, Classroom: p.Classroom, Department: p.Department, ParentEmails: p.ParentContactEmails,
		}
	}
	out := flat(p)
	if p.IsParent {
		adults, kids := directory.Household(p.Email)
		for _, a := range adults {
			out.Spouses = append(out.Spouses, flat(a))
		}
		for _, k := range kids {
			out.Children = append(out.Children, flat(k))
		}
	}
	return out
}

type partyViewer struct {
	access.Actor
	directory *Directory
	rsvps     func(eventID string) *GuestAnswers
}

func (v partyViewer) person(email string) PartyPerson {
	if p := v.directory.Person(v.directory.Resolve(email)); p != nil {
		return partyPersonOf(v.directory, p)
	}
	return PartyPerson{Email: email, Name: cells.DisplayName(email)}
}

const (
	KindParent  = "Parent"
	KindStudent = "Student"
	KindStaff   = "Staff"
	KindGuest   = "Guest"
)

func kindOf(p *Person) string {
	switch {
	case p == nil:
		return KindGuest
	case p.IsStudent:
		return KindStudent
	case p.IsStaff:
		return KindStaff
	case p.IsParent:
		return KindParent
	}
	return KindGuest
}

func (p *Party) Admits(person *Person) bool {
	if person == nil {
		return true
	}
	return ((person.IsParent || person.IsStaff) && p.Adults) || (person.IsStudent && p.Students)
}

type PartyAttendee struct {
	TicketID      string  `json:"ticketId"`
	Email         string  `json:"email,omitempty"`
	Name          string  `json:"name"`
	PhotoURL      string  `json:"photoUrl,omitempty"`
	Kind          string  `json:"kind"`
	Grade         string  `json:"grade,omitempty"`
	Line          string  `json:"line,omitempty"`
	Status        string  `json:"status"`
	Quantity      int     `json:"quantity,omitempty"`
	Added         string  `json:"added,omitempty"`
	Mine          bool    `json:"mine,omitempty"`
	Purchaser     string  `json:"purchaser,omitempty"`
	PurchaserName string  `json:"purchaserName,omitempty"`
	Price         float64 `json:"price,omitempty"`
	Note          string  `json:"note,omitempty"`
	AddedBy       string  `json:"addedBy,omitempty"`
	RSVP          string  `json:"rsvp,omitempty"`
	Household     string  `json:"household"`
	FamilyKey     string  `json:"familyKey,omitempty"`
	FamilyName    string  `json:"familyName,omitempty"`
	FamilyPhoto   string  `json:"familyPhoto,omitempty"`
}

type PartyView struct {
	*Party
	Availability string          `json:"availability"`
	Sold         int             `json:"sold"`
	Waiting      int             `json:"waiting"`
	Offered      int             `json:"offered"`
	Raised       float64         `json:"raised"`
	Remaining    int             `json:"remaining"`
	HostPeople   []PartyPerson   `json:"hostPeople"`
	Attendees    []PartyAttendee `json:"attendees"`
	Waitlisted   []PartyAttendee `json:"waitlisted"`
	Hosting      bool            `json:"hosting,omitempty"`
	Started      bool            `json:"started,omitempty"`
	Invited      bool            `json:"invited,omitempty"`
}

type PartiesUser struct {
	Email     string        `json:"email"`
	Name      string        `json:"name"`
	Initial   string        `json:"initial"`
	PhotoURL  string        `json:"photoUrl,omitempty"`
	IsStudent bool          `json:"isStudent,omitempty"`
	IsParent  bool          `json:"isParent,omitempty"`
	IsStaff   bool          `json:"isStaff,omitempty"`
	Adults    []PartyPerson `json:"adults"`
	Children  []PartyPerson `json:"children"`
}

func (v partyViewer) line(t Ticket, person *Person, purchaserName string) string {
	switch kindOf(person) {
	case KindStudent:
		return person.Grade
	case KindStaff:
		return person.JobTitle
	case KindParent:
		_, kids := v.directory.Household(person.Email)
		names := []string{}
		for _, k := range kids {
			if k.Grade != "" {
				names = append(names, fmt.Sprintf("%s (%s)", k.FullName, k.Grade))
			} else {
				names = append(names, k.FullName)
			}
		}
		if len(names) == 0 {
			return "Parent"
		}
		return "Parent to " + strings.Join(names, ", ")
	}
	if purchaserName != "" && (t.Email == "" || t.Email != t.Purchaser) {
		return "Guest of " + purchaserName
	}
	return "Guest"
}

func (v partyViewer) attendee(t Ticket, sees bool) PartyAttendee {
	var person *Person
	name := t.Name
	if t.Email != "" {
		person = v.directory.Person(v.directory.Resolve(t.Email))
		if name == "" {
			name = cells.DisplayName(t.Email)
		}
	}
	purchaserName := nameOf(v.directory, t.Purchaser)
	a := PartyAttendee{
		TicketID: t.ID, Email: t.Email, Name: name, Kind: kindOf(person),
		Line: v.line(t, person, purchaserName), Status: t.Status, Quantity: t.Quantity, Added: t.Added,
	}
	if person != nil {
		a.Name, a.PhotoURL, a.Grade = person.FullName, v.directory.HeroPhoto(person.Email), person.Grade
	}
	a.Household = cmp.Or(t.Purchaser, t.Email, t.ID)
	for _, email := range []string{t.Purchaser, t.Email} {
		if family, ok := v.directory.FamilyOf(email); ok && email != "" {
			a.Household, a.FamilyKey, a.FamilyName, a.FamilyPhoto = family.Key, family.Key, family.Name, thumb(family.PhotoURL)
			break
		}
	}
	a.Mine = v.Mine(t.Purchaser) || v.Mine(t.Email)
	if sees || a.Mine {
		a.Purchaser, a.PurchaserName, a.Price, a.Note, a.AddedBy = t.Purchaser, purchaserName, t.Price, t.Note, t.AddedBy
	}
	return a
}

func (v partyViewer) party(raw *Party, now time.Time) PartyView {
	p := raw.For(v.Actor, v.directory)
	hosting := p.Hosted(v.Email)
	sees := p.Sees(v.Actor)
	pv := PartyView{
		Party: p, Availability: p.Availability(now), Sold: p.Sold(), Waiting: p.Waiting(), Offered: raw.OfferedTo(v.directory, v.Email), Raised: raw.Raised(), Remaining: p.Remaining(),
		HostPeople: []PartyPerson{}, Attendees: []PartyAttendee{}, Waitlisted: []PartyAttendee{}, Hosting: hosting,
	}
	for _, email := range p.HostEmails {
		pv.HostPeople = append(pv.HostPeople, v.person(email))
	}
	var rsvps *GuestAnswers
	if sees {
		rsvps = v.rsvps(p.ID)
		pv.Started = rsvps != nil
		pv.Invited = rsvps != nil && rsvps.Sent
	}
	for _, t := range p.Tickets {
		a := v.attendee(t, sees)
		if pv.Invited && t.Email != "" {
			a.RSVP = rsvps.Answers[v.directory.Resolve(strings.ToLower(t.Email))]
		}
		if t.Status == TicketSold {
			pv.Attendees = append(pv.Attendees, a)
		} else {
			pv.Waitlisted = append(pv.Waitlisted, a)
		}
	}
	sort.SliceStable(pv.Waitlisted, func(i, j int) bool { return pv.Waitlisted[i].Added < pv.Waitlisted[j].Added })
	return pv
}

func partiesUserOf(directory *Directory, email string) PartiesUser {
	me := partyViewer{directory: directory}.person(email)
	user := PartiesUser{
		Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL,
		IsStudent: me.IsStudent, IsParent: me.IsParent, IsStaff: me.IsStaff, Adults: []PartyPerson{}, Children: []PartyPerson{},
	}
	adults, kids := directory.Household(email)
	for _, a := range adults {
		user.Adults = append(user.Adults, partyPersonOf(directory, a))
	}
	for _, k := range kids {
		user.Children = append(user.Children, partyPersonOf(directory, k))
	}
	return user
}

func Billable(directory *Directory, viewer string) []string {
	out := []string{}
	if me := directory.Person(viewer); me == nil || !me.IsStudent {
		out = append(out, viewer)
	}
	adults, _ := directory.Household(viewer)
	for _, a := range adults {
		if !slices.Contains(out, a.Email) {
			out = append(out, a.Email)
		}
	}
	return out
}
