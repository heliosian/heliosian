// Package who serves the school directory app, Helios Who?.
package who

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"heliosian/internal/auth"
)

var sections = []string{"people", "classrooms", "staff", "map", "email-list", "greenvelope", "my-privacy"}

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
	optIn   func() string
}

// effectiveEmail is who the directory should render as: the signed-in admin's spoof
// target if they've picked one to view as, otherwise the signed-in identity itself.
// It's read-only by construction — nothing that writes (upload.go, tags.go) calls it,
// so a write is always attributed to whoever is actually signed in.
func effectiveEmail(cache *Cache, r *http.Request) string {
	real := realEmail(cache, r)
	if target := cache.SpoofTarget(real); target != "" {
		return target
	}
	return real
}

// realEmail is the signed-in identity as the directory keys it: the address Google
// vouched for, resolved through Email Aliases, since which of a person's addresses
// their Workspace account calls primary is nobody's deliberate choice.
func realEmail(cache *Cache, r *http.Request) string {
	return cache.Model().Resolve(strings.ToLower(auth.Email(r)))
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

// noAccess is what someone the directory doesn't list gets instead of the app: the
// consent form is the only way in, so the page hands them the link to it.
const noAccess = "web/public/who/no-access.html"

func denyAccess(w http.ResponseWriter) {
	page, err := os.ReadFile(noAccess)
	if err != nil {
		serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	if _, err := w.Write(page); err != nil {
		log.Printf("[ERROR] write the no-access page: %v", err)
	}
}

func MemberGate(cache *Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Admin routes carry their own allowlist check; requiring directory membership
		// too would lock out an admin who runs the school's tools but isn't a parent
		// or staff member with their own Person row. /optin is what the no-access page
		// itself sends someone to, so it has to answer the people the gate turns away.
		if auth.Public(r.URL.Path) || r.URL.Path == "/auth/logout" || r.URL.Path == "/optin" ||
			r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/api/admin/") {
			next.ServeHTTP(w, r)
			return
		}
		if !cache.Model().Member(effectiveEmail(cache, r)) {
			denyAccess(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Register takes the opt-in form's address as a function rather than a value because
// it lives in the Config sheet, where an admin can change it between requests.
func Register(mux *http.ServeMux, cache *Cache, mapsKey string, optIn func() string) {
	a := app{cache: cache, mapsKey: mapsKey, optIn: optIn}
	for _, section := range sections {
		mux.HandleFunc("GET /"+section, a.page)
	}
	mux.HandleFunc("GET /optin", a.optInForm)
	mux.HandleFunc("GET /my-family", a.myFamily)
	mux.HandleFunc("GET /people/{email}", a.page)
	mux.HandleFunc("GET /families/{key}", a.page)
	mux.HandleFunc("GET /classrooms/{name}", a.page)
	mux.HandleFunc("GET /grades/{name}", a.page)
	mux.HandleFunc("GET /dl/", a.legacyRedirect)
	mux.HandleFunc("GET /api/directory/model", a.model)
}

func (a app) optInForm(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, a.optIn(), http.StatusFound)
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

// page serves the one static shell every directory route shares; the client
// reads who it is, and everything else, from the model.
func (a app) page(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/who/index.html")
}

// user is the signed-in identity as the shell shows it. Admin-ness (and so the
// menu link to /admin) follows who's actually being viewed, not who's signed
// in - while spoofing, the page should look exactly like it does to the person
// being spoofed. A super admin gets back to /admin through the spoofing banner.
type user struct {
	Name    string `json:"name"`
	Initial string `json:"initial"`
	Email   string `json:"email"`
	Slug    string `json:"slug"`
	IsAdmin bool   `json:"isAdmin"`
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	real := realEmail(a.cache, r)
	effective := effectiveEmail(a.cache, r)
	name := a.cache.Model().DisplayName(effective)
	slug, _, _ := strings.Cut(effective, "@")
	view := struct {
		*Model
		User       user                `json:"user"`
		MapsKey    string              `json:"mapsKey"`
		Tags       map[string][]string `json:"tags"`
		SuperEdit  bool                `json:"superEdit,omitempty"`
		SpoofingAs string              `json:"spoofingAs,omitempty"`
	}{
		Model:   a.cache.Model(),
		User:    user{Name: name, Initial: strings.ToUpper(name[:1]), Email: effective, Slug: slug, IsAdmin: a.cache.IsAdmin(effective)},
		MapsKey: a.mapsKey,
		Tags:    a.cache.Tags(effective),
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
