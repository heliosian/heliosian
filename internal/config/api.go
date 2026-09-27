package config

import (
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
)

type api struct {
	cache   *Cache
	isAdmin func(email string) bool
}

type colorBody struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type superAdminsBody struct {
	SuperAdmins []string `json:"superAdmins"`
}

type signOutBody struct {
	Email string `json:"email"`
}

func Register(mux *http.ServeMux, cache *Cache, isAdmin func(email string) bool) {
	a := api{cache: cache, isAdmin: isAdmin}
	mux.HandleFunc("GET /api/config", serve.JSON(a.settings))
	mux.HandleFunc("GET /api/config/super-admins", serve.JSON(a.superAdmins))
	mux.HandleFunc("POST /api/config/stale-years", serve.JSON(a.setStaleYears))
	mux.HandleFunc("POST /api/config/privacy-links", serve.JSON(a.setPrivacyLinks))
	mux.HandleFunc("POST /api/config/color", serve.JSON(a.setColor))
	mux.HandleFunc("POST /api/config/super-admins", serve.JSON(a.setSuperAdmins))
	mux.HandleFunc("POST /api/config/sign-out", serve.JSON(a.signOut))
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

func (a api) settings(r *http.Request, _ serve.None) (*Settings, error) {
	return a.cache.Settings(), nil
}

func (a api) superAdmins(r *http.Request, _ serve.None) (map[string][]string, error) {
	if err := requireAdmin(a.superActor(r)); err != nil {
		return nil, err
	}
	return map[string][]string{"superAdmins": a.cache.SuperAdmins()}, nil
}

func (a api) setStaleYears(r *http.Request, years StaleYears) (serve.None, error) {
	actor := a.settingsActor(r)
	ops, err := a.cache.Settings().setStaleYears(actor, years)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set stale-years thresholds", "photo", years.Photo, "facts", years.Facts, "familyPhoto", years.FamilyPhoto)
	return serve.None{}, nil
}

func (a api) setPrivacyLinks(r *http.Request, body PrivacyLinks) (serve.None, error) {
	actor := a.settingsActor(r)
	links, ops, err := a.cache.Settings().setPrivacyLinks(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
	return serve.None{}, nil
}

func (a api) setColor(r *http.Request, body colorBody) (serve.None, error) {
	actor := a.settingsActor(r)
	name, ops, err := a.cache.Settings().setColor(actor, body.Kind, body.Name, body.Color)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
	return serve.None{}, nil
}

func (a api) setSuperAdmins(r *http.Request, body superAdminsBody) (serve.None, error) {
	actor := a.superActor(r)
	admins, ops, err := a.cache.Settings().setSuperAdmins(actor, body.SuperAdmins)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set the super admin list", "admins", admins)
	return serve.None{}, nil
}

func (a api) signOut(r *http.Request, body signOutBody) (serve.None, error) {
	actor := a.superActor(r)
	email, ops, err := a.cache.Settings().signOut(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: signed out every session", "email", email, "by", actor.Email)
	return serve.None{}, nil
}
