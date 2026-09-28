package when

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
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
	PublicList  *bool    `json:"-"`
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
		var publicList *bool
		if strings.TrimSpace(row["Public Guest List"]) != "" {
			open, err := cells.YesNo(row["Public Guest List"], false)
			if err != nil {
				b.refuse("invitation for %s: public guest list %v", id, err)
			}
			publicList = &open
		}
		guests, err := cells.YesNo(row["Guests"], true)
		if err != nil {
			b.refuse("invitation for %s: guests %v", id, err)
		}
		inv := &Invitation{
			EventID: id, Hosts: []string{}, Audience: strings.ToLower(strings.TrimSpace(row["Audience"])), Guests: guests, Message: strings.TrimSpace(row["Message"]),
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

func flyerPath(id string) string {
	return "/open/flyer/" + id
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
