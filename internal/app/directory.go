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

const directoryShell = "web/who/index.html"

var directoryPages = []string{"/classrooms", "/staff", "/map", "/email-list", "/greenvelope", "/my-privacy", "/my-family", "/admin", "/g/{key}", "/p/{key}"}

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
	serve.Pages(mux, directoryShell, directoryPages...)
	mux.HandleFunc("GET /people", directoryPeople)
	mux.HandleFunc("GET /dl/", directoryLegacyRedirect)
}

func moved(prefixes map[string]string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		for from, to := range prefixes {
			rest, ok := strings.CutPrefix(r.URL.EscapedPath(), from)
			if !ok {
				continue
			}
			target := to + rest
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
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
		serve.File(w, r, directoryShell)
		return
	}
	http.Redirect(w, r, "/g/"+url.PathEscape(key), http.StatusMovedPermanently)
}
