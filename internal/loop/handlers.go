package loop

import (
	"errors"
	"log/slog"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const shell = "web/loop/index.html"

var pages = []string{"/{$}", "/new", "/groups/{name}", "/admin"}

type app struct {
	cache     *Cache
	media     *blob.Store
	sources   func() Sources
	mail      Mail
	mailer    *mailer
	describer *describe.Describer
}

type Deps struct {
	Cache     *Cache
	Media     *blob.Store
	Sources   func() Sources
	Mail      Mail
	Describer *describe.Describer
	About     *sharecard.About
}

func Register(mux *http.ServeMux, d Deps) {
	a := app{cache: d.Cache, media: d.Media, sources: d.Sources, mail: d.Mail, describer: d.Describer}
	a.mailer = newMailer(d.Cache, d.Sources, d.Mail)
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("POST /api/loop/preview", serve.JSON(a.preview))
	mux.HandleFunc("POST /api/loop/describe", serve.JSON(a.describe))
	model.RegisterAdmins(mux, a.cache.AdminList, a.actor, func(*http.Request, access.Actor) map[string]any { return map[string]any{} })
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

func Suggested(lists []model.MagicTag, groups []Group) []model.MagicTag {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []model.MagicTag{}
	for _, l := range lists {
		if (l.Kind == model.MagicTagParty || l.Kind == model.MagicTagActivity) && !named[l.Key] {
			out = append(out, l)
		}
	}
	return out
}

func SuggestedTags(tags []model.Tag, groups []Group) []model.Tag {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []model.Tag{}
	for _, t := range tags {
		if !named[model.TagKey(t.ID)] && len(t.People) > 0 {
			out = append(out, t)
		}
	}
	return out
}

const SuggestionTag = "tag"

type draftBody struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	RuleWords []string   `json:"ruleWords"`
	Rules     []Rule     `json:"rules"`
	Additions []Addition `json:"additions"`
	Excluded  []Excluded `json:"excluded"`
}

type previewMember struct {
	Email   string   `json:"email"`
	Person  string   `json:"person,omitempty"`
	Name    string   `json:"name,omitempty"`
	Outside bool     `json:"outside,omitempty"`
	Reasons []Reason `json:"reasons"`
}

func (a app) draftMembers(r *http.Request, body draftBody) ([]previewMember, []int, error) {
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
	sources := a.sources()
	if err := sources.Writable(email, draft.Managers, existing, draft.Rules); err != nil {
		return nil, nil, access.Invalid("%v", err)
	}
	for i, rule := range draft.Rules {
		if err := CheckRule(rule); err != nil {
			return nil, nil, access.Invalid("rule %d: %v", i+1, err)
		}
	}
	if err := checkAdditions(sources.Directory, draft.Additions); err != nil {
		return nil, nil, err
	}
	reasons := Reasons(draft, sources)
	inside, outside := []previewMember{}, []previewMember{}
	for _, address := range model.SortedKeys(func() map[string]bool {
		emails := map[string]bool{}
		for address := range reasons {
			emails[address] = true
		}
		return emails
	}()) {
		if p := sources.Directory.Person(address); p != nil {
			inside = append(inside, previewMember{Email: address, Person: p.ID, Reasons: reasons[address]})
			continue
		}
		name := address
		if added := draft.Addition(address); added != nil && added.Name != "" {
			name = added.Name
		}
		outside = append(outside, previewMember{Email: address, Name: name, Outside: true, Reasons: reasons[address]})
	}
	return append(inside, outside...), RuleCounts(draft, sources), nil
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
