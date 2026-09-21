package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/filter"
)

// An invite group is a rule on a guest list - a filter as Helios Who?'s
// filters have it, the same rule Loop's groups and Heliosian's Visibility
// are made of, read by internal/filter - whose matches go on the list
// when it is added and whenever someone new comes to match it: a family
// joining the classroom, a ticket sold on the party. A newcomer lands
// pending, for the host to send, unless the group's Auto is on and its
// invites have gone out, when they are sent theirs at once. The Invite
// Groups tab holds them, one row per group, and the people it
// adds carry `group:<id>` as their Via. The sweep runs when a host opens
// the list and on its own every few minutes.

// ViaGroup begins the Via of someone a group put on the list.
const ViaGroup = "group:"

// sweepEvery is how often the auto groups are read against the directory
// on their own; grace is how long someone must have matched a group before
// it puts them on the list - so a tag given by mistake can be taken back
// before anyone is invited on the strength of it.
const (
	sweepEvery = 5 * time.Minute
	grace      = 5 * time.Minute
)

// A matchClock is when each person was first seen matching each auto
// group, by event, group and address, for the grace: someone goes on
// only once they have matched for the whole of it, and is forgotten the
// moment they stop. It lives in memory, so a restart starts the clock
// again - the safe way round.
type matchClock struct {
	mu    sync.Mutex
	first map[string]time.Time
}

func (c *matchClock) key(event, group, email string) string {
	return event + "\x00" + group + "\x00" + email
}

// ripe says whether a person has matched a group for the grace, noting
// them now if they are new.
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

// keep forgets everyone on a group but those still matching.
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

// InviteGroup is one rule on an event's guest list.
type InviteGroup struct {
	ID      string      `json:"id"`
	Rule    filter.Rule `json:"rule"`
	Auto    bool        `json:"auto"`
	AddedBy string      `json:"addedBy"`
	Added   string      `json:"added"`
	// Sent is when the group's people were first sent the invitation:
	// until then a newcomer lands pending whatever Auto says, so a host
	// sends each group its invites by hand the first time.
	Sent string `json:"sent"`
	// Removed is whoever a host took off the list after the group put
	// them on: the group does not put them back, however well they still
	// match, until the host adds them again by hand.
	Removed []string `json:"removed,omitempty"`
	// Count is how many people on the list the group put there, for the
	// view.
	Count int `json:"count"`
}

func (b *builder) groups(rows []map[string]string) {
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

// splitEmails reads a cell of addresses, comma-separated.
func splitEmails(cell string) []string {
	out := []string{}
	for _, part := range strings.Split(cell, ",") {
		if email := normalizeEmail(part); email != "" && !slices.Contains(out, email) {
			out = append(out, email)
		}
	}
	return out
}

// GroupOf is one group on one event, or nil.
func (m *Model) GroupOf(id, gid string) *InviteGroup {
	for i := range m.Groups[id] {
		if m.Groups[id][i].ID == gid {
			return &m.Groups[id][i]
		}
	}
	return nil
}

// checkRule is a rule as a host sent it, cleaned and checked as Heliosian
// checks its audiences: an include rule, its owner the host, its grades
// and classrooms ones the directory has.
func (a app) checkRule(r filter.Rule, actor string) (filter.Rule, error) {
	if a.sources == nil {
		return r, fmt.Errorf("groups are not set up")
	}
	r = filter.Clean(r)
	r.Kind, r.Owner = filter.KindInclude, actor
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
	return r, nil
}

// members is everyone a group's rule picks out as the directory stands,
// each resolved to the address the directory keys them by.
func (a app) members(g InviteGroup) []string {
	if a.sources == nil {
		return nil
	}
	out := []string{}
	for _, m := range filter.Members(filter.List{Rules: []filter.Rule{g.Rule}}, a.sources()) {
		if m = a.directory.Resolve(normalizeEmail(m)); m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// groupOptions is GET /api/calendar/invites/options: what the rule editor
// offers this host - the classrooms, the grades, their own tags and lists.
func (a app) groupOptions(w http.ResponseWriter, r *http.Request) {
	if a.sources == nil {
		http.Error(w, "groups are not set up", http.StatusNotFound)
		return
	}
	actor, _ := a.who(r)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filter.OptionsFor(a.sources(), actor)); err != nil {
		slog.ErrorContext(r.Context(), "encode group options", "error", err)
	}
}

// groupPreview is POST /api/calendar/invites/preview: who a rule picks
// out as the directory stands - how many, and the first few by name - so
// the editor reads back what it is saying.
func (a app) groupPreview(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Rule filter.Rule `json:"rule"`
	}
	if !decode(w, r, &body) {
		return
	}
	rule, err := a.checkRule(body.Rule, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	members := a.members(InviteGroup{Rule: rule})
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
		slog.ErrorContext(r.Context(), "encode group preview", "error", err)
	}
}

// addGroup is POST /api/calendar/invites/group: a host putting a group
// on the list - its rule kept, its matches added now, and added again as
// they come.
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
	rule, err := a.checkRule(body.Rule, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(a.cache.Model().Groups[e.ID]) >= 20 {
		http.Error(w, "a guest list holds twenty groups at most", http.StatusBadRequest)
		return
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: rule, Auto: body.Auto == nil || *body.Auto, AddedBy: actor, Added: now().Format(DateTimeFormat)}
	row := map[string]string{"Event ID": e.ID, "Group ID": g.ID, "Auto": yesNoWord(g.Auto), "Added By": actor, "Added": g.Added, "Sent": "", "Removed": ""}
	for k, v := range filter.RuleCells(rule) {
		row[k] = v
	}
	tables, first := a.ensured(a.cache.Tables(), e.ID, actor)
	if !a.commit(r.Context(), w, tables.WithGroup(row), func() error {
		if err := first(); err != nil {
			return err
		}
		return a.writer.AppendCells(appName, InviteGroupsTab, row)
	}) {
		return
	}
	added := a.fill(r.Context(), e, g, false)
	slog.InfoContext(r.Context(), "calendar: group added", "actor", actor, "event", e.ID, "group", g.ID, "added", added)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": added})
}

// setGroup is PUT /api/calendar/invites/group: a host turning a group's
// Auto on or off. On takes whoever matches now without the grace; anyone
// already pending under the group stays for the host to send.
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
	cells := map[string]string{"Auto": yesNoWord(body.Auto)}
	if !a.commit(r.Context(), w, a.cache.Tables().WithGroupCells(e.ID, g.ID, cells), func() error {
		return a.writer.Set(appName, InviteGroupsTab, map[string]string{"Event ID": e.ID, "Group ID": g.ID}, cells)
	}) {
		return
	}
	if body.Auto {
		a.fill(r.Context(), e, *a.cache.Model().GroupOf(e.ID, g.ID), false)
	}
	slog.InfoContext(r.Context(), "calendar: group changed", "actor", actor, "event", e.ID, "group", g.ID, "auto", body.Auto)
	w.WriteHeader(http.StatusNoContent)
}

// removeGroup is DELETE /api/calendar/invites/group: a host taking a
// group off the list, and with it the people it added who have not been
// sent their invite; those who have keep their place.
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
	dropped := []string{}
	for _, inv := range model.Invites[e.ID] {
		if inv.Via == ViaGroup+g.ID && inv.Sent == "" {
			dropped = append(dropped, inv.Email)
		}
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithoutGroup(e.ID, g.ID), func() error {
		if err := a.writer.Delete(appName, InviteGroupsTab, map[string]string{"Event ID": e.ID, "Group ID": g.ID}); err != nil {
			return err
		}
		for _, email := range dropped {
			if err := a.writer.Delete(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": email}); err != nil {
				return err
			}
			if err := a.writer.Delete(appName, RSVPsTab, map[string]string{"Event ID": e.ID, "Email": email}); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: group removed", "actor", actor, "event", e.ID, "group", g.ID, "dropped", len(dropped))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"dropped": len(dropped)})
}

// fill puts a group's matches who are not on the list yet onto it, under
// the group, and sends them their invites when the group's Auto is on and
// its own have gone out - a group a host has just added, or started a
// party's list with, puts its people on unsent, and the host sends them
// from the Pending band; only after that does Auto send a newcomer theirs,
// and with Auto off a newcomer waits pending for the host. A host's own doing - adding the group, turning Auto on - takes
// everyone matching now; the sweep waits the grace for each newcomer,
// so a tag given by mistake can be taken back first. It hands back how
// many it added.
func (a app) fill(ctx context.Context, e *Event, g InviteGroup, wait bool) int {
	model := a.cache.Model()
	at := now()
	stamp := at.Format(DateTimeFormat)
	rows := []map[string]string{}
	emails := []string{}
	matching := a.members(g)
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
		name := email
		if p, known := a.directory.Person(email); known {
			name = p.Name
		}
		rows = append(rows, map[string]string{"Event ID": e.ID, "Email": email, "Name": name, "Guest Of": "", "Via": ViaGroup + g.ID, "Added By": g.AddedBy, "Added": stamp, "Sent": "", "Token": ""})
		emails = append(emails, email)
	}
	if len(rows) == 0 {
		return 0
	}
	tables := a.cache.Tables().WithInvites(rows)
	built, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		slog.ErrorContext(ctx, "calendar: fill group", "event", e.ID, "group", g.ID, "error", err)
		return 0
	}
	a.cache.set(tables, built)
	a.queue.Add(func() {
		for _, row := range rows {
			if err := a.writer.AppendCells(appName, InvitesTab, row); err != nil {
				slog.ErrorContext(ctx, "calendar write", "error", err)
			}
		}
	})
	if g.Auto && g.Sent != "" {
		a.send(ctx, g.AddedBy, e, emails, false)
	}
	return len(rows)
}

// sweepEvent reads one event's groups against the directory and fills in
// whoever has newly matched for the grace.
func (a app) sweepEvent(ctx context.Context, e *Event) {
	if a.sources == nil || e == nil || e.end.Before(now()) {
		return
	}
	for _, g := range a.cache.Model().Groups[e.ID] {
		if n := a.fill(ctx, e, g, true); n > 0 {
			slog.InfoContext(ctx, "calendar: group filled", "event", e.ID, "group", g.ID, "added", n)
		}
	}
}

// sweep is sweepEvent over every event with groups, still to come. It
// runs when a host opens a list (for that event alone), and on its own
// every sweepEvery.
func (a app) sweep(ctx context.Context) {
	if a.sources == nil {
		return
	}
	for id, groups := range a.cache.Model().Groups {
		if len(groups) > 0 {
			a.sweepEvent(ctx, a.eventFor(groups[0].AddedBy, id))
		}
	}
}

// sweepLoop is the sweep on its own clock.
func (a app) sweepLoop() {
	for range time.Tick(sweepEvery) {
		a.sweep(context.Background())
	}
}

// startParty is POST /api/calendar/invites/start: a party's host starting
// its guest list from Helios Celebrate's Create Invite - the invitation
// made, and a group for the party's ticket holders (its Magic Tag list in
// Who?, which the host has as a host) with Auto-invite on, so the list
// follows the tickets from here on. A list started already is left as it
// is; the group is added once.
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
	if e.Source != SourceCelebrate {
		http.Error(w, "only a party starts this way", http.StatusBadRequest)
		return
	}
	if a.sources == nil {
		http.Error(w, "groups are not set up", http.StatusNotFound)
		return
	}
	key := "party:" + strings.TrimPrefix(e.ID, SourceCelebrate+"/")
	model := a.cache.Model()
	for _, g := range model.Groups[e.ID] {
		if slices.Contains(g.Rule.Tags, key) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": 0})
			return
		}
	}
	g := InviteGroup{ID: strings.ToLower(newEventID()), Rule: filter.Rule{Kind: filter.KindInclude, Tags: []string{key}, Owner: actor}, Auto: true, AddedBy: actor, Added: now().Format(DateTimeFormat)}
	row := map[string]string{"Event ID": e.ID, "Group ID": g.ID, "Auto": "Yes", "Added By": actor, "Added": g.Added, "Sent": "", "Removed": ""}
	for k, v := range filter.RuleCells(g.Rule) {
		row[k] = v
	}
	tables, first := a.ensured(a.cache.Tables(), e.ID, actor)
	if !a.commit(r.Context(), w, tables.WithGroup(row), func() error {
		if err := first(); err != nil {
			return err
		}
		return a.writer.AppendCells(appName, InviteGroupsTab, row)
	}) {
		return
	}
	added := a.fill(r.Context(), e, g, false)
	slog.InfoContext(r.Context(), "calendar: party list started", "actor", actor, "event", e.ID, "group", g.ID, "added", added)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"group": g.ID, "added": added})
}
