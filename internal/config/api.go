package config

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

type api struct {
	cache   *Cache
	writer  data.Writer
	isAdmin func(email string) bool
}

// Register serves the settings to every signed-in user and takes edits from the
// app's admins. isAdmin is the app's own admin check, either tier; managing the
// super admins themselves is gated on that list alone.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, isAdmin func(email string) bool) {
	a := api{cache: cache, writer: writer, isAdmin: isAdmin}
	mux.HandleFunc("GET /api/config", a.settings)
	mux.HandleFunc("GET /api/config/super-admins", a.superAdmins)
	mux.HandleFunc("POST /api/config/stale-years", a.setStaleYears)
	mux.HandleFunc("POST /api/config/privacy-links", a.setPrivacyLinks)
	mux.HandleFunc("POST /api/config/color", a.setColor)
	mux.HandleFunc("POST /api/config/super-admins", a.setSuperAdmins)
}

func (a api) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.isAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// requireSuperAdmin answers a regular admin with the same 403 a non-admin gets,
// revealing nothing about a tier above them.
func (a api) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.cache.IsSuperAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func encode(w http.ResponseWriter, view any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		log.Printf("[ERROR] encode config: %v", err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("[ERROR] %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func (a api) settings(w http.ResponseWriter, r *http.Request) {
	encode(w, a.cache.Settings())
}

func (a api) superAdmins(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	encode(w, map[string][]string{"superAdmins": a.cache.SuperAdmins()})
}

func (a api) setStaleYears(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var years StaleYears
	if !decode(w, r, &years) {
		return
	}
	if years.Photo <= 0 || years.Facts <= 0 || years.FamilyPhoto <= 0 {
		http.Error(w, "thresholds must be positive numbers of years", http.StatusBadRequest)
		return
	}
	values := map[string]string{
		PhotoStaleYears:       FormatYears(years.Photo),
		FactsStaleYears:       FormatYears(years.Facts),
		FamilyPhotoStaleYears: FormatYears(years.FamilyPhoto),
	}
	if err := a.cache.update("stale years edit", func(t *Tables) *Tables { return t.WithSettings(values) }, func() error {
		return WriteSettings(a.writer, values)
	}); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("config: %s set stale-years thresholds to %+v", email, years)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setPrivacyLinks(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var links PrivacyLinks
	if !decode(w, r, &links) {
		return
	}
	links.VeracrossPreferences = strings.TrimSpace(links.VeracrossPreferences)
	links.HeliosWhoOptIn = strings.TrimSpace(links.HeliosWhoOptIn)
	if !strings.HasPrefix(links.VeracrossPreferences, "https://") || !strings.HasPrefix(links.HeliosWhoOptIn, "https://") {
		http.Error(w, "both links must be full https:// URLs", http.StatusBadRequest)
		return
	}
	values := map[string]string{
		VeracrossPreferences: links.VeracrossPreferences,
		HeliosWhoOptIn:       links.HeliosWhoOptIn,
	}
	if err := a.cache.update("privacy links edit", func(t *Tables) *Tables { return t.WithSettings(values) }, func() error {
		return WriteSettings(a.writer, values)
	}); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("config: %s set privacy links to %+v", email, links)
	w.WriteHeader(http.StatusNoContent)
}

// setColor upserts one grade, classroom, or the single staff color - per item
// rather than a bulk replace-all, since classrooms are recomputed from the
// directory's data on every load and have no id to key a merge on beyond the name.
func (a api) setColor(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !HexColor.MatchString(body.Color) {
		http.Error(w, "color must be a #rrggbb hex value", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	var mirror func(*Tables) *Tables
	var persist func() error
	switch body.Kind {
	case "classroom":
		mirror = func(t *Tables) *Tables { return t.WithClassroomColor(name, body.Color) }
		persist = func() error { return WriteClassroomColor(a.writer, name, body.Color) }
	case "grade":
		mirror = func(t *Tables) *Tables { return t.WithGradeColor(name, body.Color) }
		persist = func() error { return WriteGradeColor(a.writer, name, body.Color) }
	case "staff":
		values := map[string]string{StaffColor: body.Color}
		mirror = func(t *Tables) *Tables { return t.WithSettings(values) }
		persist = func() error { return WriteSettings(a.writer, values) }
	default:
		http.Error(w, "bad kind: must be classroom, grade, or staff", http.StatusBadRequest)
		return
	}
	if body.Kind != "staff" && name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	if err := a.cache.update(body.Kind+" color edit", mirror, persist); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("config: %s set the %s color for %q to %s", email, body.Kind, name, body.Color)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setSuperAdmins(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		SuperAdmins []string `json:"superAdmins"`
	}
	if !decode(w, r, &body) {
		return
	}
	admins := NormalizeEmails(body.SuperAdmins)
	if len(admins) == 0 {
		http.Error(w, "the super admin list cannot be empty", http.StatusBadRequest)
		return
	}
	current := a.cache.SuperAdmins()
	if err := a.cache.update("super admins edit", func(t *Tables) *Tables { return t.WithSuperAdmins(admins) }, func() error {
		return WriteSuperAdmins(a.writer, current, admins)
	}); err != nil {
		serverError(w, err)
		return
	}
	log.Printf("config: %s set the super admin list to %s", email, strings.Join(admins, ", "))
	w.WriteHeader(http.StatusNoContent)
}
