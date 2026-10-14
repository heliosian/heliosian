package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/db"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var directorySections = []string{"classrooms", "staff", "map", "email-list", "greenvelope", "my-privacy", "my-family", "admin"}

var directoryLegacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
}

func spoofing(s *db.Store) *auth.Spoof {
	return &auth.Spoof{
		Allowed: func(email string) bool { return s.Model().SuperAdmin(email) },
		Person: func(email string) (auth.Person, bool) {
			address, name, ok := s.Model().SignedInAs(email)
			return auth.Person{Email: address, Name: name}, ok
		},
	}
}

func optInForm(optIn func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, optIn(), http.StatusFound)
	})
}

func registerDirectoryPages(mux *http.ServeMux, about *sharecard.About, mapsKey string) {
	mux.Handle("GET /open/share/about.png", about)
	mux.HandleFunc("GET /maps.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		fmt.Fprintf(w, "export const mapsKey = %s;\n", strconv.Quote(mapsKey))
	})
	for _, section := range directorySections {
		mux.HandleFunc("GET /"+section, directoryPage)
	}
	mux.HandleFunc("GET /people", directoryPeople)
	mux.HandleFunc("GET /groups/{key}", directoryPage)
	mux.HandleFunc("GET /people/{id}", directoryPage)
	mux.HandleFunc("GET /families/{id}", directoryPage)
	mux.HandleFunc("GET /classrooms/{slug}", directoryPage)
	mux.HandleFunc("GET /grades/{slug}", directoryPage)
	mux.HandleFunc("GET /dl/", directoryLegacyRedirect)
}

func directoryLegacyRedirect(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/dl/"), "/")
	target, ok := directoryLegacy[first]
	if !ok {
		target = "/people"
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func directoryPeople(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("tag")
	if list := r.URL.Query().Get("list"); list != "" {
		_, key, _ = strings.Cut(list, ":")
		if key == "" {
			key = list
		}
	}
	if key == "" {
		directoryPage(w, r)
		return
	}
	http.Redirect(w, r, "/groups/"+url.PathEscape(key), http.StatusMovedPermanently)
}

func directoryPage(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/who/index.html")
}
