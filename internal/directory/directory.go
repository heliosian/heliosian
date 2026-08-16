// Package directory serves the school directory app.
package directory

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"heliosian/internal/auth"
)

var sections = []string{"people", "classrooms", "my-family", "staff", "map", "email-list", "data-view", "bug-report", "about"}

var legacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
	"33234e":   "/data-view",
	"ee614d":   "/bug-report",
	"255ce0":   "/about",
}

type app struct {
	cache *Cache
}

func Register(mux *http.ServeMux, cache *Cache) {
	a := app{cache: cache}
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, a.page)
	}
	mux.HandleFunc("GET /people/{email}", a.page)
	mux.HandleFunc("GET /families/{key}", a.page)
	mux.HandleFunc("GET /dl/", a.legacyRedirect)
	mux.HandleFunc("GET /api/directory/model", a.model)
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
	t, err := template.ParseFiles("web/directory/index.html")
	if err != nil {
		serverError(w, err)
		return
	}
	name := a.cache.Model().DisplayName(auth.Email(r))
	data := map[string]string{
		"UserName":    name,
		"UserInitial": strings.ToUpper(name[:1]),
	}
	if err := t.Execute(w, data); err != nil {
		log.Printf("[ERROR] render directory page: %v", err)
	}
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.cache.Model()); err != nil {
		log.Printf("[ERROR] encode model: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
