package when

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const ViaGroup = "group:"

const ViaInvited = "invited"

const (
	sweepEvery = 5 * time.Minute
	grace      = 5 * time.Minute
	sweepActor = "invite sweep"
)

type matchClock struct {
	mu    sync.Mutex
	first map[string]time.Time
}

func (c *matchClock) key(event, group, email string) string {
	return event + "\x00" + group + "\x00" + email
}

func (c *matchClock) ripe(event, group, email string, at time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.first == nil {
		c.first = map[string]time.Time{}
	}
	k := c.key(event, group, email)
	seen, ok := c.first[k]
	if !ok {
		c.first[k] = at
		return false
	}
	return !at.Before(seen.Add(grace))
}

func (c *matchClock) keep(event, group string, matching []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := c.key(event, group, "")
	for k := range c.first {
		if strings.HasPrefix(k, prefix) && !slices.Contains(matching, strings.TrimPrefix(k, prefix)) {
			delete(c.first, k)
		}
	}
}

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
	actor, _ := a.who(r)
	return filter.OptionsFor(a.sources(), actor), nil
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

type groupAdded struct {
	Group string `json:"group"`
	Added int    `json:"added"`
}

func (a app) addGroup(r *http.Request, body addGroupBody) (groupAdded, error) {
	actor := a.actor(r)
	ops, e, g, err := a.addGroupOps(actor, body.ID, body.Rule, body.Auto)
	if err != nil {
		return groupAdded{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return groupAdded{}, err
	}
	added := a.fill(r.Context(), actor, e, g, false)
	slog.InfoContext(r.Context(), "calendar: group added", "actor", actor.Email, "event", e.ID, "group", g.ID, "added", added)
	return groupAdded{Group: g.ID, Added: added}, nil
}

type setGroupBody struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Auto  bool   `json:"auto"`
}

func (a app) setGroup(r *http.Request, body setGroupBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, g, err := a.setGroupOps(actor, body.ID, body.Group, body.Auto)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	if body.Auto {
		a.fill(r.Context(), actor, e, *a.cache.Model().GroupOf(e.ID, g.ID), false)
	}
	slog.InfoContext(r.Context(), "calendar: group changed", "actor", actor.Email, "event", e.ID, "group", g.ID, "auto", body.Auto)
	return serve.None{}, nil
}

type groupBody struct {
	ID    string `json:"id"`
	Group string `json:"group"`
}

func (a app) removeGroup(r *http.Request, body groupBody) (map[string]int, error) {
	actor := a.actor(r)
	ops, e, g, err := a.removeGroupOps(actor, body.ID, body.Group)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "calendar: group removed", "actor", actor.Email, "event", e.ID, "group", g.ID, "dropped", len(ops)-1)
	return map[string]int{"dropped": len(ops) - 1}, nil
}

func (a app) fill(ctx context.Context, actor access.Actor, e *Event, g InviteGroup, wait bool) int {
	ops, emails := a.fillOps(actor, e, g, wait)
	if len(ops) == 0 {
		return 0
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "calendar: fill group", "event", e.ID, "group", g.ID, "error", err)
		return 0
	}
	if g.Auto && g.Sent != "" {
		a.send(ctx, actor, g.AddedBy, e, emails, "")
	}
	return len(ops)
}

func (a app) sweepEvent(ctx context.Context, e *Event) {
	if e == nil || e.end.Before(now()) {
		return
	}
	for _, g := range a.cache.Model().Groups[e.ID] {
		if n := a.fill(ctx, access.System(sweepActor), e, g, true); n > 0 {
			slog.InfoContext(ctx, "calendar: group filled", "event", e.ID, "group", g.ID, "added", n)
		}
	}
}

func (a app) sweep(ctx context.Context) {
	for id, groups := range a.cache.Model().Groups {
		if len(groups) > 0 {
			a.sweepEvent(ctx, a.sweptEvent(a.as(groups[0].AddedBy), id))
		}
	}
}

func (a app) sweepLoop() {
	for range time.Tick(sweepEvery) {
		a.sweep(context.Background())
	}
}

func (a app) startParty(r *http.Request, body idBody) (groupAdded, error) {
	actor := a.actor(r)
	ops, e, g, err := a.startPartyOps(actor, body.ID)
	if err != nil {
		return groupAdded{}, err
	}
	if len(ops) == 0 {
		return groupAdded{Group: g.ID}, nil
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return groupAdded{}, err
	}
	added := a.fill(r.Context(), actor, e, g, false)
	slog.InfoContext(r.Context(), "calendar: party list started", "actor", actor.Email, "event", e.ID, "group", g.ID, "added", added)
	return groupAdded{Group: g.ID, Added: added}, nil
}
