package model

import (
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const emailListsShell = "web/loop/index.html"

var emailListsPages = []string{"/{$}", "/new", "/groups/{name}", "/admin"}

type emailListsApp struct {
	store *Store
	media *blob.Store
}

type EmailListsDeps struct {
	Store *Store
	Media *blob.Store
	About *sharecard.About
}

func RegisterEmailLists(mux *http.ServeMux, d EmailListsDeps) {
	a := emailListsApp{store: d.Store, media: d.Media}
	for _, page := range emailListsPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.Handle("GET /open/share/about.png", d.About)
}

func (a emailListsApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, emailListsShell)
}

func (a emailListsApp) actor(r *http.Request) access.Actor {
	return a.store.Model().actor(r, "loop")
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
