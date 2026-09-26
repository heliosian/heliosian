package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/filter"
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
		b.model.Groups[id] = append(b.model.Groups[id], InviteGroup{
			ID: gid, Rule: rule, Auto: !strings.EqualFold(strings.TrimSpace(row["Auto"]), "No"),
			AddedBy: normalizeEmail(row["Added By"]), Added: strings.TrimSpace(row["Added"]), Sent: strings.TrimSpace(row["Sent"]),
			Removed: splitEmails(row["Removed"]),
		})
	}
}

func splitEmails(cell string) []string {
	out := []string{}
	for _, part := range strings.Split(cell, ",") {
		if email := normalizeEmail(part); email != "" && !slices.Contains(out, email) {
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

func (a app) checkRule(r filter.Rule, actor string, e *Event) (filter.Rule, error) {
	if a.sources == nil {
		return r, fmt.Errorf("groups are not set up")
	}
	r = filter.Clean(r)
	r.Kind = filter.KindInclude
	if err := filter.Check(r); err != nil {
		return r, err
	}
	options := filter.OptionsFor(a.sources(), actor)
	for _, g := range r.Grades {
		if !slices.Contains(options.Grades, g) {
			return r, fmt.Errorf("the directory has no grade %s", g)
		}
	}
	for _, c := range r.Classrooms {
		if !slices.Contains(options.Classrooms, c) {
			return r, fmt.Errorf("the directory has no classroom %s", c)
		}
	}
	if err := filter.Writable(a.sources(), actor, a.hostsOf(e), nil, []filter.Rule{r}); err != nil {
		return r, err
	}
	return r, nil
}

func (a app) members(e *Event, g InviteGroup) []string {
	if a.sources == nil {
		return nil
	}
	out := []string{}
	for _, m := range filter.Members(filter.List{Rules: []filter.Rule{g.Rule}, Editors: a.hostsOf(e)}, a.sources()) {
		if m = a.directory.Resolve(normalizeEmail(m)); m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return a.ticketHolders(g, out)
}

func (a app) ticketHolders(g InviteGroup, members []string) []string {
	if a.parties == nil || len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return members
	}
	p := a.parties(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return members
	}
	keep := map[string]bool{}
	for _, host := range p.Hosts {
		keep[a.directory.Resolve(normalizeEmail(host))] = true
	}
	for _, t := range p.Attendees {
		if t.Email != "" {
			keep[a.directory.Resolve(normalizeEmail(t.Email))] = true
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

// ticketGuests are a party's ticket holders the directory does not hold but
// who came with an address - an alum, a cousin whose address the family gave
// - by address, with the name on the ticket. The directory's lists count
// such guests without naming them, so the ticket holders' group adds them
// here, each as someone from outside with their own link.
func (a app) ticketGuests(g InviteGroup) map[string]string {
	out := map[string]string{}
	if a.parties == nil || len(g.Rule.Tags) != 1 || !strings.HasPrefix(g.Rule.Tags[0], "party:") {
		return out
	}
	p := a.parties(strings.TrimPrefix(g.Rule.Tags[0], "party:"))
	if p == nil {
		return out
	}
	for _, t := range p.Attendees {
		email := a.directory.Resolve(normalizeEmail(t.Email))
		if email == "" || t.Status == "waitlist" || !emailForm.MatchString(email) {
			continue
		}
		if _, known := a.directory.Person(email); known {
			continue
		}
		if out[email] == "" {
			out[email] = strings.TrimSpace(t.Name)
		}
	}
	return out
}

func (a app) groupOptions(w http.ResponseWriter, r *http.Request) {
	if a.sources == nil {
		http.Error(w, "groups are not set up", http.StatusNotFound)
		return
	}
	actor, _ := a.who(r)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filter.OptionsFor(a.sources(), actor)); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] encode group options", "error", err)
	}
}

func (a app) groupPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   string      `json:"id"`
		Rule filter.Rule `json:"rule"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	rule, err := a.checkRule(body.Rule, actor, e)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	members := a.members(e, InviteGroup{Rule: rule})
	names := []string{}
	for _, m := range members {
		if p, known := a.directory.Person(m); known && p.Name != "" {
			names = append(names, p.Name)
		} else {
			names = append(names, displayName(m))
		}
	}
	slices.Sort(names)
	view := struct {
		Count int      `json:"count"`
		Names []string `json:"names"`
	}{Count: len(members), Names: names[:min(len(names), 12)]}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] encode group preview", "error", err)
	}
}

func groupRow(e *Event, g InviteGroup) store.Row {
	row := store.Row{"Event ID": e.ID, "Group ID": g.ID, "Auto": yesNoWord(g.Auto), "Added By": g.AddedBy, "Added": g.Added}
	maps.Copy(row, filter.RuleCells(g.Rule))
	return row
}

func (a app) addGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID   string      `json:"id"`
		Rule filter.Rule `json:"rule"`
		Auto *bool       `json:"auto"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	rule, err := a.checkRule(body.Rule, actor, e)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(a.cache.Model().Groups[e.ID]) >= 20 {
		http.Error(w, "a guest list holds twenty groups at most", http.StatusBadRequest)
		return
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: rule, Auto: body.Auto == nil || *body.Auto, AddedBy: actor, Added: now().Format(DateTimeFormat)}
	if !a.commit(w, r, actor, append(a.invitationOps(e.ID, actor, nil), store.Insert(InviteGroupsTab, groupRow(e, g)))...) {
		return
	}
	added := a.fill(r.Context(), actor, e, g, false)
	slog.InfoContext(r.Context(), "calendar: group added", "actor", actor, "event", e.ID, "group", g.ID, "added", added)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": added})
}

func (a app) setGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Group string `json:"group"`
		Auto  bool   `json:"auto"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	g := a.cache.Model().GroupOf(e.ID, body.Group)
	if g == nil {
		http.Error(w, "that group is not on the list", http.StatusNotFound)
		return
	}
	if !a.commit(w, r, actor, store.Update(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID}, store.Row{"Auto": yesNoWord(body.Auto)})) {
		return
	}
	if body.Auto {
		a.fill(r.Context(), actor, e, *a.cache.Model().GroupOf(e.ID, g.ID), false)
	}
	slog.InfoContext(r.Context(), "calendar: group changed", "actor", actor, "event", e.ID, "group", g.ID, "auto", body.Auto)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) removeGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Group string `json:"group"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	model := a.cache.Model()
	g := model.GroupOf(e.ID, body.Group)
	if g == nil {
		http.Error(w, "that group is not on the list", http.StatusNotFound)
		return
	}
	ops := []store.Op{store.Delete(InviteGroupsTab, store.Row{"Event ID": e.ID, "Group ID": g.ID})}
	for _, inv := range model.Invites[e.ID] {
		if inv.Via == ViaGroup+g.ID && inv.Sent == "" {
			ops = append(ops, store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": inv.Email}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: group removed", "actor", actor, "event", e.ID, "group", g.ID, "dropped", len(ops)-1)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"dropped": len(ops) - 1})
}

func (a app) fill(ctx context.Context, actor string, e *Event, g InviteGroup, wait bool) int {
	model := a.cache.Model()
	at := now()
	stamp := at.Format(DateTimeFormat)
	ops := []store.Op{}
	emails := []string{}
	matching := a.members(e, g)
	guests := a.ticketGuests(g)
	if wait {
		a.clock.keep(e.ID, g.ID, matching)
	}
	for _, email := range matching {
		if model.InviteOf(e.ID, email) != nil || slices.Contains(g.Removed, email) {
			continue
		}
		if wait && !a.clock.ripe(e.ID, g.ID, email, at) {
			slog.DebugContext(ctx, "calendar: group match waits the grace", "event", e.ID, "group", g.ID, "email", email)
			continue
		}
		name, token := email, ""
		if p, known := a.directory.Person(email); known {
			name = p.Name
		} else if guest, ok := guests[email]; ok {
			name, token = guest, NewToken()
			if name == "" {
				name = displayName(email)
			}
		}
		ops = append(ops, store.Insert(InvitesTab, store.Row{"Event ID": e.ID, "Email": email, "Name": name, "Via": ViaGroup + g.ID, "Added By": g.AddedBy, "Added": stamp, "Token": token}))
		emails = append(emails, email)
	}
	if len(ops) == 0 {
		return 0
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: fill group", "event", e.ID, "group", g.ID, "error", err)
		return 0
	}
	if g.Auto && g.Sent != "" {
		a.send(ctx, actor, g.AddedBy, e, emails, "")
	}
	return len(ops)
}

func (a app) sweepEvent(ctx context.Context, e *Event) {
	if a.sources == nil || e == nil || e.end.Before(now()) {
		return
	}
	for _, g := range a.cache.Model().Groups[e.ID] {
		if n := a.fill(ctx, sweepActor, e, g, true); n > 0 {
			slog.InfoContext(ctx, "calendar: group filled", "event", e.ID, "group", g.ID, "added", n)
		}
	}
}

func (a app) sweep(ctx context.Context) {
	if a.sources == nil {
		return
	}
	for id, groups := range a.cache.Model().Groups {
		if len(groups) > 0 {
			a.sweepEvent(ctx, a.eventFor(groups[0].AddedBy, false, id))
		}
	}
}

func (a app) sweepLoop() {
	for range time.Tick(sweepEvery) {
		a.sweep(context.Background())
	}
}

func (a app) startParty(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	if !e.linked() {
		http.Error(w, "only a party or an HCA event starts this way", http.StatusBadRequest)
		return
	}
	if a.sources == nil {
		http.Error(w, "groups are not set up", http.StatusNotFound)
		return
	}
	key := "party:" + e.linkedID()
	if e.Source != SourceCelebrate {
		key = "activity:" + e.linkedID()
	}
	model := a.cache.Model()
	for _, g := range model.Groups[e.ID] {
		if slices.Contains(g.Rule.Tags, key) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": 0})
			return
		}
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: filter.Rule{Kind: filter.KindInclude, Tags: []string{key}}, Auto: true, AddedBy: actor, Added: now().Format(DateTimeFormat)}
	if !a.commit(w, r, actor, append(a.invitationOps(e.ID, actor, nil), store.Insert(InviteGroupsTab, groupRow(e, g)))...) {
		return
	}
	added := a.fill(r.Context(), actor, e, g, false)
	slog.InfoContext(r.Context(), "calendar: party list started", "actor", actor, "event", e.ID, "group", g.ID, "added", added)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": added})
}
