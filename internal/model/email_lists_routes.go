package model

import (
	"errors"
	"log/slog"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const emailListsShell = "web/loop/index.html"

var emailListsPages = []string{"/{$}", "/new", "/groups/{name}", "/admin"}

type emailListsApp struct {
	cache      *EmailListsCache
	directory  func() *Directory
	parties    *PartiesCache
	activities *ActivitiesCache
	media      *blob.Store
	mail       ListMail
	mailer     *mailer
	describer  *describe.Describer
}

type EmailListsDeps struct {
	Cache      *EmailListsCache
	Directory  func() *Directory
	Parties    *PartiesCache
	Activities *ActivitiesCache
	Media      *blob.Store
	Mail       ListMail
	Describer  *describe.Describer
	About      *sharecard.About
}

func RegisterEmailLists(mux *http.ServeMux, d EmailListsDeps) {
	a := emailListsApp{cache: d.Cache, directory: d.Directory, parties: d.Parties, activities: d.Activities, media: d.Media, mail: d.Mail, describer: d.Describer}
	a.mailer = newMailer(d.Cache, a.sources, d.Mail)
	for _, page := range emailListsPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("POST /api/loop/preview", serve.JSON(a.preview))
	mux.HandleFunc("POST /api/loop/describe", serve.JSON(a.describe))
	RegisterAdmins(mux, a.cache.AdminList, a.actor, func(*http.Request, access.Actor) map[string]any { return map[string]any{} })
	mux.HandleFunc("POST /hooks/mail/mime", a.inbound)
	mux.HandleFunc("POST /hooks/events", a.events)
	mux.Handle("GET /open/share/about.png", d.About)
	mux.HandleFunc("GET /open/unsubscribe/{token}", a.unsubscribePage)
	mux.HandleFunc("POST /open/unsubscribe/{token}", a.unsubscribe)
	a.mailer.recover()
}

func (a emailListsApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, emailListsShell)
}

func (a emailListsApp) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

func (a emailListsApp) sources() AudienceSources {
	return EmailListAudience(a.directory(), a.parties.Model(), a.activities.Model(), a.activities, now())
}

func Suggested(lists []MagicTag, groups []EmailList) []MagicTag {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []MagicTag{}
	for _, l := range lists {
		if (l.Kind == MagicTagParty || l.Kind == MagicTagActivity) && !l.Archived && !named[l.Key] {
			out = append(out, l)
		}
	}
	return out
}

func SuggestedTags(tags []Tag, groups []EmailList) []Tag {
	named := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Rules {
			for _, tag := range r.Tags {
				named[tag] = true
			}
		}
	}
	out := []Tag{}
	for _, t := range tags {
		if !named[TagKey(t.ID)] && len(t.People) > 0 {
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

func (a emailListsApp) draftMembers(r *http.Request, body draftBody) ([]previewMember, []int, error) {
	actor := a.actor(r)
	email := actor.Email
	draft := NormalizeList(EmailList{Name: "preview", Title: "preview", Managers: []string{email}, Rules: body.Rules, Additions: body.Additions, Excluded: body.Excluded})
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
		if err := CheckListRule(rule); err != nil {
			return nil, nil, access.Invalid("rule %d: %v", i+1, err)
		}
	}
	if err := checkAdditions(sources.Directory, draft.Additions); err != nil {
		return nil, nil, err
	}
	reasons := draft.Reasons(sources)
	inside, outside := []previewMember{}, []previewMember{}
	for _, address := range SortedKeys(func() map[string]bool {
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
	return append(inside, outside...), draft.RuleCounts(sources), nil
}

func (a emailListsApp) preview(r *http.Request, body draftBody) (map[string]any, error) {
	members, counts, err := a.draftMembers(r, body)
	if err != nil {
		return nil, err
	}
	return map[string]any{"members": members, "ruleCounts": counts}, nil
}

func (a emailListsApp) describe(r *http.Request, body draftBody) (map[string]string, error) {
	email := a.actor(r).Email
	members, _, err := a.draftMembers(r, body)
	if err != nil {
		return nil, err
	}
	facts := describe.GroupFacts{Title: body.Title, Rules: body.RuleWords, Members: len(members), Roles: map[string]int{}, Grades: map[string]int{}, Classrooms: map[string]int{}}
	d := a.sources().Directory
	for _, m := range members {
		p := d.Person(m.Email)
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
