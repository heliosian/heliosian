package calendar

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/mail"
)

// An invitation is a guest list kept for one event - a party on Helios
// Celebrate, or an event shared here, private or public - built by its
// hosts in one step and sent in another. Whoever is
// on a sent list has the event on their calendar, their household with
// them, and answers for everyone in their household who is on it; a host
// reads and corrects every answer. The Invitations tab holds the list's
// settings, the Invites tab its people, and the RSVPs tab the answers, the
// same rows a yes or no anywhere else writes.

// The words an invitation's settings take: who a family or a classroom
// adds to the list, and who may read who is coming.
const (
	AudienceAdults   = "adults"
	AudienceStudents = "students"
	AudienceBoth     = "both"
	GuestListPublic  = "public"
	GuestListPrivate = "private"
	// ViaGuest marks someone brought along by an invitee rather than put
	// on the list by a host; the other Via words say how a host added them.
	ViaGuest = "guest"
	// guestPrefix begins the key of a guest named without an address, in
	// place of the address the Invites and RSVPs tabs are keyed by.
	guestPrefix = "guest-"
)

// Invitation is one event's guest list settings.
type Invitation struct {
	EventID string `json:"eventId"`
	// Hosts are co-hosts by address, beyond the event's own - the person
	// who shared it, a party's hosts on Celebrate - and the admins.
	Hosts []string `json:"hosts"`
	// Audience is who adding a family or a classroom puts on the list:
	// its adults, its students, or both.
	Audience string `json:"audience"`
	// Guests is always on: invitees may bring guests by name. The tab's
	// column is kept for the rows that have it, and no longer read.
	Guests bool `json:"guests"`
	// GuestList says who reads who is coming: everyone invited, or the
	// hosts alone.
	GuestList string `json:"guestList"`
	// Message is the hosts' own words on the invitation.
	Message   string `json:"message"`
	CreatedBy string `json:"createdBy"`
	Created   string `json:"created"`
	// Sent is when invites first went out; blank while the list is a draft.
	Sent string `json:"sent,omitempty"`
	// Notify is the hosts who asked to hear by email as answers come in -
	// each host's own choice.
	Notify []string `json:"-"`
	// A party's invitation may carry words of its own in place of the
	// party's - a title, when, where, a description - each blank for the
	// party's own: what the invitation email, the calendar invite, the
	// outside person's page and everyone invited see.
	Title       string `json:"title,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	// Flyer is a picture the hosts put on the invitation, shown whole - an
	// upload into the shared store, named as the Events tab names one - on
	// the event's page, the outside person's page and the invitation email,
	// where it is fetched from /open/flyer/{event id}, public.
	Flyer string `json:"flyer,omitempty"`
}

// Invite is one person on one event's guest list. Email is the address the
// list keys them by - a guest named without one gets a key instead.
type Invite struct {
	EventID string `json:"-"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	// GuestOf is the invitee who brought them, for a guest.
	GuestOf string `json:"guestOf,omitempty"`
	// Via is how they came to be on the list: a search, a family, a
	// classroom, a list, the party's tickets, or as someone's guest.
	Via     string `json:"via,omitempty"`
	AddedBy string `json:"addedBy"`
	Added   string `json:"added"`
	// Sent is when their invite went out; blank until it has.
	Sent string `json:"sent,omitempty"`
	// Token is the secret in the page of someone from outside the
	// community (ext.go): the whole of what lets them in, minted when they
	// are put on the list; blank for anyone the directory holds.
	Token string `json:"-"`
	// Household groups people from outside into a family: the address of
	// the one they were added with, the same on each of theirs, so they
	// sit together on the list, are named together in the invitation, and
	// any of them answers for all on their own page. Blank for anyone the
	// directory holds, whose household is the directory's.
	Household string `json:"household,omitempty"`
	// Opened is when they first opened the invitation - the event's page
	// here, or their own page from outside - blank until they have. Mail
	// opens are not read: a tracking pixel says little, and less every
	// year, while a page opened is a page opened.
	Opened string `json:"opened,omitempty"`
}

// PartyPeople is a party as its guest list needs it: who hosts it, and who
// holds a ticket or a waitlist place.
type PartyPeople struct {
	Hosts     []string
	Attendees []Attendee
}

// An Attendee is one ticket on a party: whose, by address where the ticket
// names one, and the name on it.
type Attendee struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name"`
	// Status is ticket for one bought, free for one the hosts gave, or
	// waitlist.
	Status string `json:"status"`
}

// A List is one of a person's lists in Helios Who? - a tag of their own,
// or one the other apps give them - as the guest list picker offers it.
type List struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	People []string `json:"people"`
}

// invitations reads the two tabs: each event's settings, and its list in
// the sheet's order, one row per address on it, the first kept. A sent
// invite puts the event on the calendar of its household.
func (b *builder) invitations(settings, rows []map[string]string) {
	for _, row := range settings {
		id := strings.TrimSpace(row["Event ID"])
		if id == "" {
			continue
		}
		inv := &Invitation{
			EventID: id, Hosts: []string{}, Audience: strings.ToLower(strings.TrimSpace(row["Audience"])), Guests: true,
			GuestList: strings.ToLower(strings.TrimSpace(row["Guest List"])), Message: strings.TrimSpace(row["Message"]),
			CreatedBy: normalizeEmail(row["Created By"]), Created: strings.TrimSpace(row["Created"]), Sent: strings.TrimSpace(row["Sent"]),
			Title: strings.TrimSpace(row["Title"]), Start: strings.TrimSpace(row["Start"]), End: strings.TrimSpace(row["End"]),
			Location: strings.TrimSpace(row["Location"]), Description: strings.TrimSpace(row["Description"]),
			Flyer: strings.Trim(strings.TrimSpace(row["Flyer"]), "/"), Notify: splitEmails(row["Notify"]),
		}
		// A start the sheet cannot read is dropped with a log line, never
		// a refusal.
		if inv.Start != "" {
			if _, _, _, err := parseWhen(inv.Start, inv.End); err != nil {
				slog.Warn("calendar: invitation's own time skipped", "event", id, "error", err)
				inv.Start, inv.End = "", ""
			}
		}
		for _, h := range SplitList(row["Hosts"]) {
			inv.Hosts = append(inv.Hosts, normalizeEmail(h))
		}
		if inv.Audience != AudienceAdults && inv.Audience != AudienceStudents {
			inv.Audience = AudienceBoth
		}
		if inv.GuestList != GuestListPrivate {
			inv.GuestList = GuestListPublic
		}
		b.model.Invitations[id] = inv
	}
	for _, row := range rows {
		id, email := strings.TrimSpace(row["Event ID"]), normalizeEmail(row["Email"])
		if id == "" || email == "" {
			continue
		}
		if slices.ContainsFunc(b.model.Invites[id], func(i Invite) bool { return i.Email == email }) {
			continue
		}
		inv := Invite{
			EventID: id, Email: email, Name: strings.TrimSpace(row["Name"]), GuestOf: normalizeEmail(row["Guest Of"]), Via: strings.TrimSpace(row["Via"]),
			AddedBy: normalizeEmail(row["Added By"]), Added: strings.TrimSpace(row["Added"]), Sent: strings.TrimSpace(row["Sent"]), Token: strings.TrimSpace(row["Token"]),
			Household: normalizeEmail(row["Household"]), Opened: strings.TrimSpace(row["Opened"]),
		}
		b.model.Invites[id] = append(b.model.Invites[id], inv)
		if inv.Token != "" {
			b.model.byInvite[inv.Token] = inv
		}
		if inv.Sent == "" {
			continue
		}
		// The invitation is theirs alone - and a student's is their
		// parents' too, who answer for them; a partner's is not.
		for _, who := range append([]string{email}, b.model.Roster.Parents[email]...) {
			if b.model.invited[who] == nil {
				b.model.invited[who] = map[string]bool{}
			}
			b.model.invited[who][id] = true
		}
	}
}

// InviteOf is one person's row on one event's list, or nil.
func (m *Model) InviteOf(id, email string) *Invite {
	for i := range m.Invites[id] {
		if m.Invites[id][i].Email == normalizeEmail(email) {
			return &m.Invites[id][i]
		}
	}
	return nil
}

// InviteByToken is the row an outside person's secret names, or nothing.
func (m *Model) InviteByToken(token string) (Invite, bool) {
	inv, ok := m.byInvite[strings.TrimSpace(token)]
	return inv, ok
}

// Invited says an event is on a person's calendar by invitation: theirs,
// sent - or, for a parent, a child's.
func (m *Model) Invited(email, id string) bool {
	return m.invited[normalizeEmail(email)][id]
}

// hasDetails says an invitation carries words of its own.
func (inv *Invitation) hasDetails() bool {
	return inv != nil && (inv.Title != "" || inv.Start != "" || inv.Location != "" || inv.Description != "")
}

// invitedEvent is an event as its invitation says it: the invitation's
// own title, when, where and description over the party's where it has
// them, on a copy - for the invitation email, the calendar invite, the
// outside person's page, and everyone invited. Any other event, or one
// whose invitation says nothing of its own, is itself.
func (m *Model) invitedEvent(e *Event) *Event {
	inv := m.Invitations[e.ID]
	if e == nil || !e.linked() || !inv.hasDetails() {
		return e
	}
	c := *e
	c.Invitation = true
	if inv.Title != "" {
		c.Title = inv.Title
	}
	if inv.Location != "" {
		c.Location = inv.Location
	}
	if inv.Description != "" {
		c.Description = inv.Description
	}
	if inv.Start != "" {
		if start, end, allDay, err := parseWhen(inv.Start, inv.End); err == nil {
			c.Start, c.End, c.AllDay, c.start, c.end = inv.Start, inv.End, allDay, start, end
			if c.End == "" {
				c.End = c.Start
			}
		}
	}
	return &c
}

// withInvitation is an event flagged as carrying a guest list, on a copy
// when it does, so the page knows to fetch it.
func (m *Model) withInvitation(e *Event) *Event {
	if e == nil || e.Invitation || m.Invitations[e.ID] == nil {
		return e
	}
	c := *e
	c.Invitation = true
	return &c
}

// isGuestKey says an address is the key of a guest named without one.
func isGuestKey(email string) bool {
	return strings.HasPrefix(email, guestPrefix) || !strings.Contains(email, "@")
}

// newGuestKey mints the key for a guest named without an address, in
// lower case as the tabs' addresses are read.
func newGuestKey() string {
	return guestPrefix + strings.ToLower(newEventID())
}

// hostsOf is everyone who runs an event's guest list: who shared a
// hand-added event, a party's hosts on Celebrate, and the invitation's own
// co-hosts - each as the directory resolves them.
func (a app) hostsOf(e *Event) []string {
	out := []string{}
	add := func(email string) {
		if email = a.directory.Resolve(normalizeEmail(email)); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	switch e.Source {
	case SourceSheet:
		add(e.AddedBy)
	case SourceCelebrate:
		if a.parties != nil {
			if p := a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/")); p != nil {
				for _, h := range p.Hosts {
					add(h)
				}
			}
		}
	}
	// A linked event's own hosts - an HCA event's chairs.
	for _, h := range e.Hosts {
		add(h)
	}
	if inv := a.cache.Model().Invitations[e.ID]; inv != nil {
		for _, h := range inv.Hosts {
			add(h)
		}
	}
	return out
}

// isHost says a person runs an event's guest list: one of its hosts - a
// calendar admin is not one, the list being the hosts' own business, and
// reads the page as anyone else invited would.
func (a app) isHost(email string, _ bool, e *Event) bool {
	return slices.Contains(a.hostsOf(e), email)
}

// party is the party behind a linked event, or nil for any other event.
func (a app) party(e *Event) *PartyPeople {
	if e == nil || e.Source != SourceCelebrate || a.parties == nil {
		return nil
	}
	return a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/"))
}

// inviterEvent is the event someone asking to invite people to it may:
// a host's, or one on their own calendar - a public event, or a private
// one they were invited to; a private event's link alone does not let
// its bearer invite others. Whether they are a host comes back with it.
func (a app) inviterEvent(w http.ResponseWriter, r *http.Request, id string) (string, *Event, bool, bool) {
	actor, admin := a.who(r)
	e := a.eventFor(actor, strings.TrimSpace(id))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return actor, nil, false, false
	}
	if e.Source != SourceSheet && !e.linked() {
		http.Error(w, "only a hand-added event, a party or an HCA event keeps a guest list", http.StatusBadRequest)
		return actor, nil, false, false
	}
	host := a.isHost(actor, admin, e)
	if !host && e.InviteOnly && !a.cache.Model().Invited(actor, e.ID) {
		http.Error(w, "only a host, or someone invited, may invite others", http.StatusForbidden)
		return actor, nil, false, false
	}
	return actor, e, host, true
}

// hostedEvent is the event a host is asking about, as they see it, with
// whether they may run its list: a hand-added event, or a party.
func (a app) hostedEvent(w http.ResponseWriter, r *http.Request, id string) (string, *Event, bool) {
	actor, admin := a.who(r)
	e := a.eventFor(actor, strings.TrimSpace(id))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return actor, nil, false
	}
	if e.Source != SourceSheet && !e.linked() {
		http.Error(w, "only a hand-added event, a party or an HCA event keeps a guest list", http.StatusBadRequest)
		return actor, nil, false
	}
	if !a.isHost(actor, admin, e) {
		http.Error(w, "only a host can change the guest list", http.StatusForbidden)
		return actor, nil, false
	}
	return actor, e, true
}

// household is everyone in a person's families, as the roster lists them,
// the person first.
func (a app) household(email string) []string {
	return append([]string{email}, a.cache.Model().Roster.Households[email]...)
}

// householdOn is a person's household as one event's list has it: the
// directory's for anyone it holds, and for someone from outside the
// family they were added with - everyone on the list sharing their
// Household - themselves first.
func (a app) householdOn(e *Event, email string) []string {
	if _, known := a.directory.Person(email); known {
		return a.household(email)
	}
	model := a.cache.Model()
	inv := model.InviteOf(e.ID, email)
	if inv == nil || inv.Household == "" {
		return []string{email}
	}
	out := []string{email}
	for _, other := range model.Invites[e.ID] {
		if other.Household == inv.Household && other.Email != email && other.GuestOf == "" {
			out = append(out, other.Email)
		}
	}
	return out
}

// isAdult says a person may answer for their household: anyone the
// directory does not list as a student alone.
func (a app) isAdult(email string) bool {
	p, known := a.directory.Person(email)
	return !known || !p.IsStudent || p.IsParent || p.IsStaff
}

// mayAnswerFor says who an actor may answer for on an event: themselves,
// anyone in their household when they are an adult, and anyone at all when
// they host it.
func (a app) mayAnswerFor(actor, subject string, admin bool, e *Event) bool {
	if actor == subject || a.isHost(actor, admin, e) {
		return true
	}
	if !a.isAdult(actor) {
		return false
	}
	if slices.Contains(a.household(actor), subject) {
		return true
	}
	// A guest the household brought is theirs to answer for too.
	if inv := a.cache.Model().InviteOf(e.ID, subject); inv != nil && inv.GuestOf != "" && slices.Contains(a.household(actor), inv.GuestOf) {
		return true
	}
	return false
}

// GuestRow is one person on a guest list as the page shows them: who they
// are, how they came to be on it, and where they stand.
type GuestRow struct {
	Person
	// Key is what the list keys them by - their address, or a guest's
	// key - for answering and removing; Email is blank for a guest named
	// without one.
	Key         string `json:"key"`
	GuestOf     string `json:"guestOf,omitempty"`
	GuestOfName string `json:"guestOfName,omitempty"`
	Via         string `json:"via,omitempty"`
	// Invited says they are on the list, rather than someone who answered
	// by the event's link.
	Invited bool   `json:"invited"`
	Sent    string `json:"sent,omitempty"`
	Answer  string `json:"answer,omitempty"`
	// AnsweredBy names who gave the answer when it was not the person
	// themselves - a parent, a host.
	AnsweredBy string `json:"answeredBy,omitempty"`
	AnsweredAt string `json:"answeredAt,omitempty"`
	// AnsweredVia is how: page, or calendar for a calendar app's reply.
	AnsweredVia string `json:"answeredVia,omitempty"`
	// Opened is when they first opened the invitation, for a host.
	Opened string `json:"opened,omitempty"`
	// InvitedBy names who invited them when it was a guest, not a host.
	InvitedBy string `json:"invitedBy,omitempty"`
	// Ticket is a party's word on them: ticket (bought), free (the hosts'
	// gift), waitlist, or nothing.
	Ticket string `json:"ticket,omitempty"`
	// Outside marks someone the directory does not hold.
	Outside bool `json:"outside,omitempty"`
	// Mine marks someone the viewer may answer for.
	Mine bool `json:"mine,omitempty"`
	// Household groups the rows of one household, by its first address.
	Household string `json:"household,omitempty"`
	// Link is an outside person's own page, for a host to pass on.
	Link string `json:"link,omitempty"`
	// Warning says the address may not reach them: "bounced" when the
	// mail provider has reported it undeliverable, "unknown" for a school
	// address the directory does not hold - an alum's, a typo - each with
	// its words. A host is told, and may change the address or send anyway.
	Warning      string `json:"warning,omitempty"`
	WarningWords string `json:"warningWords,omitempty"`
	// Grades and Classrooms are a student's own, or a parent's children's,
	// as Who?'s filters read a person - the grades youngest first - for
	// the guest table.
	Grades     []string `json:"grades,omitempty"`
	Classrooms []string `json:"classrooms,omitempty"`
}

// facetsOf is a student's own grade and classroom, or a parent's
// children's, each once, the grades kindergarten first.
func (a app) facetsOf(p Person) (grades, classrooms []string) {
	add := func(k Person) {
		if k.Grade != "" && !slices.Contains(grades, k.Grade) {
			grades = append(grades, k.Grade)
		}
		if k.Classroom != "" && !slices.Contains(classrooms, k.Classroom) {
			classrooms = append(classrooms, k.Classroom)
		}
	}
	if p.IsStudent {
		add(p)
	}
	if p.IsParent {
		for _, k := range a.directory.Children(p.Email) {
			add(k)
		}
	}
	slices.SortFunc(grades, func(x, y string) int { return cmp.Compare(gradeRank(x), gradeRank(y)) })
	slices.Sort(classrooms)
	return grades, classrooms
}

// addressWarning is a word of caution on an address on the list: bounced,
// when the provider has reported it undeliverable; unknown, when it is a
// school address the directory does not hold, which an alum's or a typo
// would be; else nothing.
func (a app) addressWarning(model *Model, email string, known bool) (string, string) {
	if isGuestKey(email) {
		return "", ""
	}
	if b, ok := model.Bounced[email]; ok {
		words := "Mail to this address bounced"
		if b.When != "" {
			words += " on " + b.When
		}
		if b.Reason != "" {
			words += " (" + b.Reason + ")"
		}
		return "bounced", words + "."
	}
	if !known && strings.HasSuffix(email, "@"+auth.Domain) {
		return "unknown", "A school address the directory does not have - an alum's, or a typo - which may not reach anyone."
	}
	return "", ""
}

// gradeRank orders grades as a school does: kindergarten, then the
// numbered grades, anything else after by name.
func gradeRank(grade string) int {
	if strings.HasPrefix(strings.ToLower(grade), "k") {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(grade, "Grade ")); err == nil {
		return n
	}
	return 100
}

// EventWords are an event's own words, for a form to start from.
type EventWords struct {
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

// Counts sums a guest list up.
type Counts struct {
	Invited int `json:"invited"`
	Yes     int `json:"yes"`
	Maybe   int `json:"maybe"`
	No      int `json:"no"`
	Waiting int `json:"waiting"`
	Guests  int `json:"guests"`
	// Tickets counts, on a party, the ticket holders on the list, and
	// TicketsWaiting those of them who have not answered.
	Tickets        int `json:"tickets,omitempty"`
	TicketsWaiting int `json:"ticketsWaiting,omitempty"`
}

// InviteView is a guest list as one viewer may see it: their own
// household's part for anyone invited, who is coming when the list is
// open, and the whole list with its settings for a host.
type InviteView struct {
	Host bool `json:"host"`
	// MayInvite says the viewer may invite people one at a time - a host,
	// or anyone the event is on the calendar of, invited to a private one.
	MayInvite bool `json:"mayInvite,omitempty"`
	// NotifyMe says the viewer, a host, asked to hear as answers come in.
	NotifyMe bool `json:"notifyMe,omitempty"`
	// Linked says another app runs the event - a party on Celebrate, an
	// HCA event on Team - whose hosts are that app's and whose invitation
	// may say the event its own way; Party that it is the party.
	Linked    bool        `json:"linked,omitempty"`
	Party     bool        `json:"party,omitempty"`
	Settings  *Invitation `json:"settings,omitempty"`
	Guests    bool        `json:"guests"`
	GuestList string      `json:"guestList"`
	Sent      string      `json:"sent,omitempty"`
	// Flyer is the invitation's flyer as a path to fetch, when there is one.
	Flyer string   `json:"flyer,omitempty"`
	Hosts []Person `json:"hosts"`
	// Original is a party's own words on Celebrate - title, when, where,
	// description - for a host editing the invitation's own, so the form
	// shows what stands and sends only what differs.
	Original *EventWords `json:"original,omitempty"`
	Mine     []GuestRow  `json:"mine"`
	// Coming and List are null for a viewer who may not read them.
	// Coming is every row but the nos - the yeses, the maybes and those
	// still to answer - and List everyone, the nos among them.
	Coming []GuestRow `json:"coming"`
	List   []GuestRow `json:"list"`
	// Groups are the list's invite groups, for a host.
	Groups []InviteGroup `json:"groups,omitempty"`
	Counts Counts        `json:"counts"`
}

// personOf is someone as a guest list names them: the directory's person,
// else the name the list holds over their address.
func (a app) personOf(email, name string) (Person, bool) {
	p, known := a.directory.Person(email)
	if known {
		p.PhotoURL = thumb(p.PhotoURL)
		p.Line = contactLine(a.directory, p)
		return p, true
	}
	p = Person{Email: email, Name: name}
	if p.Name == "" && !isGuestKey(email) {
		p.Name = displayName(email)
	}
	if isGuestKey(email) {
		p.Email = ""
	}
	return p, false
}

// rows is an event's guest list as rows: everyone on it, then anyone who
// answered by the link, each with their answer and, on a party, their
// ticket.
func (a app) rows(viewer string, admin bool, e *Event) []GuestRow {
	model := a.cache.Model()
	tickets := map[string]string{}
	if p := a.party(e); p != nil {
		for _, t := range p.Attendees {
			if t.Email != "" {
				tickets[normalizeEmail(t.Email)] = t.Status
			}
		}
	}
	names := map[string]string{}
	for _, inv := range model.Invites[e.ID] {
		names[inv.Email] = inv.Name
	}
	nameOf := func(email string) string {
		if p, known := a.directory.Person(email); known && p.Name != "" {
			return p.Name
		}
		if names[email] != "" {
			return names[email]
		}
		return displayName(email)
	}
	householdOf := func(email string) string {
		members := a.householdOn(e, email)
		slices.Sort(members)
		return members[0]
	}
	out := []GuestRow{}
	seen := map[string]bool{}
	row := func(email, name string, invited bool) GuestRow {
		p, known := a.personOf(email, name)
		g := GuestRow{Person: p, Key: email, Invited: invited, Outside: !known, Ticket: tickets[email], Mine: a.mayAnswerFor(viewer, email, admin, e), Household: householdOf(email)}
		if known {
			g.Grades, g.Classrooms = a.facetsOf(p)
		}
		g.Warning, g.WarningWords = a.addressWarning(model, email, known)
		if ans, ok := model.Answered[email][e.ID]; ok && ans.Answer != AnswerHidden {
			g.Answer = ans.Answer
			g.AnsweredAt = ans.At
			g.AnsweredVia = ans.Via
			if ans.By != "" && ans.By != email {
				g.AnsweredBy = nameOf(ans.By)
			}
		}
		return g
	}
	host := a.isHost(viewer, admin, e)
	for _, inv := range model.Invites[e.ID] {
		g := row(inv.Email, inv.Name, true)
		g.GuestOf, g.Via, g.Sent, g.Opened = inv.GuestOf, inv.Via, inv.Sent, inv.Opened
		if inv.Via == ViaInvited {
			g.InvitedBy = nameOf(inv.AddedBy)
		}
		if host && inv.Token != "" {
			g.Link = extPath(inv.Token)
		}
		if inv.GuestOf != "" {
			g.GuestOfName = nameOf(inv.GuestOf)
			g.Household = householdOf(inv.GuestOf)
			if g.Outside {
				g.Line = "Guest of " + g.GuestOfName
			}
		} else if g.Outside {
			g.Line = "Outside Helios"
		}
		seen[inv.Email] = true
		out = append(out, g)
	}
	// Whoever answered without being on the list came by the link.
	linked := []GuestRow{}
	for email, answers := range model.Answered {
		if ans, ok := answers[e.ID]; ok && !seen[email] && ans.Answer != AnswerHidden {
			linked = append(linked, row(email, "", false))
		}
	}
	sort.Slice(linked, func(i, j int) bool { return linked[i].Name < linked[j].Name })
	return append(out, linked...)
}

// invitesView answers /api/calendar/invites?id= with the guest list as
// the viewer may see it.
func (a app) invitesView(w http.ResponseWriter, r *http.Request) {
	viewer, admin := a.who(r)
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	e := a.eventFor(viewer, id)
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return
	}
	host := a.isHost(viewer, admin, e)
	// Someone invited opening the page has opened the invitation.
	a.noteOpened(r.Context(), e, viewer)
	// A host opening the list brings its auto groups up to date first.
	if host {
		a.sweepEvent(r.Context(), e)
	}
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	view := InviteView{Host: host, MayInvite: host || !e.InviteOnly || a.cache.Model().Invited(viewer, e.ID), Party: e.Source == SourceCelebrate, Linked: e.linked(), Guests: true, GuestList: GuestListPublic, Hosts: []Person{}, Mine: []GuestRow{}}
	if inv != nil && inv.Flyer != "" {
		view.Flyer = flyerPath(e.ID)
	}
	if host && e.linked() {
		view.Original = &EventWords{Title: e.Title, Start: e.Start, End: e.End, Location: e.Location, Description: e.Description}
	}
	for _, h := range a.hostsOf(e) {
		p, _ := a.personOf(h, "")
		view.Hosts = append(view.Hosts, p)
	}
	if inv != nil {
		view.Guests, view.GuestList, view.Sent = inv.Guests, inv.GuestList, inv.Sent
		if host {
			view.Settings = inv
			view.NotifyMe = slices.Contains(inv.Notify, viewer)
		}
	}
	rows := a.rows(viewer, admin, e)
	// The ask is for the viewer's own invitations: theirs, their
	// children's - a partner's alone is the partner's to answer.
	mine := []string{viewer}
	for _, member := range a.household(viewer) {
		if slices.Contains(a.cache.Model().Roster.Parents[member], viewer) {
			mine = append(mine, member)
		}
	}
	for _, g := range rows {
		if g.Invited && (slices.Contains(mine, g.Email) || (g.GuestOf != "" && slices.Contains(mine, g.GuestOf))) {
			view.Mine = append(view.Mine, g)
		}
		if !g.Invited {
			if g.Answer != "" {
				view.Counts.Yes += b2i(g.Answer == AnswerYes)
				view.Counts.Maybe += b2i(g.Answer == AnswerMaybe)
				view.Counts.No += b2i(g.Answer == AnswerNo)
			}
			continue
		}
		view.Counts.Invited++
		view.Counts.Guests += b2i(g.GuestOf != "")
		switch g.Answer {
		case AnswerYes:
			view.Counts.Yes++
		case AnswerMaybe:
			view.Counts.Maybe++
		case AnswerNo:
			view.Counts.No++
		default:
			view.Counts.Waiting++
		}
		if g.Ticket == "ticket" || g.Ticket == "free" {
			view.Counts.Tickets++
			view.Counts.TicketsWaiting += b2i(g.Answer == "")
		}
	}
	// The viewer's own row leads their household's.
	sort.SliceStable(view.Mine, func(i, j int) bool { return view.Mine[i].Email == viewer && view.Mine[j].Email != viewer })
	if host || view.GuestList == GuestListPublic {
		// Who is coming, for everyone who may read it: the yeses, the
		// maybes, and who has not answered yet - never the nos, which are
		// the hosts' alone.
		view.Coming = []GuestRow{}
		for _, g := range rows {
			if g.Answer == AnswerYes || g.Answer == AnswerMaybe || (g.Invited && g.Answer == "") {
				if !host {
					// A guest reads who is coming and nothing of the hosts'
					// side: not the tickets, the sending, an address's
					// trouble, nor who gave an answer for whom.
					g.Ticket, g.Sent, g.Via, g.Warning, g.WarningWords = "", "", "", "", ""
					g.AnsweredBy, g.AnsweredAt, g.AnsweredVia, g.Link, g.Opened = "", "", "", "", ""
				}
				view.Coming = append(view.Coming, g)
			}
		}
	}
	if host {
		view.List = rows
		view.Groups = []InviteGroup{}
		for _, g := range model.Groups[e.ID] {
			for _, inv := range model.Invites[e.ID] {
				if inv.Via == ViaGroup+g.ID {
					g.Count++
				}
			}
			view.Groups = append(view.Groups, g)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode guest list", "error", err)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PickerPerson is one person as the guest list picker offers them: who
// they are, the rest of their household by address, and among them their
// parents, children and siblings, for Add family.
type PickerPerson struct {
	Person
	Household []string `json:"household,omitempty"`
	Parents   []string `json:"parents,omitempty"`
	Children  []string `json:"children,omitempty"`
	Siblings  []string `json:"siblings,omitempty"`
}

// PickerView is what a host builds a guest list from: everyone in the
// directory, the classrooms, the host's own lists, a party's ticket
// holders, and who is on the list already.
type PickerView struct {
	People     []PickerPerson `json:"people"`
	Classrooms []Classroom    `json:"classrooms"`
	Lists      []List         `json:"lists"`
	Attendees  []Attendee     `json:"attendees,omitempty"`
	OnList     []string       `json:"onList"`
}

// invitePeople answers /api/calendar/invites/people?id= for a host - or,
// with no id, for anyone about to share an event, whose list is built
// with the form before the event exists.
func (a app) invitePeople(w http.ResponseWriter, r *http.Request) {
	var e *Event
	actor, _ := a.who(r)
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		var ok bool
		if actor, e, _, ok = a.inviterEvent(w, r, id); !ok {
			return
		}
	}
	model := a.cache.Model()
	view := PickerView{People: []PickerPerson{}, Classrooms: model.Roster.Classrooms, Lists: []List{}, OnList: []string{}}
	for _, p := range a.directory.People() {
		p.PhotoURL = thumb(p.PhotoURL)
		p.Line = contactLine(a.directory, p)
		pp := PickerPerson{Person: p, Household: model.Roster.Households[p.Email]}
		// The relations as the directory has them: a student's parents are
		// the household's parents and their siblings its other students; a
		// parent's children are its students.
		for _, other := range pp.Household {
			o, known := a.directory.Person(other)
			if !known {
				continue
			}
			switch {
			case p.IsStudent && o.IsParent:
				pp.Parents = append(pp.Parents, other)
			case p.IsStudent && o.IsStudent:
				pp.Siblings = append(pp.Siblings, other)
			case p.IsParent && o.IsStudent:
				pp.Children = append(pp.Children, other)
			}
		}
		view.People = append(view.People, pp)
	}
	sort.Slice(view.People, func(i, j int) bool { return view.People[i].Name < view.People[j].Name })
	if lists := a.directory.Lists(actor); lists != nil {
		view.Lists = lists
	}
	if e != nil {
		if p := a.party(e); p != nil {
			view.Attendees = p.Attendees
		}
		for _, inv := range model.Invites[e.ID] {
			view.OnList = append(view.OnList, inv.Email)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode guest picker", "error", err)
	}
}

// invitationCells is a fresh Invitations row for an event, under the host
// who started its list.
func invitationCells(id, actor string) map[string]string {
	return map[string]string{"Event ID": id, "Hosts": "", "Audience": "Both", "Guests": "Yes", "Guest List": "Public", "Message": "", "Created By": actor, "Created": now().Format(DateTimeFormat), "Sent": ""}
}

// ensured is the tables with an Invitations row for the event, made now
// under the actor when there was none, and the flush that writes it.
func (a app) ensured(tables *Tables, id, actor string) (*Tables, func() error) {
	if slices.ContainsFunc(tables.Invitations, func(row map[string]string) bool { return row["Event ID"] == id }) {
		return tables, func() error { return nil }
	}
	cells := invitationCells(id, actor)
	return tables.WithInvitation(id, cells), func() error { return a.writer.AppendCells(appName, InvitationsTab, cells) }
}

// inviteSettings is PUT /api/calendar/invites/settings: a host setting the
// list's audience, guests, who reads it, the message, and the co-hosts.
func (a app) inviteSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string   `json:"id"`
		Audience  string   `json:"audience"`
		GuestList string   `json:"guestList"`
		Message   *string  `json:"message"`
		Hosts     []string `json:"hosts"`
		// A party's invitation's own words, each blank for the party's.
		Title       *string `json:"title"`
		Start       *string `json:"start"`
		End         *string `json:"end"`
		Location    *string `json:"location"`
		Description *string `json:"description"`
		// Flyer names an upload (POST /api/calendar/image), blank for none.
		Flyer *string `json:"flyer"`
		// NotifyMe is the host's own wish to hear as answers come in.
		NotifyMe *bool `json:"notifyMe"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	cells := map[string]string{}
	if body.NotifyMe != nil {
		notify := []string{}
		if inv := a.cache.Model().Invitations[e.ID]; inv != nil {
			notify = append(notify, inv.Notify...)
		}
		notify = slices.DeleteFunc(notify, func(h string) bool { return h == actor })
		if *body.NotifyMe {
			notify = append(notify, actor)
		}
		cells["Notify"] = strings.Join(notify, ", ")
	}
	if body.Flyer != nil {
		flyer := strings.Trim(strings.TrimSpace(*body.Flyer), "/")
		if flyer != "" && a.store != nil && a.readImage(flyer) == nil {
			http.Error(w, "that picture is not here", http.StatusBadRequest)
			return
		}
		cells["Flyer"] = flyer
	}
	if e.linked() {
		for col, v := range map[string]*string{"Title": body.Title, "Location": body.Location, "Description": body.Description} {
			if v != nil {
				if len(*v) > maxTextLength {
					http.Error(w, "the "+strings.ToLower(col)+" is too long", http.StatusBadRequest)
					return
				}
				cells[col] = strings.TrimSpace(*v)
			}
		}
		if body.Start != nil {
			start, end := strings.TrimSpace(*body.Start), ""
			if body.End != nil {
				end = strings.TrimSpace(*body.End)
			}
			if start != "" {
				if _, _, _, err := parseWhen(start, end); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			} else {
				end = ""
			}
			cells["Start"], cells["End"] = start, end
		}
	}
	switch strings.ToLower(strings.TrimSpace(body.Audience)) {
	case AudienceAdults:
		cells["Audience"] = "Adults"
	case AudienceStudents:
		cells["Audience"] = "Students"
	case AudienceBoth:
		cells["Audience"] = "Both"
	case "":
	default:
		http.Error(w, "the audience is adults, students, or both", http.StatusBadRequest)
		return
	}
	switch strings.ToLower(strings.TrimSpace(body.GuestList)) {
	case GuestListPublic:
		cells["Guest List"] = "Public"
	case GuestListPrivate:
		cells["Guest List"] = "Private"
	case "":
	default:
		http.Error(w, "the guest list is public or private", http.StatusBadRequest)
		return
	}
	if body.Message != nil {
		if len(*body.Message) > maxTextLength {
			http.Error(w, "the message is too long", http.StatusBadRequest)
			return
		}
		cells["Message"] = strings.TrimSpace(*body.Message)
	}
	// A new co-host hears of it by email.
	newHosts := []string{}
	if body.Hosts != nil {
		hosts := []string{}
		for _, h := range body.Hosts {
			h = a.directory.Resolve(normalizeEmail(h))
			if _, known := a.directory.Person(h); !known {
				http.Error(w, h+" is not in the directory", http.StatusBadRequest)
				return
			}
			if !slices.Contains(hosts, h) {
				hosts = append(hosts, h)
			}
			if !slices.Contains(a.hostsOf(e), h) {
				newHosts = append(newHosts, h)
			}
		}
		cells["Hosts"] = JoinList(hosts)
	}
	tables, first := a.ensured(a.cache.Tables(), e.ID, actor)
	if !a.commit(r.Context(), w, tables.WithInvitation(e.ID, cells), func() error {
		if err := first(); err != nil {
			return err
		}
		if len(cells) == 0 {
			return nil
		}
		return a.writer.Set(appName, InvitationsTab, map[string]string{"Event ID": e.ID}, cells)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: guest list settings", "actor", actor, "event", e.ID)
	if a.mail.Sender != nil {
		for _, h := range newHosts {
			go a.sendCohostNote(context.WithoutCancel(r.Context()), h, actor, e)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendCohostNote mails someone that they have been made a co-host: who
// did it, the event, what a co-host may do, and the way to its page.
func (a app) sendCohostNote(ctx context.Context, to, actor string, e *Event) {
	e = a.cache.Model().invitedEvent(e)
	who := actor
	if p, known := a.directory.Person(actor); known && p.Name != "" {
		who = p.Name
	}
	link := "https://when.heliosian.com" + EventPath(e)
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " \u00b7 " + hours
	}
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	var text, htm strings.Builder
	fmt.Fprintf(&text, "%s made you a co-host of %s.\n\n%s\n", who, e.Title, when)
	if e.Location != "" {
		fmt.Fprintf(&text, "%s\n", e.Location)
	}
	fmt.Fprintf(&text, "\nAs a co-host you can build and send the guest list, read every answer, message the guests, and replies to the invitation reach you. The event's page: %s\n", link)
	fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 %s\">%s made you a co-host of <strong>%s</strong>.</p>", font, html.EscapeString(who), html.EscapeString(e.Title))
	fmt.Fprintf(&htm, "<p style=\"font:15px/1.5 %s;color:#0e4d54\">%s", font, html.EscapeString(when))
	if e.Location != "" {
		fmt.Fprintf(&htm, "<br>%s", html.EscapeString(e.Location))
	}
	htm.WriteString("</p>")
	fmt.Fprintf(&htm, "<p style=\"font:14px/1.5 %s;color:#333\">As a co-host you can build and send the guest list, read every answer, message the guests, and replies to the invitation reach you.</p>", font)
	fmt.Fprintf(&htm, "<p style=\"margin:20px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px %s;text-decoration:none\">Open the event</a></p>", html.EscapeString(link), font)
	err := a.mail.Sender.Send(ctx, mail.Message{
		To:      []string{to},
		ReplyTo: []string{actor},
		Subject: "[" + e.Title + "] You're a co-host",
		Text:    text.String(),
		HTML:    htm.String(),
	})
	if err != nil {
		slog.ErrorContext(ctx, "calendar: send co-host note", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: co-host told", "to", to, "event", e.ID)
}

// addInvites is POST /api/calendar/invites/people: a host putting people
// on the list - by address, with a name for anyone the directory does not
// hold, and a word on how they were found. Someone on it already stays as
// they were.
func (a app) addInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		People []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
			Via   string `json:"via"`
			// Household is, for someone from outside, the address of the
			// one they were added with - their family on the list; someone
			// named with no address of their own goes on under a key.
			Household string `json:"household"`
		} `json:"people"`
	}
	if !decode(w, r, &body) {
		return
	}
	// Anyone the event is on the calendar of may invite people one at a
	// time; the groups, and everything else of the list, are the hosts'.
	actor, e, host, ok := a.inviterEvent(w, r, body.ID)
	if !ok {
		return
	}
	if !host && len(body.People) > 20 {
		http.Error(w, "invite up to twenty people at a time", http.StatusBadRequest)
		return
	}
	if len(body.People) == 0 || len(body.People) > 500 {
		http.Error(w, "add between one and five hundred people at a time", http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	stamp := now().Format(DateTimeFormat)
	rows := []map[string]string{}
	added := map[string]bool{}
	for _, p := range body.People {
		name := strings.TrimSpace(p.Name)
		household := normalizeEmail(p.Household)
		email := a.directory.Resolve(normalizeEmail(p.Email))
		token := ""
		switch {
		case email == "" && household != "" && name != "":
			// A family member from outside named with no address of their
			// own: on the list under a key, answered for by their family.
			email = newGuestKey()
		case !emailForm.MatchString(email):
			http.Error(w, fmt.Sprintf("%q is not an email address", p.Email), http.StatusBadRequest)
			return
		default:
			// Someone the directory does not hold gets a page of their own,
			// outside sign-in, found by a secret minted now.
			token = NewToken()
			if person, known := a.directory.Person(email); known {
				name, token, household = person.Name, "", ""
			}
		}
		if added[email] || model.InviteOf(e.ID, email) != nil {
			continue
		}
		added[email] = true
		if len(name) > maxTitleLength {
			http.Error(w, "a name is too long", http.StatusBadRequest)
			return
		}
		if household != "" && !emailForm.MatchString(household) {
			http.Error(w, "a family is named by an address", http.StatusBadRequest)
			return
		}
		via := strings.TrimSpace(p.Via)
		if !host {
			// Someone other than a host invited them: the row says so.
			via = ViaInvited
		}
		rows = append(rows, map[string]string{"Event ID": e.ID, "Email": email, "Name": name, "Guest Of": "", "Via": via, "Added By": actor, "Added": stamp, "Sent": "", "Token": token, "Household": household})
	}
	tables, first := a.ensured(a.cache.Tables(), e.ID, actor)
	if !a.commit(r.Context(), w, tables.WithInvites(rows), func() error {
		if err := first(); err != nil {
			return err
		}
		for _, row := range rows {
			if err := a.writer.AppendCells(appName, InvitesTab, row); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	// Once the invitation is out, someone invited by a guest is sent it
	// now - the guest's doing, not the host's to hold in Pending.
	sent := 0
	if !host {
		if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Sent != "" {
			emails := []string{}
			for _, row := range rows {
				emails = append(emails, row["Email"])
			}
			sent = a.send(r.Context(), actor, e, emails, "")
		}
	}
	slog.InfoContext(r.Context(), "calendar: guests added", "actor", actor, "event", e.ID, "count", len(rows), "host", host, "sent", sent)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"added": len(rows), "sent": sent})
}

// removeInvite is DELETE /api/calendar/invites/people: a host taking
// someone off the list, or an invitee taking back a guest their household
// brought - their answer going with them.
func (a app) removeInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, admin := a.who(r)
	e := a.eventFor(actor, strings.TrimSpace(body.ID))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return
	}
	email := normalizeEmail(body.Email)
	inv := a.cache.Model().InviteOf(e.ID, email)
	if inv == nil {
		http.Error(w, "they are not on the list", http.StatusNotFound)
		return
	}
	if !a.isHost(actor, admin, e) && !(inv.GuestOf != "" && a.mayAnswerFor(actor, inv.GuestOf, admin, e)) {
		http.Error(w, "only a host can take someone off the list", http.StatusForbidden)
		return
	}
	// Someone a group put on is remembered on the group as removed, so
	// the sweep that keeps the group up to date does not put them back.
	tables := a.cache.Tables().WithoutInvite(e.ID, email)
	var group *InviteGroup
	removed := ""
	if gid, ok := strings.CutPrefix(inv.Via, ViaGroup); ok {
		if group = a.cache.Model().GroupOf(e.ID, gid); group != nil {
			removed = strings.Join(append(append([]string{}, group.Removed...), email), ", ")
			tables = tables.WithGroupCells(e.ID, gid, map[string]string{"Removed": removed})
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": email}); err != nil {
			return err
		}
		if err := a.writer.Delete(appName, RSVPsTab, map[string]string{"Event ID": e.ID, "Email": email}); err != nil {
			return err
		}
		if group != nil {
			return a.writer.Set(appName, InviteGroupsTab, map[string]string{"Event ID": e.ID, "Group ID": group.ID}, map[string]string{"Removed": removed})
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: guest removed", "actor", actor, "event", e.ID, "email", email, "from group", group != nil)
	w.WriteHeader(http.StatusNoContent)
}

// addGuest is POST /api/calendar/invites/guest: someone invited bringing a
// guest by name - with an address, if they have one - or a host adding one
// for them.
func (a app) addGuest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
		Of    string `json:"of"`
		// Answer is the guest's word to record - yes, or nothing to leave
		// them to answer - and Invite whether to send them their own
		// invitation now; both on unless said otherwise.
		Answer *string `json:"answer"`
		Invite *bool   `json:"invite"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, admin := a.who(r)
	e := a.eventFor(actor, strings.TrimSpace(body.ID))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return
	}
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	answer, invite := AnswerYes, true
	if body.Answer != nil {
		answer = strings.ToLower(strings.TrimSpace(*body.Answer))
		if answer != "" && answer != AnswerYes {
			http.Error(w, "a guest is put down as yes, or left to answer", http.StatusBadRequest)
			return
		}
	}
	if body.Invite != nil {
		invite = *body.Invite
	}
	of := actor
	if body.Of != "" {
		of = a.directory.Resolve(normalizeEmail(body.Of))
	}
	if !a.isHost(actor, admin, e) {
		if inv == nil || !inv.Guests {
			http.Error(w, "this event is not taking guests", http.StatusForbidden)
			return
		}
		if !a.mayAnswerFor(actor, of, admin, e) || model.InviteOf(e.ID, of) == nil {
			http.Error(w, "a guest comes with someone on the list", http.StatusForbidden)
			return
		}
	}
	key, err := a.bringGuest(r.Context(), actor, e, of, body.Name, body.Email, answer, invite)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"email": key})
}

// bringGuest puts a guest on the list under whoever brought them - as a
// yes, or left to answer - and hands back the key the list holds them
// by. With invite, one with an address is sent their own invitation now. A guest with an
// address the directory does not hold gets a page of their own, as any
// outside person does, and their invite at once when the invites have
// gone out; one named without an address has nothing to be sent, so their
// row counts as sent from the start.
func (a app) bringGuest(ctx context.Context, actor string, e *Event, of, name, email, answer string, invite bool) (string, error) {
	model := a.cache.Model()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxTitleLength {
		return "", fmt.Errorf("a guest needs a name")
	}
	email = normalizeEmail(email)
	stamp := now().Format(DateTimeFormat)
	row := map[string]string{"Event ID": e.ID, "Email": email, "Name": name, "Guest Of": of, "Via": ViaGuest, "Added By": actor, "Added": stamp, "Sent": "", "Token": ""}
	if email != "" {
		if !emailForm.MatchString(email) {
			return "", fmt.Errorf("that is not an email address")
		}
		email = a.directory.Resolve(email)
		if model.InviteOf(e.ID, email) != nil {
			return "", fmt.Errorf("they are on the list already")
		}
		if p, known := a.directory.Person(email); known {
			row["Name"] = p.Name
		} else {
			row["Token"] = NewToken()
		}
		row["Email"] = email
	} else {
		email = newGuestKey()
		row["Email"], row["Sent"] = email, stamp
	}
	cells := map[string]string{"Email": email, "Event ID": e.ID, "Answer": answer, "Answered": stamp, "Answered By": actor, "Via": ViaPage}
	tables, first := a.ensured(a.cache.Tables(), e.ID, actor)
	tables = tables.WithInvites([]map[string]string{row}).WithAnswer(email, e.ID, answer, cells)
	built, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		return "", err
	}
	a.cache.set(tables, built)
	a.queue.Add(func() {
		if err := first(); err != nil {
			slog.ErrorContext(ctx, "calendar write", "error", err)
		}
		if err := a.writer.AppendCells(appName, InvitesTab, row); err != nil {
			slog.ErrorContext(ctx, "calendar write", "error", err)
		}
		if answer != "" {
			if err := a.writer.Set(appName, RSVPsTab, map[string]string{"Email": email, "Event ID": e.ID}, cells); err != nil {
				slog.ErrorContext(ctx, "calendar write", "error", err)
			}
		}
	})
	slog.InfoContext(ctx, "calendar: guest brought", "actor", actor, "event", e.ID, "of", of, "guest", email, "answer", answer, "invite", invite)
	// Their own invitation, when asked and they have somewhere to send it.
	if invite && !isGuestKey(email) {
		a.send(ctx, actor, e, []string{email}, "")
	}
	return email, nil
}

// answerFor is POST /api/calendar/invites/answer: an answer given for
// someone else - a parent's for a child, or anyone's in their household,
// a host's for anyone on the list.
func (a app) answerFor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Answer string `json:"answer"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, admin := a.who(r)
	e := a.eventFor(actor, strings.TrimSpace(body.ID))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return
	}
	subject := normalizeEmail(body.Email)
	if !isGuestKey(subject) {
		subject = a.directory.Resolve(subject)
	}
	if !a.mayAnswerFor(actor, subject, admin, e) {
		http.Error(w, "you can answer for yourself and your household", http.StatusForbidden)
		return
	}
	answer := strings.ToLower(strings.TrimSpace(body.Answer))
	if answer == AnswerHidden {
		http.Error(w, "hiding is a person's own", http.StatusBadRequest)
		return
	}
	// A yes for someone else sends them no invite: their household's, or
	// the invitation itself, is theirs already.
	if err := a.recordBy(r.Context(), actor, subject, e.ID, answer, ViaPage, subject == actor); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "calendar: answered for", "actor", actor, "subject", subject, "event", e.ID, "answer", answer)
	w.WriteHeader(http.StatusNoContent)
}

// sendInvites is POST /api/calendar/invites/send: a host sending the
// invites - to everyone not yet sent one, to everyone who has not
// answered (a reminder), or to everyone.
func (a app) sendInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		To string `json:"to"`
		// Emails names particular people on the list to send to, in place
		// of To.
		Emails []string `json:"emails"`
		// Update says the details changed: the invitation goes again as an
		// update, its calendar invite replacing the one they have.
		Update bool `json:"update"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	model := a.cache.Model()
	emails := []string{}
	reminder := false
	if len(body.Emails) > 0 {
		body.To = "these"
	}
	for _, inv := range model.Invites[e.ID] {
		switch body.To {
		case "sent":
			// Everyone the invitation has reached - for an update, when
			// the details have changed - but not whoever said no, for whom
			// the details no longer matter.
			if inv.Sent != "" && model.AnswerOf(inv.Email, e.ID) != AnswerNo {
				emails = append(emails, inv.Email)
			}
		case "these":
			if slices.Contains(body.Emails, inv.Email) {
				emails = append(emails, inv.Email)
				reminder = reminder || inv.Sent != ""
			}
		case "new", "":
			if inv.Sent == "" {
				emails = append(emails, inv.Email)
			}
		case "unanswered":
			if model.AnswerOf(inv.Email, e.ID) == "" {
				emails = append(emails, inv.Email)
				reminder = reminder || inv.Sent != ""
			}
		case "all":
			emails = append(emails, inv.Email)
			reminder = reminder || inv.Sent != ""
		default:
			http.Error(w, "send to new, unanswered, sent, or all", http.StatusBadRequest)
			return
		}
	}
	if len(emails) == 0 {
		http.Error(w, "nobody to send to", http.StatusBadRequest)
		return
	}
	kind := ""
	switch {
	case body.Update:
		kind = inviteUpdate
	case reminder:
		kind = inviteReminder
	}
	sent := a.send(r.Context(), actor, e, emails, kind)
	slog.InfoContext(r.Context(), "calendar: invites sent", "actor", actor, "event", e.ID, "invites", len(emails), "messages", sent)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"invites": len(emails), "messages": sent})
}

// markSent stamps Sent on the rows named - the invitation's too, the
// first time - and marks live any group whose people have now all been
// sent: from here on Auto-invite sends a newcomer theirs, rather than
// only putting them on the list. Sending and skipping both come here.
func (a app) markSent(ctx context.Context, e *Event, emails []string) {
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	stamp := now().Format(DateTimeFormat)
	tables := a.cache.Tables().WithInviteCells(e.ID, emails, map[string]string{"Sent": stamp})
	cells := map[string]string{}
	if inv != nil && inv.Sent == "" {
		cells["Sent"] = stamp
		tables = tables.WithInvitation(e.ID, cells)
	}
	live := []string{}
	for _, g := range model.Groups[e.ID] {
		if g.Sent == "" && !tables.groupUnsent(e.ID, g.ID) {
			live = append(live, g.ID)
			tables = tables.WithGroupCells(e.ID, g.ID, map[string]string{"Sent": stamp})
		}
	}
	built, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		slog.ErrorContext(ctx, "calendar: mark invites sent", "error", err)
		return
	}
	a.cache.set(tables, built)
	a.queue.Add(func() {
		for _, email := range emails {
			if err := a.writer.Set(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": email}, map[string]string{"Sent": stamp}); err != nil {
				slog.ErrorContext(ctx, "calendar write", "error", err)
			}
		}
		if len(cells) > 0 {
			if err := a.writer.Set(appName, InvitationsTab, map[string]string{"Event ID": e.ID}, cells); err != nil {
				slog.ErrorContext(ctx, "calendar write", "error", err)
			}
		}
		for _, gid := range live {
			if err := a.writer.Set(appName, InviteGroupsTab, map[string]string{"Event ID": e.ID, "Group ID": gid}, map[string]string{"Sent": stamp}); err != nil {
				slog.ErrorContext(ctx, "calendar write", "error", err)
			}
		}
	})
}

// noteOpened marks, once, that someone on the list has opened the
// invitation - the event's page, or their page from outside - the moment
// kept on their row for the hosts.
func (a app) noteOpened(ctx context.Context, e *Event, email string) {
	model := a.cache.Model()
	inv := model.InviteOf(e.ID, email)
	if inv == nil || inv.Opened != "" || inv.Sent == "" {
		return
	}
	stamp := now().Format(DateTimeFormat)
	tables := a.cache.Tables().WithInviteCells(e.ID, []string{email}, map[string]string{"Opened": stamp})
	built, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		slog.ErrorContext(ctx, "calendar: note opened", "error", err)
		return
	}
	a.cache.set(tables, built)
	a.queue.Add(func() {
		if err := a.writer.Set(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": email}, map[string]string{"Opened": stamp}); err != nil {
			slog.ErrorContext(ctx, "calendar write", "error", err)
		}
	})
}

// skipInvites is POST /api/calendar/invites/skip: a host passing over
// someone still to be sent their invite - marked sent without an email,
// so they leave Pending and stand among those with no reply yet; a
// reminder from then on reaches them like anyone else.
func (a app) skipInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string   `json:"id"`
		Emails []string `json:"emails"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	model := a.cache.Model()
	emails := []string{}
	for _, email := range body.Emails {
		email = normalizeEmail(email)
		if inv := model.InviteOf(e.ID, email); inv != nil && inv.Sent == "" {
			emails = append(emails, email)
		}
	}
	if len(emails) == 0 {
		http.Error(w, "nobody pending to skip", http.StatusBadRequest)
		return
	}
	a.markSent(r.Context(), e, emails)
	slog.InfoContext(r.Context(), "calendar: invites skipped", "actor", actor, "event", e.ID, "skipped", len(emails))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"skipped": len(emails)})
}

// ccFor is who hears with one person on the list: for a student, their
// parents, on the Cc of every message the student is sent, so they answer
// for the child; for anyone else nobody. Its second answer is false when
// the person's address is a placeholder nothing can reach, and nothing
// is sent. A message with a Cc carries no calendar invite, since whose
// place it holds would be ambiguous to the calendar apps reading it.
func (a app) ccFor(email string) ([]string, bool) {
	p, known := a.directory.Person(email)
	if !known {
		return nil, true
	}
	if p.EmailMasked {
		return nil, false
	}
	cc := []string{}
	if p.IsStudent && !p.IsParent && !p.IsStaff {
		for _, member := range a.cache.Model().Roster.Households[email] {
			if a.isAdult(member) && !slices.Contains(cc, member) {
				cc = append(cc, member)
			}
		}
	}
	return cc, true
}

// send mails the invites for the addresses given and marks them sent: one
// message per person - a student's to them and their parents, naming
// everyone in the household who is invited - with the calendar invite
// attached, the recipient its attendee. It hands back how many messages
// went out.
// The kinds of invitation: the first, a reminder to whoever has not
// answered, and an update once the details have changed.
const (
	inviteReminder = "reminder"
	inviteUpdate   = "update"
)

func (a app) send(ctx context.Context, actor string, e *Event, emails []string, kind string) int {
	model := a.cache.Model()
	e = model.invitedEvent(e)
	inv := model.Invitations[e.ID]
	// Who hears, and who they hear about: one message per person in the
	// batch - a student's to them with their parents on the Cc, so the
	// parents hear with the child and answer for them - naming everyone in
	// their household on the list, sent or not, themselves first.
	order := []string{}
	cc := map[string][]string{}
	for _, email := range emails {
		row := model.InviteOf(e.ID, email)
		if row == nil || isGuestKey(email) || slices.Contains(order, email) {
			continue
		}
		with, reachable := a.ccFor(email)
		if !reachable {
			continue
		}
		cc[email] = with
		order = append(order, email)
	}
	recipients := map[string][]string{}
	for _, t := range order {
		household := a.householdOn(e, t)
		names := []string{}
		for _, row := range model.Invites[e.ID] {
			if row.Email != t && !slices.Contains(household, row.Email) {
				continue
			}
			name := row.Name
			if p, known := a.directory.Person(row.Email); known && p.Name != "" {
				name = p.Name
			}
			if row.Email == t {
				names = append([]string{firstWord(name)}, names...)
			} else {
				names = append(names, firstWord(name))
			}
		}
		if len(names) == 0 {
			names = []string{firstWord(displayName(t))}
		}
		recipients[t] = names
	}
	a.markSent(ctx, e, emails)
	if a.mail.Sender == nil {
		return 0
	}
	hostName := actor
	if p, known := a.directory.Person(actor); known && p.Name != "" {
		hostName = p.Name
	}
	message := ""
	if inv != nil {
		message = inv.Message
	}
	for _, to := range order {
		// Someone from outside gets the page of their own; everyone else
		// the event's page here, behind sign-in.
		link := "https://when.heliosian.com" + EventPath(e)
		if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
			link = "https://when.heliosian.com" + extPath(row.Token)
		}
		go a.sendInvitation(context.WithoutCancel(ctx), to, cc[to], recipients[to], hostName, message, e, link, a.replyTo(e, to), kind)
	}
	return len(order)
}

// andList is names as a sentence lists them: "Sam", "Sam and Ella",
// "Sam, Ella and Robin".
func andList(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// replyTo is where a reply to one person's invitation goes: every host, so
// the person's reply reaches whoever is running it, with their invite's
// organizer beside them, so a calendar app's Accept or Decline still comes
// back here to be recorded when it answers the message rather than the
// organizer.
func (a app) replyTo(e *Event, to string) []string {
	return append(append([]string{}, a.hostsOf(e)...), a.organizer(e.ID, to))
}

// firstWord is a person's first name: the first word of their name.
func firstWord(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}

// sendInvitation mails one person their invitation, under "[the event]
// You're invited!": who is hosting, the event's title, when and where,
// its picture (the share card, public as every /open/share/ path is), the
// hosts' message, the description, who in their household is invited, an
// RSVP button to their page - the event's here, or an outside person's
// own - and, when the message is theirs alone, the calendar invite to
// accept into their own calendar, the recipient its attendee. A student's
// goes with their parents on the Cc (cc) and no calendar invite (ccFor);
// the lead names whose the invitation is - "sent Sam an invitation for" -
// so a parent on the Cc reads it as the child's. A reminder says so in
// the subject and the lead; an update says the details have changed, its
// calendar invite replacing the one they have (the same UID, a later
// SEQUENCE).
func (a app) sendInvitation(ctx context.Context, to string, cc, names []string, host, message string, e *Event, link string, replyTo []string, kind string) {
	origin := "https://when.heliosian.com"
	outside := strings.Contains(link, "/ext/")
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " \u00b7 " + hours
	}
	hosts := []string{}
	for _, h := range a.hostsOf(e) {
		if p, known := a.directory.Person(h); known && p.Name != "" {
			hosts = append(hosts, p.Name)
		}
	}
	if len(hosts) == 0 {
		hosts = []string{host}
	}
	hosting := "Hosted by " + strings.Join(hosts, " and ")
	invited := strings.Join(names, ", ")
	rsvpFor := "RSVP for " + andList(names) + " here"
	// The email as a card: who sent it over the title and the date, the
	// hosts' words, Open invitation, the picture - the flyer, else the
	// event's own - a word that the page is theirs, then the place with a
	// map, the hours, and the calendar links, as a printed invitation reads.
	picture := origin + "/open/share/" + e.ID + ".png"
	if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Flyer != "" {
		picture = origin + flyerPath(e.ID)
	}
	// The one invited, by name - "sent Sam an invitation for" - so the
	// parents on a student's Cc read whose it is, and everyone else theirs.
	whom := firstWord(displayName(to))
	if len(names) > 0 {
		whom = names[0]
	}
	sentBy := host + " sent " + whom + " an invitation for"
	switch kind {
	case inviteReminder:
		sentBy = host + " is still hoping to hear from " + whom + " about"
	case inviteUpdate:
		sentBy = host + " has updated the details of"
	}
	year := e.start.Format(", 2006")
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n%s%s\n", sentBy, e.Title, day, year)
	if hours != "" {
		fmt.Fprintf(&text, "%s\n", hours)
	}
	if message != "" {
		fmt.Fprintf(&text, "\n%s\n", message)
	}
	if e.Description != "" {
		fmt.Fprintf(&text, "\n%s\n", e.Description)
	}
	fmt.Fprintf(&text, "\n%s\nOpen the invitation: %s\n", rsvpFor, link)
	if e.Location != "" {
		fmt.Fprintf(&text, "\n%s\n", e.Location)
	}
	fmt.Fprintf(&text, "%s\n%s\n\nInvited: %s\n", hosting, when, invited)
	attached := ""
	if len(cc) == 0 {
		attached = " The invite attached puts it on your calendar."
		text.WriteString("\nThe invite attached puts it on your calendar.\n")
	}
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	esc := html.EscapeString
	var htm strings.Builder
	fmt.Fprintf(&htm, "<div style=\"max-width:600px;margin:0 auto;padding:8px 0;font-family:%s;color:#1b2a2c\">", font)
	htm.WriteString("<div style=\"background:#fff;border:1px solid #e6e6e6;border-radius:6px;padding:36px 32px 28px\">")
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 14px;font-size:16px;line-height:1.4;text-align:center;color:#1b2a2c\">%s</p>", esc(sentBy))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 10px;font-size:26px;line-height:1.25;font-weight:400;text-align:center;color:#1b2a2c\">%s</p>", esc(e.Title))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 28px;font-size:15px;text-align:center;color:#444\">%s%s</p>", esc(day), esc(year))
	if message != "" {
		fmt.Fprintf(&htm, "<p style=\"margin:0 0 20px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap\">%s</p>", esc(message))
	}
	if e.Description != "" {
		fmt.Fprintf(&htm, "<p style=\"margin:0 0 24px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap\">%s</p>", esc(e.Description))
	}
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 12px;font-size:15px;font-weight:700;text-align:center\"><a href=\"%s\" style=\"color:#1b2a2c;font-weight:700\">%s</a></p>", esc(link), esc(rsvpFor))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 28px;text-align:center\"><a href=\"%s\" style=\"display:inline-block;padding:13px 26px;border-radius:4px;background:#9a9a9a;color:#fff;font-size:14px;font-weight:600;letter-spacing:0.06em;text-decoration:none\">OPEN INVITATION</a></p>", esc(link))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 24px;text-align:center\"><a href=\"%s\"><img src=\"%s\" alt=\"%s\" width=\"480\" style=\"display:inline-block;width:100%%;max-width:480px;height:auto;border-radius:4px\"></a></p>", esc(link), esc(picture), esc(e.Title))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 6px;font-size:13px;font-style:italic;text-align:center;color:#777\">This email is for %s. Please do not forward it.</p>", esc(invited))
	htm.WriteString("<hr style=\"border:0;border-top:1px solid #e6e6e6;margin:22px 0\">")
	htm.WriteString("<div style=\"text-align:center;font-size:14px;line-height:1.7;color:#333\">")
	fmt.Fprintf(&htm, "<p style=\"margin:0;font-weight:700;color:#1b2a2c\">%s</p>", esc(hosting))
	if e.Location != "" {
		maps := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(e.Location)
		fmt.Fprintf(&htm, "<p style=\"margin:0\"><a href=\"%s\" style=\"color:#1a73e8\">%s</a> <a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none\">(View Map)</a></p>", esc(maps), esc(e.Location), esc(maps))
	}
	fmt.Fprintf(&htm, "<p style=\"margin:0\">%s</p>", esc(when))
	fmt.Fprintf(&htm, "<p style=\"margin:10px 0 0\"><a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">Add to Google</a> <a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">RSVP</a></p>", esc(googleCalendarURL(e, link)), esc(link))
	if outside {
		fmt.Fprintf(&htm, "<p style=\"margin:12px 0 0;font-size:12px;color:#777\">The page is yours alone - no account needed.%s</p>", attached)
	} else {
		fmt.Fprintf(&htm, "<p style=\"margin:12px 0 0;font-size:12px;color:#777\">Yes, no or maybe on the page answers for everyone in your household who is invited.%s</p>", attached)
	}
	htm.WriteString("</div></div>")
	fmt.Fprintf(&htm, "<p style=\"margin:14px 0 0;font-size:11px;letter-spacing:0.08em;text-align:center;color:#999\">SENT WITH HELIOS WHEN</p>")
	htm.WriteString("</div>")
	subject := "[" + e.Title + "] You're invited!"
	switch kind {
	case inviteReminder:
		subject = "[" + e.Title + "] Reminder: you're invited!"
	case inviteUpdate:
		subject = "[" + e.Title + "] Updated: the details have changed"
	}
	msg := mail.Message{
		To:       []string{to},
		CC:       cc,
		ReplyTo:  replyTo,
		FromName: strings.Join(hosts, " and "),
		Subject:  subject,
		Text:     text.String(),
		HTML:     htm.String(),
	}
	if len(cc) == 0 {
		msg.Attachments = []mail.Attachment{{
			Name:        "invite.ics",
			ContentType: "text/calendar; method=REQUEST; charset=utf-8",
			Content:     []byte(invite(a.organizer(e.ID, to), to, e, link, now())),
		}}
	}
	if err := a.mail.Sender.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "calendar: send invitation", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invitation sent", "to", to, "event", e.ID, "kind", kind)
}

// googleCalendarURL is the event as a Google Calendar template link, the
// page's address in its details.
func googleCalendarURL(e *Event, link string) string {
	stamp := func(t time.Time) string {
		if e.AllDay {
			return t.Format("20060102")
		}
		return t.UTC().Format("20060102T150405Z")
	}
	until := e.end
	if e.AllDay {
		until = e.end.AddDate(0, 0, 1)
	} else if !e.end.After(e.start) {
		until = e.start.Add(time.Hour)
	}
	q := url.Values{}
	q.Set("action", "TEMPLATE")
	q.Set("text", e.Title)
	q.Set("dates", stamp(e.start)+"/"+stamp(until))
	q.Set("details", strings.TrimSpace(e.Description+"\n\n"+link))
	q.Set("location", e.Location)
	return "https://calendar.google.com/calendar/render?" + q.Encode()
}

// answerWord is an answer as a message says it.
func answerWord(answer string) string {
	switch answer {
	case AnswerYes:
		return "Yes"
	case AnswerMaybe:
		return "Maybe"
	case AnswerNo:
		return "No"
	}
	return "No response yet"
}

// messageInvites is POST /api/calendar/invites/message: a host writing
// to the people on the list by where they stand - the yeses, the maybes,
// the nos, those with no response - one email per person, a student's to
// them and their parents, each carrying the host's words, then the recipient's own
// answer and their household's, and a nudge to answer for anyone who has
// not. Replies go to the host.
func (a app) messageInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string   `json:"id"`
		Subject string   `json:"subject"`
		Message string   `json:"message"`
		To      []string `json:"to"`
		// Emails names particular people on the list to write to, beside
		// or instead of To.
		Emails []string `json:"emails"`
		// Attach sends the calendar invite with it, as a reminder does.
		Attach bool `json:"attach"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len(message) > maxTextLength {
		http.Error(w, "a message needs some words", http.StatusBadRequest)
		return
	}
	subject := strings.Join(strings.Fields(body.Subject), " ")
	if subject == "" || len(subject) > maxTitleLength {
		http.Error(w, "a message needs a subject", http.StatusBadRequest)
		return
	}
	wanted := map[string]bool{}
	for _, t := range body.To {
		switch t {
		case AnswerYes, AnswerMaybe, AnswerNo, "none":
			wanted[t] = true
		default:
			http.Error(w, "send to yes, maybe, no, or none", http.StatusBadRequest)
			return
		}
	}
	if len(wanted) == 0 && len(body.Emails) == 0 {
		http.Error(w, "pick who to send to", http.StatusBadRequest)
		return
	}
	if a.mail.Sender == nil {
		http.Error(w, "mail is not set up", http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	standing := func(email string) string {
		if answer := model.AnswerOf(email, e.ID); answer != "" && answer != AnswerHidden {
			return answer
		}
		return "none"
	}
	// Who hears: those whose standing is wanted, a student's parents on the
	// Cc with them.
	targets := []string{}
	cc := map[string][]string{}
	for _, inv := range model.Invites[e.ID] {
		if (!wanted[standing(inv.Email)] && !slices.Contains(body.Emails, inv.Email)) || isGuestKey(inv.Email) || slices.Contains(targets, inv.Email) {
			continue
		}
		with, reachable := a.ccFor(inv.Email)
		if !reachable {
			continue
		}
		cc[inv.Email] = with
		targets = append(targets, inv.Email)
	}
	if len(targets) == 0 {
		http.Error(w, "nobody on the list stands where you chose", http.StatusBadRequest)
		return
	}
	hostName := actor
	if p, known := a.directory.Person(actor); known && p.Name != "" {
		hostName = p.Name
	}
	// A reply reaches every host, the writer among them.
	replyTo := a.hostsOf(e)
	if !slices.Contains(replyTo, actor) {
		replyTo = append([]string{actor}, replyTo...)
	}
	for _, to := range targets {
		go a.sendMessage(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, subject, message, e, body.Attach)
	}
	slog.InfoContext(r.Context(), "calendar: message sent", "actor", actor, "event", e.ID, "to", len(targets))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"messages": len(targets)})
}

// sendMessage mails one person a host's message - under the host's
// subject, the event's name before it in brackets - the words, the
// event's line, then their own answer and their household's - each on
// the list, the guests they brought too - and, for anyone among them
// still to answer, the way to. A student's goes with their parents on the
// Cc (cc), and the calendar invite the host may ask for (attach) rides
// only on a message with no Cc (ccFor).
func (a app) sendMessage(ctx context.Context, to string, cc, replyTo []string, hostName, subject, message string, e *Event, attach bool) {
	model := a.cache.Model()
	e = model.invitedEvent(e)
	origin := "https://when.heliosian.com"
	link := origin + EventPath(e)
	if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
		link = origin + extPath(row.Token)
	}
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " \u00b7 " + hours
	}
	// The household's rows: the recipient first, then the rest on the
	// list, then the guests any of them brought.
	household := a.householdOn(e, to)
	type line struct{ name, answer string }
	lines := []line{}
	waiting := false
	for _, inv := range model.Invites[e.ID] {
		mine := slices.Contains(household, inv.Email) || (inv.GuestOf != "" && slices.Contains(household, inv.GuestOf))
		if !mine {
			continue
		}
		name := inv.Name
		if p, known := a.directory.Person(inv.Email); known && p.Name != "" {
			name = p.Name
		}
		if inv.Email == to {
			name = "You"
		}
		answer := model.AnswerOf(inv.Email, e.ID)
		if answer == AnswerHidden {
			answer = ""
		}
		if answer == "" {
			waiting = true
		}
		l := line{name, answerWord(answer)}
		if inv.Email == to {
			lines = append([]line{l}, lines...)
		} else {
			lines = append(lines, l)
		}
	}
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	var text, htm strings.Builder
	fmt.Fprintf(&text, "A message from %s about %s:\n\n%s\n\n%s\n%s\n", hostName, e.Title, message, e.Title, when)
	fmt.Fprintf(&htm, "<p style=\"font:14px/1.5 %s;color:#647071\">A message from %s about <strong>%s</strong></p>", font, html.EscapeString(hostName), html.EscapeString(e.Title))
	fmt.Fprintf(&htm, "<div style=\"font:16px/1.55 %s;white-space:pre-wrap\">%s</div>", font, html.EscapeString(message))
	fmt.Fprintf(&htm, "<p style=\"font:15px/1.5 %s;color:#0e4d54;margin-top:18px\"><strong>%s</strong><br>%s", font, html.EscapeString(e.Title), html.EscapeString(when))
	if e.Location != "" {
		fmt.Fprintf(&text, "%s\n", e.Location)
		fmt.Fprintf(&htm, "<br>%s", html.EscapeString(e.Location))
	}
	htm.WriteString("</p>")
	if len(lines) > 0 {
		text.WriteString("\nYour RSVP:\n")
		fmt.Fprintf(&htm, "<table style=\"border-collapse:collapse;font:14px/1.5 %s;margin-top:10px\"><tr><td colspan=\"2\" style=\"padding:0 0 4px;font-weight:700\">Your RSVP</td></tr>", font)
		for _, l := range lines {
			fmt.Fprintf(&text, "  %s: %s\n", l.name, l.answer)
			color := "#1d6b48"
			switch l.answer {
			case "Maybe":
				color = "#8a5a00"
			case "No":
				color = "#333"
			case "No response yet":
				color = "#b3261e"
			}
			fmt.Fprintf(&htm, "<tr><td style=\"padding:2px 14px 2px 0\">%s</td><td style=\"padding:2px 0;color:%s;font-weight:600\">%s</td></tr>", html.EscapeString(l.name), color, l.answer)
		}
		htm.WriteString("</table>")
	}
	if waiting {
		fmt.Fprintf(&text, "\nSomeone in your household has not answered yet - please RSVP: %s\n", link)
		fmt.Fprintf(&htm, "<p style=\"font:14px/1.5 %s;color:#b3261e;margin-top:14px\">Someone in your household has not answered yet - the hosts would love to know.</p>", font)
		fmt.Fprintf(&htm, "<p style=\"margin:12px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px %s;text-decoration:none\">RSVP now</a></p>", html.EscapeString(link), font)
	} else {
		fmt.Fprintf(&text, "\nThe event's page: %s\n", link)
		fmt.Fprintf(&htm, "<p style=\"margin:16px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px %s;text-decoration:none\">Open the event</a></p>", html.EscapeString(link), font)
	}
	msg := mail.Message{
		To:      []string{to},
		CC:      cc,
		ReplyTo: replyTo,
		Subject: "[" + e.Title + "] " + subject,
		Text:    text.String(),
		HTML:    htm.String(),
	}
	if attach && len(cc) == 0 {
		msg.Attachments = []mail.Attachment{{Name: "invite.ics", ContentType: "text/calendar; method=REQUEST; charset=utf-8", Content: []byte(invite(a.organizer(e.ID, to), to, e, link, now()))}}
	}
	err := a.mail.Sender.Send(ctx, msg)
	if err != nil {
		slog.ErrorContext(ctx, "calendar: send message", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: message sent", "to", to, "event", e.ID)
}

// PartyRSVPs is a party's guest list as Helios Celebrate shows it to the
// party's hosts: whether the invites have gone out, and each invitee's
// answer - yes, maybe, no, or "none" for one still to answer - by the
// address the directory keys them by. Nil for a party with no list.
func (c *Cache) PartyRSVPs(partyID string) (sent bool, answers map[string]string, ok bool) {
	return c.LinkedRSVPs(nil, SourceCelebrate, partyID)
}

// LinkedRSVPs is the same for any linked event - an HCA event's, as
// HCA-Team shows its chairs - by the source and the id it has there,
// given the linked events as the calendar sees them for nobody in
// particular, since an HCA event the school also lists is folded into
// the school's listing and keeps its guest list under the school's id.
func (c *Cache) LinkedRSVPs(linked []Linked, source, id string) (sent bool, answers map[string]string, ok bool) {
	model := c.Model()
	key := source + "/" + id
	for _, e := range withLinked(model.Events, linked) {
		if e.Link != "" && e.LinkedID == id && e.Source != SourceCelebrate {
			key = e.ID
			break
		}
	}
	id = key
	inv := model.Invitations[id]
	if inv == nil {
		return false, nil, false
	}
	answers = map[string]string{}
	for _, row := range model.Invites[id] {
		answer := model.AnswerOf(row.Email, id)
		if answer == "" || answer == AnswerHidden {
			answer = "none"
		}
		answers[row.Email] = answer
	}
	return inv.Sent != "", answers, true
}

// flyerPath is where an event's invitation flyer is fetched, public as
// every /open/ path is, so a mail client and an outside person's page
// can show it.
func flyerPath(id string) string {
	return "/open/flyer/" + id
}

// flyer serves GET /open/flyer/{id}: the invitation's flyer as bytes.
// banner is GET /open/banner/{id}: the picture an event's page wears -
// its own, its first tag's, or the calendar's header - public, for an
// outside person's page, which sits before sign-in. The share card
// already shows the same picture to anyone with the link.
func (a app) banner(w http.ResponseWriter, r *http.Request) {
	e := a.event(strings.TrimSpace(r.PathValue("id")))
	if e == nil {
		http.NotFound(w, r)
		return
	}
	data := a.readImage(a.cache.Model().pictureOf(e))
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

func (a app) flyer(w http.ResponseWriter, r *http.Request) {
	inv := a.cache.Model().Invitations[strings.TrimSpace(r.PathValue("id"))]
	if inv == nil || inv.Flyer == "" {
		http.NotFound(w, r)
		return
	}
	data := a.readImage(inv.Flyer)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}
