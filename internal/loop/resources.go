package loop

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	host           = "loop"
	kindMember     = "email-list-member"
	kindSuggestion = "email-list-suggestion"
	kindSettings   = "loop-settings"
)

type World struct {
	Model       *Model
	Directory   *model.Directory
	GradeColors map[string]string
	lists       func(owner string, now time.Time) []model.MagicTag
	keys        func() []string
	now         time.Time
	request     *request
}

type request struct {
	placedOnce  sync.Once
	placed      placement
	keysOnce    sync.Once
	suggestions map[string]bool
}

type placement struct {
	byGroup map[string]map[string][]Reason
	members map[string]memberKey
}

type memberKey struct {
	group, email string
}

func NewWorld(m *Model, d *model.Directory, gradeColors map[string]string, lists func(owner string, now time.Time) []model.MagicTag, keys func() []string) World {
	return World{Model: m, Directory: d, GradeColors: gradeColors, lists: lists, keys: keys}
}

func (w World) At(now time.Time) World {
	w.now, w.request = now, &request{}
	return w
}

func (w World) Sources() Sources {
	d := w.Directory
	return Sources{Directory: d, MagicTags: func(owner string) []model.MagicTag { return w.lists(owner, w.now) }}
}

func (w World) placement() placement {
	w.request.placedOnce.Do(func() {
		p := placement{byGroup: map[string]map[string][]Reason{}, members: map[string]memberKey{}}
		s := w.Sources()
		for _, g := range w.Model.Groups {
			l := list(g)
			l.Excluded = nil
			placed := l.Reasons(s)
			p.byGroup[g.ID] = placed
			for email := range placed {
				if !g.HasExcluded(email) {
					p.members[w.Model.memberID(g.ID, email)] = memberKey{group: g.ID, email: email}
				}
			}
		}
		w.request.placed = p
	})
	return w.request.placed
}

func (m *Model) memberID(groupID, email string) string {
	return id.Of(m.idKey, kindMember, groupID+"\x00"+email)
}

func (m *Model) suggestionID(key string) string {
	return id.Of(m.idKey, kindSuggestion, key)
}

func (m *Model) settingsID() string {
	return id.Of(m.idKey, kindSettings, "")
}

func (w World) onList(g Group, email string) bool {
	_, ok := w.placement().byGroup[g.ID][email]
	return ok
}

func (w World) visible(g Group, v access.Actor) bool {
	return g.visibleWith(v, func() bool { return w.onList(g, v.Email) })
}

func (w World) members(g Group) []string {
	inside, outside := []string{}, []string{}
	for email := range w.placement().byGroup[g.ID] {
		switch {
		case g.HasExcluded(email):
		case w.Directory.Person(email) != nil:
			inside = append(inside, email)
		default:
			outside = append(outside, email)
		}
	}
	slices.Sort(inside)
	slices.Sort(outside)
	return append(inside, outside...)
}

func (w World) personID(email string) []string {
	if p := w.Directory.Person(w.Directory.Resolve(email)); p != nil && p.ID != "" {
		return []string{p.ID}
	}
	return nil
}

func (w World) shown(key string, q api.Query) *Group {
	g := w.Model.Group(key)
	if g == nil || !w.visible(*g, q.Actor) {
		return nil
	}
	return g
}

type RuleView struct {
	Rule
	TagLabels []string `json:"tagLabels"`
}

type ManagerView struct {
	Rules     []RuleView `json:"rules"`
	Additions []Addition `json:"additions"`
	Excluded  []Excluded `json:"excluded"`
	Sent      int        `json:"sent"`
}

type listMe struct {
	Managing     bool `json:"managing"`
	Member       bool `json:"member"`
	Unsubscribed bool `json:"unsubscribed"`
	Archived     bool `json:"archived"`
}

type listResource struct {
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Aliases     []string `json:"aliases"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Prefix      bool     `json:"prefix"`
	Visibility  string   `json:"visibility"`
	Posting     string   `json:"posting"`
	Replying    string   `json:"replying"`
	Created     string   `json:"created,omitempty"`
	*ManagerView
	MemberCount     int      `json:"memberCount"`
	ManagersOutside []string `json:"managersOutside,omitempty"`
	Slug            string   `json:"slug"`
	Path            string   `json:"path"`
	App             string   `json:"app"`
	Me              listMe   `json:"me"`
}

type memberResource struct {
	Email   string   `json:"email,omitempty"`
	Name    string   `json:"name,omitempty"`
	Outside bool     `json:"outside,omitempty"`
	Reasons []Reason `json:"reasons,omitempty"`
}

type messageResource struct {
	Received   string `json:"received"`
	Subject    string `json:"subject"`
	Recipients int    `json:"recipients"`
	Delivered  int    `json:"delivered"`
	Failed     int    `json:"failed"`
	Pending    int    `json:"pending"`
	FromEmail  string `json:"fromEmail,omitempty"`
	FromName   string `json:"fromName,omitempty"`
}

type copyResource struct {
	State    string    `json:"state"`
	When     string    `json:"when,omitempty"`
	Attempts []Attempt `json:"attempts"`
	Email    string    `json:"email,omitempty"`
}

type suggestionResource struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type settingsResource struct {
	Domain      string                 `json:"domain"`
	GradeColors map[string]string      `json:"gradeColors,omitempty"`
	MagicTags   []model.MagicTagOption `json:"magicTags"`
	Roles       []string               `json:"roles"`
	Relations   []string               `json:"relations"`
}

type resources struct {
	cache     *Cache
	documents Documents
}

func Resources(c *Cache, documents Documents) []api.Type[World] {
	r := resources{cache: c, documents: documents}
	return []api.Type[World]{r.emailLists(), members(), messages(), copies(), suggestions(), settings()}
}

func (r resources) stage(wr api.Write[World], ops []store.Op, err error) error {
	if err != nil {
		return err
	}
	return r.cache.Stage(wr.Tx, ops...)
}

func logAfter(wr api.Write[World], msg string, args ...any) {
	actor := wr.Query.Actor.Email
	ctx := wr.Request.Context()
	wr.Tx.After(func() { slog.InfoContext(ctx, msg, append([]any{"actor", actor}, args...)...) })
}

func can(err error) bool {
	return err == nil
}

type groupPatch struct {
	Name        *string     `json:"name"`
	Aliases     *[]string   `json:"aliases"`
	Title       *string     `json:"title"`
	Description *string     `json:"description"`
	Prefix      *bool       `json:"prefix"`
	Visibility  *string     `json:"visibility"`
	Posting     *string     `json:"posting"`
	Replying    *string     `json:"replying"`
	Managers    *[]string   `json:"managers"`
	Rules       *[]Rule     `json:"rules"`
	Additions   *[]Addition `json:"additions"`
	Excluded    *[]Excluded `json:"excluded"`
}

func set[T any](into *T, from *T) {
	if from != nil {
		*into = *from
	}
}

func (p groupPatch) over(g Group) Group {
	set(&g.Name, p.Name)
	set(&g.Aliases, p.Aliases)
	set(&g.Title, p.Title)
	set(&g.Description, p.Description)
	set(&g.Prefix, p.Prefix)
	set(&g.Visibility, p.Visibility)
	set(&g.Posting, p.Posting)
	set(&g.Replying, p.Replying)
	set(&g.Managers, p.Managers)
	set(&g.Rules, p.Rules)
	set(&g.Additions, p.Additions)
	set(&g.Excluded, p.Excluded)
	return g
}

func (r resources) emailLists() api.Type[World] {
	edits := func(w World, q api.Query, key string) bool { return w.Model.Group(key).Edits(q.Actor) }
	subscription := func(subscribed bool) api.Action[World] {
		check := func(w World, q api.Query, key string) ([]store.Op, error) {
			g := w.Model.Group(key)
			return g.subscription(q.Actor, w.onList(*g, q.Actor.Email), subscribed)
		}
		return api.Do(func(w World, q api.Query, key string) bool { _, err := check(w, q, key); return can(err) }, func(wr api.Write[World], _ serve.None) error {
			ops, err := check(wr.S, wr.Query, wr.ID)
			if !subscribed {
				logAfter(wr, "loop:unsubscribed", "group", wr.S.Model.Group(wr.ID).Name, "how", loopPage)
			} else {
				logAfter(wr, "loop:resubscribed", "group", wr.S.Model.Group(wr.ID).Name)
			}
			return r.stage(wr, ops, err)
		})
	}
	archive := func(archived bool) api.Action[World] {
		return api.Do(func(w World, q api.Query, key string) bool {
			_, err := w.Model.archiving(q.Actor, w.Model.Group(key), archived)
			return can(err)
		}, func(wr api.Write[World], _ serve.None) error {
			g := wr.S.Model.Group(wr.ID)
			ops, err := wr.S.Model.archiving(wr.Query.Actor, g, archived)
			logAfter(wr, "loop:archived", "id", g.ID, "group", g.Name, "archived", archived)
			return r.stage(wr, ops, err)
		})
	}
	return api.Type[World]{
		Name:  "email-lists",
		Shape: listResource{},
		Has:   func(w World, key string) bool { return w.Model.Group(key) != nil },
		Get: func(w World, q api.Query, key string) (any, bool) {
			g := w.shown(key, q)
			if g == nil {
				return nil, false
			}
			viewer := q.Actor.Email
			out := listResource{
				Name: g.Name, Address: g.Address(), Aliases: g.Aliases, Title: g.Title, Description: g.Description, Prefix: g.Prefix,
				Visibility: g.Visibility, Posting: g.Posting, Replying: g.Replying, Created: g.Created,
				MemberCount: len(w.members(*g)), Slug: g.Name, Path: g.Path(), App: host,
				Me: listMe{Managing: g.Manages(viewer), Member: w.onList(*g, viewer), Unsubscribed: g.HasExcluded(viewer), Archived: w.Model.Archived(g.ID, viewer)},
			}
			for _, m := range g.Managers {
				if w.personID(m) == nil {
					out.ManagersOutside = append(out.ManagersOutside, m)
				}
			}
			if g.Sees(q.Actor) {
				view := &ManagerView{Rules: []RuleView{}, Additions: g.Additions, Excluded: g.Excluded, Sent: len(w.Model.sent[g.ID])}
				s := w.Sources()
				for _, rule := range g.Rules {
					view.Rules = append(view.Rules, RuleView{Rule: rule, TagLabels: s.TagLabels(rule, g.Managers, viewer)})
				}
				out.ManagerView = view
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, g := range w.Model.Groups {
				if w.visible(g, q.Actor) {
					out = append(out, g.ID)
				}
			}
			return out
		},
		Aliases: func(w World) map[string]string {
			out := map[string]string{}
			for _, g := range w.Model.Groups {
				for _, local := range g.Names() {
					out[local] = g.ID
				}
			}
			return out
		},
		Create: api.Make(func(wr api.Write[World], in Group) (string, error) {
			in.ID = ""
			ops, g, _, err := wr.S.Model.SaveGroup(wr.Query.Actor, wr.S.Sources(), in, wr.Taken)
			if err != nil {
				return "", err
			}
			logAfter(wr, "loop:saved group", "action", "add", "id", g.ID, "group", g.Name, "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions))
			return g.ID, r.stage(wr, ops, nil)
		}),
		Relations: map[string]api.Relation[World]{
			"managers": {Type: "people", Many: true, List: func(w World, _ api.Query, key string) []string {
				out := []string{}
				for _, m := range w.Model.Group(key).Managers {
					out = append(out, w.personID(m)...)
				}
				return out
			}},
			"members": {Type: "email-list-members", Many: true, List: func(w World, _ api.Query, key string) []string {
				g := w.Model.Group(key)
				out := []string{}
				for _, email := range w.members(*g) {
					out = append(out, w.Model.memberID(g.ID, email))
				}
				return out
			}},
			"messages": {Type: "email-list-messages", Many: true, List: func(w World, _ api.Query, key string) []string {
				out := []string{}
				for _, s := range w.Model.sent[key] {
					out = append(out, s.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[World]{
			"edit": api.Do(edits, func(wr api.Write[World], patch groupPatch) error {
				current := wr.S.Model.Group(wr.ID)
				ops, g, _, err := wr.S.Model.SaveGroup(wr.Query.Actor, wr.S.Sources(), patch.over(*current), wr.Taken)
				if err != nil {
					return err
				}
				logAfter(wr, "loop:saved group", "action", "edit", "id", g.ID, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
				return r.stage(wr, ops, nil)
			}),
			"delete": api.Do(edits, func(wr api.Write[World], _ serve.None) error {
				ops, g, err := wr.S.Model.DeleteGroup(wr.Query.Actor, wr.ID)
				if err != nil {
					return err
				}
				actor, ctx, name := wr.Query.Actor, wr.Request.Context(), g.Name
				wr.Tx.After(func() {
					if err := r.documents.Remove(ctx, actor, name); err != nil {
						slog.ErrorContext(ctx, "loop:filed mail not removed", "group", name, "error", err)
					}
				})
				logAfter(wr, "loop:deleted group", "id", g.ID, "group", g.Name)
				return r.stage(wr, ops, nil)
			}),
			"unsubscribe": subscription(false),
			"resubscribe": subscription(true),
			"archive":     archive(true),
			"unarchive":   archive(false),
		},
	}
}

func members() api.Type[World] {
	find := func(w World, q api.Query, key string) (*Group, string, bool) {
		mk, ok := w.placement().members[key]
		if !ok {
			return nil, "", false
		}
		g := w.shown(mk.group, q)
		return g, mk.email, g != nil
	}
	return api.Type[World]{
		Name:  "email-list-members",
		Shape: memberResource{},
		Has: func(w World, key string) bool {
			_, ok := w.placement().members[key]
			return ok
		},
		Get: func(w World, q api.Query, key string) (any, bool) {
			g, email, ok := find(w, q, key)
			if !ok {
				return nil, false
			}
			out := memberResource{}
			if w.Directory.Person(email) == nil {
				out.Email, out.Name, out.Outside = email, email, true
				if added := g.Addition(email); added != nil && added.Name != "" {
					out.Name = added.Name
				}
			}
			if g.Sees(q.Actor) {
				out.Reasons = w.placement().byGroup[g.ID][email]
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, g := range w.Model.Groups {
				if !w.visible(g, q.Actor) {
					continue
				}
				for _, email := range w.members(g) {
					out = append(out, w.Model.memberID(g.ID, email))
				}
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"person": {Type: "people", List: func(w World, q api.Query, key string) []string {
				_, email, ok := find(w, q, key)
				if !ok || w.Directory.Person(email) == nil {
					return nil
				}
				return w.personID(email)
			}},
			"email-list": {Type: "email-lists", List: func(w World, q api.Query, key string) []string {
				g, _, ok := find(w, q, key)
				if !ok {
					return nil
				}
				return []string{g.ID}
			}},
		},
	}
}

func (w World) sentMessage(key string, q api.Query) *Sent {
	s := w.Model.sentByID[key]
	if s == nil {
		return nil
	}
	if g := w.Model.Group(s.Message.Group); g == nil || !g.Sees(q.Actor) {
		return nil
	}
	return s
}

func (w World) sender(from string) (string, *model.Person) {
	email := strings.ToLower(mail.AddressOf(from))
	return email, w.Directory.Person(w.Directory.Resolve(email))
}

func (w World) name(email string) string {
	if p := w.Directory.Person(email); p != nil {
		return p.FullName
	}
	return email
}

func messages() api.Type[World] {
	return api.Type[World]{
		Name:  "email-list-messages",
		Shape: messageResource{},
		Has:   func(w World, key string) bool { return w.Model.sentByID[key] != nil },
		Get: func(w World, q api.Query, key string) (any, bool) {
			s := w.sentMessage(key, q)
			if s == nil {
				return nil, false
			}
			out := messageResource{Received: s.Message.Received, Subject: s.Message.Subject, Recipients: s.Recipients, Delivered: s.Delivered, Failed: s.Failed, Pending: s.Pending}
			if email, p := w.sender(s.Message.From); p == nil {
				out.FromEmail, out.FromName = email, senderName(s.Message.From)
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, g := range w.Model.Groups {
				if !g.Sees(q.Actor) {
					continue
				}
				for _, s := range w.Model.sent[g.ID] {
					out = append(out, s.ID)
				}
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"sender": {Type: "people", List: func(w World, q api.Query, key string) []string {
				s := w.sentMessage(key, q)
				if s == nil {
					return nil
				}
				if _, p := w.sender(s.Message.From); p != nil && p.ID != "" {
					return []string{p.ID}
				}
				return nil
			}},
			"copies": {Type: "email-list-copies", Many: true, List: func(w World, q api.Query, key string) []string {
				s := w.sentMessage(key, q)
				if s == nil {
					return nil
				}
				copies := slices.Clone(s.Copies)
				slices.SortStableFunc(copies, func(x, y *Copy) int {
					return cmp.Or(cmp.Compare(copyOrder[x.State], copyOrder[y.State]), cmp.Compare(strings.ToLower(w.name(x.Email)), strings.ToLower(w.name(y.Email))))
				})
				out := []string{}
				for _, c := range copies {
					out = append(out, c.ID)
				}
				return out
			}},
		},
	}
}

func copies() api.Type[World] {
	find := func(w World, q api.Query, key string) *Copy {
		c := w.Model.copyByID[key]
		if c == nil {
			return nil
		}
		if g := w.Model.Group(c.Group); g == nil || !g.Sees(q.Actor) {
			return nil
		}
		return c
	}
	return api.Type[World]{
		Name:  "email-list-copies",
		Shape: copyResource{},
		Has:   func(w World, key string) bool { return w.Model.copyByID[key] != nil },
		Get: func(w World, q api.Query, key string) (any, bool) {
			c := find(w, q, key)
			if c == nil {
				return nil, false
			}
			out := copyResource{State: c.State, When: c.When, Attempts: c.Attempts}
			if w.Directory.Person(c.Email) == nil {
				out.Email = c.Email
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, g := range w.Model.Groups {
				if !g.Sees(q.Actor) {
					continue
				}
				for _, s := range w.Model.sent[g.ID] {
					for _, c := range s.Copies {
						out = append(out, c.ID)
					}
				}
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"person": {Type: "people", List: func(w World, q api.Query, key string) []string {
				c := find(w, q, key)
				if c == nil || w.Directory.Person(c.Email) == nil {
					return nil
				}
				return w.personID(c.Email)
			}},
		},
	}
}

type suggestion struct {
	id, key, name, kind string
	managers            []string
}

func (w World) suggestions(viewer string) []suggestion {
	out := []suggestion{}
	for _, t := range SuggestedTags(w.Directory.Tags(viewer), w.Model.Groups) {
		key := model.TagKey(t.ID)
		out = append(out, suggestion{id: w.Model.suggestionID(key), key: key, name: t.Name, kind: SuggestionTag, managers: []string{viewer}})
	}
	for _, l := range Suggested(w.lists(viewer, w.now), w.Model.Groups) {
		managers := []string{viewer}
		for _, h := range l.Hosts {
			if h != viewer && w.Directory.Person(h) != nil {
				managers = append(managers, h)
			}
		}
		out = append(out, suggestion{id: w.Model.suggestionID(l.Key), key: l.Key, name: l.Name, kind: l.Kind, managers: managers})
	}
	return out
}

func (w World) suggestionFor(key string, q api.Query) (suggestion, bool) {
	for _, s := range w.suggestions(q.Actor.Email) {
		if s.id == key {
			return s, true
		}
	}
	return suggestion{}, false
}

func (w World) suggestionKnown(key string) bool {
	w.request.keysOnce.Do(func() {
		w.request.suggestions = map[string]bool{}
		for _, tag := range w.Directory.TagIDs() {
			w.request.suggestions[w.Model.suggestionID(model.TagKey(tag))] = true
		}
		for _, k := range w.keys() {
			w.request.suggestions[w.Model.suggestionID(k)] = true
		}
	})
	return w.request.suggestions[key]
}

func suggestions() api.Type[World] {
	return api.Type[World]{
		Name:  "email-list-suggestions",
		Shape: suggestionResource{},
		Has:   func(w World, key string) bool { return w.suggestionKnown(key) },
		Get: func(w World, q api.Query, key string) (any, bool) {
			s, ok := w.suggestionFor(key, q)
			if !ok {
				return nil, false
			}
			return suggestionResource{Key: s.key, Name: s.name, Kind: s.kind}, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, s := range w.suggestions(q.Actor.Email) {
				out = append(out, s.id)
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"managers": {Type: "people", Many: true, List: func(w World, q api.Query, key string) []string {
				s, ok := w.suggestionFor(key, q)
				if !ok {
					return nil
				}
				out := []string{}
				for _, m := range s.managers {
					out = append(out, w.personID(m)...)
				}
				return out
			}},
		},
	}
}

func settings() api.Type[World] {
	return api.Type[World]{
		Name:  "loop-settings",
		Shape: settingsResource{},
		Has:   func(w World, key string) bool { return key == w.Model.settingsID() },
		Get: func(w World, q api.Query, key string) (any, bool) {
			if key != w.Model.settingsID() {
				return nil, false
			}
			options := w.Sources().Options(q.Actor.Email)
			return settingsResource{Domain: Domain, GradeColors: w.GradeColors, MagicTags: options.Lists, Roles: options.Roles, Relations: options.Relations}, true
		},
		List: func(w World, _ api.Query) []string { return []string{w.Model.settingsID()} },
		Relations: map[string]api.Relation[World]{
			"viewer": {Type: "people", List: func(w World, q api.Query, _ string) []string { return w.personID(q.Actor.Email) }},
		},
	}
}
