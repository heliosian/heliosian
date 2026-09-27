package celebrate

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
)

type Person struct {
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	PhotoURL     string   `json:"photoUrl,omitempty"`
	IsStudent    bool     `json:"isStudent,omitempty"`
	IsParent     bool     `json:"isParent,omitempty"`
	IsStaff      bool     `json:"isStaff,omitempty"`
	Grade        string   `json:"grade,omitempty"`
	JobTitle     string   `json:"jobTitle,omitempty"`
	Title        string   `json:"title,omitempty"`
	Pronouns     string   `json:"pronouns,omitempty"`
	Phone        string   `json:"phone,omitempty"`
	Classroom    string   `json:"classroom,omitempty"`
	Department   string   `json:"department,omitempty"`
	ParentEmails []string `json:"parentEmails,omitempty"`
	Spouses      []Person `json:"spouses,omitempty"`
	Children     []Person `json:"children,omitempty"`
}

type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	Household(email string) (adults, kids []Person)
	Family(email string) map[string]bool
	People() []Person
	Alerts(email string) (stale []string, privacy []string)
}

type Alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

func DisplayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

type viewer struct {
	access.Actor
	directory Directory
	rsvps     RSVPLookup
}

func (v viewer) person(email string) Person {
	if p, ok := v.directory.Person(v.directory.Resolve(email)); ok {
		return p
	}
	return Person{Email: email, Name: DisplayName(email)}
}

const (
	KindParent  = "Parent"
	KindStudent = "Student"
	KindStaff   = "Staff"
	KindGuest   = "Guest"
)

func kindOf(p Person, known bool) string {
	switch {
	case !known:
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

func (p *Party) Admits(person Person, known bool) bool {
	if !known {
		return true
	}
	return ((person.IsParent || person.IsStaff) && p.Adults) || (person.IsStudent && p.Students)
}

type Attendee struct {
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
}

type PartyRSVPs struct {
	Sent    bool
	Answers map[string]string
}

type RSVPLookup func(partyID string) *PartyRSVPs

type PartyView struct {
	*Party
	Availability string     `json:"availability"`
	Sold         int        `json:"sold"`
	Waiting      int        `json:"waiting"`
	Raised       float64    `json:"raised"`
	Remaining    int        `json:"remaining"`
	HostPeople   []Person   `json:"hostPeople"`
	Attendees    []Attendee `json:"attendees"`
	Waitlisted   []Attendee `json:"waitlisted"`
	CanEdit      bool       `json:"canEdit"`
	Hosting      bool       `json:"hosting,omitempty"`
	Started      bool       `json:"started,omitempty"`
	Invited      bool       `json:"invited,omitempty"`
}

type User struct {
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	Initial      string   `json:"initial"`
	PhotoURL     string   `json:"photoUrl,omitempty"`
	IsAdmin      bool     `json:"isAdmin"`
	IsSuperAdmin bool     `json:"isSuperAdmin,omitempty"`
	IsStudent    bool     `json:"isStudent,omitempty"`
	IsParent     bool     `json:"isParent,omitempty"`
	IsStaff      bool     `json:"isStaff,omitempty"`
	Adults       []Person `json:"adults"`
	Children     []Person `json:"children"`
}

type View struct {
	User         User           `json:"user"`
	Today        string         `json:"today"`
	Now          string         `json:"now"`
	Settings     Settings       `json:"settings"`
	Celebrations []*Celebration `json:"celebrations"`
	Current      string         `json:"current,omitempty"`
	Banner       string         `json:"banner,omitempty"`
	Categories   []string       `json:"categories"`
	Parties      []PartyView    `json:"parties"`
	Redirects    []Redirect     `json:"redirects"`
	Invoicing    []InvoiceLine  `json:"invoicing,omitempty"`
	ImageSearch  bool           `json:"imageSearch"`
	Alerts       Alerts         `json:"alerts"`
}

func (v viewer) line(t Ticket, person Person, known bool, purchaserName string) string {
	switch kindOf(person, known) {
	case KindStudent:
		return person.Grade
	case KindStaff:
		return person.JobTitle
	case KindParent:
		_, kids := v.directory.Household(person.Email)
		names := []string{}
		for _, k := range kids {
			if k.Grade != "" {
				names = append(names, fmt.Sprintf("%s (%s)", k.Name, k.Grade))
			} else {
				names = append(names, k.Name)
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

func (v viewer) attendee(t Ticket, editor bool) Attendee {
	var person Person
	known := false
	if t.Email != "" {
		if p, ok := v.directory.Person(v.directory.Resolve(t.Email)); ok {
			person, known = p, true
		} else {
			person = Person{Email: t.Email, Name: t.Name}
			if person.Name == "" {
				person.Name = DisplayName(t.Email)
			}
		}
	} else {
		person = Person{Name: t.Name}
	}
	purchaser := v.person(t.Purchaser)
	a := Attendee{
		TicketID: t.ID, Email: t.Email, Name: person.Name, PhotoURL: person.PhotoURL, Kind: kindOf(person, known),
		Grade: person.Grade, Line: v.line(t, person, known, purchaser.Name), Status: t.Status, Quantity: t.Quantity, Added: t.Added,
	}
	a.Mine = v.Mine(t.Purchaser) || v.Mine(t.Email)
	if editor || a.Mine {
		a.Purchaser, a.PurchaserName, a.Price, a.Note, a.AddedBy = t.Purchaser, purchaser.Name, t.Price, t.Note, t.AddedBy
	}
	return a
}

func (v viewer) party(raw *Party, now time.Time) PartyView {
	p := raw.For(v.Actor, v.directory)
	hosting := p.Hosted(v.Email)
	editor := p.Edits(v.Actor)
	pv := PartyView{
		Party: p, Availability: p.Availability(now), Sold: p.Sold(), Waiting: p.Waiting(), Raised: raw.Raised(), Remaining: p.Remaining(),
		HostPeople: []Person{}, Attendees: []Attendee{}, Waitlisted: []Attendee{}, CanEdit: editor, Hosting: hosting,
	}
	for _, email := range p.HostEmails {
		pv.HostPeople = append(pv.HostPeople, v.person(email))
	}
	var rsvps *PartyRSVPs
	if editor && v.rsvps != nil {
		rsvps = v.rsvps(p.ID)
		pv.Started = rsvps != nil
		pv.Invited = rsvps != nil && rsvps.Sent
	}
	for _, t := range p.Tickets {
		a := v.attendee(t, editor)
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

func Render(model *Model, directory Directory, as access.Actor, now time.Time) View {
	return RenderWith(model, directory, nil, as, now)
}

func RenderWith(model *Model, directory Directory, rsvps RSVPLookup, as access.Actor, now time.Time) View {
	v := viewer{Actor: as, directory: directory, rsvps: rsvps}
	email, admin := as.Email, as.Admin
	me := v.person(email)
	adults, kids := directory.Household(email)
	view := View{
		User: User{
			Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: admin,
			IsStudent: me.IsStudent, IsParent: me.IsParent, IsStaff: me.IsStaff, Adults: adults, Children: kids,
		},
		Today:        now.Format(DateFormat),
		Now:          now.Format(DateTimeFormat),
		Settings:     model.Settings,
		Celebrations: model.Celebrations,
		Categories:   model.Categories,
		Parties:      []PartyView{},
		Redirects:    model.Redirects,
	}
	if view.User.Adults == nil {
		view.User.Adults = []Person{}
	}
	if view.User.Children == nil {
		view.User.Children = []Person{}
	}
	if c := model.Current(); c != nil {
		view.Current = c.Code
	}
	if c := model.Banner(); c != nil {
		view.Banner = c.Code
	}
	if admin {
		view.Invoicing = model.Invoicing
	}
	stale, privacy := directory.Alerts(email)
	view.Alerts = Alerts{Stale: stale, Privacy: privacy}
	for _, p := range model.SortedParties("") {
		if !p.VisibleTo(as) {
			continue
		}
		view.Parties = append(view.Parties, v.party(p, now))
	}
	return view
}

func Billable(directory Directory, viewer string) []string {
	out := []string{}
	me, known := directory.Person(viewer)
	if !known || !me.IsStudent {
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
