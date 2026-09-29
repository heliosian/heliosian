package when

import (
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const ViaGroup = "group:"

const ViaInvited = "invited"

const sweepActor = "invite sweep"

type InviteGroup struct {
	ID      string      `json:"id"`
	Rule    filter.Rule `json:"rule"`
	Auto    bool        `json:"auto"`
	AddedBy string      `json:"addedBy"`
	Added   string      `json:"added"`
	Sent    string      `json:"sent"`
	Removed []string    `json:"removed,omitempty"`
	Count   int         `json:"count"`
}

func (b *builder) groups(rows []store.Row) {
	for _, row := range rows {
		id, gid := strings.TrimSpace(row["Event ID"]), strings.TrimSpace(row["Group ID"])
		if id == "" || gid == "" {
			continue
		}
		rule := filter.Clean(filter.RuleFromRow(row))
		if rule.Kind == "" {
			rule.Kind = filter.KindInclude
		}
		auto, err := cells.YesNo(row["Auto"], true)
		if err != nil {
			b.refuse("invite group %s on %s: auto %v", gid, id, err)
		}
		b.model.Groups[id] = append(b.model.Groups[id], InviteGroup{
			ID: gid, Rule: rule, Auto: auto,
			AddedBy: config.NormalizeEmail(row["Added By"]), Added: strings.TrimSpace(row["Added"]), Sent: strings.TrimSpace(row["Sent"]),
			Removed: splitEmails(row["Removed"]),
		})
	}
}

func splitEmails(cell string) []string {
	out := []string{}
	for _, part := range strings.Split(cell, ",") {
		if email := config.NormalizeEmail(part); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	return out
}

func (m *Model) GroupOf(id, gid string) *InviteGroup {
	gid = m.aliases.Resolve(gid)
	for i := range m.Groups[id] {
		if m.Groups[id][i].ID == gid {
			return &m.Groups[id][i]
		}
	}
	return nil
}

func (a app) members(e *Event, g InviteGroup) []string {
	out := []string{}
	for _, m := range filter.Members(filter.List{Rules: []filter.Rule{g.Rule}, Editors: a.hostsOf(e)}, a.sources()) {
		if m = a.directory().Resolve(config.NormalizeEmail(m)); m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return a.ticketHolders(g, out)
}

func (a app) ticketHolders(g InviteGroup, members []string) []string {
	if len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return members
	}
	p := a.parties(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return members
	}
	keep := map[string]bool{}
	for _, host := range p.Hosts {
		keep[a.directory().Resolve(config.NormalizeEmail(host))] = true
	}
	for _, t := range p.Attendees {
		if t.Email != "" {
			keep[a.directory().Resolve(config.NormalizeEmail(t.Email))] = true
		}
	}
	out := []string{}
	for _, m := range members {
		if keep[m] {
			out = append(out, m)
		}
	}
	for email := range a.ticketGuests(g) {
		if !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	return out
}

func (a app) ticketGuests(g InviteGroup) map[string]string {
	out := map[string]string{}
	if len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return out
	}
	p := a.parties(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return out
	}
	for _, t := range p.Attendees {
		email := a.directory().Resolve(config.NormalizeEmail(t.Email))
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

func (a app) groupOptions(r *http.Request, _ serve.None) (filter.Options, error) {
	return filter.OptionsFor(a.sources(), a.actor(r).Email), nil
}

type ruleBody struct {
	ID   string      `json:"id"`
	Rule filter.Rule `json:"rule"`
}

type groupPreviewView struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

func (a app) groupPreview(r *http.Request, body ruleBody) (groupPreviewView, error) {
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
	ID   string      `json:"id"`
	Rule filter.Rule `json:"rule"`
	Auto *bool       `json:"auto"`
}

type setGroupBody struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Auto  bool   `json:"auto"`
}
