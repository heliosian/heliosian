package who

import (
	"net/http"
	"strings"

	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var sections = []string{"people", "classrooms", "staff", "map", "email-list", "greenvelope", "my-privacy", "admin"}

var legacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
}

type app struct {
	cache *model.DirectoryCache
}

func OptInForm(optIn func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, optIn(), http.StatusFound)
	})
}

func Register(mux *http.ServeMux, cache *model.DirectoryCache, about *sharecard.About) {
	a := app{cache: cache}
	mux.Handle("GET /open/share/about.png", about)
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, a.page)
	}
	mux.HandleFunc("GET /my-family", a.myFamily)
	mux.HandleFunc("GET /people/{email}", a.page)
	mux.HandleFunc("GET /families/{key}", a.page)
	mux.HandleFunc("GET /classrooms/{name}", a.page)
	mux.HandleFunc("GET /grades/{name}", a.page)
	mux.HandleFunc("GET /dl/", a.legacyRedirect)
}

func (a app) myFamily(w http.ResponseWriter, r *http.Request) {
	email := a.cache.Actor(r, a.cache.Held).Email
	if family, ok := a.cache.Model().FamilyOf(email); ok {
		http.Redirect(w, r, model.FamilyPath(family.Key), http.StatusFound)
		return
	}
	http.Error(w, "no family record for "+email, http.StatusNotFound)
}

func (a app) legacyRedirect(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/dl/"), "/")
	target, ok := legacy[first]
	if !ok {
		target = "/people"
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/who/index.html")
}
