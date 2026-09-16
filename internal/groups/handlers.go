package groups

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/serve"
	"heliosian/internal/theme"
	"heliosian/internal/who"
)

const shell = "web/groups/index.html"

var pages = []string{"/{$}", "/new", "/groups/{name}", "/admin"}

// Person is someone as the pickers and the member lists show them.
type Person struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl,omitempty"`
	Words    string `json:"words,omitempty"`
}

// Directory is what the app needs of Helios Who?: who an address resolves
// to, the model the rules are read against, a person's tags, Magic Tags
// and the tags shared with them, everyone for the pickers, and the
// toolbar's badges.
type Directory interface {
	Resolve(email string) string
	Model() *who.Model
	Tags(owner string) map[string][]string
	Lists(owner string) []who.List
	Shared(email string) []who.SharedTag
	Person(email string) (Person, bool)
	People() []Person
	Alerts(email string) (int, bool)
}

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	syncer      *Syncer
}

// Register wires the app: one shell for every page, the model, the preview,
// the writes, and Admin Tools. Every route already sits behind sign-in.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string, syncer *Syncer) {
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins, syncer: syncer}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/groups/model", a.model)
	mux.HandleFunc("GET /api/groups/status", a.status)
	mux.HandleFunc("POST /api/groups/preview", a.preview)
	mux.HandleFunc("POST /api/groups/group", a.saveGroup)
	mux.HandleFunc("DELETE /api/groups/group", a.deleteGroup)
	mux.HandleFunc("POST /api/groups/theme", a.setTheme)
	mux.HandleFunc("POST /api/groups/theme/picture", theme.Upload(store, func(w http.ResponseWriter, r *http.Request) bool {
		_, ok := a.requireSuperAdmin(w, r)
		return ok
	}))
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// who is the signed-in person as the app keys them: the address Google
// vouched for, resolved through the directory's aliases.
func (a app) who(r *http.Request) (string, bool) {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return email, a.cache.IsAdmin(email)
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email, admin := a.who(r)
	if !admin {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func (a app) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email, _ := a.who(r)
	if !a.cache.IsSuperAdmin(email) {
		http.Error(w, "super admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func sourcesOf(directory Directory) Sources {
	return Sources{Directory: directory.Model(), Tags: directory.Tags, Lists: directory.Lists, Shared: directory.Shared}
}

func (a app) sources() Sources {
	return sourcesOf(a.directory)
}

// PlanFor is every group as Google should hold it, read against the models
// as they stand when called; the syncer calls it on every change.
func PlanFor(cache *Cache, directory Directory) func() []Desired {
	return func() []Desired {
		return Plan(cache.Model(), sourcesOf(directory))
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

type user struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	Initial      string `json:"initial"`
	PhotoURL     string `json:"photoUrl,omitempty"`
	IsAdmin      bool   `json:"isAdmin"`
	IsSuperAdmin bool   `json:"isSuperAdmin"`
}

type alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

// ruleView is a rule with the words for its tags: a tag's own name, a
// Magic Tag's name resolved through its owner, and a note when a tag the
// owner no longer has.
type ruleView struct {
	Rule
	TagLabels []string `json:"tagLabels"`
}

// groupView is a group as its page shows it: the rules with their words,
// the managers by name, the members, and where it stands with Google.
type groupView struct {
	Group
	Address  string     `json:"address"`
	Rules    []ruleView `json:"rules"`
	Managers []Person   `json:"managers"`
	Members  []Person   `json:"members"`
	Mine     bool       `json:"mine"`
	Status   Status     `json:"status"`
}

// listOption is one of the viewer's Magic Tags as the rule editor offers
// them.
type listOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// sharedOption is a tag shared with the viewer as the rule editor offers
// it: its key, its name, and whose it is.
type sharedOption struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	OwnerName string `json:"ownerName"`
}

type options struct {
	Classrooms []string       `json:"classrooms"`
	Grades     []string       `json:"grades"`
	Tags       []string       `json:"tags"`
	Lists      []listOption   `json:"lists"`
	Shared     []sharedOption `json:"shared"`
	Roles      []string       `json:"roles"`
	Relations  []string       `json:"relations"`
}

func (a app) person(email string) Person {
	if p, ok := a.directory.Person(email); ok {
		return p
	}
	return Person{Email: email, Name: email}
}

func (a app) people(emails []string) []Person {
	out := make([]Person, 0, len(emails))
	for _, email := range emails {
		out = append(out, a.person(email))
	}
	return out
}

// tagLabels is the words for a rule's tags, read as its owner: a tag's own
// name, a Magic Tag's name, a shared tag's name with whose it is, and a
// note for one the owner no longer has.
func (a app) tagLabels(r Rule) []string {
	names := map[string]string{}
	for _, list := range a.directory.Lists(r.Owner) {
		names[list.Key] = list.Name
	}
	for _, shared := range a.directory.Shared(r.Owner) {
		names[SharedKey(shared.Owner, shared.Name)] = shared.Name + " (" + shared.OwnerName + "'s)"
	}
	tags := a.directory.Tags(r.Owner)
	out := make([]string, 0, len(r.Tags))
	for _, tag := range r.Tags {
		switch {
		case names[tag] != "":
			out = append(out, names[tag])
		case tags[tag] != nil:
			out = append(out, tag)
		case strings.HasPrefix(tag, sharedPrefix):
			out = append(out, strings.TrimPrefix(tag, sharedPrefix)+" (no longer shared)")
		default:
			out = append(out, tag+" (no longer a tag)")
		}
	}
	return out
}

// sharedRules refuses a rule of the viewer's naming a shared tag that is not
// shared with them, unless the group already holds the rule word for word.
func (a app) sharedRules(viewer string, existing []Rule, rules []Rule) error {
	mine := map[string]bool{}
	for _, shared := range a.directory.Shared(viewer) {
		mine[SharedKey(shared.Owner, shared.Name)] = true
	}
	for i, r := range rules {
		if r.Owner != viewer || slices.ContainsFunc(existing, func(e Rule) bool { return sameRule(e, r) }) {
			continue
		}
		for _, tag := range r.Tags {
			if strings.HasPrefix(tag, sharedPrefix) && !mine[tag] {
				return fmt.Errorf("rule %d names a tag that is not shared with you", i+1)
			}
		}
	}
	return nil
}

func (a app) view(g Group, viewer string) groupView {
	v := groupView{Group: g, Address: g.Address(), Rules: []ruleView{}, Managers: a.people(g.Managers), Mine: g.Manages(viewer), Status: a.syncer.Status(g.Name)}
	for _, r := range g.Rules {
		v.Rules = append(v.Rules, ruleView{Rule: r, TagLabels: a.tagLabels(r)})
	}
	v.Members = a.people(Members(g, a.sources()))
	return v
}

func (a app) options(viewer string) options {
	model := a.directory.Model()
	classrooms := []string{}
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, c.Name)
	}
	present := map[string]bool{}
	for _, p := range model.People {
		if p.IsStudent && p.Grade != "" {
			present[p.Grade] = true
		}
	}
	grades := []string{}
	for _, g := range model.Grades {
		if present[g.Name] {
			grades = append(grades, g.Name)
		}
	}
	tags := []string{}
	for name := range a.directory.Tags(viewer) {
		tags = append(tags, name)
	}
	slices.Sort(tags)
	lists := []listOption{}
	for _, l := range a.directory.Lists(viewer) {
		lists = append(lists, listOption{Key: l.Key, Name: l.Name, Kind: l.Kind})
	}
	slices.SortFunc(lists, func(x, y listOption) int { return strings.Compare(x.Name, y.Name) })
	shared := []sharedOption{}
	for _, s := range a.directory.Shared(viewer) {
		shared = append(shared, sharedOption{Key: SharedKey(s.Owner, s.Name), Name: s.Name, OwnerName: s.OwnerName})
	}
	return options{Classrooms: classrooms, Grades: grades, Tags: tags, Lists: lists, Shared: shared, Roles: Roles, Relations: Relations}
}

// model serves the app: the groups the viewer manages - every group, for
// an admin - each with its members and standing, and what the editor offers.
func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	me := a.person(email)
	view := struct {
		User    user        `json:"user"`
		Domain  string      `json:"domain"`
		Groups  []groupView `json:"groups"`
		Options options     `json:"options"`
		People  []Person    `json:"people"`
		Alerts  alerts      `json:"alerts"`
		Theme   theme.Theme `json:"theme"`
	}{
		User:    user{Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: admin, IsSuperAdmin: a.cache.IsSuperAdmin(email)},
		Domain:  Domain,
		Groups:  []groupView{},
		Options: a.options(email),
		People:  a.directory.People(),
		Theme:   a.cache.Model().Theme,
	}
	for _, g := range a.cache.Model().Groups {
		if admin || g.Manages(email) {
			view.Groups = append(view.Groups, a.view(g, email))
		}
	}
	view.Alerts.Stale, view.Alerts.Privacy = a.directory.Alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode groups model", "error", err)
	}
}

// status answers /api/groups/status?name=<group> with where the group
// stands with Google, for a page polling after a change.
func (a app) status(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name")))
	g := a.cache.Model().Group(name)
	if g == nil {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	if !admin && !g.Manages(email) {
		http.Error(w, "you do not manage this group", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.syncer.Status(name)); err != nil {
		slog.ErrorContext(r.Context(), "encode group status", "error", err)
	}
}

// ownRules refuses a rule read as anyone but the viewer unless the group
// already holds it word for word: a rule carried over unchanged from
// another manager keeps reading their tags, and no other way exists to
// read them.
func ownRules(viewer string, existing []Rule, rules []Rule) error {
	for i, r := range rules {
		if r.Owner == viewer {
			continue
		}
		if !slices.ContainsFunc(existing, func(e Rule) bool { return sameRule(e, r) }) {
			return fmt.Errorf("rule %d reads someone else's tags", i+1)
		}
	}
	return nil
}

func sameRule(a, b Rule) bool {
	return a.Kind == b.Kind && a.Search == b.Search && a.Owner == b.Owner &&
		slices.Equal(a.Roles, b.Roles) && slices.Equal(a.Classrooms, b.Classrooms) &&
		slices.Equal(a.Grades, b.Grades) && slices.Equal(a.Tags, b.Tags) && slices.Equal(a.Family, b.Family)
}

// preview answers the editor with who a draft's rules pick out, before it
// is saved.
func (a app) preview(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	var body struct {
		Name  string `json:"name"`
		Rules []Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	draft := Normalize(Group{Name: "preview", Title: "preview", Managers: []string{email}, Rules: body.Rules})
	var existing []Rule
	if g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(body.Name))); g != nil {
		if !admin && !g.Manages(email) {
			http.Error(w, "you do not manage this group", http.StatusForbidden)
			return
		}
		existing = g.Rules
	}
	if err := ownRules(email, existing, draft.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.sharedRules(email, existing, draft.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for i, rule := range draft.Rules {
		if err := CheckRule(rule); err != nil {
			http.Error(w, fmt.Sprintf("rule %d: %v", i+1, err), http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"members": a.people(Members(draft, a.sources()))}); err != nil {
		slog.ErrorContext(r.Context(), "encode groups preview", "error", err)
	}
}

// commit rebuilds the model over the proposed tables first, so a change the
// sheet rules reject never reaches the sheet, then applies it in memory and
// queues the writes behind every earlier one.
func (a app) commit(r *http.Request, w http.ResponseWriter, tables *Tables, flush func() error) bool {
	ctx := r.Context()
	model, err := BuildModel(tables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	applied := make(chan struct{})
	a.queue.Add(func() {
		a.cache.set(tables, model)
		close(applied)
		if err := flush(); err != nil {
			slog.ErrorContext(ctx, "groups write", "error", err)
		}
	})
	<-applied
	return true
}

func (a app) logChange(actor, action, group, detail string) error {
	return a.writer.Append(appName, changeLogTab, []string{time.Now().Format(time.RFC3339), actor, action, group, detail})
}

func rowOf(columns []string, cells map[string]string) []string {
	row := make([]string, len(columns))
	for i, column := range columns {
		row[i] = cells[column]
	}
	return row
}

// saveGroup makes a group or changes one. Anyone may make one and becomes
// its first manager; only its managers and the admins change it; its name
// never changes, being its address.
func (a app) saveGroup(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	var body struct {
		Original string `json:"original"`
		Group
	}
	if !decode(w, r, &body) {
		return
	}
	g := Normalize(body.Group)
	original := strings.ToLower(strings.TrimSpace(body.Original))
	var existing []Rule
	action := "add"
	if original == "" {
		if a.cache.Model().Group(g.Name) != nil {
			http.Error(w, fmt.Sprintf("%s is taken", g.Address()), http.StatusBadRequest)
			return
		}
		if !g.Manages(email) {
			g.Managers = append([]string{email}, g.Managers...)
		}
		g.CreatedBy = email
		g.Created = time.Now().Format("2006-01-02")
	} else {
		current := a.cache.Model().Group(original)
		if current == nil {
			http.Error(w, "no such group", http.StatusNotFound)
			return
		}
		if !admin && !current.Manages(email) {
			http.Error(w, "you do not manage this group", http.StatusForbidden)
			return
		}
		if g.Name != original {
			http.Error(w, "a group's name is its address and cannot change; make a new group", http.StatusBadRequest)
			return
		}
		g.CreatedBy, g.Created = current.CreatedBy, current.Created
		existing = current.Rules
		action = "edit"
	}
	if err := ownRules(email, existing, g.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.sharedRules(email, existing, g.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := CheckGroup(g); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tables := a.cache.Tables().withGroup(g)
	if !a.commit(r, w, tables, func() error {
		if action == "add" {
			if err := a.writer.AppendCells(appName, groupsTab, groupCells(g)); err != nil {
				return err
			}
		} else if err := a.writer.Upsert(appName, groupsTab, "Name", g.Name, groupCells(g)); err != nil {
			return err
		}
		for _, tab := range []string{managersTab, rulesTab} {
			if err := a.writer.Delete(appName, tab, map[string]string{"Group": g.Name}); err != nil {
				return err
			}
		}
		managers := [][]string{}
		for _, m := range g.Managers {
			managers = append(managers, []string{g.Name, m})
		}
		if err := a.writer.AppendAll(appName, managersTab, managers); err != nil {
			return err
		}
		rules := [][]string{}
		for _, rule := range g.Rules {
			rules = append(rules, rowOf(RuleColumns, ruleCells(g.Name, rule)))
		}
		if err := a.writer.AppendAll(appName, rulesTab, rules); err != nil {
			return err
		}
		return a.logChange(email, action, g.Name, fmt.Sprintf("%s; %d managers; %d rules", g.Title, len(g.Managers), len(g.Rules)))
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: saved group", "action", action, "group", g.Name, "rules", len(g.Rules), "managers", len(g.Managers))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.view(g, email)); err != nil {
		slog.ErrorContext(r.Context(), "encode saved group", "error", err)
	}
}

func (a app) deleteGroup(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(body.Name))
	current := a.cache.Model().Group(name)
	if current == nil {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	if !admin && !current.Manages(email) {
		http.Error(w, "you do not manage this group", http.StatusForbidden)
		return
	}
	tables := a.cache.Tables().withoutGroup(name)
	if !a.commit(r, w, tables, func() error {
		if err := a.writer.Delete(appName, groupsTab, map[string]string{"Name": name}); err != nil {
			return err
		}
		for _, tab := range []string{managersTab, rulesTab} {
			if err := a.writer.Delete(appName, tab, map[string]string{"Group": name}); err != nil {
				return err
			}
		}
		return a.logChange(email, "delete", name, current.Title)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: deleted group", "group", name)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email        string      `json:"email"`
		HasStore     bool        `json:"hasStore"`
		Admins       []string    `json:"admins"`
		Theme        theme.Theme `json:"theme"`
		IsSuperAdmin bool        `json:"isSuperAdmin"`
	}{Email: email, HasStore: a.store != nil, Admins: a.cache.Admins(a.superAdmins()), Theme: a.cache.Model().Theme, IsSuperAdmin: a.cache.IsSuperAdmin(email)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode groups admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	super := map[string]bool{}
	for _, e := range a.superAdmins() {
		super[e] = true
	}
	admins := []string{}
	for _, e := range cleanEmails(body.Admins) {
		if !super[e] {
			admins = append(admins, e)
		}
	}
	current := a.cache.tabAdmins()
	tables := a.cache.Tables().withAdmins(admins)
	if !a.commit(r, w, tables, func() error {
		for _, e := range current {
			if !slices.Contains(admins, e) {
				if err := a.writer.Delete(appName, adminsTab, map[string]string{"Email": e}); err != nil {
					return err
				}
			}
		}
		for _, e := range admins {
			if !slices.Contains(current, e) {
				if err := a.writer.Append(appName, adminsTab, []string{e}); err != nil {
					return err
				}
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: set the admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

// setTheme takes the app's colours at once and writes every row of the
// Settings tab, for the platform's super admins alone.
func (a app) setTheme(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var body theme.Theme
	if !decode(w, r, &body) {
		return
	}
	t, err := theme.Of(body.Values())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	values := t.Values()
	tables := a.cache.Tables()
	for _, key := range theme.Keys {
		tables = tables.withSetting(key, values[key])
	}
	if !a.commit(r, w, tables, func() error {
		for _, key := range theme.Keys {
			if err := a.writer.Upsert(appName, settingsTab, "Key", key, map[string]string{"Value": values[key]}); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: set the theme", "theme", t)
	w.WriteHeader(http.StatusNoContent)
}
