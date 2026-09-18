// Package who serves the school directory app, Helios Who?.
package who

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/serve"
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
	lister  Lister
}

// effectiveEmail is who the directory renders as, and acts as: the person
// sign-in says - the signed-in address, or whoever a super admin is viewing
// as in Spoof Mode (internal/auth) - resolved through Email Aliases, since
// which of a person's addresses their Workspace account calls primary is
// nobody's deliberate choice. Reads and writes alike run as them, so a
// spoofed view does exactly what that person could.
func effectiveEmail(cache *Cache, r *http.Request) string {
	return cache.Model().Resolve(strings.ToLower(auth.Email(r)))
}

// noAccess is what someone the directory doesn't list gets instead of the app: the
// consent form is the only way in, so the page hands them the link to it.
const noAccess = "web/public/who/no-access.html"

func denyAccess(w http.ResponseWriter, r *http.Request) {
	page, err := os.ReadFile(noAccess)
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	if _, err := w.Write(page); err != nil {
		slog.ErrorContext(r.Context(), "write the no-access page", "error", err)
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
			denyAccess(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Register takes the opt-in form's address as a function rather than a value because
// it lives in the Config sheet, where an admin can change it between requests.
func Register(mux *http.ServeMux, cache *Cache, mapsKey string, optIn func() string, lister Lister) {
	a := app{cache: cache, mapsKey: mapsKey, optIn: optIn, lister: lister}
	mux.HandleFunc("GET /open/share/about.png", a.shareCard)
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
		http.Redirect(w, r, FamilyPath(keys[0]), http.StatusFound)
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
	serve.File(w, r, "web/who/index.html")
}

// user is the signed-in identity as the shell shows it. Admin-ness (and so the
// menu link to /admin) follows who's actually being viewed, not who's signed
// in - while spoofing, the page should look exactly like it does to the person
// being spoofed. The toolbar's switch, which asks sign-in rather than the
// directory, is the way back.
type user struct {
	Name    string `json:"name"`
	Initial string `json:"initial"`
	Email   string `json:"email"`
	Slug    string `json:"slug"`
	IsAdmin bool   `json:"isAdmin"`
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	effective := effectiveEmail(a.cache, r)
	name := a.cache.Model().DisplayName(effective)
	slug := Slug(effective)
	view := struct {
		*Model
		User    user                `json:"user"`
		MapsKey string              `json:"mapsKey"`
		Tags    map[string][]string `json:"tags"`
		// TagManagers is who else manages each of this person's tags (tags
		// with none are absent); SharedTags the tags others have let them
		// manage.
		TagManagers map[string][]string `json:"tagManagers"`
		SharedTags  []SharedTag         `json:"sharedTags"`
		Lists       []List              `json:"lists"`
		SuperEdit   bool                `json:"superEdit,omitempty"`
	}{
		Model:       a.cache.Model(),
		User:        user{Name: name, Initial: strings.ToUpper(name[:1]), Email: effective, Slug: slug, IsAdmin: a.cache.IsAdmin(effective)},
		MapsKey:     a.mapsKey,
		Tags:        a.cache.Tags(effective),
		TagManagers: a.cache.TagManagers(effective),
		SharedTags:  a.cache.SharedTags(effective),
		Lists:       append(a.cache.Model().RoomParentLists(effective), a.lister.Lists(effective)...),
		// Both computed from the effective identity, so a spoofed view shows exactly
		// what that person sees — a regular parent's simulated view never carries the
		// real admin's super-edit powers along with it.
		SuperEdit: a.cache.IsAdmin(effective) && a.cache.SuperEditEnabled(effective),
	}
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode model", "error", err)
	}
}

func serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "directory request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
