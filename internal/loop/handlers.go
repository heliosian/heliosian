package loop

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/config"
	"heliosian/internal/describe"
	"heliosian/internal/filter"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
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

func personView(model *who.Model, p *who.Person) Person {
	out := Person{Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Words: p.Words()}
	switch {
	case p.IsStaff:
		out.Role, out.Context = "Staff", p.Words()
	case p.IsStudent:
		out.Role, out.Grade, out.Context = "Student", p.Grade, p.Words()
	case p.IsParent:
		out.Role = "Parent"
		kids := []string{}
		_, children := model.Household(p.Email)
		for _, k := range children {
			if k.Grade != "" {
				kids = append(kids, k.FullName+" ("+k.Grade+")")
			} else {
				kids = append(kids, k.FullName)
			}
		}
		out.Context = "Parent"
		if len(kids) > 0 {
			out.Context = "Parent to " + strings.Join(kids, ", ")
		}
	}
	return out
}

func lookup(model *who.Model, email string) (Person, bool) {
	p := model.Person(email)
	if p == nil {
		return Person{}, false
	}
	return personView(model, p), true
}

type app struct {
	cache     *Cache
	media     *blob.Store
	sources   func() Sources
	settings  func() *config.Settings
	mail      Mail
	mailer    *mailer
	describer *describe.Describer
}

type Deps struct {
	Cache     *Cache
	Media     *blob.Store
	Sources   func() Sources
	Settings  func() *config.Settings
	Mail      Mail
	Describer *describe.Describer
	About     *sharecard.About
}

func Register(mux *http.ServeMux, d Deps) {
	a := app{cache: d.Cache, media: d.Media, sources: d.Sources, settings: d.Settings, mail: d.Mail, describer: d.Describer}
	a.mailer = newMailer(d.Cache, d.Sources, d.Mail)
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/loop/model", serve.JSON(a.model))
	mux.HandleFunc("POST /api/loop/preview", serve.JSON(a.preview))
	mux.HandleFunc("POST /api/loop/describe", serve.JSON(a.describe))
	mux.HandleFunc("POST /api/loop/group", serve.JSON(a.saveGroup))
	mux.HandleFunc("DELETE /api/loop/group", serve.JSON(a.deleteGroup))
	mux.HandleFunc("GET /api/loop/messages", serve.JSON(a.messages))
	mux.HandleFunc("POST /api/loop/subscription", serve.JSON(a.subscription))
	mux.HandleFunc("POST /api/loop/archive", serve.JSON(a.archive))
	admins.Register(mux, a.cache.List, a.actor, func(*http.Request, access.Actor) map[string]any { return map[string]any{} })
	mux.HandleFunc("POST /hooks/mail/mime", a.inbound)
	mux.HandleFunc("POST /hooks/events", a.events)
	mux.Handle("GET /open/share/about.png", d.About)
	mux.HandleFunc("GET /open/unsubscribe/{token}", a.unsubscribePage)
	mux.HandleFunc("POST /open/unsubscribe/{token}", a.unsubscribe)
	a.mailer.recover()
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	return a.sources().Directory.Actor(r, a.cache.Held)
}

type user struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
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

func SuggestedTags(tags []who.Tag, groups []Group) []who.Tag {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []who.Tag{}
	for _, t := range tags {
		if !named[filter.TagKey(t.ID)] && len(t.People) > 0 {
			out = append(out, t)
		}
	}
	return out
}

const SuggestionTag = "tag"

func (a app) suggestions(viewer string) []suggestion {
	out := []suggestion{}
	sources := a.sources()
	for _, t := range SuggestedTags(sources.Tags(viewer), a.cache.Model().Groups) {
		out = append(out, suggestion{Key: filter.TagKey(t.ID), Name: t.Name, Kind: SuggestionTag, Managers: []Person{a.person(viewer)}})
	}
	for _, l := range Suggested(sources.Lists(viewer), a.cache.Model().Groups) {
		managers := []Person{a.person(viewer)}
		for _, host := range l.Hosts {
			if p, ok := lookup(sources.Directory, host); ok && host != viewer {
				managers = append(managers, p)
			}
		}
		out = append(out, suggestion{Key: l.Key, Name: l.Name, Kind: l.Kind, Managers: managers})
	}
	return out
}

type options = filter.Options

func (a app) person(email string) Person {
	if p, ok := lookup(a.sources().Directory, email); ok {
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
	sources := a.sources()
	reasons := Reasons(g, sources)
	inside, outside := []Member{}, []Member{}
	for _, email := range filter.SortedKeys(func() map[string]bool {
		emails := map[string]bool{}
		for email := range reasons {
			emails[email] = true
		}
		return emails
	}()) {
		if p, ok := lookup(sources.Directory, email); ok {
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
	v := groupView{Group: *shown, Address: g.Address(), Rules: []ruleView{}, Managers: a.people(g.Managers), Mine: g.Manages(viewer), Member: OnList(g, a.sources(), viewer), Open: g.VisibleTo(access.Actor{Email: viewer}, a.sources()), Unsubscribed: g.HasExcluded(viewer), Archived: a.cache.Model().Archived(g.ID, viewer), Sent: a.sentCount(g.ID)}
	v.Members = a.members(g)
	if !g.Sees(as) {
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

func (a app) shown(groupID string, as access.Actor) *groupView {
	v, ok := a.view(*a.cache.Model().Group(groupID), as)
	if !ok {
		return nil
	}
	return &v
}

func (a app) options(viewer string) options {
	return filter.OptionsFor(a.sources(), viewer)
}

type modelView struct {
	User        user              `json:"user"`
	Domain      string            `json:"domain"`
	Groups      []groupView       `json:"groups"`
	Suggestions []suggestion      `json:"suggestions"`
	Options     options           `json:"options"`
	GradeColors map[string]string `json:"gradeColors,omitempty"`
}

func (a app) model(r *http.Request, _ serve.None) (modelView, error) {
	actor := a.actor(r)
	email := actor.Email
	me := a.person(email)
	view := modelView{
		User:        user{Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: actor.May(SeeAll)},
		Domain:      Domain,
		Groups:      []groupView{},
		Suggestions: a.suggestions(email),
		Options:     a.options(email),
		GradeColors: a.settings().GradeColors,
	}
	for _, g := range a.cache.Model().Groups {
		if v, ok := a.view(g, actor); ok {
			view.Groups = append(view.Groups, v)
		}
	}
	return view, nil
}

type draftBody struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	RuleWords []string   `json:"ruleWords"`
	Rules     []Rule     `json:"rules"`
	Additions []Addition `json:"additions"`
	Excluded  []Excluded `json:"excluded"`
}

func (a app) draftMembers(r *http.Request, body draftBody) ([]Member, []int, error) {
	actor := a.actor(r)
	email := actor.Email
	draft := Normalize(Group{Name: "preview", Title: "preview", Managers: []string{email}, Rules: body.Rules, Additions: body.Additions, Excluded: body.Excluded})
	var existing []Rule
	if g := a.cache.Model().Group(body.ID); g != nil {
		if !g.Edits(actor) {
			return nil, nil, access.Forbidden("you do not manage this email list")
		}
		existing, draft.Managers = g.Rules, g.Managers
	}
	if err := filter.Writable(a.sources(), email, draft.Managers, existing, draft.Rules); err != nil {
		return nil, nil, access.Invalid("%v", err)
	}
	for i, rule := range draft.Rules {
		if err := CheckRule(rule); err != nil {
			return nil, nil, access.Invalid("rule %d: %v", i+1, err)
		}
	}
	if err := checkAdditions(a.sources().Directory, draft.Additions); err != nil {
		return nil, nil, err
	}
	return a.members(draft), RuleCounts(draft, a.sources()), nil
}

func (a app) preview(r *http.Request, body draftBody) (map[string]any, error) {
	members, counts, err := a.draftMembers(r, body)
	if err != nil {
		return nil, err
	}
	return map[string]any{"members": members, "ruleCounts": counts}, nil
}

func (a app) describe(r *http.Request, body draftBody) (map[string]string, error) {
	email := a.actor(r).Email
	members, _, err := a.draftMembers(r, body)
	if err != nil {
		return nil, err
	}
	facts := describe.GroupFacts{Title: body.Title, Rules: body.RuleWords, Members: len(members), Roles: map[string]int{}, Grades: map[string]int{}, Classrooms: map[string]int{}}
	model := a.sources().Directory
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
		return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
	}
	if errors.Is(err, describe.ErrTooLong) {
		return nil, access.Invalid("%v", err)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "loop:describe", "actor", email, "title", body.Title, "error", err)
		return nil, access.Refuse(http.StatusBadGateway, "could not write a description right now")
	}
	slog.InfoContext(r.Context(), "loop:described", "actor", email, "title", body.Title, "members", len(members))
	return map[string]string{"description": description}, nil
}

func (a app) saveGroup(r *http.Request, body Group) (*groupView, error) {
	actor := a.actor(r)
	ops, g, action, err := a.cache.Model().SaveGroup(actor, a.sources(), body)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "loop:saved group", "action", action, "id", g.ID, "group", g.Name, "aliases", len(g.Aliases), "rules", len(g.Rules), "managers", len(g.Managers), "additions", len(g.Additions), "excluded", len(g.Excluded), "prefix", g.Prefix, "visibility", g.Visibility, "posting", g.Posting, "replying", g.Replying)
	return a.shown(g.ID, actor), nil
}

type groupRef struct {
	ID string `json:"id"`
}

func (a app) deleteGroup(r *http.Request, body groupRef) (serve.None, error) {
	actor := a.actor(r)
	ops, g, err := a.cache.Model().DeleteGroup(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	if err := a.mail.Documents.Remove(r.Context(), actor, g.Name); err != nil {
		slog.ErrorContext(r.Context(), "loop:filed mail not removed", "group", g.Name, "error", err)
	}
	slog.InfoContext(r.Context(), "loop:deleted group", "id", g.ID, "group", g.Name)
	return serve.None{}, nil
}

type subscriptionBody struct {
	ID         string `json:"id"`
	Subscribed bool   `json:"subscribed"`
}

func (a app) subscription(r *http.Request, body subscriptionBody) (*groupView, error) {
	actor := a.actor(r)
	ops, g, err := a.cache.Model().SetSubscription(actor, a.sources(), body.ID, body.Subscribed)
	if err != nil {
		return nil, err
	}
	how := ""
	if !body.Subscribed {
		how = loopPage
	}
	if err := a.commitSubscription(r.Context(), actor, *g, how, ops); err != nil {
		return nil, err
	}
	return a.shown(g.ID, actor), nil
}

type archiveBody struct {
	ID       string `json:"id"`
	Archived bool   `json:"archived"`
}

func (a app) archive(r *http.Request, body archiveBody) (*groupView, error) {
	actor := a.actor(r)
	ops, g, err := a.cache.Model().SetArchived(actor, a.sources(), body.ID, body.Archived)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "loop:archived", "id", g.ID, "group", g.Name, "email", actor.Email, "archived", body.Archived)
	return a.shown(g.ID, actor), nil
}
