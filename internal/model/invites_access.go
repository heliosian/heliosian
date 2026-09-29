package model

import (
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/mail"
)

const notOnCalendar = "that event is not on the calendar"

func (e *Event) guestsWithoutInvite() bool {
	return (e.Sharing == SharingPublic || e.Sharing == SharingLink) && e.Source != SourceCelebrate
}

func (e *Event) openToAll() bool {
	return e.Sharing == SharingPublic && e.Source != SourceCelebrate && !e.Cancelled
}

func (e *Event) asks(p *Person) bool {
	if slices.Contains(e.Tags, TagStaff) {
		return p.IsStaff
	}
	if isAdult(p) {
		return true
	}
	return !slices.Contains(e.Tags, TagParents) && (len(e.Classrooms) == 0 || slices.Contains(e.Classrooms, p.Classroom))
}

func (e *Event) keepsGuestList() bool {
	return e.Source == SourceSheet || e.linked() || e.imported()
}

func (a calendarApp) findEvent(actor access.Actor, id string) (*Event, error) {
	e := a.eventFor(actor, strings.TrimSpace(id))
	if e == nil {
		return nil, access.Missing(notOnCalendar)
	}
	return e, nil
}

func (a calendarApp) party(e *Event) *PartyPeople {
	if e == nil || e.Source != SourceCelebrate {
		return nil
	}
	return a.parties().PartyPeople(e.linkedID())
}

func isAdult(p *Person) bool {
	return !p.IsStudent || p.IsParent || p.IsStaff
}

func (m *Calendar) hostsOf(directory *Directory, e *Event) []string {
	out := []string{}
	add := func(email string) {
		if email = directory.Resolve(mail.Normalize(email)); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	if e.Source == SourceSheet && !e.PosterLeft {
		add(e.AddedBy)
	}
	for _, h := range e.Hosts {
		add(h)
	}
	if inv := m.Invitations[e.ID]; inv != nil {
		for _, h := range inv.Hosts {
			add(h)
		}
	}
	return out
}

func (m *Calendar) hostedBy(directory *Directory, email string, e *Event) bool {
	return e.keepsGuestList() && slices.Contains(m.hostsOf(directory, e), mail.Normalize(email))
}

func (a calendarApp) hostsOf(e *Event) []string {
	return a.model().hostsOf(a.directory(), e)
}

func (a calendarApp) isHost(actor access.Actor, e *Event) bool {
	return slices.Contains(a.hostsOf(e), actor.Email) || (actor.May(ActAsHost) && e.keepsGuestList())
}

func (a calendarApp) guestListEvent(actor access.Actor, id string) (*Event, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, err
	}
	if !e.keepsGuestList() {
		return nil, access.Invalid("that event keeps no guest list")
	}
	return e, nil
}

func (m *Calendar) othersInvite(id string) bool {
	inv := m.Invitations[id]
	return inv == nil || inv.Guests
}

func (m *Calendar) listPrivate(e *Event) bool {
	if inv := m.Invitations[e.ID]; inv != nil && inv.PublicList != nil {
		return !*inv.PublicList
	}
	return e.imported()
}

func (a calendarApp) inviterEvent(actor access.Actor, id string) (*Event, bool, error) {
	e, err := a.guestListEvent(actor, id)
	if err != nil {
		return nil, false, err
	}
	host := a.isHost(actor, e)
	if host {
		return e, true, nil
	}
	model := a.model()
	if !model.othersInvite(e.ID) {
		return nil, false, access.Forbidden("only the hosts invite people to this event")
	}
	if e.Sharing != SharingPublic && !model.Invited(a.directory(), actor.Email, e.ID) {
		return nil, false, access.Forbidden("only a host, or someone invited, may invite others")
	}
	return e, false, nil
}

func (a calendarApp) hostedEvent(actor access.Actor, id string) (*Event, error) {
	e, err := a.guestListEvent(actor, id)
	if err != nil {
		return nil, err
	}
	if !a.isHost(actor, e) {
		return nil, access.Forbidden("only a host can change the guest list")
	}
	return e, nil
}

func (a calendarApp) household(email string) []string {
	out := []string{email}
	adults, kids := a.directory().Household(email)
	for _, p := range append(adults, kids...) {
		out = append(out, p.Email)
	}
	return out
}

func (a calendarApp) householdOn(e *Event, email string) []string {
	if a.directory().Person(email) != nil {
		return a.household(email)
	}
	model := a.model()
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

func (a calendarApp) mayAnswerFor(actor access.Actor, subject string, e *Event) bool {
	return a.isHost(actor, e) || a.speaksFor(actor, subject, e)
}

func (a calendarApp) speaksFor(actor access.Actor, subject string, e *Event) bool {
	if actor.Mine(subject) {
		return true
	}
	inv := a.model().InviteOf(e.ID, subject)
	return inv != nil && inv.GuestOf != "" && actor.Mine(inv.GuestOf)
}
