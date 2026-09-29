package model

import (
	"log/slog"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/serve"
)

type configRoutes struct {
	cache     *ConfigCache
	directory *DirectoryCache
	held      func(email string) []access.Allowance
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

func RegisterConfig(mux *http.ServeMux, cache *ConfigCache, directory *DirectoryCache, held func(email string) []access.Allowance) {
	a := configRoutes{cache: cache, directory: directory, held: held}
	mux.HandleFunc("GET /api/config", serve.JSON(a.settings))
	mux.HandleFunc("GET /api/config/super-admins", serve.JSON(a.superAdmins))
	mux.HandleFunc("POST /api/config/stale-years", serve.JSON(a.setStaleYears))
	mux.HandleFunc("POST /api/config/privacy-links", serve.JSON(a.setPrivacyLinks))
	mux.HandleFunc("POST /api/config/color", serve.JSON(a.setColor))
	mux.HandleFunc("POST /api/config/super-admins", serve.JSON(a.setSuperAdmins))
	mux.HandleFunc("POST /api/config/sign-out", serve.JSON(a.signOut))
}

func (a configRoutes) settingsActor(r *http.Request) access.Actor {
	return a.directory.Actor(r, a.held)
}

func (a configRoutes) superActor(r *http.Request) access.Actor {
	return a.directory.Actor(r, a.cache.SuperHeld)
}

func (a configRoutes) settings(r *http.Request, _ serve.None) (*Config, error) {
	return a.cache.Config(), nil
}

func (a configRoutes) superAdmins(r *http.Request, _ serve.None) (map[string][]string, error) {
	if err := require(a.superActor(r), ManageSuperAdmins); err != nil {
		return nil, err
	}
	return map[string][]string{"superAdmins": a.cache.SuperAdmins()}, nil
}

func (a configRoutes) setStaleYears(r *http.Request, years StaleYears) (serve.None, error) {
	actor := a.settingsActor(r)
	ops, err := a.cache.Config().setStaleYears(actor, years)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set stale-years thresholds", "photo", years.Photo, "facts", years.Facts, "familyPhoto", years.FamilyPhoto)
	return serve.None{}, nil
}

func (a configRoutes) setPrivacyLinks(r *http.Request, body PrivacyLinks) (serve.None, error) {
	actor := a.settingsActor(r)
	links, ops, err := a.cache.Config().setPrivacyLinks(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
	return serve.None{}, nil
}

func (a configRoutes) setColor(r *http.Request, body colorBody) (serve.None, error) {
	actor := a.settingsActor(r)
	name, ops, err := a.cache.Config().setColor(actor, body.Kind, body.Name, body.Color)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
	return serve.None{}, nil
}

func (a configRoutes) setSuperAdmins(r *http.Request, body superAdminsBody) (serve.None, error) {
	actor := a.superActor(r)
	admins, ops, err := a.cache.Config().setSuperAdmins(actor, body.SuperAdmins)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set the super admin list", "admins", admins)
	return serve.None{}, nil
}

func (a configRoutes) signOut(r *http.Request, body signOutBody) (serve.None, error) {
	actor := a.superActor(r)
	email, ops, err := a.cache.Config().signOut(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: signed out every session", "email", email, "by", actor.Email)
	return serve.None{}, nil
}
