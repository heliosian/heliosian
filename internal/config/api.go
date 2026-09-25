package config

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/store"
)

type api struct {
	cache   *Cache
	isAdmin func(email string) bool
}

func Register(mux *http.ServeMux, cache *Cache, isAdmin func(email string) bool) {
	a := api{cache: cache, isAdmin: isAdmin}
	mux.HandleFunc("GET /api/config", a.settings)
	mux.HandleFunc("GET /api/config/super-admins", a.superAdmins)
	mux.HandleFunc("POST /api/config/stale-years", a.setStaleYears)
	mux.HandleFunc("POST /api/config/privacy-links", a.setPrivacyLinks)
	mux.HandleFunc("POST /api/config/color", a.setColor)
	mux.HandleFunc("POST /api/config/super-admins", a.setSuperAdmins)
	mux.HandleFunc("POST /api/config/sign-out", a.signOut)
}

func (a api) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.isAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

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

func encode(w http.ResponseWriter, r *http.Request, view any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode config", "error", err)
	}
}

func (a api) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (a api) settings(w http.ResponseWriter, r *http.Request) {
	encode(w, r, a.cache.Settings())
}

func (a api) superAdmins(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperAdmin(w, r); !ok {
		return
	}
	encode(w, r, map[string][]string{"superAdmins": a.cache.SuperAdmins()})
}

func (a api) setStaleYears(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
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
	if !a.commit(w, r, actor, setSettings(map[string]string{
		PhotoStaleYears:       FormatYears(years.Photo),
		FactsStaleYears:       FormatYears(years.Facts),
		FamilyPhotoStaleYears: FormatYears(years.FamilyPhoto),
	})...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set stale-years thresholds", "photo", years.Photo, "facts", years.Facts, "familyPhoto", years.FamilyPhoto)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setPrivacyLinks(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
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
	if !a.commit(w, r, actor, setSettings(map[string]string{
		VeracrossPreferences: links.VeracrossPreferences,
		HeliosWhoOptIn:       links.HeliosWhoOptIn,
	})...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setColor(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
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
	if body.Kind != "staff" && name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}
	var ops []store.Op
	switch body.Kind {
	case "classroom":
		ops = []store.Op{store.Set(ClassroomColorsTab, store.Row{ClassroomColumn: name}, store.Row{ColorColumn: body.Color})}
	case "grade":
		ops = []store.Op{store.Set(GradeColorsTab, store.Row{GradeColumn: name}, store.Row{ColorColumn: body.Color})}
	case "staff":
		ops = setSettings(map[string]string{StaffColor: body.Color})
	default:
		http.Error(w, "bad kind: must be classroom, grade, or staff", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setSuperAdmins(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireSuperAdmin(w, r)
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
	ops := []store.Op{}
	for _, e := range current {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(SuperAdminsTab, store.Row{EmailColumn: e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(current, e) {
			ops = append(ops, store.Insert(SuperAdminsTab, store.Row{EmailColumn: e}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set the super admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) signOut(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if !strings.Contains(email, "@") {
		http.Error(w, "missing email", http.StatusBadRequest)
		return
	}
	if err := a.cache.signOut(r.Context(), admin, email); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "config: signed out every session", "email", email, "by", admin)
	w.WriteHeader(http.StatusNoContent)
}
