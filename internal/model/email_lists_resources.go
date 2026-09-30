package model

import (
	"cmp"
	"log/slog"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	emailListsHost   = "loop"
	kindMember       = "email-list-member"
	kindSuggestion   = "email-list-suggestion"
	listKindSettings = "loop-settings"
)

type placement struct {
	byGroup map[string]map[string][]Reason
	members map[string]memberKey
}

type memberKey struct {
	group, email string
}

func (m *Model) listSources() AudienceSources {
	return m.Audience(m.scope.now)
}

func (m *Model) listPlacement() placement {
	m.scope.placedOnce.Do(func() {
		p := placement{byGroup: map[string]map[string][]Reason{}, members: map[string]memberKey{}}
		s := m.listSources()
		for _, g := range m.EmailLists.Groups {
			l := g.audience()
			l.Excluded = nil
			placed := l.Reasons(s)
			p.byGroup[g.ID] = placed
			for email := range placed {
				if !g.HasExcluded(email) {
					p.members[m.EmailLists.memberID(g.ID, email)] = memberKey{group: g.ID, email: email}
				}
			}
		}
		m.scope.placed = p
	})
	return m.scope.placed
}

func (m *EmailLists) memberID(groupID, email string) string {
	return id.Of(m.idKey, kindMember, groupID+"\x00"+email)
}

func (m *EmailLists) suggestionID(key string) string {
	return id.Of(m.idKey, kindSuggestion, key)
}

func (m *EmailLists) settingsID() string {
	return id.Of(m.idKey, listKindSettings, "")
}

func (m *Model) onEmailList(g EmailList, email string) bool {
	_, ok := m.listPlacement().byGroup[g.ID][email]
	return ok
}

func (m *Model) listVisible(g EmailList, v access.Actor) bool {
	return g.visibleWith(v, func() bool { return m.onEmailList(g, v.Email) })
}

func (m *Model) listMembers(g EmailList) []string {
	inside, outside := []string{}, []string{}
	for email := range m.listPlacement().byGroup[g.ID] {
		switch {
		case g.HasExcluded(email):
		case m.Directory.Person(email) != nil:
			inside = append(inside, email)
		default:
			outside = append(outside, email)
		}
	}
	slices.Sort(inside)
	slices.Sort(outside)
	return append(inside, outside...)
}

func (m *Model) shownList(key string, q api.Query) *EmailList {
	g := m.EmailLists.Group(key)
	if g == nil || !m.listVisible(*g, q.Actor) {
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
	Mine bool   `json:"mine"`
}

type emailListSettingsResource struct {
	Domain      string            `json:"domain"`
	GradeColors map[string]string `json:"gradeColors,omitempty"`
	Roles       []string          `json:"roles"`
	Relations   []string          `json:"relations"`
}

type emailListResources struct {
	store     *Store
	documents *DocumentFiler
}

func EmailListResources(s *Store, documents *DocumentFiler) []api.Type[*Model] {
	r := emailListResources{store: s, documents: documents}
	return []api.Type[*Model]{r.emailLists(), members(), messages(), copies(), suggestions(), emailListSettings()}
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

func (p groupPatch) over(g EmailList) EmailList {
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

func (r emailListResources) emailLists() api.Type[*Model] {
	edits := func(m *Model, q api.Query, key string) bool { return m.EmailLists.Group(key).Edits(q.Actor) }
	subscription := func(subscribed bool) api.Action[*Model] {
		check := func(m *Model, q api.Query, key string) ([]store.Op, error) {
			g := m.EmailLists.Group(key)
			return g.subscription(q.Actor, m.onEmailList(*g, q.Actor.Email), subscribed)
		}
		return api.Do(func(m *Model, q api.Query, key string) bool { _, err := check(m, q, key); return permitted(err) }, func(wr api.Write[*Model], _ serve.None) error {
			ops, err := check(wr.S, wr.Query, wr.ID)
			if !subscribed {
				logAfter(wr, "loop:unsubscribed", "group", wr.S.EmailLists.Group(wr.ID).Name, "how", loopPage)
			} else {
				logAfter(wr, "loop:resubscribed", "group", wr.S.EmailLists.Group(wr.ID).Name)
			}
			return r.store.stage(wr, emailListsAppName, ops, err)
		})
	}
	archive := func(archived bool) api.Action[*Model] {
		return api.Do(func(m *Model, q api.Query, key string) bool {
			_, err := m.EmailLists.archiving(q.Actor, m.EmailLists.Group(key), archived)
			return permitted(err)
		}, func(wr api.Write[*Model], _ serve.None) error {
			g := wr.S.EmailLists.Group(wr.ID)
			ops, err := wr.S.EmailLists.archiving(wr.Query.Actor, g, archived)
			logAfter(wr, "loop:archived", "id", g.ID, "group", g.Name, "archived", archived)
			return r.store.stage(wr, emailListsAppName, ops, err)
		})
	}
	return api.Type[*Model]{
		Name:  "email-lists",
		Shape: listResource{},
		Has:   func(m *Model, key string) bool { return m.EmailLists.Group(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			g := m.shownList(key, q)
			if g == nil {
				return nil, false
			}
			viewer := q.Actor.Email
			out := listResource{
				Name: g.Name, Address: g.Address(), Aliases: g.Aliases, Title: g.Title, Description: g.Description, Prefix: g.Prefix,
				Visibility: g.Visibility, Posting: g.Posting, Replying: g.Replying, Created: g.Created,
				MemberCount: len(m.listMembers(*g)), Slug: g.Name, Path: g.Path(), App: emailListsHost,
				Me: listMe{Managing: g.Manages(viewer), Member: m.onEmailList(*g, viewer), Unsubscribed: g.HasExcluded(viewer), Archived: m.EmailLists.Archived(g.ID, viewer)},
			}
			for _, manager := range g.Managers {
				if m.personID(manager) == nil {
					out.ManagersOutside = append(out.ManagersOutside, manager)
				}
			}
			if g.Sees(q.Actor) {
				view := &ManagerView{Rules: []RuleView{}, Additions: g.Additions, Excluded: g.Excluded, Sent: len(m.EmailLists.sent[g.ID])}
				s := m.listSources()
				for _, rule := range g.Rules {
					view.Rules = append(view.Rules, RuleView{Rule: rule, TagLabels: s.TagLabels(rule, g.Managers, viewer)})
				}
				out.ManagerView = view
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, g := range m.EmailLists.Groups {
				if m.listVisible(g, q.Actor) {
					out = append(out, g.ID)
				}
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, g := range m.EmailLists.Groups {
				for _, local := range g.Names() {
					out[local] = g.ID
				}
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], in EmailList) (string, error) {
			in.ID = ""
			ops, g, _, err := wr.S.EmailLists.SaveGroup(wr.Query.Actor, wr.S.listSources(), in, wr.Taken)
			if err != nil {
				return "", err
			}
			logAfter(wr, "loop:saved group", "action", "add", "id", g.ID, "group", g.Name, "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions))
			return g.ID, r.store.stage(wr, emailListsAppName, ops, nil)
		}),
		Relations: map[string]api.Relation[*Model]{
			"managers": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				out := []string{}
				for _, manager := range m.EmailLists.Group(key).Managers {
					out = append(out, m.personID(manager)...)
				}
				return out
			}},
			"members": {Type: "email-list-members", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				g := m.EmailLists.Group(key)
				out := []string{}
				for _, email := range m.listMembers(*g) {
					out = append(out, m.EmailLists.memberID(g.ID, email))
				}
				return out
			}},
			"messages": {Type: "email-list-messages", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				out := []string{}
				for _, s := range m.EmailLists.sent[key] {
					out = append(out, s.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(edits, func(wr api.Write[*Model], patch groupPatch) error {
				current := wr.S.EmailLists.Group(wr.ID)
				ops, g, _, err := wr.S.EmailLists.SaveGroup(wr.Query.Actor, wr.S.listSources(), patch.over(*current), wr.Taken)
				if err != nil {
					return err
				}
				logAfter(wr, "loop:saved group", "action", "edit", "id", g.ID, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
				return r.store.stage(wr, emailListsAppName, ops, nil)
			}),
			"delete": api.Do(edits, func(wr api.Write[*Model], _ serve.None) error {
				ops, g, err := wr.S.EmailLists.DeleteGroup(wr.Query.Actor, wr.ID)
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
				return r.store.stage(wr, emailListsAppName, ops, nil)
			}),
			"unsubscribe": subscription(false),
			"resubscribe": subscription(true),
			"archive":     archive(true),
			"unarchive":   archive(false),
		},
	}
}

func members() api.Type[*Model] {
	find := func(m *Model, q api.Query, key string) (*EmailList, string, bool) {
		mk, ok := m.listPlacement().members[key]
		if !ok {
			return nil, "", false
		}
		g := m.shownList(mk.group, q)
		return g, mk.email, g != nil
	}
	return api.Type[*Model]{
		Name:  "email-list-members",
		Shape: memberResource{},
		Has: func(m *Model, key string) bool {
			_, ok := m.listPlacement().members[key]
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			g, email, ok := find(m, q, key)
			if !ok {
				return nil, false
			}
			out := memberResource{}
			if m.Directory.Person(email) == nil {
				out.Email, out.Name, out.Outside = email, email, true
				if added := g.Addition(email); added != nil && added.Name != "" {
					out.Name = added.Name
				}
			}
			if g.Sees(q.Actor) {
				out.Reasons = m.listPlacement().byGroup[g.ID][email]
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, g := range m.EmailLists.Groups {
				if !m.listVisible(g, q.Actor) {
					continue
				}
				for _, email := range m.listMembers(g) {
					out = append(out, m.EmailLists.memberID(g.ID, email))
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"person": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				_, email, ok := find(m, q, key)
				if !ok || m.Directory.Person(email) == nil {
					return nil
				}
				return m.personID(email)
			}},
			"email-list": {Type: "email-lists", List: func(m *Model, q api.Query, key string) []string {
				g, _, ok := find(m, q, key)
				if !ok {
					return nil
				}
				return []string{g.ID}
			}},
		},
	}
}

func (m *Model) listSentMessage(key string, q api.Query) *Sent {
	s := m.EmailLists.sentByID[key]
	if s == nil {
		return nil
	}
	if g := m.EmailLists.Group(s.Message.Group); g == nil || !g.Sees(q.Actor) {
		return nil
	}
	return s
}

func (m *Model) listSender(from string) (string, *Person) {
	email := strings.ToLower(mail.AddressOf(from))
	return email, m.Directory.Person(m.Directory.Resolve(email))
}

func (m *Model) listPersonName(email string) string {
	if p := m.Directory.Person(email); p != nil {
		return p.FullName
	}
	return email
}

func messages() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "email-list-messages",
		Shape: messageResource{},
		Has:   func(m *Model, key string) bool { return m.EmailLists.sentByID[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			s := m.listSentMessage(key, q)
			if s == nil {
				return nil, false
			}
			out := messageResource{Received: s.Message.Received, Subject: s.Message.Subject, Recipients: s.Recipients, Delivered: s.Delivered, Failed: s.Failed, Pending: s.Pending}
			if email, p := m.listSender(s.Message.From); p == nil {
				out.FromEmail, out.FromName = email, senderName(s.Message.From)
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, g := range m.EmailLists.Groups {
				if !g.Sees(q.Actor) {
					continue
				}
				for _, s := range m.EmailLists.sent[g.ID] {
					out = append(out, s.ID)
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"sender": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				s := m.listSentMessage(key, q)
				if s == nil {
					return nil
				}
				if _, p := m.listSender(s.Message.From); p != nil && p.ID != "" {
					return []string{p.ID}
				}
				return nil
			}},
			"copies": {Type: "email-list-copies", Many: true, List: func(m *Model, q api.Query, key string) []string {
				s := m.listSentMessage(key, q)
				if s == nil {
					return nil
				}
				copies := slices.Clone(s.Copies)
				slices.SortStableFunc(copies, func(x, y *Copy) int {
					return cmp.Or(cmp.Compare(copyOrder[x.State], copyOrder[y.State]), cmp.Compare(strings.ToLower(m.listPersonName(x.Email)), strings.ToLower(m.listPersonName(y.Email))))
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

func copies() api.Type[*Model] {
	find := func(m *Model, q api.Query, key string) *Copy {
		c := m.EmailLists.copyByID[key]
		if c == nil {
			return nil
		}
		if g := m.EmailLists.Group(c.Group); g == nil || !g.Sees(q.Actor) {
			return nil
		}
		return c
	}
	return api.Type[*Model]{
		Name:  "email-list-copies",
		Shape: copyResource{},
		Has:   func(m *Model, key string) bool { return m.EmailLists.copyByID[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			c := find(m, q, key)
			if c == nil {
				return nil, false
			}
			out := copyResource{State: c.State, When: c.When, Attempts: c.Attempts}
			if m.Directory.Person(c.Email) == nil {
				out.Email = c.Email
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, g := range m.EmailLists.Groups {
				if !g.Sees(q.Actor) {
					continue
				}
				for _, s := range m.EmailLists.sent[g.ID] {
					for _, c := range s.Copies {
						out = append(out, c.ID)
					}
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"person": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				c := find(m, q, key)
				if c == nil || m.Directory.Person(c.Email) == nil {
					return nil
				}
				return m.personID(c.Email)
			}},
		},
	}
}

type suggestion struct {
	id, key, name, kind string
	managers            []string
	mine                bool
}

func (m *Model) listSuggestions(viewer string) []suggestion {
	out := []suggestion{}
	for _, t := range SuggestedTags(m.Directory.Tags(viewer), m.EmailLists.Groups) {
		key := TagKey(t.ID)
		out = append(out, suggestion{id: m.EmailLists.suggestionID(key), key: key, name: t.Name, kind: SuggestionTag, managers: []string{viewer}, mine: true})
	}
	lists := m.listSources().MagicTags(viewer)
	byKey := map[string]MagicTag{}
	for _, l := range lists {
		byKey[l.Key] = l
	}
	hosted := func(l MagicTag) bool {
		for {
			if slices.Contains(l.Hosts, viewer) {
				return true
			}
			parent, ok := byKey[l.Parent]
			if !ok {
				return false
			}
			l = parent
		}
	}
	for _, l := range Suggested(lists, m.EmailLists.Groups) {
		managers := []string{viewer}
		for _, h := range l.Hosts {
			if h != viewer && m.Directory.Person(h) != nil {
				managers = append(managers, h)
			}
		}
		out = append(out, suggestion{id: m.EmailLists.suggestionID(l.Key), key: l.Key, name: l.Name, kind: l.Kind, managers: managers, mine: hosted(l)})
	}
	return out
}

func (m *Model) listSuggestionFor(key string, q api.Query) (suggestion, bool) {
	for _, s := range m.listSuggestions(q.Actor.Email) {
		if s.id == key {
			return s, true
		}
	}
	return suggestion{}, false
}

func (m *Model) listSuggestionKnown(key string) bool {
	m.scope.keysOnce.Do(func() {
		m.scope.suggestions = map[string]bool{}
		for _, tag := range m.Directory.TagIDs() {
			m.scope.suggestions[m.EmailLists.suggestionID(TagKey(tag))] = true
		}
		for _, k := range m.magicTagKeys() {
			m.scope.suggestions[m.EmailLists.suggestionID(k)] = true
		}
	})
	return m.scope.suggestions[key]
}

func suggestions() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "email-list-suggestions",
		Shape: suggestionResource{},
		Has:   func(m *Model, key string) bool { return m.listSuggestionKnown(key) },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			s, ok := m.listSuggestionFor(key, q)
			if !ok {
				return nil, false
			}
			return suggestionResource{Key: s.key, Name: s.name, Kind: s.kind, Mine: s.mine}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, s := range m.listSuggestions(q.Actor.Email) {
				out = append(out, s.id)
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"managers": {Type: "people", Many: true, List: func(m *Model, q api.Query, key string) []string {
				s, ok := m.listSuggestionFor(key, q)
				if !ok {
					return nil
				}
				out := []string{}
				for _, manager := range s.managers {
					out = append(out, m.personID(manager)...)
				}
				return out
			}},
			"magic-tag": {Type: "magic-tags", List: func(m *Model, q api.Query, key string) []string {
				s, ok := m.listSuggestionFor(key, q)
				if !ok || s.kind == SuggestionTag {
					return nil
				}
				return []string{m.magicTagID(s.key)}
			}},
		},
	}
}

func emailListSettings() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "loop-settings",
		Shape: emailListSettingsResource{},
		Has:   func(m *Model, key string) bool { return key == m.EmailLists.settingsID() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != m.EmailLists.settingsID() {
				return nil, false
			}
			return emailListSettingsResource{Domain: ListDomain, GradeColors: m.Config.GradeColors, Roles: AudienceRoles, Relations: AudienceRelations}, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{m.EmailLists.settingsID()} },
		Relations: map[string]api.Relation[*Model]{
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
		},
	}
}
