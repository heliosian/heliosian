package model

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
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	emailListsHost   = "loop"
	kindMember       = "email-list-member"
	kindSuggestion   = "email-list-suggestion"
	listKindSettings = "loop-settings"
)

type EmailListsWorld struct {
	Model          *EmailLists
	Directory      *Directory
	GradeColors    map[string]string
	parties        *Parties
	activities     *Activities
	activityAdmins *ActivitiesCache
	now            time.Time
	request        *emailListRequest
}

type emailListRequest struct {
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

func NewEmailListsWorld(m *EmailLists, d *Directory, gradeColors map[string]string, parties *Parties, activities *Activities, activityAdmins *ActivitiesCache) EmailListsWorld {
	return EmailListsWorld{Model: m, Directory: d, GradeColors: gradeColors, parties: parties, activities: activities, activityAdmins: activityAdmins}
}

func (w EmailListsWorld) At(now time.Time) EmailListsWorld {
	w.now, w.request = now, &emailListRequest{}
	return w
}

func (w EmailListsWorld) Sources() AudienceSources {
	return EmailListAudience(w.Directory, w.parties, w.activities, w.activityAdmins, w.now)
}

func (w EmailListsWorld) placement() placement {
	w.request.placedOnce.Do(func() {
		p := placement{byGroup: map[string]map[string][]Reason{}, members: map[string]memberKey{}}
		s := w.Sources()
		for _, g := range w.Model.Groups {
			l := g.audience()
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

func (m *EmailLists) memberID(groupID, email string) string {
	return id.Of(m.idKey, kindMember, groupID+"\x00"+email)
}

func (m *EmailLists) suggestionID(key string) string {
	return id.Of(m.idKey, kindSuggestion, key)
}

func (m *EmailLists) settingsID() string {
	return id.Of(m.idKey, listKindSettings, "")
}

func (w EmailListsWorld) onList(g EmailList, email string) bool {
	_, ok := w.placement().byGroup[g.ID][email]
	return ok
}

func (w EmailListsWorld) visible(g EmailList, v access.Actor) bool {
	return g.visibleWith(v, func() bool { return w.onList(g, v.Email) })
}

func (w EmailListsWorld) members(g EmailList) []string {
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

func (w EmailListsWorld) personID(email string) []string {
	if p := w.Directory.Person(w.Directory.Resolve(email)); p != nil && p.ID != "" {
		return []string{p.ID}
	}
	return nil
}

func (w EmailListsWorld) shown(key string, q api.Query) *EmailList {
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
	Mine bool   `json:"mine"`
}

type emailListSettingsResource struct {
	Domain      string            `json:"domain"`
	GradeColors map[string]string `json:"gradeColors,omitempty"`
	MagicTags   []MagicTagOption  `json:"magicTags"`
	Roles       []string          `json:"roles"`
	Relations   []string          `json:"relations"`
}

type emailListResources struct {
	cache     *EmailListsCache
	documents Documents
}

func EmailListResources(c *EmailListsCache, documents Documents) []api.Type[EmailListsWorld] {
	r := emailListResources{cache: c, documents: documents}
	return []api.Type[EmailListsWorld]{r.emailLists(), members(), messages(), copies(), suggestions(), emailListSettings()}
}

func (r emailListResources) stage(wr api.Write[EmailListsWorld], ops []store.Op, err error) error {
	if err != nil {
		return err
	}
	return r.cache.Stage(wr.Tx, ops...)
}

func listLogAfter(wr api.Write[EmailListsWorld], msg string, args ...any) {
	actor := wr.Query.Actor.Email
	ctx := wr.Request.Context()
	wr.Tx.After(func() { slog.InfoContext(ctx, msg, append([]any{"actor", actor}, args...)...) })
}

func listCan(err error) bool {
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

func (r emailListResources) emailLists() api.Type[EmailListsWorld] {
	edits := func(w EmailListsWorld, q api.Query, key string) bool { return w.Model.Group(key).Edits(q.Actor) }
	subscription := func(subscribed bool) api.Action[EmailListsWorld] {
		check := func(w EmailListsWorld, q api.Query, key string) ([]store.Op, error) {
			g := w.Model.Group(key)
			return g.subscription(q.Actor, w.onList(*g, q.Actor.Email), subscribed)
		}
		return api.Do(func(w EmailListsWorld, q api.Query, key string) bool { _, err := check(w, q, key); return listCan(err) }, func(wr api.Write[EmailListsWorld], _ serve.None) error {
			ops, err := check(wr.S, wr.Query, wr.ID)
			if !subscribed {
				listLogAfter(wr, "loop:unsubscribed", "group", wr.S.Model.Group(wr.ID).Name, "how", loopPage)
			} else {
				listLogAfter(wr, "loop:resubscribed", "group", wr.S.Model.Group(wr.ID).Name)
			}
			return r.stage(wr, ops, err)
		})
	}
	archive := func(archived bool) api.Action[EmailListsWorld] {
		return api.Do(func(w EmailListsWorld, q api.Query, key string) bool {
			_, err := w.Model.archiving(q.Actor, w.Model.Group(key), archived)
			return listCan(err)
		}, func(wr api.Write[EmailListsWorld], _ serve.None) error {
			g := wr.S.Model.Group(wr.ID)
			ops, err := wr.S.Model.archiving(wr.Query.Actor, g, archived)
			listLogAfter(wr, "loop:archived", "id", g.ID, "group", g.Name, "archived", archived)
			return r.stage(wr, ops, err)
		})
	}
	return api.Type[EmailListsWorld]{
		Name:  "email-lists",
		Shape: listResource{},
		Has:   func(w EmailListsWorld, key string) bool { return w.Model.Group(key) != nil },
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
			g := w.shown(key, q)
			if g == nil {
				return nil, false
			}
			viewer := q.Actor.Email
			out := listResource{
				Name: g.Name, Address: g.Address(), Aliases: g.Aliases, Title: g.Title, Description: g.Description, Prefix: g.Prefix,
				Visibility: g.Visibility, Posting: g.Posting, Replying: g.Replying, Created: g.Created,
				MemberCount: len(w.members(*g)), Slug: g.Name, Path: g.Path(), App: emailListsHost,
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
		List: func(w EmailListsWorld, q api.Query) []string {
			out := []string{}
			for _, g := range w.Model.Groups {
				if w.visible(g, q.Actor) {
					out = append(out, g.ID)
				}
			}
			return out
		},
		Aliases: func(w EmailListsWorld) map[string]string {
			out := map[string]string{}
			for _, g := range w.Model.Groups {
				for _, local := range g.Names() {
					out[local] = g.ID
				}
			}
			return out
		},
		Create: api.Make(func(wr api.Write[EmailListsWorld], in EmailList) (string, error) {
			in.ID = ""
			ops, g, _, err := wr.S.Model.SaveGroup(wr.Query.Actor, wr.S.Sources(), in, wr.Taken)
			if err != nil {
				return "", err
			}
			listLogAfter(wr, "loop:saved group", "action", "add", "id", g.ID, "group", g.Name, "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions))
			return g.ID, r.stage(wr, ops, nil)
		}),
		Relations: map[string]api.Relation[EmailListsWorld]{
			"managers": {Type: "people", Many: true, List: func(w EmailListsWorld, _ api.Query, key string) []string {
				out := []string{}
				for _, m := range w.Model.Group(key).Managers {
					out = append(out, w.personID(m)...)
				}
				return out
			}},
			"members": {Type: "email-list-members", Many: true, List: func(w EmailListsWorld, _ api.Query, key string) []string {
				g := w.Model.Group(key)
				out := []string{}
				for _, email := range w.members(*g) {
					out = append(out, w.Model.memberID(g.ID, email))
				}
				return out
			}},
			"messages": {Type: "email-list-messages", Many: true, List: func(w EmailListsWorld, _ api.Query, key string) []string {
				out := []string{}
				for _, s := range w.Model.sent[key] {
					out = append(out, s.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[EmailListsWorld]{
			"edit": api.Do(edits, func(wr api.Write[EmailListsWorld], patch groupPatch) error {
				current := wr.S.Model.Group(wr.ID)
				ops, g, _, err := wr.S.Model.SaveGroup(wr.Query.Actor, wr.S.Sources(), patch.over(*current), wr.Taken)
				if err != nil {
					return err
				}
				listLogAfter(wr, "loop:saved group", "action", "edit", "id", g.ID, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
				return r.stage(wr, ops, nil)
			}),
			"delete": api.Do(edits, func(wr api.Write[EmailListsWorld], _ serve.None) error {
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
				listLogAfter(wr, "loop:deleted group", "id", g.ID, "group", g.Name)
				return r.stage(wr, ops, nil)
			}),
			"unsubscribe": subscription(false),
			"resubscribe": subscription(true),
			"archive":     archive(true),
			"unarchive":   archive(false),
		},
	}
}

func members() api.Type[EmailListsWorld] {
	find := func(w EmailListsWorld, q api.Query, key string) (*EmailList, string, bool) {
		mk, ok := w.placement().members[key]
		if !ok {
			return nil, "", false
		}
		g := w.shown(mk.group, q)
		return g, mk.email, g != nil
	}
	return api.Type[EmailListsWorld]{
		Name:  "email-list-members",
		Shape: memberResource{},
		Has: func(w EmailListsWorld, key string) bool {
			_, ok := w.placement().members[key]
			return ok
		},
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
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
		List: func(w EmailListsWorld, q api.Query) []string {
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
		Relations: map[string]api.Relation[EmailListsWorld]{
			"person": {Type: "people", List: func(w EmailListsWorld, q api.Query, key string) []string {
				_, email, ok := find(w, q, key)
				if !ok || w.Directory.Person(email) == nil {
					return nil
				}
				return w.personID(email)
			}},
			"email-list": {Type: "email-lists", List: func(w EmailListsWorld, q api.Query, key string) []string {
				g, _, ok := find(w, q, key)
				if !ok {
					return nil
				}
				return []string{g.ID}
			}},
		},
	}
}

func (w EmailListsWorld) sentMessage(key string, q api.Query) *Sent {
	s := w.Model.sentByID[key]
	if s == nil {
		return nil
	}
	if g := w.Model.Group(s.Message.Group); g == nil || !g.Sees(q.Actor) {
		return nil
	}
	return s
}

func (w EmailListsWorld) sender(from string) (string, *Person) {
	email := strings.ToLower(mail.AddressOf(from))
	return email, w.Directory.Person(w.Directory.Resolve(email))
}

func (w EmailListsWorld) name(email string) string {
	if p := w.Directory.Person(email); p != nil {
		return p.FullName
	}
	return email
}

func messages() api.Type[EmailListsWorld] {
	return api.Type[EmailListsWorld]{
		Name:  "email-list-messages",
		Shape: messageResource{},
		Has:   func(w EmailListsWorld, key string) bool { return w.Model.sentByID[key] != nil },
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
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
		List: func(w EmailListsWorld, q api.Query) []string {
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
		Relations: map[string]api.Relation[EmailListsWorld]{
			"sender": {Type: "people", List: func(w EmailListsWorld, q api.Query, key string) []string {
				s := w.sentMessage(key, q)
				if s == nil {
					return nil
				}
				if _, p := w.sender(s.Message.From); p != nil && p.ID != "" {
					return []string{p.ID}
				}
				return nil
			}},
			"copies": {Type: "email-list-copies", Many: true, List: func(w EmailListsWorld, q api.Query, key string) []string {
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

func copies() api.Type[EmailListsWorld] {
	find := func(w EmailListsWorld, q api.Query, key string) *Copy {
		c := w.Model.copyByID[key]
		if c == nil {
			return nil
		}
		if g := w.Model.Group(c.Group); g == nil || !g.Sees(q.Actor) {
			return nil
		}
		return c
	}
	return api.Type[EmailListsWorld]{
		Name:  "email-list-copies",
		Shape: copyResource{},
		Has:   func(w EmailListsWorld, key string) bool { return w.Model.copyByID[key] != nil },
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
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
		List: func(w EmailListsWorld, q api.Query) []string {
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
		Relations: map[string]api.Relation[EmailListsWorld]{
			"person": {Type: "people", List: func(w EmailListsWorld, q api.Query, key string) []string {
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
	mine                bool
}

func (w EmailListsWorld) suggestions(viewer string) []suggestion {
	out := []suggestion{}
	for _, t := range SuggestedTags(w.Directory.Tags(viewer), w.Model.Groups) {
		key := TagKey(t.ID)
		out = append(out, suggestion{id: w.Model.suggestionID(key), key: key, name: t.Name, kind: SuggestionTag, managers: []string{viewer}, mine: true})
	}
	lists := w.Sources().MagicTags(viewer)
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
	for _, l := range Suggested(lists, w.Model.Groups) {
		managers := []string{viewer}
		for _, h := range l.Hosts {
			if h != viewer && w.Directory.Person(h) != nil {
				managers = append(managers, h)
			}
		}
		out = append(out, suggestion{id: w.Model.suggestionID(l.Key), key: l.Key, name: l.Name, kind: l.Kind, managers: managers, mine: hosted(l)})
	}
	return out
}

func (w EmailListsWorld) suggestionFor(key string, q api.Query) (suggestion, bool) {
	for _, s := range w.suggestions(q.Actor.Email) {
		if s.id == key {
			return s, true
		}
	}
	return suggestion{}, false
}

func (w EmailListsWorld) suggestionKnown(key string) bool {
	w.request.keysOnce.Do(func() {
		w.request.suggestions = map[string]bool{}
		for _, tag := range w.Directory.TagIDs() {
			w.request.suggestions[w.Model.suggestionID(TagKey(tag))] = true
		}
		for _, k := range MagicTagKeys(w.parties, w.activities) {
			w.request.suggestions[w.Model.suggestionID(k)] = true
		}
	})
	return w.request.suggestions[key]
}

func suggestions() api.Type[EmailListsWorld] {
	return api.Type[EmailListsWorld]{
		Name:  "email-list-suggestions",
		Shape: suggestionResource{},
		Has:   func(w EmailListsWorld, key string) bool { return w.suggestionKnown(key) },
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
			s, ok := w.suggestionFor(key, q)
			if !ok {
				return nil, false
			}
			return suggestionResource{Key: s.key, Name: s.name, Kind: s.kind, Mine: s.mine}, true
		},
		List: func(w EmailListsWorld, q api.Query) []string {
			out := []string{}
			for _, s := range w.suggestions(q.Actor.Email) {
				out = append(out, s.id)
			}
			return out
		},
		Relations: map[string]api.Relation[EmailListsWorld]{
			"managers": {Type: "people", Many: true, List: func(w EmailListsWorld, q api.Query, key string) []string {
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

func emailListSettings() api.Type[EmailListsWorld] {
	return api.Type[EmailListsWorld]{
		Name:  "loop-settings",
		Shape: emailListSettingsResource{},
		Has:   func(w EmailListsWorld, key string) bool { return key == w.Model.settingsID() },
		Get: func(w EmailListsWorld, q api.Query, key string) (any, bool) {
			if key != w.Model.settingsID() {
				return nil, false
			}
			options := w.Sources().Options(q.Actor.Email)
			return emailListSettingsResource{Domain: ListDomain, GradeColors: w.GradeColors, MagicTags: options.Lists, Roles: options.Roles, Relations: options.Relations}, true
		},
		List: func(w EmailListsWorld, _ api.Query) []string { return []string{w.Model.settingsID()} },
		Relations: map[string]api.Relation[EmailListsWorld]{
			"viewer": {Type: "people", List: func(w EmailListsWorld, q api.Query, _ string) []string { return w.personID(q.Actor.Email) }},
		},
	}
}
