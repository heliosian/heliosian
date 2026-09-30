package model

import (
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const ViaGroup = "group:"

const ViaInvited = "invited"

const sweepActor = "invite sweep"

type InviteGroup struct {
	ID      string `json:"id"`
	Rule    Rule   `json:"rule"`
	Auto    bool   `json:"auto"`
	AddedBy string `json:"addedBy"`
	Added   string `json:"added"`
	Sent    string `json:"sent"`
	Count   int    `json:"count"`

	TagNames []string `json:"tagNames"`
}

func (b *builder) groups(rows []store.Row) {
	for _, row := range rows {
		id, gid := strings.TrimSpace(row["Event ID"]), strings.TrimSpace(row["Group ID"])
		if id == "" || gid == "" {
			continue
		}
		rule := RuleFromRow(row).Clean()
		if rule.Kind == "" {
			rule.Kind = RuleInclude
		}
		auto, err := cells.YesNo(row["Auto"], true)
		if err != nil {
			b.refuse("invite group %s on %s: auto %v", gid, id, err)
		}
		b.model.Groups[id] = append(b.model.Groups[id], InviteGroup{
			ID: gid, Rule: rule, Auto: auto,
			AddedBy: mail.Normalize(row["Added By"]), Added: strings.TrimSpace(row["Added"]), Sent: strings.TrimSpace(row["Sent"]),
		})
	}
}

func splitEmails(cell string) []string {
	out := []string{}
	for _, part := range strings.Split(cell, ",") {
		if email := mail.Normalize(part); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	return out
}

func (m *Calendar) GroupOf(id, gid string) *InviteGroup {
	gid = m.aliases.Resolve(gid)
	for i := range m.Groups[id] {
		if m.Groups[id][i].ID == gid {
			return &m.Groups[id][i]
		}
	}
	return nil
}

func (a calendarApp) members(e *Event, g InviteGroup) []string {
	matched := g.Rule
	matched.Kind = RuleInclude
	holds := a.ticketHolders(g)
	if holds != nil {
		matched.Family = nil
	}
	d := a.directory()
	out := []string{}
	for _, m := range (Audience{Rules: []Rule{matched}, Editors: a.hostsOf(e)}).Members(a.sources()) {
		if m = d.Resolve(mail.Normalize(m)); m != "" && !slices.Contains(out, m) && (holds == nil || holds[m]) {
			out = append(out, m)
		}
	}
	if holds == nil {
		return out
	}
	d.relatives(slices.Clone(out), g.Rule.Family, func(email, _, _ string) {
		if p := d.Person(email); p != nil && !p.EmailMasked && inRole(p, g.Rule.Roles) && !slices.Contains(out, email) {
			out = append(out, email)
		}
	})
	for email := range a.ticketGuests(g) {
		if !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	return out
}

func (a calendarApp) excluded(e *Event, groups []InviteGroup) map[string]bool {
	out := map[string]bool{}
	for _, g := range groups {
		if g.Rule.Kind != RuleExclude {
			continue
		}
		for _, m := range a.members(e, g) {
			out[m] = true
		}
	}
	return out
}

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

func (a calendarApp) ticketHolders(g InviteGroup) map[string]bool {
	if len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return nil
	}
	p := a.parties().PartyPeople(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return nil
	}
	keep := map[string]bool{}
	for _, host := range p.Hosts {
		keep[a.directory().Resolve(mail.Normalize(host))] = true
	}
	for _, t := range p.Attendees {
		if t.Email != "" {
			keep[a.directory().Resolve(mail.Normalize(t.Email))] = true
		}
	}
	return keep
}

func (a calendarApp) ticketGuests(g InviteGroup) map[string]string {
	out := map[string]string{}
	if len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return out
	}
	p := a.parties().PartyPeople(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return out
	}
	for _, t := range p.Attendees {
		email := a.directory().Resolve(mail.Normalize(t.Email))
		if email == "" || t.Status == "waitlist" || !emailForm.MatchString(email) {
			continue
		}
		if a.directory().Person(email) != nil {
			continue
		}
		if out[email] == "" {
			out[email] = strings.TrimSpace(t.Name)
		}
	}
	return out
}

func (a calendarApp) groupOptions(r *http.Request, _ serve.None) (AudienceOptions, error) {
	return a.sources().Options(a.actor(r).Email), nil
}

type ruleBody struct {
	ID   string `json:"id"`
	Rule Rule   `json:"rule"`
}

type groupPreviewView struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

func (a calendarApp) groupPreview(r *http.Request, body ruleBody) (groupPreviewView, error) {
	actor := a.actor(r)
	e, err := a.hostedEvent(actor, body.ID)
	if err != nil {
		return groupPreviewView{}, err
	}
	rule, err := a.checkRule(actor, body.Rule, e)
	if err != nil {
		return groupPreviewView{}, access.Invalid("%v", err)
	}
	members := a.members(e, InviteGroup{Rule: rule})
	names := []string{}
	for _, m := range members {
		if p := a.directory().Person(m); p != nil && p.FullName != "" {
			names = append(names, p.FullName)
		} else {
			names = append(names, cells.DisplayName(m))
		}
	}
	slices.Sort(names)
	return groupPreviewView{Count: len(members), Names: names[:min(len(names), 12)]}, nil
}

type addGroupBody struct {
	ID   string `json:"id"`
	Rule Rule   `json:"rule"`
	Auto *bool  `json:"auto"`
}

type setGroupBody struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Auto  bool   `json:"auto"`
	Rule  *Rule  `json:"rule"`
}
