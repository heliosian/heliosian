package celebrate

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// Person is what the directory knows about someone the app names: enough to
// picture them and to say what kind of ticket they may hold.
type Person struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	PhotoURL  string `json:"photoUrl,omitempty"`
	IsStudent bool   `json:"isStudent,omitempty"`
	IsParent  bool   `json:"isParent,omitempty"`
	IsStaff   bool   `json:"isStaff,omitempty"`
	Grade     string `json:"grade,omitempty"`
	JobTitle  string `json:"jobTitle,omitempty"`
	// Title is the one word that places them in a picker: a grade, a job,
	// "Parent".
	Title string `json:"title,omitempty"`
	// The rest fills the card that opens from a person's face: how to reach
	// them (as the directory would show it, so a masked phone stays blank),
	// what places them, and their household.
	Pronouns     string   `json:"pronouns,omitempty"`
	Phone        string   `json:"phone,omitempty"`
	Classroom    string   `json:"classroom,omitempty"`
	Department   string   `json:"department,omitempty"`
	ParentEmails []string `json:"parentEmails,omitempty"`
	Spouses      []Person `json:"spouses,omitempty"`
	Children     []Person `json:"children,omitempty"`
}

// Directory is what the app asks of the school directory: who a signed-in
// address really is, who someone is, their household, everyone for the
// picker, and the toolbar's badges.
type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	// Household is a person's family as the directory lists it: the other
	// adults in it, then the children (themselves left out of both).
	Household(email string) (adults, kids []Person)
	People() []Person
	Alerts(email string) (stale int, privacy bool)
}

// Alerts carries the directory's badge reckoning to the toolbar.
type Alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

// displayName reads a name out of an address for someone the directory does
// not list, so a list never shows a bare email.
func displayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

type viewer struct {
	email     string
	admin     bool
	directory Directory
	// family is the viewer's household by address: the tickets they may see
	// the details of and take back.
	family map[string]bool
}

func (v viewer) person(email string) Person {
	if p, ok := v.directory.Person(v.directory.Resolve(email)); ok {
		return p
	}
	return Person{Email: email, Name: displayName(email)}
}

// Kind is what sort of ticket holder someone is, for the attendee tile and
// the party's audience rules.
const (
	KindParent  = "Parent"
	KindStudent = "Student"
	KindStaff   = "Staff"
	KindGuest   = "Guest"
)

// kindOf is the one word for a person: a student first, else staff, else a
// parent; someone the directory does not know is a guest.
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

// Admits says whether the party's audience rules let this kind of person
// hold a ticket. A guest fits wherever anyone does, since the host is the one
// who reads the name; someone who is both a parent and staff fits if either
// does.
func (p *Party) Admits(person Person, known bool) bool {
	if !known {
		return true
	}
	return (person.IsParent && p.Parents) || (person.IsStudent && p.Students) || (person.IsStaff && p.Staff)
}

// Attendee is one ticket as the page shows it: a face, a name, a line that
// places them - "Grade 7", "Parent to Sam Whitfield (Grade 3)", "Guest of
// Jordan Whitfield" - and, for whoever may act on it, the ticket itself.
type Attendee struct {
	TicketID string `json:"ticketId"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl,omitempty"`
	Kind     string `json:"kind"`
	Grade    string `json:"grade,omitempty"`
	Line     string `json:"line,omitempty"`
	Status   string `json:"status"`
	Added    string `json:"added,omitempty"`
	// Mine says the viewer bought it or it is for their household - so the
	// page offers the way to take it back.
	Mine bool `json:"mine,omitempty"`
	// The rest reaches the party's hosts, admins, and the household itself.
	Purchaser     string  `json:"purchaser,omitempty"`
	PurchaserName string  `json:"purchaserName,omitempty"`
	Price         float64 `json:"price,omitempty"`
	Note          string  `json:"note,omitempty"`
	Invoice       string  `json:"invoice,omitempty"`
	AddedBy       string  `json:"addedBy,omitempty"`
}

// PartyView is a party as one viewer sees it: the row, its counts and
// availability, its attendees, and what the viewer may do with it.
type PartyView struct {
	*Party
	Availability string `json:"availability"`
	Sold         int    `json:"sold"`
	Waiting      int    `json:"waiting"`
	// Remaining is what is left against the cap, or -1 with no cap.
	Remaining  int        `json:"remaining"`
	HostPeople []Person   `json:"hostPeople"`
	Attendees  []Attendee `json:"attendees"`
	Waitlisted []Attendee `json:"waitlisted"`
	// CanEdit says the viewer runs it: an admin, or one of its hosts. Hosting
	// says they are a host, admin or not.
	CanEdit bool `json:"canEdit"`
	Hosting bool `json:"hosting,omitempty"`
}

type User struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	Initial   string `json:"initial"`
	PhotoURL  string `json:"photoUrl,omitempty"`
	IsAdmin   bool   `json:"isAdmin"`
	IsStudent bool   `json:"isStudent,omitempty"`
	IsParent  bool   `json:"isParent,omitempty"`
	IsStaff   bool   `json:"isStaff,omitempty"`
	// Adults and Children are the viewer's household: who may be billed, and
	// who they may take tickets for.
	Adults   []Person `json:"adults"`
	Children []Person `json:"children"`
}

type View struct {
	User         User           `json:"user"`
	Today        string         `json:"today"`
	Now          string         `json:"now"`
	Settings     Settings       `json:"settings"`
	Celebrations []*Celebration `json:"celebrations"`
	Current      string         `json:"current,omitempty"`
	Categories   []string       `json:"categories"`
	Parties      []PartyView    `json:"parties"`
	// Redirects let the client send an old friendly address to where the
	// party is now, without a round trip.
	Redirects []Redirect `json:"redirects"`
	// ImageSearch says the server can search for a picture, and ImageSources
	// lists where it can look, for the picker.
	ImageSearch  bool     `json:"imageSearch"`
	ImageSources []string `json:"imageSources,omitempty"`
	Alerts       Alerts   `json:"alerts"`
}

// visible decides whether a party reaches this viewer: an open one reaches
// everyone; a pending or hidden one only its hosts and admins.
func (v viewer) visible(p *Party, editor bool) bool {
	return p.Status == StatusOpen || editor
}

// line places an attendee under their name, the way the old site did: a
// student's grade, a parent's children, a staff member's job, a guest's
// purchaser.
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
				person.Name = displayName(t.Email)
			}
		}
	} else {
		person = Person{Name: t.Name}
	}
	purchaser := v.person(t.Purchaser)
	a := Attendee{
		TicketID: t.ID, Email: t.Email, Name: person.Name, PhotoURL: person.PhotoURL, Kind: kindOf(person, known),
		Grade: person.Grade, Line: v.line(t, person, known, purchaser.Name), Status: t.Status, Added: t.Added,
	}
	a.Mine = t.Purchaser == v.email || v.family[t.Purchaser] || (t.Email != "" && (t.Email == v.email || v.family[t.Email]))
	if editor || a.Mine {
		a.Purchaser, a.PurchaserName, a.Price, a.Note, a.AddedBy = t.Purchaser, purchaser.Name, t.Price, t.Note, t.AddedBy
	}
	if editor {
		a.Invoice = t.Invoice
	}
	return a
}

func (v viewer) party(p *Party, now time.Time) PartyView {
	hosting := p.Hosted(v.email)
	editor := v.admin || hosting
	pv := PartyView{
		Party: p, Availability: p.Availability(now), Sold: p.Sold(), Waiting: p.Waiting(), Remaining: p.Remaining(),
		HostPeople: []Person{}, Attendees: []Attendee{}, Waitlisted: []Attendee{}, CanEdit: editor, Hosting: hosting,
	}
	for _, email := range p.HostEmails {
		pv.HostPeople = append(pv.HostPeople, v.person(email))
	}
	for _, t := range p.Tickets {
		a := v.attendee(t, editor)
		if t.Status == TicketSold {
			pv.Attendees = append(pv.Attendees, a)
		} else {
			pv.Waitlisted = append(pv.Waitlisted, a)
		}
	}
	// The waitlist is a queue: earliest first.
	sort.SliceStable(pv.Waitlisted, func(i, j int) bool { return pv.Waitlisted[i].Added < pv.Waitlisted[j].Added })
	return pv
}

// Render is the model as one signed-in person sees it.
func Render(model *Model, directory Directory, email string, admin bool, now time.Time) View {
	v := viewer{email: email, admin: admin, directory: directory, family: map[string]bool{}}
	me := v.person(email)
	adults, kids := directory.Household(email)
	for _, p := range append(append([]Person{}, adults...), kids...) {
		v.family[p.Email] = true
	}
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
	stale, privacy := directory.Alerts(email)
	view.Alerts = Alerts{Stale: stale, Privacy: privacy}
	for _, p := range model.SortedParties("") {
		editor := admin || p.Hosted(email)
		if !v.visible(p, editor) {
			continue
		}
		view.Parties = append(view.Parties, v.party(p, now))
	}
	return view
}

// InHousehold reports whether email is the viewer or someone in their family.
func InHousehold(directory Directory, viewer, email string) bool {
	if email == viewer {
		return true
	}
	adults, kids := directory.Household(viewer)
	for _, p := range append(adults, kids...) {
		if strings.EqualFold(p.Email, email) {
			return true
		}
	}
	return false
}

// Billable says who may be invoiced for a ticket the viewer takes: an adult
// - themselves when they are one, and the other adults of their household -
// so a student's tickets go to a parent.
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
