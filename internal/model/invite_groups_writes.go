package model

import (
	"heliosian/internal/access"
	"heliosian/internal/store"
)

func (a calendarApp) regroupOps(actor access.Actor, e *Event, was *InviteGroup, g InviteGroup) ([]store.Op, []string) {
	model := a.model()
	before := model.Groups[e.ID]
	after := []InviteGroup{}
	for _, other := range before {
		if other.ID != g.ID {
			after = append(after, other)
		}
	}
	after = append(after, g)
	left := a.excluded(e, after)
	ops := []store.Op{}
	if g.Rule.Kind == RuleExclude {
		had := a.excluded(e, before)
		dropped := []string{}
		for email := range left {
			if had[email] {
				continue
			}
			if model.InviteOf(e.ID, email) != nil {
				ops = append(ops, store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}))
				dropped = append(dropped, email)
			}
			if ans := model.AnswerOf(email, e.ID); ans != "" && ans != AnswerHidden {
				ops = append(ops, store.Delete(RSVPsTab, store.Row{"Event ID": e.ID, "Email": email}))
			}
		}
		return ops, dropped
	}
	if was != nil {
		keep := map[string]bool{}
		for _, m := range a.members(e, g) {
			keep[m] = true
		}
		for _, inv := range model.Invites[e.ID] {
			if inv.Via == ViaGroup+g.ID && inv.Sent == "" && !keep[inv.Email] {
				ops = append(ops, store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": inv.Email}))
			}
		}
	}
	filled, emails := a.fillOps(actor, e, g, left)
	return append(ops, filled...), emails
}
