package model

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const homeShell = "web/home/index.html"

type homeApp struct {
	store  *Store
	search imagesearch.Search
	style  *sharecard.Style
}

type HomeDeps struct {
	Store  *Store
	Images blob.Images
	Search imagesearch.Search
	Style  *sharecard.Style
}

type HomeHooks struct {
	app homeApp
}

func RegisterHome(mux *http.ServeMux, d HomeDeps) HomeHooks {
	d.Search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	a := homeApp{store: d.Store, search: d.Search, style: d.Style}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", a.shareApps)
	mux.HandleFunc("POST /api/apps/audience/preview", serve.JSON(a.audiencePreview))
	a.search.Register(mux, "/api/apps", d.Images.Folder(), a.requireAdminFunc)
	a.discover()
	return HomeHooks{app: a}
}

func (a homeApp) discover() {
	actor := access.System("app discovery")
	ops, found := a.store.Model().Home.discover()
	for _, app := range found {
		slog.Info("home: found a new app, listed for nobody yet", "app", app.Key)
	}
	if err := a.store.Commit(context.Background(), actor, homeAppName, ops...); err != nil {
		slog.Error("home: discover apps and widgets", "error", err)
	}
}

func homeAppWithMark() App {
	app := HomeApp
	app.Mark = markVersion(app.Key)
	return app
}

func (a homeApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, homeShell)
}

func (a homeApp) actor(r *http.Request) access.Actor {
	return a.store.Model().actor(r, "home")
}

func (a homeApp) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireHomeAdmin(a.actor(r)); err != nil {
			serve.Error(w, r, err)
			return
		}
		next(w, r)
	}
}

func rulesOf(m *Home, key string) []Rule {
	for _, c := range m.Categories {
		if thingCategory+c.ID == key {
			return c.Rules
		}
		for _, l := range c.Links {
			if thingLink+l.ID == key {
				return l.Rules
			}
		}
	}
	if name, ok := strings.CutPrefix(key, thingWidget); ok {
		return m.WidgetRules[name]
	}
	return m.Visibility[strings.TrimPrefix(key, thingApp)].Rules
}

type previewBody struct {
	Thing string `json:"thing"`
	Rules []Rule `json:"rules"`
}

type previewView struct {
	Count      int      `json:"count"`
	Names      []string `json:"names"`
	RuleCounts []int    `json:"ruleCounts"`
}

func (a homeApp) audiencePreview(r *http.Request, body previewBody) (previewView, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	if err := requireHomeAdmin(actor); err != nil {
		return previewView{}, err
	}
	rules, err := m.checkHomeRules(rulesOf(m.Home, body.Thing), body.Rules, actor.Email)
	if err != nil {
		return previewView{}, err
	}
	sources := m.Audience(now())
	list := Audience{Rules: rules, Editors: m.AdminList("home").Admins()}
	members := list.Members(sources)
	names := []string{}
	for _, member := range members {
		names = append(names, sources.Directory.DisplayName(member))
	}
	slices.Sort(names)
	return previewView{Count: len(members), Names: names[:min(len(names), 12)], RuleCounts: list.RuleCounts(sources)}, nil
}
