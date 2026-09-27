package config

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/access"
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

func (a api) actor(r *http.Request, admin func(email string) bool) access.Actor {
	email := strings.ToLower(auth.Email(r))
	return access.Actor{Email: email, Admin: admin(email)}
}

func (a api) settingsActor(r *http.Request) access.Actor {
	return a.actor(r, a.isAdmin)
}

func (a api) superActor(r *http.Request) access.Actor {
	return a.actor(r, a.cache.IsSuperAdmin)
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

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func (a api) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
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
	if err := requireAdmin(a.superActor(r)); err != nil {
		refuse(w, err)
		return
	}
	encode(w, r, map[string][]string{"superAdmins": a.cache.SuperAdmins()})
}

func (a api) setStaleYears(w http.ResponseWriter, r *http.Request) {
	var years StaleYears
	if !decode(w, r, &years) {
		return
	}
	actor := a.settingsActor(r)
	ops, err := a.cache.Settings().setStaleYears(actor, years)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set stale-years thresholds", "photo", years.Photo, "facts", years.Facts, "familyPhoto", years.FamilyPhoto)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setPrivacyLinks(w http.ResponseWriter, r *http.Request) {
	var body PrivacyLinks
	if !decode(w, r, &body) {
		return
	}
	actor := a.settingsActor(r)
	links, ops, err := a.cache.Settings().setPrivacyLinks(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setColor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"`
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.settingsActor(r)
	name, ops, err := a.cache.Settings().setColor(actor, body.Kind, body.Name, body.Color)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) setSuperAdmins(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SuperAdmins []string `json:"superAdmins"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.superActor(r)
	admins, ops, err := a.cache.Settings().setSuperAdmins(actor, body.SuperAdmins)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: set the super admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a api) signOut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.superActor(r)
	email, ops, err := a.cache.Settings().signOut(actor, body.Email)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "config: signed out every session", "email", email, "by", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}
