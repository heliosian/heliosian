package calendar

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	AudienceAdults   = "adults"
	AudienceStudents = "students"
	AudienceBoth     = "both"
	ViaGuest         = "guest"
	guestPrefix      = "guest-"
)

type Invitation struct {
	EventID     string   `json:"eventId"`
	Hosts       []string `json:"hosts"`
	Audience    string   `json:"audience"`
	Guests      bool     `json:"guests"`
	Message     string   `json:"message"`
	CreatedBy   string   `json:"createdBy"`
	Created     string   `json:"created"`
	Sent        string   `json:"sent,omitempty"`
	Notify      []string `json:"-"`
	SteppedDown string   `json:"-"`
	HideHosts   bool     `json:"hideHosts"`
	PublicList  bool     `json:"publicList"`
	Title       string   `json:"title,omitempty"`
	Start       string   `json:"start,omitempty"`
	End         string   `json:"end,omitempty"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Flyer       string   `json:"flyer,omitempty"`
}

type Invite struct {
	EventID   string `json:"-"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	GuestOf   string `json:"guestOf,omitempty"`
	Via       string `json:"via,omitempty"`
	AddedBy   string `json:"addedBy"`
	Added     string `json:"added"`
	Sent      string `json:"sent,omitempty"`
	Token     string `json:"-"`
	Household string `json:"household,omitempty"`
	Opened    string `json:"opened,omitempty"`
}

type PartyPeople struct {
	Hosts     []string
	Attendees []Attendee
}

// Celebrate is what the calendar asks of Helios Celebrate: a party's hosts
// and tickets, whether someone is one of its admins, and moving an address
// on every party - its tickets there, and the guest lists here - when an
// alum's school account closes.
type Celebrate struct {
	Party       func(id string) *PartyPeople
	IsAdmin     func(email string) bool
	MoveAddress func(ctx context.Context, actor, old, to, name string) error
}

type Attendee struct {
	Email  string `json:"email,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type List struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	People []string `json:"people"`
}

func (b *builder) invitations(settings, rows []store.Row) {
	for _, row := range settings {
		id := strings.TrimSpace(row["Event ID"])
		if id == "" {
			continue
		}
		inv := &Invitation{
			EventID: id, Hosts: []string{}, Audience: strings.ToLower(strings.TrimSpace(row["Audience"])), Guests: true, Message: strings.TrimSpace(row["Message"]),
			CreatedBy: normalizeEmail(row["Created By"]), Created: strings.TrimSpace(row["Created"]), Sent: strings.TrimSpace(row["Sent"]),
			Title: strings.TrimSpace(row["Title"]), Start: strings.TrimSpace(row["Start"]), End: strings.TrimSpace(row["End"]),
			Location: strings.TrimSpace(row["Location"]), Description: strings.TrimSpace(row["Description"]),
			Flyer: strings.Trim(strings.TrimSpace(row["Flyer"]), "/"), Notify: splitEmails(row["Notify"]),
			SteppedDown: normalizeEmail(row["Stepped Down"]),
			HideHosts:   strings.EqualFold(strings.TrimSpace(row["Hide Hosts"]), "Yes"),
			PublicList:  strings.EqualFold(strings.TrimSpace(row["Public Guest List"]), "Yes"),
		}
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
		if b.model.listed[email] == nil {
			b.model.listed[email] = map[string]bool{}
		}
		b.model.listed[email][id] = true
		if inv.Sent == "" {
			continue
		}
		if b.model.invited[email] == nil {
			b.model.invited[email] = map[string]bool{}
		}
		b.model.invited[email][id] = true
	}
}

func (m *Model) mine(directory Directory, email string) []string {
	email = normalizeEmail(email)
	out := []string{email}
	for _, member := range directory.Household(email) {
		if slices.Contains(directory.Parents(member), email) {
			out = append(out, member)
		}
	}
	return out
}

func (m *Model) Listed(directory Directory, email, id string) bool {
	return slices.ContainsFunc(m.mine(directory, email), func(who string) bool { return m.listed[who][id] })
}

func (m *Model) InviteOf(id, email string) *Invite {
	for i := range m.Invites[id] {
		if m.Invites[id][i].Email == normalizeEmail(email) {
			return &m.Invites[id][i]
		}
	}
	return nil
}

func (m *Model) InviteByToken(token string) (Invite, bool) {
	inv, ok := m.byInvite[strings.TrimSpace(token)]
	return inv, ok
}

func (m *Model) Invited(directory Directory, email, id string) bool {
	return m.invitedAny(m.mine(directory, email), id)
}

func (m *Model) invitedAny(mine []string, id string) bool {
	return slices.ContainsFunc(mine, func(who string) bool { return m.invited[who][id] })
}

func (inv *Invitation) hasDetails() bool {
	return inv != nil && (inv.Title != "" || inv.Start != "" || inv.Location != "" || inv.Description != "")
}

func (m *Model) invitedEvent(e *Event) *Event {
	if e == nil {
		return nil
	}
	inv := m.Invitations[e.ID]
	if !e.linked() || !inv.hasDetails() {
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

func (m *Model) withInvitation(e *Event) *Event {
	if e == nil || e.Invitation || m.Invitations[e.ID] == nil {
		return e
	}
	c := *e
	c.Invitation = true
	return &c
}

func isGuestKey(email string) bool {
	return strings.HasPrefix(email, guestPrefix) || !strings.Contains(email, "@")
}

func newGuestKey() string {
	return guestPrefix + strings.ToLower(newEventID())
}

func (a app) hostsOf(e *Event) []string {
	out := []string{}
	add := func(email string) {
		if email = a.directory.Resolve(normalizeEmail(email)); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	switch e.Source {
	case SourceSheet:
		if !e.PosterLeft {
			add(e.AddedBy)
		}
	case SourceCelebrate:
		if a.parties != nil {
			if p := a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/")); p != nil {
				for _, h := range p.Hosts {
					add(h)
				}
			}
		}
	}
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

func (a app) isHost(email string, admin bool, e *Event) bool {
	return slices.Contains(a.hostsOf(e), email) || (admin && (e.Source == SourceSheet || e.linked() || e.imported()))
}

func (a app) party(e *Event) *PartyPeople {
	if e == nil || e.Source != SourceCelebrate || a.parties == nil {
		return nil
	}
	return a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/"))
}

func (a app) inviterEvent(w http.ResponseWriter, r *http.Request, id string) (string, *Event, bool, bool) {
	actor, admin := a.who(r)
	e := a.eventFor(actor, admin, strings.TrimSpace(id))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return actor, nil, false, false
	}
	if e.Source != SourceSheet && !e.linked() && !e.imported() {
		http.Error(w, "that event keeps no guest list", http.StatusBadRequest)
		return actor, nil, false, false
	}
	host := a.isHost(actor, admin, e)
	if !host && e.Sharing != SharingPublic && !a.cache.Model().Invited(a.directory, actor, e.ID) {
		http.Error(w, "only a host, or someone invited, may invite others", http.StatusForbidden)
		return actor, nil, false, false
	}
	return actor, e, host, true
}

func (a app) hostedEvent(w http.ResponseWriter, r *http.Request, id string) (string, *Event, bool) {
	actor, admin := a.who(r)
	e := a.eventFor(actor, admin, strings.TrimSpace(id))
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return actor, nil, false
	}
	if e.Source != SourceSheet && !e.linked() && !e.imported() {
		http.Error(w, "that event keeps no guest list", http.StatusBadRequest)
		return actor, nil, false
	}
	if !a.isHost(actor, admin, e) {
		http.Error(w, "only a host can change the guest list", http.StatusForbidden)
		return actor, nil, false
	}
	return actor, e, true
}

func (a app) household(email string) []string {
	return append([]string{email}, a.directory.Household(email)...)
}

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

func (a app) isAdult(email string) bool {
	p, known := a.directory.Person(email)
	return !known || !p.IsStudent || p.IsParent || p.IsStaff
}

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
	if inv := a.cache.Model().InviteOf(e.ID, subject); inv != nil && inv.GuestOf != "" && slices.Contains(a.household(actor), inv.GuestOf) {
		return true
	}
	return false
}

type GuestRow struct {
	Person
	Key          string   `json:"key"`
	GuestOf      string   `json:"guestOf,omitempty"`
	GuestOfName  string   `json:"guestOfName,omitempty"`
	Via          string   `json:"via,omitempty"`
	Invited      bool     `json:"invited"`
	Sent         string   `json:"sent,omitempty"`
	Answer       string   `json:"answer,omitempty"`
	AnsweredBy   string   `json:"answeredBy,omitempty"`
	AnsweredAt   string   `json:"answeredAt,omitempty"`
	AnsweredVia  string   `json:"answeredVia,omitempty"`
	Opened       string   `json:"opened,omitempty"`
	InvitedBy    string   `json:"invitedBy,omitempty"`
	Ticket       string   `json:"ticket,omitempty"`
	Outside      bool     `json:"outside,omitempty"`
	Mine         bool     `json:"mine,omitempty"`
	Household    string   `json:"household,omitempty"`
	Link         string   `json:"link,omitempty"`
	Warning      string   `json:"warning,omitempty"`
	WarningWords string   `json:"warningWords,omitempty"`
	Grades       []string `json:"grades,omitempty"`
	Classrooms   []string `json:"classrooms,omitempty"`
}

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

func gradeRank(grade string) int {
	if strings.HasPrefix(strings.ToLower(grade), "k") {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(grade, "Grade ")); err == nil {
		return n
	}
	return 100
}

type EventWords struct {
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

type Counts struct {
	Invited        int `json:"invited"`
	Yes            int `json:"yes"`
	Maybe          int `json:"maybe"`
	No             int `json:"no"`
	Waiting        int `json:"waiting"`
	Guests         int `json:"guests"`
	Tickets        int `json:"tickets,omitempty"`
	TicketsWaiting int `json:"ticketsWaiting,omitempty"`
}

type InviteView struct {
	Host      bool `json:"host"`
	AdminHost bool `json:"adminHost,omitempty"`
	// MoveEverywhere says the viewer may move an address on every Celebrate
	// party at once: a party's list, opened by one of Celebrate's admins.
	MoveEverywhere bool          `json:"moveEverywhere,omitempty"`
	Poster         string        `json:"poster,omitempty"`
	MayInvite      bool          `json:"mayInvite,omitempty"`
	NotifyMe       bool          `json:"notifyMe,omitempty"`
	Linked         bool          `json:"linked,omitempty"`
	Party          bool          `json:"party,omitempty"`
	Settings       *Invitation   `json:"settings,omitempty"`
	Guests         bool          `json:"guests"`
	Sent           string        `json:"sent,omitempty"`
	Flyer          string        `json:"flyer,omitempty"`
	HostsHidden    bool          `json:"hostsHidden,omitempty"`
	ListPrivate    bool          `json:"listPrivate,omitempty"`
	Hosts          []Person      `json:"hosts"`
	Original       *EventWords   `json:"original,omitempty"`
	Mine           []GuestRow    `json:"mine"`
	Coming         []GuestRow    `json:"coming"`
	List           []GuestRow    `json:"list"`
	Groups         []InviteGroup `json:"groups,omitempty"`
	Counts         Counts        `json:"counts"`
}

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
	linked := []GuestRow{}
	for email, answers := range model.Answered {
		if ans, ok := answers[e.ID]; ok && !seen[email] && ans.Answer != AnswerHidden {
			linked = append(linked, row(email, "", false))
		}
	}
	sort.Slice(linked, func(i, j int) bool { return linked[i].Name < linked[j].Name })
	return append(out, linked...)
}

func (a app) invitesView(w http.ResponseWriter, r *http.Request) {
	viewer, admin := a.who(r)
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	e := a.eventFor(viewer, admin, id)
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return
	}
	host := a.isHost(viewer, admin, e)
	a.noteOpened(r.Context(), e, viewer)
	if host {
		a.sweepEvent(r.Context(), e)
	}
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	adminHost := host && !slices.Contains(a.hostsOf(e), viewer)
	poster := ""
	if e.Source == SourceSheet && !e.PosterLeft {
		poster = a.directory.Resolve(normalizeEmail(e.AddedBy))
	}
	view := InviteView{Host: host, AdminHost: adminHost, Poster: poster, MayInvite: host || e.Sharing == SharingPublic || model.Invited(a.directory, viewer, e.ID), Party: e.Source == SourceCelebrate, Linked: e.linked(), Guests: true, Hosts: []Person{}, Mine: []GuestRow{}}
	view.MoveEverywhere = host && view.Party && a.celebrate.IsAdmin != nil && a.celebrate.IsAdmin(viewer)
	if inv != nil && inv.Flyer != "" {
		view.Flyer = flyerPath(e.ID)
	}
	if host && e.linked() {
		view.Original = &EventWords{Title: e.Title, Start: e.Start, End: e.End, Location: e.Location, Description: e.Description}
	}
	view.HostsHidden = inv != nil && inv.HideHosts
	for _, h := range a.hostsOf(e) {
		if view.HostsHidden && !host {
			break
		}
		p, _ := a.personOf(h, "")
		view.Hosts = append(view.Hosts, p)
	}
	if inv != nil {
		view.Guests, view.Sent = inv.Guests, inv.Sent
		if host {
			view.Settings = inv
			view.NotifyMe = slices.Contains(inv.Notify, viewer)
		}
	}
	rows := a.rows(viewer, admin, e)
	mine := model.mine(a.directory, viewer)
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
	sort.SliceStable(view.Mine, func(i, j int) bool { return view.Mine[i].Email == viewer && view.Mine[j].Email != viewer })
	view.Coming = []GuestRow{}
	view.ListPrivate = e.imported() && !(inv != nil && inv.PublicList)
	for _, g := range rows {
		if view.ListPrivate && !host {
			break
		}
		if g.Answer == AnswerYes || g.Answer == AnswerMaybe || (g.Invited && g.Answer == "") {
			if !host {
				g.Ticket, g.Sent, g.Via, g.Warning, g.WarningWords = "", "", "", "", ""
				g.AnsweredBy, g.AnsweredAt, g.AnsweredVia, g.Link, g.Opened = "", "", "", "", ""
			}
			view.Coming = append(view.Coming, g)
		}
	}
	if view.ListPrivate && !host {
		view.Coming = nil
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
		slog.ErrorContext(r.Context(), "[ERROR] encode guest list", "error", err)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

type PickerPerson struct {
	Person
	Household []string `json:"household,omitempty"`
	Parents   []string `json:"parents,omitempty"`
	Children  []string `json:"children,omitempty"`
	Siblings  []string `json:"siblings,omitempty"`
}

type PickerView struct {
	People     []PickerPerson `json:"people"`
	Classrooms []Classroom    `json:"classrooms"`
	Lists      []List         `json:"lists"`
	Attendees  []Attendee     `json:"attendees,omitempty"`
	OnList     []string       `json:"onList"`
}

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
		pp := PickerPerson{Person: p, Household: a.directory.Household(p.Email)}
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
		slog.ErrorContext(r.Context(), "[ERROR] encode guest picker", "error", err)
	}
}

func (a app) invitationOps(id, actor string, cells store.Row) []store.Op {
	if a.cache.Model().Invitations[id] != nil {
		if len(cells) == 0 {
			return nil
		}
		return []store.Op{store.Update(InvitationsTab, store.Row{"Event ID": id}, cells)}
	}
	row := store.Row{"Event ID": id, "Audience": "Both", "Guests": "Yes", "Created By": actor, "Created": now().Format(DateTimeFormat)}
	maps.Copy(row, cells)
	return []store.Op{store.Insert(InvitationsTab, row)}
}

func (a app) inviteSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string   `json:"id"`
		Audience    string   `json:"audience"`
		Message     *string  `json:"message"`
		Hosts       []string `json:"hosts"`
		Title       *string  `json:"title"`
		Start       *string  `json:"start"`
		End         *string  `json:"end"`
		Location    *string  `json:"location"`
		Description *string  `json:"description"`
		Flyer       *string  `json:"flyer"`
		NotifyMe    *bool    `json:"notifyMe"`
		HideHosts   *bool    `json:"hideHosts"`
		PublicList  *bool    `json:"publicList"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	cells := store.Row{}
	if body.HideHosts != nil {
		cells["Hide Hosts"] = ""
		if *body.HideHosts {
			cells["Hide Hosts"] = "Yes"
		}
	}
	if body.PublicList != nil {
		cells["Public Guest List"] = ""
		if *body.PublicList {
			cells["Public Guest List"] = "Yes"
		}
	}
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
	if body.Message != nil {
		if len(*body.Message) > maxTextLength {
			http.Error(w, "the message is too long", http.StatusBadRequest)
			return
		}
		cells["Message"] = strings.TrimSpace(*body.Message)
	}
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
	if !a.commit(w, r, actor, a.invitationOps(e.ID, actor, cells)...) {
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

func (a app) stepDown(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	who := actor
	if email := a.directory.Resolve(normalizeEmail(body.Email)); email != "" && email != actor {
		if _, admin := a.who(r); !admin {
			http.Error(w, "only a calendar admin can step someone else down", http.StatusForbidden)
			return
		}
		who = email
	}
	inv := a.cache.Model().Invitations[e.ID]
	cohost := inv != nil && slices.Contains(inv.Hosts, who)
	poster := e.Source == SourceSheet && !e.PosterLeft && a.directory.Resolve(normalizeEmail(e.AddedBy)) == who
	if !cohost && !poster {
		http.Error(w, "that person hosts this event on the app that runs it, or not at all - step down there", http.StatusBadRequest)
		return
	}
	cells := store.Row{}
	if poster {
		cells["Stepped Down"] = normalizeEmail(e.AddedBy)
	}
	if inv != nil {
		if cohost {
			cells["Hosts"] = JoinList(slices.DeleteFunc(slices.Clone(inv.Hosts), func(h string) bool { return h == who }))
		}
		if slices.Contains(inv.Notify, who) {
			cells["Notify"] = strings.Join(slices.DeleteFunc(slices.Clone(inv.Notify), func(h string) bool { return h == who }), ", ")
		}
	}
	if !a.commit(w, r, actor, a.invitationOps(e.ID, actor, cells)...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: host stepped down", "actor", actor, "who", who, "event", e.ID, "poster", poster)
	w.WriteHeader(http.StatusNoContent)
}

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
		slog.ErrorContext(ctx, "[ERROR] calendar: send co-host note", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: co-host told", "to", to, "event", e.ID)
}

func (a app) addInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		People []struct {
			Email     string `json:"email"`
			Name      string `json:"name"`
			Via       string `json:"via"`
			Household string `json:"household"`
		} `json:"people"`
	}
	if !decode(w, r, &body) {
		return
	}
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
	ops := a.invitationOps(e.ID, actor, nil)
	emails := []string{}
	for _, p := range body.People {
		name := strings.TrimSpace(p.Name)
		household := normalizeEmail(p.Household)
		email := a.directory.Resolve(normalizeEmail(p.Email))
		token := ""
		switch {
		case email == "" && household != "" && name != "":
			email = newGuestKey()
		case !emailForm.MatchString(email):
			http.Error(w, fmt.Sprintf("%q is not an email address", p.Email), http.StatusBadRequest)
			return
		default:
			token = NewToken()
			if person, known := a.directory.Person(email); known {
				name, token, household = person.Name, "", ""
			}
		}
		if slices.Contains(emails, email) || model.InviteOf(e.ID, email) != nil {
			continue
		}
		emails = append(emails, email)
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
			via = ViaInvited
		}
		ops = append(ops, store.Insert(InvitesTab, store.Row{"Event ID": e.ID, "Email": email, "Name": name, "Via": via, "Added By": actor, "Added": stamp, "Token": token, "Household": household}))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	sent := 0
	if !host {
		if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Sent != "" {
			sent = a.send(r.Context(), actor, actor, e, emails, "")
		}
	}
	slog.InfoContext(r.Context(), "calendar: guests added", "actor", actor, "event", e.ID, "count", len(emails), "host", host, "sent", sent)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"added": len(emails), "sent": sent})
}

func (a app) removeInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, admin := a.who(r)
	e := a.eventFor(actor, admin, strings.TrimSpace(body.ID))
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
	ops := []store.Op{store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": email})}
	var group *InviteGroup
	if gid, ok := strings.CutPrefix(inv.Via, ViaGroup); ok {
		if group = a.cache.Model().GroupOf(e.ID, gid); group != nil {
			ops = append(ops, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": gid}, store.Row{"Removed": strings.Join(append(slices.Clone(group.Removed), email), ", ")}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: guest removed", "actor", actor, "event", e.ID, "email", email, "from group", group != nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addGuest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		Email  string  `json:"email"`
		Of     string  `json:"of"`
		Answer *string `json:"answer"`
		Invite *bool   `json:"invite"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, admin := a.who(r)
	e := a.eventFor(actor, admin, strings.TrimSpace(body.ID))
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

func (a app) bringGuest(ctx context.Context, actor string, e *Event, of, name, email, answer string, invite bool) (string, error) {
	model := a.cache.Model()
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxTitleLength {
		return "", fmt.Errorf("a guest needs a name")
	}
	email = normalizeEmail(email)
	stamp := now().Format(DateTimeFormat)
	row := store.Row{"Event ID": e.ID, "Name": name, "Guest Of": of, "Via": ViaGuest, "Added By": actor, "Added": stamp}
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
	} else {
		email = newGuestKey()
		row["Sent"] = stamp
	}
	row["Email"] = email
	ops := append(a.invitationOps(e.ID, actor, nil), store.Insert(InvitesTab, row))
	if answer != "" {
		ops = append(ops, store.Set(RSVPsTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Answer": answer, "Answered": stamp, "Answered By": actor, "Via": ViaPage}))
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "calendar: guest brought", "actor", actor, "event", e.ID, "of", of, "guest", email, "answer", answer, "invite", invite)
	if invite && !isGuestKey(email) {
		a.send(ctx, actor, actor, e, []string{email}, "")
	}
	return email, nil
}

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
	e := a.eventFor(actor, admin, strings.TrimSpace(body.ID))
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
	if err := a.recordBy(r.Context(), actor, subject, e.ID, answer, ViaPage, subject == actor, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "calendar: answered for", "actor", actor, "subject", subject, "event", e.ID, "answer", answer)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) sendInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string   `json:"id"`
		To     string   `json:"to"`
		Emails []string `json:"emails"`
		Update bool     `json:"update"`
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
	sent := a.send(r.Context(), actor, actor, e, emails, kind)
	slog.InfoContext(r.Context(), "calendar: invites sent", "actor", actor, "event", e.ID, "invites", len(emails), "messages", sent)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"invites": len(emails), "messages": sent})
}

func (a app) markSent(ctx context.Context, actor string, e *Event, emails []string) {
	model := a.cache.Model()
	stamp := now().Format(DateTimeFormat)
	ops := []store.Op{}
	for _, email := range emails {
		ops = append(ops, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Sent": stamp}))
	}
	if inv := model.Invitations[e.ID]; inv != nil && inv.Sent == "" {
		ops = append(ops, store.Update(InvitationsTab, store.Row{"Event ID": e.ID}, store.Row{"Sent": stamp}))
	}
	for _, g := range model.Groups[e.ID] {
		unsent := slices.ContainsFunc(model.Invites[e.ID], func(inv Invite) bool {
			return inv.Via == ViaGroup+g.ID && inv.Sent == "" && !slices.Contains(emails, inv.Email)
		})
		if g.Sent == "" && !unsent {
			ops = append(ops, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Sent": stamp}))
		}
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: mark invites sent", "event", e.ID, "error", err)
	}
}

func (a app) noteOpened(ctx context.Context, e *Event, email string) {
	inv := a.cache.Model().InviteOf(e.ID, email)
	if inv == nil || inv.Opened != "" || inv.Sent == "" {
		return
	}
	if err := a.cache.Commit(ctx, email, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Opened": now().Format(DateTimeFormat)})); err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: note opened", "event", e.ID, "error", err)
	}
}

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
	a.markSent(r.Context(), actor, e, emails)
	slog.InfoContext(r.Context(), "calendar: invites skipped", "actor", actor, "event", e.ID, "skipped", len(emails))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"skipped": len(emails)})
}

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
		for _, member := range a.directory.Household(email) {
			if a.isAdult(member) && !slices.Contains(cc, member) {
				cc = append(cc, member)
			}
		}
	}
	return cc, true
}

const (
	inviteReminder = "reminder"
	inviteUpdate   = "update"
)

func (a app) send(ctx context.Context, actor, host string, e *Event, emails []string, kind string) int {
	model := a.cache.Model()
	e = model.invitedEvent(e)
	inv := model.Invitations[e.ID]
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
	a.markSent(ctx, actor, e, emails)
	if a.mail.Sender == nil {
		return 0
	}
	hostName := host
	if p, known := a.directory.Person(host); known && p.Name != "" {
		hostName = p.Name
	}
	message := ""
	if inv != nil {
		message = inv.Message
	}
	for _, to := range order {
		link := "https://when.heliosian.com" + EventPath(e)
		if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
			link = "https://when.heliosian.com" + extPath(row.Token)
		}
		go a.sendInvitation(context.WithoutCancel(ctx), to, cc[to], recipients[to], hostName, message, e, link, a.replyTo(e, to), kind)
	}
	return len(order)
}

func andList(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func (a app) replyTo(e *Event, to string) []string {
	return append(append([]string{}, a.hostsOf(e)...), a.organizer(e.ID, to))
}

func firstWord(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}

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
	picture := origin + "/open/share/" + e.ID + ".png"
	if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Flyer != "" {
		picture = origin + flyerPath(e.ID)
	}
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
		slog.ErrorContext(ctx, "[ERROR] calendar: send invitation", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invitation sent", "to", to, "event", e.ID, "kind", kind)
}

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

func (a app) messageInvites(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string   `json:"id"`
		Subject string   `json:"subject"`
		Message string   `json:"message"`
		To      []string `json:"to"`
		Emails  []string `json:"emails"`
		Attach  bool     `json:"attach"`
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
		slog.ErrorContext(ctx, "[ERROR] calendar: send message", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: message sent", "to", to, "event", e.ID)
}

func (c *Cache) PartyRSVPs(partyID string) (sent bool, answers map[string]string, ok bool) {
	return c.LinkedRSVPs(nil, SourceCelebrate, partyID)
}

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

func flyerPath(id string) string {
	return "/open/flyer/" + id
}

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
