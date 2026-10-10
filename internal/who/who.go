package who

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var sections = []string{"classrooms", "staff", "map", "email-list", "greenvelope", "my-privacy", "my-family", "admin"}

var legacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
}

func OptInForm(optIn func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, optIn(), http.StatusFound)
	})
}

func Register(mux *http.ServeMux, about *sharecard.About, mapsKey string) {
	mux.Handle("GET /open/share/about.png", about)
	mux.HandleFunc("GET /maps.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		fmt.Fprintf(w, "export const mapsKey = %s;\n", strconv.Quote(mapsKey))
	})
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, page)
	}
	mux.HandleFunc("GET /people", people)
	mux.HandleFunc("GET /groups/{key}", page)
	mux.HandleFunc("GET /people/{id}", page)
	mux.HandleFunc("GET /families/{id}", page)
	mux.HandleFunc("GET /classrooms/{slug}", page)
	mux.HandleFunc("GET /grades/{slug}", page)
	mux.HandleFunc("GET /dl/", legacyRedirect)
}

func legacyRedirect(w http.ResponseWriter, r *http.Request) {
	first, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/dl/"), "/")
	target, ok := legacy[first]
	if !ok {
		target = "/people"
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func people(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("tag")
	if list := r.URL.Query().Get("list"); list != "" {
		_, key, _ = strings.Cut(list, ":")
		if key == "" {
			key = list
		}
	}
	if key == "" {
		page(w, r)
		return
	}
	http.Redirect(w, r, "/groups/"+url.PathEscape(key), http.StatusMovedPermanently)
}

func page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/who/index.html")
}
