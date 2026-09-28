package when

import (
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/who"
)

const notOnCalendar = "that event is not on the calendar"

func (e *Event) keepsGuestList() bool {
	return e.Source == SourceSheet || e.linked() || e.imported()
}

func (a app) findEvent(actor access.Actor, id string) (*Event, error) {
	e := a.eventFor(actor.Email, actor.Admin, strings.TrimSpace(id))
	if e == nil {
		return nil, access.Missing(notOnCalendar)
	}
	return e, nil
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

func (a app) hostsOf(e *Event) []string {
	out := []string{}
	add := func(email string) {
		if email = a.directory().Resolve(config.NormalizeEmail(email)); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	switch e.Source {
	case SourceSheet:
		if !e.PosterLeft {
			add(e.AddedBy)
		}
	case SourceCelebrate:
		if p := a.parties(strings.TrimPrefix(e.ID, SourceCelebrate+"/")); p != nil {
			for _, h := range p.Hosts {
				add(h)
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

func (a app) isHost(actor access.Actor, e *Event) bool {
	return slices.Contains(a.hostsOf(e), actor.Email) || (actor.Admin && e.keepsGuestList())
}

func (a app) guestListEvent(actor access.Actor, id string) (*Event, error) {
	e, err := a.findEvent(actor, id)
	if err != nil {
		return nil, err
	}
	if !e.keepsGuestList() {
		return nil, access.Invalid("that event keeps no guest list")
	}
	return e, nil
}

func (a app) inviterEvent(actor access.Actor, id string) (*Event, bool, error) {
	e, err := a.guestListEvent(actor, id)
	if err != nil {
		return nil, false, err
	}
	host := a.isHost(actor, e)
	if !host && e.Sharing != SharingPublic && !a.cache.Model().Invited(a.directory(), actor.Email, e.ID) {
		return nil, false, access.Forbidden("only a host, or someone invited, may invite others")
	}
	return e, host, nil
}

func (a app) hostedEvent(actor access.Actor, id string) (*Event, error) {
	e, err := a.guestListEvent(actor, id)
	if err != nil {
		return nil, err
	}
	if !a.isHost(actor, e) {
		return nil, access.Forbidden("only a host can change the guest list")
	}
	return e, nil
}

func (a app) household(email string) []string {
	out := []string{email}
	adults, kids := a.directory().Household(email)
	for _, p := range append(adults, kids...) {
		out = append(out, p.Email)
	}
	return out
}

func (a app) householdOn(e *Event, email string) []string {
	if a.directory().Person(email) != nil {
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

func (a app) mayAnswerFor(actor access.Actor, subject string, e *Event) bool {
	return a.isHost(actor, e) || a.speaksFor(actor, subject, e)
}

func (a app) speaksFor(actor access.Actor, subject string, e *Event) bool {
	if actor.Mine(subject) {
		return true
	}
	inv := a.cache.Model().InviteOf(e.ID, subject)
	return inv != nil && inv.GuestOf != "" && actor.Mine(inv.GuestOf)
}
