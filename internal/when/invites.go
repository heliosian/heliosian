package when

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/who"
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

type Celebrate struct {
	Party       func(id string) *PartyPeople
	IsAdmin     func(email string) bool
	MoveAddress func(ctx context.Context, actor access.Actor, old, to, name string) error
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
		hideHosts, err := cells.YesNo(row["Hide Hosts"], false)
		if err != nil {
			b.refuse("invitation for %s: hide hosts %v", id, err)
		}
		publicList, err := cells.YesNo(row["Public Guest List"], false)
		if err != nil {
			b.refuse("invitation for %s: public guest list %v", id, err)
		}
		inv := &Invitation{
			EventID: id, Hosts: []string{}, Audience: strings.ToLower(strings.TrimSpace(row["Audience"])), Guests: true, Message: strings.TrimSpace(row["Message"]),
			CreatedBy: config.NormalizeEmail(row["Created By"]), Created: strings.TrimSpace(row["Created"]), Sent: strings.TrimSpace(row["Sent"]),
			Title: strings.TrimSpace(row["Title"]), Start: strings.TrimSpace(row["Start"]), End: strings.TrimSpace(row["End"]),
			Location: strings.TrimSpace(row["Location"]), Description: strings.TrimSpace(row["Description"]),
			Flyer: strings.Trim(strings.TrimSpace(row["Flyer"]), "/"), Notify: splitEmails(row["Notify"]),
			SteppedDown: config.NormalizeEmail(row["Stepped Down"]),
			HideHosts:   hideHosts,
			PublicList:  publicList,
		}
		if inv.Start != "" {
			if _, _, _, err := parseWhen(inv.Start, inv.End); err != nil {
				slog.Warn("calendar: invitation's own time skipped", "event", id, "error", err)
				inv.Start, inv.End = "", ""
			}
		}
		for _, h := range cells.SplitList(row["Hosts"]) {
			inv.Hosts = append(inv.Hosts, config.NormalizeEmail(h))
		}
		if inv.Audience != AudienceAdults && inv.Audience != AudienceStudents {
			inv.Audience = AudienceBoth
		}
		b.model.Invitations[id] = inv
	}
	for _, row := range rows {
		id, email := strings.TrimSpace(row["Event ID"]), config.NormalizeEmail(row["Email"])
		if id == "" || email == "" {
			continue
		}
		if slices.ContainsFunc(b.model.Invites[id], func(i Invite) bool { return i.Email == email }) {
			continue
		}
		inv := Invite{
			EventID: id, Email: email, Name: strings.TrimSpace(row["Name"]), GuestOf: config.NormalizeEmail(row["Guest Of"]), Via: strings.TrimSpace(row["Via"]),
			AddedBy: config.NormalizeEmail(row["Added By"]), Added: strings.TrimSpace(row["Added"]), Sent: strings.TrimSpace(row["Sent"]), Token: strings.TrimSpace(row["Token"]),
			Household: config.NormalizeEmail(row["Household"]), Opened: strings.TrimSpace(row["Opened"]),
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

func (m *Model) mine(directory *who.Model, email string) []string {
	email = config.NormalizeEmail(email)
	out := []string{email}
	adults, kids := directory.Household(email)
	for _, member := range append(adults, kids...) {
		if slices.ContainsFunc(directory.Parents(member.Email), func(p *who.Person) bool { return p.Email == email }) {
			out = append(out, member.Email)
		}
	}
	return out
}

func (m *Model) Listed(directory *who.Model, email, id string) bool {
	return slices.ContainsFunc(m.mine(directory, email), func(who string) bool { return m.listed[who][id] })
}

func (m *Model) InviteOf(id, email string) *Invite {
	for i := range m.Invites[id] {
		if m.Invites[id][i].Email == config.NormalizeEmail(email) {
			return &m.Invites[id][i]
		}
	}
	return nil
}

func (m *Model) InviteByToken(token string) (Invite, bool) {
	inv, ok := m.byInvite[strings.TrimSpace(token)]
	return inv, ok
}

func (m *Model) Invited(directory *who.Model, email, id string) bool {
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

func (a app) party(e *Event) *PartyPeople {
	if e == nil || e.Source != SourceCelebrate {
		return nil
	}
	return a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/"))
}

func isAdult(p *who.Person) bool {
	return !p.IsStudent || p.IsParent || p.IsStaff
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

func (a app) facetsOf(p *who.Person) (grades, classrooms []string) {
	add := func(k *who.Person) {
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
		for _, k := range a.directory().Children(p.Email) {
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
	Host           bool          `json:"host"`
	AdminHost      bool          `json:"adminHost,omitempty"`
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

func (a app) personOf(email, name string) (Person, *who.Person) {
	directory := a.directory()
	if known := directory.Person(email); known != nil {
		return personView(directory, known), known
	}
	p := Person{Email: email, Name: name}
	if p.Name == "" && !isGuestKey(email) {
		p.Name = cells.DisplayName(email)
	}
	if isGuestKey(email) {
		p.Email = ""
	}
	return p, nil
}

func (a app) rows(viewer access.Actor, e *Event) []GuestRow {
	model := a.cache.Model()
	tickets := map[string]string{}
	if p := a.party(e); p != nil {
		for _, t := range p.Attendees {
			if t.Email != "" {
				tickets[config.NormalizeEmail(t.Email)] = t.Status
			}
		}
	}
	names := map[string]string{}
	for _, inv := range model.Invites[e.ID] {
		names[inv.Email] = inv.Name
	}
	nameOf := func(email string) string {
		if p := a.directory().Person(email); p != nil && p.FullName != "" {
			return p.FullName
		}
		if names[email] != "" {
			return names[email]
		}
		return cells.DisplayName(email)
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
		g := GuestRow{Person: p, Key: email, Invited: invited, Outside: known == nil, Ticket: tickets[email], Mine: a.mayAnswerFor(viewer, email, e), Household: householdOf(email)}
		if known != nil {
			g.Grades, g.Classrooms = a.facetsOf(known)
		}
		g.Warning, g.WarningWords = a.addressWarning(model, email, known != nil)
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
	host := a.isHost(viewer, e)
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

func (a app) invitesView(r *http.Request, _ serve.None) (InviteView, error) {
	actor := a.actor(r)
	viewer := actor.Email
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	e := a.eventFor(viewer, actor.Admin, id)
	if e == nil {
		return InviteView{}, access.Missing("that event is not on the calendar")
	}
	host := a.isHost(actor, e)
	a.noteOpened(r.Context(), actor, e)
	if host {
		a.sweepEvent(r.Context(), e)
	}
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	adminHost := host && !slices.Contains(a.hostsOf(e), viewer)
	poster := ""
	if e.Source == SourceSheet && !e.PosterLeft {
		poster = a.directory().Resolve(config.NormalizeEmail(e.AddedBy))
	}
	view := InviteView{Host: host, AdminHost: adminHost, Poster: poster, MayInvite: host || e.Sharing == SharingPublic || model.Invited(a.directory(), viewer, e.ID), Party: e.Source == SourceCelebrate, Linked: e.linked(), Guests: true, Hosts: []Person{}, Mine: []GuestRow{}}
	view.MoveEverywhere = host && view.Party && a.celebrate.IsAdmin(viewer)
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
	rows := a.rows(actor, e)
	mine := model.mine(a.directory(), viewer)
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
	return view, nil
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

func (a app) invitePeople(r *http.Request, _ serve.None) (PickerView, error) {
	var e *Event
	actor := a.actor(r)
	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		var err error
		if e, _, err = a.inviterEvent(actor, id); err != nil {
			return PickerView{}, err
		}
	}
	model := a.cache.Model()
	view := PickerView{People: []PickerPerson{}, Classrooms: model.Roster.Classrooms, Lists: []List{}, OnList: []string{}}
	directory := a.directory()
	for _, p := range directory.Listed() {
		pp := PickerPerson{Person: personView(directory, p), Household: []string{}}
		adults, kids := directory.Household(p.Email)
		for _, o := range append(adults, kids...) {
			pp.Household = append(pp.Household, o.Email)
			switch {
			case p.IsStudent && o.IsParent:
				pp.Parents = append(pp.Parents, o.Email)
			case p.IsStudent && o.IsStudent:
				pp.Siblings = append(pp.Siblings, o.Email)
			case p.IsParent && o.IsStudent:
				pp.Children = append(pp.Children, o.Email)
			}
		}
		view.People = append(view.People, pp)
	}
	if lists := a.lists(actor.Email); lists != nil {
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
	return view, nil
}

type settingsBody struct {
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

func (a app) inviteSettings(r *http.Request, body settingsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, newHosts, err := a.settingsOps(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: guest list settings", "actor", actor.Email, "event", e.ID)
	for _, h := range newHosts {
		go a.sendCohostNote(context.WithoutCancel(r.Context()), h, actor.Email, e)
	}
	return serve.None{}, nil
}

type personBody struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (a app) stepDown(r *http.Request, body personBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, who, poster, err := a.stepDownOps(actor, body.ID, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: host stepped down", "actor", actor.Email, "who", who, "event", e.ID, "poster", poster)
	return serve.None{}, nil
}

func (a app) sendCohostNote(ctx context.Context, to, actor string, e *Event) {
	e = a.cache.Model().invitedEvent(e)
	who := actor
	if p := a.directory().Person(actor); p != nil && p.FullName != "" {
		who = p.FullName
	}
	l := a.letterFor(e, EventPath(e))
	l.Heading = "You're a co-host"
	l.Intro = fmt.Sprintf("%s made you a co-host of %s. As a co-host you can build and send the guest list, read every answer, message the guests, and replies to the invitation reach you.", who, e.Title)
	l.Button = "Open the event"
	if err := a.mail.Sender.Send(ctx, l.Message("["+e.Title+"] You're a co-host", []string{to}, nil, []string{actor})); err != nil {
		slog.ErrorContext(ctx, "calendar: send co-host note", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: co-host told", "to", to, "event", e.ID)
}

type invitee struct {
	Email     string `json:"email"`
	Name      string `json:"name"`
	Via       string `json:"via"`
	Household string `json:"household"`
}

type inviteesBody struct {
	ID     string    `json:"id"`
	People []invitee `json:"people"`
}

func (a app) addInvites(r *http.Request, body inviteesBody) (map[string]int, error) {
	actor := a.actor(r)
	ops, e, emails, host, err := a.inviteOps(actor, body.ID, body.People)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	sent := 0
	if !host {
		if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Sent != "" {
			sent = a.send(r.Context(), actor, actor.Email, e, emails, "")
		}
	}
	slog.InfoContext(r.Context(), "calendar: guests added", "actor", actor.Email, "event", e.ID, "count", len(emails), "host", host, "sent", sent)
	return map[string]int{"added": len(emails), "sent": sent}, nil
}

func (a app) removeInvite(r *http.Request, body personBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, email, fromGroup, err := a.uninviteOps(actor, body.ID, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: guest removed", "actor", actor.Email, "event", e.ID, "email", email, "from group", fromGroup)
	return serve.None{}, nil
}

type guestBody struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Email  string  `json:"email"`
	Of     string  `json:"of"`
	Answer *string `json:"answer"`
	Invite *bool   `json:"invite"`
}

type guestKey struct {
	Email string `json:"email"`
}

func (a app) addGuest(r *http.Request, body guestBody) (guestKey, error) {
	actor := a.actor(r)
	ops, g, err := a.bringGuestOps(actor, body)
	if err != nil {
		return guestKey{}, err
	}
	if err := a.bringGuest(r.Context(), actor, ops, g); err != nil {
		return guestKey{}, err
	}
	return guestKey{Email: g.key}, nil
}

func (a app) bringGuest(ctx context.Context, actor access.Actor, ops []store.Op, g broughtGuest) error {
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: guest brought", "actor", actor.Email, "event", g.event.ID, "of", g.of, "guest", g.key, "answer", g.answer, "invite", g.invite)
	if g.invite && !isGuestKey(g.key) {
		a.send(ctx, actor, actor.Email, g.event, []string{g.key}, "")
	}
	return nil
}

type answerForBody struct {
	ID     string `json:"id"`
	Email  string `json:"email"`
	Answer string `json:"answer"`
}

func (a app) answerFor(r *http.Request, body answerForBody) (serve.None, error) {
	actor := a.actor(r)
	e, subject, err := a.answerSubject(actor, body.ID, body.Email, body.Answer)
	if err != nil {
		return serve.None{}, err
	}
	answer := strings.ToLower(strings.TrimSpace(body.Answer))
	if err := a.recordBy(r.Context(), actor, subject, e.ID, answer, ViaPage, subject == actor.Email, false); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: answered for", "actor", actor.Email, "subject", subject, "event", e.ID, "answer", answer)
	return serve.None{}, nil
}

type sendBody struct {
	ID     string   `json:"id"`
	To     string   `json:"to"`
	Emails []string `json:"emails"`
	Update bool     `json:"update"`
}

func (a app) sendInvites(r *http.Request, body sendBody) (map[string]int, error) {
	actor := a.actor(r)
	e, err := a.hostedEvent(actor, body.ID)
	if err != nil {
		return nil, err
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
			return nil, access.Invalid("send to new, unanswered, sent, or all")
		}
	}
	if len(emails) == 0 {
		return nil, access.Invalid("nobody to send to")
	}
	kind := ""
	switch {
	case body.Update:
		kind = inviteUpdate
	case reminder:
		kind = inviteReminder
	}
	sent := a.send(r.Context(), actor, actor.Email, e, emails, kind)
	slog.InfoContext(r.Context(), "calendar: invites sent", "actor", actor.Email, "event", e.ID, "invites", len(emails), "messages", sent)
	return map[string]int{"invites": len(emails), "messages": sent}, nil
}

func (a app) markSent(ctx context.Context, actor access.Actor, e *Event, emails []string) {
	if err := a.cache.Commit(ctx, actor, a.sentOps(actor, e, emails)...); err != nil {
		slog.ErrorContext(ctx, "calendar: mark invites sent", "event", e.ID, "error", err)
	}
}

func (a app) noteOpened(ctx context.Context, actor access.Actor, e *Event) {
	ops := a.openedOps(actor, e)
	if len(ops) == 0 {
		return
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "calendar: note opened", "event", e.ID, "error", err)
	}
}

type skipBody struct {
	ID     string   `json:"id"`
	Emails []string `json:"emails"`
}

func (a app) skipInvites(r *http.Request, body skipBody) (map[string]int, error) {
	actor := a.actor(r)
	e, err := a.hostedEvent(actor, body.ID)
	if err != nil {
		return nil, err
	}
	model := a.cache.Model()
	emails := []string{}
	for _, email := range body.Emails {
		email = config.NormalizeEmail(email)
		if inv := model.InviteOf(e.ID, email); inv != nil && inv.Sent == "" {
			emails = append(emails, email)
		}
	}
	if len(emails) == 0 {
		return nil, access.Invalid("nobody pending to skip")
	}
	a.markSent(r.Context(), actor, e, emails)
	slog.InfoContext(r.Context(), "calendar: invites skipped", "actor", actor.Email, "event", e.ID, "skipped", len(emails))
	return map[string]int{"skipped": len(emails)}, nil
}

func (a app) ccFor(email string) ([]string, bool) {
	directory := a.directory()
	p := directory.Person(email)
	if p == nil {
		return nil, true
	}
	if p.EmailMasked {
		return nil, false
	}
	cc := []string{}
	if !isAdult(p) {
		adults, kids := directory.Household(email)
		for _, member := range append(adults, kids...) {
			if isAdult(member) && !slices.Contains(cc, member.Email) {
				cc = append(cc, member.Email)
			}
		}
	}
	return cc, true
}

const (
	inviteReminder = "reminder"
	inviteUpdate   = "update"
)

func (a app) send(ctx context.Context, actor access.Actor, host string, e *Event, emails []string, kind string) int {
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
			if p := a.directory().Person(row.Email); p != nil && p.FullName != "" {
				name = p.FullName
			}
			if row.Email == t {
				names = append([]string{FirstWord(name)}, names...)
			} else {
				names = append(names, FirstWord(name))
			}
		}
		if len(names) == 0 {
			names = []string{FirstWord(cells.DisplayName(t))}
		}
		recipients[t] = names
	}
	a.markSent(ctx, actor, e, emails)
	hostName := host
	if p := a.directory().Person(host); p != nil && p.FullName != "" {
		hostName = p.FullName
	}
	message := ""
	if inv != nil {
		message = inv.Message
	}
	for _, to := range order {
		link := a.mail.Base + EventPath(e)
		if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
			link = a.mail.Base + extPath(row.Token)
		}
		go a.sendInvitation(context.WithoutCancel(ctx), to, cc[to], recipients[to], hostName, message, e, link, a.replyTo(e, to), kind)
	}
	return len(order)
}

func (a app) replyTo(e *Event, to string) []string {
	return append(append([]string{}, a.hostsOf(e)...), a.organizer(e.ID, to))
}

func FirstWord(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}

func (a app) sendInvitation(ctx context.Context, to string, cc, names []string, host, message string, e *Event, link string, replyTo []string, kind string) {
	origin := a.mail.Base
	outside := strings.Contains(link, "/ext/")
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " \u00b7 " + hours
	}
	hosts := []string{}
	for _, h := range a.hostsOf(e) {
		if p := a.directory().Person(h); p != nil && p.FullName != "" {
			hosts = append(hosts, p.FullName)
		}
	}
	if len(hosts) == 0 {
		hosts = []string{host}
	}
	hosting := "Hosted by " + strings.Join(hosts, " and ")
	invited := strings.Join(names, ", ")
	rsvpFor := "RSVP for " + joinNames(names) + " here"
	picture := origin + "/open/share/" + e.ID + ".png"
	if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Flyer != "" {
		picture = origin + flyerPath(e.ID)
	}
	whom := FirstWord(cells.DisplayName(to))
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
	fmt.Fprintf(&htm, "<p style=\"margin:10px 0 0\"><a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">Add to Google</a> <a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">RSVP</a></p>", esc(e.mailEvent(link).GoogleLink()), esc(link))
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
		msg.Attachments = []mail.Attachment{a.invite(e, to, link, mail.MethodRequest)}
	}
	if err := a.mail.Sender.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "calendar: send invitation", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invitation sent", "to", to, "event", e.ID, "kind", kind)
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

type messageBody struct {
	ID      string   `json:"id"`
	Subject string   `json:"subject"`
	Message string   `json:"message"`
	To      []string `json:"to"`
	Emails  []string `json:"emails"`
	Attach  bool     `json:"attach"`
}

func (a app) messageInvites(r *http.Request, body messageBody) (map[string]int, error) {
	actor := a.actor(r)
	e, err := a.hostedEvent(actor, body.ID)
	if err != nil {
		return nil, err
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len(message) > maxTextLength {
		return nil, access.Invalid("a message needs some words")
	}
	subject := strings.Join(strings.Fields(body.Subject), " ")
	if subject == "" || len(subject) > maxTitleLength {
		return nil, access.Invalid("a message needs a subject")
	}
	wanted := map[string]bool{}
	for _, t := range body.To {
		switch t {
		case AnswerYes, AnswerMaybe, AnswerNo, "none":
			wanted[t] = true
		default:
			return nil, access.Invalid("send to yes, maybe, no, or none")
		}
	}
	if len(wanted) == 0 && len(body.Emails) == 0 {
		return nil, access.Invalid("pick who to send to")
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
		return nil, access.Invalid("nobody on the list stands where you chose")
	}
	hostName := actor.Email
	if p := a.directory().Person(actor.Email); p != nil && p.FullName != "" {
		hostName = p.FullName
	}
	replyTo := a.hostsOf(e)
	if !slices.Contains(replyTo, actor.Email) {
		replyTo = append([]string{actor.Email}, replyTo...)
	}
	for _, to := range targets {
		go a.sendMessage(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, subject, message, e, body.Attach)
	}
	slog.InfoContext(r.Context(), "calendar: message sent", "actor", actor.Email, "event", e.ID, "to", len(targets))
	return map[string]int{"messages": len(targets)}, nil
}

func (a app) sendMessage(ctx context.Context, to string, cc, replyTo []string, hostName, subject, message string, e *Event, attach bool) {
	model := a.cache.Model()
	e = model.invitedEvent(e)
	path := EventPath(e)
	if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
		path = extPath(row.Token)
	}
	household := a.householdOn(e, to)
	lines := [][2]string{}
	waiting := false
	for _, inv := range model.Invites[e.ID] {
		mine := slices.Contains(household, inv.Email) || (inv.GuestOf != "" && slices.Contains(household, inv.GuestOf))
		if !mine {
			continue
		}
		name := inv.Name
		if p := a.directory().Person(inv.Email); p != nil && p.FullName != "" {
			name = p.FullName
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
		l := [2]string{name, answerWord(answer)}
		if inv.Email == to {
			lines = append([][2]string{l}, lines...)
		} else {
			lines = append(lines, l)
		}
	}
	l := a.letterFor(e, path)
	l.Heading = subject
	l.Intro = fmt.Sprintf("A message from %s about %s:", hostName, e.Title)
	l.Note = message
	l.Rows = lines
	l.Button = "Open the event"
	if waiting {
		l.Button = "RSVP now"
		l.Footnote = "Someone in your household has not answered yet - the hosts would love to know."
	}
	msg := l.Message("["+e.Title+"] "+subject, []string{to}, cc, replyTo)
	if attach && len(cc) == 0 {
		msg.Attachments = []mail.Attachment{a.invite(e, to, l.Path, mail.MethodRequest)}
	}
	err := a.mail.Sender.Send(ctx, msg)
	if err != nil {
		slog.ErrorContext(ctx, "calendar: send message", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: message sent", "to", to, "event", e.ID)
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
	data := a.images.Read(a.cache.Model().pictureOf(e))
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
	data := a.images.Read(inv.Flyer)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}
