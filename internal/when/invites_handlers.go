package when

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

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
	host := a.isHost(viewer, e)
	row := func(email, name string, invited bool) GuestRow {
		p, known := a.personOf(email, name)
		g := GuestRow{Person: p, Key: email, Invited: invited, Outside: known == nil, Ticket: tickets[email], Mine: host || a.speaksFor(viewer, email, e), Household: householdOf(email)}
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
	e, err := a.findEvent(actor, r.URL.Query().Get("id"))
	if err != nil {
		return InviteView{}, err
	}
	host := a.isHost(actor, e)
	a.noteOpened(r.Context(), actor, e)
	if host {
		a.sweepEvent(r.Context(), e)
	}
	model := a.cache.Model()
	inv := model.Invitations[e.ID]
	poster := ""
	if e.Source == SourceSheet && !e.PosterLeft {
		poster = a.directory().Resolve(config.NormalizeEmail(e.AddedBy))
	}
	guests := model.othersInvite(e.ID)
	view := InviteView{Host: host, Poster: poster, MayInvite: host || guests && (e.Sharing == SharingPublic || model.Invited(a.directory(), viewer, e.ID)), Party: e.Source == SourceCelebrate, Linked: e.linked(), Guests: guests, Hosts: []Person{}, Mine: []GuestRow{}}
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
		view.Sent = inv.Sent
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
				view.Counts.Invited++
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
	view.ListPrivate = model.listPrivate(e)
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
	Guests      *bool    `json:"guests"`
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
	chosen := []string{}
	for _, inv := range model.Invites[e.ID] {
		if wanted[standing(inv.Email)] || slices.Contains(body.Emails, inv.Email) {
			chosen = append(chosen, inv.Email)
		}
	}
	targets, cc := a.recipients(e, chosen)
	if len(targets) == 0 {
		return nil, access.Invalid("nobody on the list stands where you chose")
	}
	hostName, replyTo := a.senderAndReplyTo(actor, e)
	for _, to := range targets {
		go a.sendMessage(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, subject, message, e, body.Attach)
	}
	slog.InfoContext(r.Context(), "calendar: message sent", "actor", actor.Email, "event", e.ID, "to", len(targets))
	return map[string]int{"messages": len(targets)}, nil
}

func (a app) serveImage(w http.ResponseWriter, r *http.Request, name string) {
	data := a.images.Read(name)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

func (a app) banner(w http.ResponseWriter, r *http.Request) {
	e := a.event(strings.TrimSpace(r.PathValue("id")))
	if e == nil {
		http.NotFound(w, r)
		return
	}
	a.serveImage(w, r, a.cache.Model().pictureOf(e))
}

func (a app) flyer(w http.ResponseWriter, r *http.Request) {
	inv := a.cache.Model().Invitations[strings.TrimSpace(r.PathValue("id"))]
	if inv == nil || inv.Flyer == "" {
		http.NotFound(w, r)
		return
	}
	a.serveImage(w, r, inv.Flyer)
}
