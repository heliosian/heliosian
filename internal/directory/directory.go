// Package directory serves the school directory app.
package directory

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"heliosian/internal/auth"
)

var sections = []string{"people", "classrooms", "staff", "map", "email-list", "my-privacy"}

var legacy = map[string]string{
	"people":   "/people",
	"explore":  "/classrooms",
	"myfamily": "/my-family",
	"staff":    "/staff",
	"map":      "/map",
	"emails":   "/email-list",
}

type app struct {
	cache   *Cache
	mapsKey string
}

// effectiveEmail is who the directory should render as: the signed-in admin's spoof
// target if they've picked one to view as, otherwise the signed-in identity itself.
// It's read-only by construction — nothing that writes (upload.go, tags.go) calls it,
// so a write is always attributed to whoever is actually signed in.
func effectiveEmail(cache *Cache, r *http.Request) string {
	real := strings.ToLower(auth.Email(r))
	if target := cache.SpoofTarget(real); target != "" {
		return target
	}
	return real
}

// spoofDisplayName reports who a super admin is currently spoofing as, by name where
// possible, for the two places (the admin page, the on-app banner) that tell them so.
func spoofDisplayName(cache *Cache, realEmail string) string {
	target := cache.SpoofTarget(realEmail)
	if target == "" {
		return ""
	}
	if p := cache.Model().Person(target); p != nil {
		return p.FullName
	}
	return target
}

func MemberGate(cache *Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Admin routes carry their own allowlist check; requiring directory membership
		// too would lock out an admin who runs the school's tools but isn't a parent
		// or staff member with their own Person row.
		if auth.Public(r.URL.Path) || r.URL.Path == "/auth/logout" ||
			r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/api/admin/") {
			next.ServeHTTP(w, r)
			return
		}
		if !cache.Model().Member(effectiveEmail(cache, r)) {
			http.Error(w, "account is not in the directory", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func Register(mux *http.ServeMux, cache *Cache, mapsKey string) {
	a := app{cache: cache, mapsKey: mapsKey}
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, a.page)
	}
	mux.HandleFunc("GET /my-family", a.myFamily)
	mux.HandleFunc("GET /people/{email}", a.page)
	mux.HandleFunc("GET /families/{key}", a.page)
	mux.HandleFunc("GET /classrooms/{name}", a.page)
	mux.HandleFunc("GET /grades/{name}", a.page)
	mux.HandleFunc("GET /dl/", a.legacyRedirect)
	mux.HandleFunc("GET /api/directory/model", a.model)
}

func (a app) myFamily(w http.ResponseWriter, r *http.Request) {
	model := a.cache.Model()
	email := effectiveEmail(a.cache, r)
	if keys := model.FamilyKeysOf(email); len(keys) > 0 {
		http.Redirect(w, r, "/families/"+url.PathEscape(keys[0]), http.StatusFound)
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
	t, err := template.ParseFiles("web/directory/index.html")
	if err != nil {
		serverError(w, err)
		return
	}
	effective := effectiveEmail(a.cache, r)
	name := a.cache.Model().DisplayName(effective)
	emailPrefix, _, _ := strings.Cut(effective, "@")
	data := struct {
		UserName      string
		UserInitial   string
		UserEmail     string
		UserEmailSlug string
		MapsKey       string
		IsAdmin       bool
	}{
		UserName:      name,
		UserInitial:   strings.ToUpper(name[:1]),
		UserEmail:     effective,
		UserEmailSlug: emailPrefix,
		MapsKey:       a.mapsKey,
		// Admin-ness (and so the menu link to /admin) follows who's actually being
		// viewed, not who's signed in — while spoofing, the page should look exactly
		// like it does to the person being spoofed. A super admin gets back to /admin
		// through the persistent spoofing banner instead.
		IsAdmin: a.cache.IsAdmin(effective),
	}
	if err := t.Execute(w, data); err != nil {
		log.Printf("[ERROR] render directory page: %v", err)
	}
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	real := strings.ToLower(auth.Email(r))
	effective := effectiveEmail(a.cache, r)
	view := struct {
		*Model
		Tags       map[string][]string `json:"tags"`
		SuperEdit  bool                `json:"superEdit,omitempty"`
		SpoofingAs string              `json:"spoofingAs,omitempty"`
	}{
		Model: a.cache.Model(),
		Tags:  a.cache.Tags(effective),
		// Both computed from the effective identity, so a spoofed view shows exactly
		// what that person sees — a regular parent's simulated view never carries the
		// real admin's super-edit powers along with it.
		SuperEdit: a.cache.IsAdmin(effective) && a.cache.SuperEditEnabled(effective),
	}
	// The spoofing notice itself is the one thing keyed on the real identity: it's
	// what lets the super admin's own browser show "you're viewing as X" and find its
	// way back, regardless of what the simulated view otherwise looks like.
	view.SpoofingAs = spoofDisplayName(a.cache, real)
	if err := json.NewEncoder(w).Encode(view); err != nil {
		log.Printf("[ERROR] encode model: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
