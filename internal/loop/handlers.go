package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
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
	Alerts(email string) ([]string, []string)
	GradeColors() map[string]string
}

type app struct {
	cache       *Cache
	media       *blob.Store
	directory   Directory
	superAdmins func() []string
	mail        Mail
	mailer      *mailer
	describer   Describer
}

type Describer interface {
	Group(ctx context.Context, actor string, facts describe.GroupFacts) (string, error)
}

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, directory Directory, superAdmins func() []string, mailbox Mail, describer Describer, about *sharecard.About) {
	a := app{cache: cache, media: media, directory: directory, superAdmins: superAdmins, mail: mailbox, describer: describer}
	a.mailer = newMailer(cache, directory, mailbox)
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
	mux.Handle("GET /open/share/about.png", about)
	mux.HandleFunc("GET /open/unsubscribe/{token}", a.unsubscribePage)
	mux.HandleFunc("POST /open/unsubscribe/{token}", a.unsubscribe)
	a.mailer.recover()
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email)}
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
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
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
	Open         bool       `json:"open"`
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
	out := []Person{}
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

func (a app) view(g Group, as access.Actor) (groupView, bool) {
	shown := g.For(as, a.sources())
	if shown == nil {
		return groupView{}, false
	}
	viewer := as.Email
	v := groupView{Group: *shown, Address: g.Address(), Rules: []ruleView{}, Managers: a.people(g.Managers), Mine: g.Manages(viewer), Member: OnList(g, a.sources(), viewer), Open: g.VisibleTo(access.Actor{Email: viewer}, a.sources()), Unsubscribed: g.HasExcluded(viewer), Archived: a.cache.Model().Archived(g.Name, viewer), Sent: a.sentCount(g.Name)}
	v.Members = a.members(g)
	if !g.Edits(as) {
		for i := range v.Members {
			v.Members[i].Reasons = nil
		}
		return v, true
	}
	for _, r := range g.Rules {
		v.Rules = append(v.Rules, ruleView{Rule: r, TagLabels: a.sources().TagLabels(r, g.Managers, viewer)})
	}
	return v, true
}

func (a app) sendView(w http.ResponseWriter, r *http.Request, g Group, as access.Actor) {
	v, ok := a.view(g, as)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.ErrorContext(r.Context(), "encode group", "error", err)
	}
}

func (a app) options(viewer string) options {
	return filter.OptionsFor(a.sources(), viewer)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	email := actor.Email
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
		User:        user{Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: actor.Admin, IsSuperAdmin: a.cache.IsSuperAdmin(email)},
		Domain:      Domain,
		Groups:      []groupView{},
		Suggestions: a.suggestions(email),
		Options:     a.options(email),
		People:      a.directory.People(),
		GradeColors: a.directory.GradeColors(),
	}
	for _, g := range a.cache.Model().Groups {
		if v, ok := a.view(g, actor); ok {
			view.Groups = append(view.Groups, v)
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
	actor := a.actor(r)
	email := actor.Email
	draft := Normalize(Group{Name: "preview", Title: "preview", Managers: []string{email}, Rules: body.Rules, Additions: body.Additions, Excluded: body.Excluded})
	var existing []Rule
	if g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(body.Name))); g != nil {
		if !g.Edits(actor) {
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
	if err := checkAdditions(a.directory, draft.Additions); err != nil {
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
	email := a.actor(r).Email
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
		slog.ErrorContext(r.Context(), "loop:describe", "actor", email, "title", body.Title, "error", err)
		http.Error(w, "could not write a description right now", http.StatusBadGateway)
		return
	}
	slog.InfoContext(r.Context(), "loop:described", "actor", email, "title", body.Title, "members", len(members))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"description": description}); err != nil {
		slog.ErrorContext(r.Context(), "encode groups description", "error", err)
	}
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return false
	}
	return true
}

func (a app) saveGroup(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Original string `json:"original"`
		Group
	}
	if !decode(w, r, &body) {
		return
	}
	ops, g, action, err := a.cache.Model().SaveGroup(actor, a.directory, body.Original, body.Group)
	if err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "loop:saved group", "action", action, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
	a.sendView(w, r, *a.cache.Model().Group(g.Name), actor)
}

func (a app) deleteGroup(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, name, err := a.cache.Model().DeleteGroup(actor, body.Name)
	if err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	if err := a.mail.Documents.Remove(r.Context(), actor, name); err != nil {
		slog.ErrorContext(r.Context(), "loop:filed mail not removed", "group", name, "error", err)
	}
	slog.InfoContext(r.Context(), "loop:deleted group", "group", name)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) subscription(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Name       string `json:"name"`
		Subscribed bool   `json:"subscribed"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, g, err := a.cache.Model().SetSubscription(actor, a.sources(), body.Name, body.Subscribed)
	if err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	how := ""
	if !body.Subscribed {
		how = loopPage
	}
	if err := a.commitSubscription(r.Context(), actor, *g, how, ops); err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	a.sendView(w, r, *a.cache.Model().Group(g.Name), actor)
}

func (a app) archive(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, g, err := a.cache.Model().SetArchived(actor, a.sources(), body.Name, body.Archived)
	if err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "loop:archived", "group", g.Name, "email", actor.Email, "archived", body.Archived)
	a.sendView(w, r, *a.cache.Model().Group(g.Name), actor)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	if !actor.Admin {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	view := struct {
		Email        string   `json:"email"`
		Admins       []string `json:"admins"`
		IsSuperAdmin bool     `json:"isSuperAdmin"`
	}{Email: actor.Email, Admins: a.cache.Admins(a.superAdmins()), IsSuperAdmin: a.cache.IsSuperAdmin(actor.Email)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode groups admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, admins, err := a.cache.Model().SetAdmins(actor, a.superAdmins(), body.Admins)
	if err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "loop:set the admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}
