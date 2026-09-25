package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
	"heliosian/internal/who"
)

const shell = "web/loop/index.html"

var pages = []string{"/{$}", "/new", "/groups/{name}", "/admin"}

type Person struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl,omitempty"`
	Words    string `json:"words,omitempty"`
	Role     string `json:"role,omitempty"`
	Grade    string `json:"grade,omitempty"`
	Context  string `json:"context,omitempty"`
}

type Directory interface {
	Resolve(email string) string
	Model() *who.Model
	Tags(owner string) map[string][]string
	Lists(owner string) []who.List
	Shared(email string) []who.SharedTag
	Person(email string) (Person, bool)
	People() []Person
	Alerts(email string) (int, bool)
	GradeColors() map[string]string
}

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	mail        Mail
	mailer      *mailer
	describer   Describer
}

type Describer interface {
	Group(ctx context.Context, actor string, facts describe.GroupFacts) (string, error)
}

func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string, mailbox Mail, describer Describer) {
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins, mail: mailbox, describer: describer}
	a.mailer = newMailer(cache, writer, queue, directory, mailbox)
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/loop/model", a.model)
	mux.HandleFunc("POST /api/loop/preview", a.preview)
	mux.HandleFunc("POST /api/loop/describe", a.describe)
	mux.HandleFunc("POST /api/loop/group", a.saveGroup)
	mux.HandleFunc("DELETE /api/loop/group", a.deleteGroup)
	mux.HandleFunc("GET /api/loop/messages", a.messages)
	mux.HandleFunc("POST /api/loop/subscription", a.subscription)
	mux.HandleFunc("POST /api/loop/archive", a.archive)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /hooks/mail/mime", a.inbound)
	mux.HandleFunc("POST /hooks/events", a.events)
	mux.HandleFunc("GET /open/share/about.png", a.shareCard)
	mux.HandleFunc("GET /open/unsubscribe/{token}", a.unsubscribePage)
	mux.HandleFunc("POST /open/unsubscribe/{token}", a.unsubscribe)
	a.mailer.recover()
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

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

func SourcesOf(directory Directory) Sources {
	return Sources{Directory: directory.Model(), Tags: directory.Tags, Lists: directory.Lists, Shared: directory.Shared}
}

func (a app) sources() Sources {
	return SourcesOf(a.directory)
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

type ruleView struct {
	Rule
	TagLabels []string `json:"tagLabels"`
}

type Member struct {
	Person
	Reasons []Reason `json:"reasons"`
	Outside bool     `json:"outside,omitempty"`
}

type groupView struct {
	Group
	Address      string     `json:"address"`
	Rules        []ruleView `json:"rules"`
	Managers     []Person   `json:"managers"`
	Members      []Member   `json:"members"`
	Mine         bool       `json:"mine"`
	Member       bool       `json:"member"`
	Unsubscribed bool       `json:"unsubscribed"`
	Archived     bool       `json:"archived"`
	Sent         int        `json:"sent"`
}

type suggestion struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Managers []Person `json:"managers"`
}

func Suggested(lists []who.List, groups []Group) []who.List {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []who.List{}
	for _, l := range lists {
		if (l.Kind == who.ListParty || l.Kind == who.ListActivity) && !named[l.Key] {
			out = append(out, l)
		}
	}
	return out
}

func SuggestedTags(tags map[string][]string, groups []Group, owner string) []string {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []string{}
	for name, people := range tags {
		if !named[filter.TagKey(owner, name)] && len(people) > 0 {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

const SuggestionTag = "tag"

func (a app) suggestions(viewer string) []suggestion {
	out := []suggestion{}
	for _, name := range SuggestedTags(a.directory.Tags(viewer), a.cache.Model().Groups, viewer) {
		out = append(out, suggestion{Key: filter.TagKey(viewer, name), Name: name, Kind: SuggestionTag, Managers: []Person{a.person(viewer)}})
	}
	for _, l := range Suggested(a.directory.Lists(viewer), a.cache.Model().Groups) {
		managers := []Person{a.person(viewer)}
		for _, host := range l.Hosts {
			if p, ok := a.directory.Person(host); ok && host != viewer {
				managers = append(managers, p)
			}
		}
		out = append(out, suggestion{Key: l.Key, Name: l.Name, Kind: l.Kind, Managers: managers})
	}
	return out
}

type options = filter.Options

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

func (a app) members(g Group) []Member {
	reasons := Reasons(g, a.sources())
	inside, outside := []Member{}, []Member{}
	for _, email := range filter.SortedKeys(func() map[string]bool {
		emails := map[string]bool{}
		for email := range reasons {
			emails[email] = true
		}
		return emails
	}()) {
		if p, ok := a.directory.Person(email); ok {
			inside = append(inside, Member{Person: p, Reasons: reasons[email]})
			continue
		}
		name := email
		if added := g.Addition(email); added != nil && added.Name != "" {
			name = added.Name
		}
		outside = append(outside, Member{Person: Person{Email: email, Name: name, Words: "Outside the directory"}, Reasons: reasons[email], Outside: true})
	}
	return append(inside, outside...)
}

func (a app) checkAdditions(additions []Addition) error {
	for _, added := range additions {
		if p, ok := a.directory.Person(a.directory.Resolve(added.Email)); ok {
			return fmt.Errorf("%s is in the directory as %s; add them with a rule", added.Email, p.Name)
		}
	}
	return nil
}

func (a app) sees(g Group, viewer string, admin bool) bool {
	return g.VisibleTo(viewer, admin, a.sources())
}

func (a app) view(g Group, viewer string, edit bool) groupView {
	v := groupView{Group: g, Address: g.Address(), Rules: []ruleView{}, Managers: a.people(g.Managers), Mine: g.Manages(viewer), Member: OnList(g, a.sources(), viewer), Unsubscribed: g.HasExcluded(viewer), Archived: a.cache.Model().Archived(g.Name, viewer), Sent: a.sentCount(g.Name)}
	v.Members = a.members(g)
	if !edit {
		v.Group.Rules = []Rule{}
		v.Group.Excluded = []Excluded{}
		for i := range v.Members {
			v.Members[i].Reasons = nil
		}
		return v
	}
	for _, r := range g.Rules {
		v.Rules = append(v.Rules, ruleView{Rule: r, TagLabels: a.sources().TagLabels(r, g.Managers, viewer)})
	}
	return v
}

func (a app) options(viewer string) options {
	return filter.OptionsFor(a.sources(), viewer)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	me := a.person(email)
	view := struct {
		User        user              `json:"user"`
		Domain      string            `json:"domain"`
		Groups      []groupView       `json:"groups"`
		Suggestions []suggestion      `json:"suggestions"`
		Options     options           `json:"options"`
		People      []Person          `json:"people"`
		Alerts      alerts            `json:"alerts"`
		GradeColors map[string]string `json:"gradeColors,omitempty"`
	}{
		User:        user{Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: admin, IsSuperAdmin: a.cache.IsSuperAdmin(email)},
		Domain:      Domain,
		Groups:      []groupView{},
		Suggestions: a.suggestions(email),
		Options:     a.options(email),
		People:      a.directory.People(),
		GradeColors: a.directory.GradeColors(),
	}
	for _, g := range a.cache.Model().Groups {
		if a.sees(g, email, admin) {
			view.Groups = append(view.Groups, a.view(g, email, admin || g.Manages(email)))
		}
	}
	view.Alerts.Stale, view.Alerts.Privacy = a.directory.Alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode groups model", "error", err)
	}
}

type draftBody struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	RuleWords []string   `json:"ruleWords"`
	Rules     []Rule     `json:"rules"`
	Additions []Addition `json:"additions"`
	Excluded  []Excluded `json:"excluded"`
}

func (a app) draftMembers(w http.ResponseWriter, r *http.Request, body draftBody) ([]Member, []int, bool) {
	email, admin := a.who(r)
	draft := Normalize(Group{Name: "preview", Title: "preview", Managers: []string{email}, Rules: body.Rules, Additions: body.Additions, Excluded: body.Excluded})
	var existing []Rule
	if g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(body.Name))); g != nil {
		if !admin && !g.Manages(email) {
			http.Error(w, "you do not manage this group", http.StatusForbidden)
			return nil, nil, false
		}
		existing, draft.Managers = g.Rules, g.Managers
	}
	if err := filter.Writable(a.sources(), email, draft.Managers, existing, draft.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}
	for i, rule := range draft.Rules {
		if err := CheckRule(rule); err != nil {
			http.Error(w, fmt.Sprintf("rule %d: %v", i+1, err), http.StatusBadRequest)
			return nil, nil, false
		}
	}
	if err := a.checkAdditions(draft.Additions); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}
	return a.members(draft), RuleCounts(draft, a.sources()), true
}

func (a app) preview(w http.ResponseWriter, r *http.Request) {
	var body draftBody
	if !decode(w, r, &body) {
		return
	}
	members, counts, ok := a.draftMembers(w, r, body)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"members": members, "ruleCounts": counts}); err != nil {
		slog.ErrorContext(r.Context(), "encode groups preview", "error", err)
	}
}

func (a app) describe(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	var body draftBody
	if !decode(w, r, &body) {
		return
	}
	if a.describer == nil {
		http.Error(w, "writing a description is not set up on this server", http.StatusServiceUnavailable)
		return
	}
	members, _, ok := a.draftMembers(w, r, body)
	if !ok {
		return
	}
	facts := describe.GroupFacts{Title: body.Title, Rules: body.RuleWords, Members: len(members), Roles: map[string]int{}, Grades: map[string]int{}, Classrooms: map[string]int{}}
	model := a.directory.Model()
	for _, m := range members {
		p := model.Person(m.Email)
		if p == nil {
			facts.Roles["Guest"]++
			continue
		}
		switch {
		case p.IsStudent:
			facts.Roles["Student"]++
			if p.Grade != "" {
				facts.Grades[p.Grade]++
			}
			if p.Classroom != "" {
				facts.Classrooms[p.Classroom]++
			}
		case p.IsStaff:
			facts.Roles["Staff"]++
		case p.IsParent:
			facts.Roles["Parent"]++
		}
	}
	description, err := a.describer.Group(r.Context(), email, facts)
	if errors.Is(err, claude.ErrTooMany) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, describe.ErrTooLong) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "groups: describe", "actor", email, "title", body.Title, "error", err)
		http.Error(w, "could not write a description right now", http.StatusBadGateway)
		return
	}
	slog.InfoContext(r.Context(), "groups: described", "actor", email, "title", body.Title, "members", len(members))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"description": description}); err != nil {
		slog.ErrorContext(r.Context(), "encode groups description", "error", err)
	}
}

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

func (a app) logChange(real, actor, action, group, detail string) error {
	return a.writer.Append(appName, changeLogTab, []string{time.Now().Format(time.RFC3339), actor, action, group, detail, real})
}

func rowOf(columns []string, cells map[string]string) []string {
	row := make([]string, len(columns))
	for i, column := range columns {
		row[i] = cells[column]
	}
	return row
}

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
	for _, local := range g.Names() {
		if other := a.cache.Model().Resolve(local); other != nil && other.Name != original {
			http.Error(w, fmt.Sprintf("%s@%s is taken", local, Domain), http.StatusBadRequest)
			return
		}
	}
	if original == "" {
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
	if err := filter.Writable(a.sources(), email, g.Managers, existing, g.Rules); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := CheckGroup(g); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.checkAdditions(g.Additions); err != nil {
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
		for _, tab := range []string{managersTab, rulesTab, additionsTab, excludedTab, aliasesTab} {
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
		additions := [][]string{}
		for _, added := range g.Additions {
			additions = append(additions, rowOf(AdditionColumns, additionCells(g.Name, added)))
		}
		if err := a.writer.AppendAll(appName, additionsTab, additions); err != nil {
			return err
		}
		excluded := [][]string{}
		for _, e := range g.Excluded {
			excluded = append(excluded, rowOf(ExcludedColumns, excludedCells(g.Name, e)))
		}
		if err := a.writer.AppendAll(appName, excludedTab, excluded); err != nil {
			return err
		}
		aliases := [][]string{}
		for _, alias := range g.Aliases {
			aliases = append(aliases, rowOf(AliasColumns, aliasCells(g.Name, alias)))
		}
		if err := a.writer.AppendAll(appName, aliasesTab, aliases); err != nil {
			return err
		}
		return a.logChange(auth.RealEmail(r), email, action, g.Name, fmt.Sprintf("%s; %d aliases; %d managers; %d rules; %d added by hand; %d excluded; prefix %v; visibility %s; posting %s; replying %s", g.Title, len(g.Aliases), len(g.Managers), len(g.Rules), len(g.Additions), len(g.Excluded), g.Prefix, g.Visibility, g.Posting, g.Replying))
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: saved group", "action", action, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.view(g, email, true)); err != nil {
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
		for _, tab := range []string{managersTab, rulesTab, additionsTab, excludedTab, aliasesTab, archivedTab, messagesTab, deliveriesTab} {
			if err := a.writer.Delete(appName, tab, map[string]string{"Group": name}); err != nil {
				return err
			}
		}
		if err := a.mail.Documents.Remove(name); err != nil {
			return err
		}
		return a.logChange(auth.RealEmail(r), email, "delete", name, current.Title)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: deleted group", "group", name)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) subscription(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	var body struct {
		Name       string `json:"name"`
		Subscribed bool   `json:"subscribed"`
	}
	if !decode(w, r, &body) {
		return
	}
	g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(body.Name)))
	if g == nil || !a.sees(*g, email, admin) {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	if !OnList(*g, a.sources(), email) {
		http.Error(w, "you are not on this group", http.StatusForbidden)
		return
	}
	var err error
	if body.Subscribed {
		err = a.resubscribeAddress(r.Context(), auth.RealEmail(r), g, email)
	} else {
		err = a.unsubscribeAddress(r.Context(), auth.RealEmail(r), g, email, loopPage)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.view(*a.cache.Model().Group(g.Name), email, admin || g.Manages(email))); err != nil {
		slog.ErrorContext(r.Context(), "encode subscription", "error", err)
	}
}

func (a app) archive(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	var body struct {
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}
	if !decode(w, r, &body) {
		return
	}
	g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(body.Name)))
	if g == nil || !a.sees(*g, email, admin) {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	tables := a.cache.Tables().withArchived(g.Name, email, body.Archived)
	if !a.commit(r, w, tables, func() error {
		if body.Archived {
			return a.writer.Append(appName, archivedTab, []string{g.Name, email})
		}
		return a.writer.Delete(appName, archivedTab, map[string]string{"Group": g.Name, "Email": email})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "groups: archived", "group", g.Name, "email", email, "archived", body.Archived)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.view(*a.cache.Model().Group(g.Name), email, admin || g.Manages(email))); err != nil {
		slog.ErrorContext(r.Context(), "encode archive", "error", err)
	}
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email        string   `json:"email"`
		Admins       []string `json:"admins"`
		IsSuperAdmin bool     `json:"isSuperAdmin"`
	}{Email: email, Admins: a.cache.Admins(a.superAdmins()), IsSuperAdmin: a.cache.IsSuperAdmin(email)}
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
