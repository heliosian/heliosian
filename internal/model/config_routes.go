package model

import (
	"log/slog"
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/serve"
)

type configRoutes struct {
	store *Store
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

func RegisterConfig(mux *http.ServeMux, s *Store) {
	a := configRoutes{store: s}
	mux.HandleFunc("GET /api/config", serve.JSON(a.settings))
	mux.HandleFunc("GET /api/config/super-admins", serve.JSON(a.superAdmins))
	mux.HandleFunc("POST /api/config/stale-years", serve.JSON(a.setStaleYears))
	mux.HandleFunc("POST /api/config/privacy-links", serve.JSON(a.setPrivacyLinks))
	mux.HandleFunc("POST /api/config/color", serve.JSON(a.setColor))
	mux.HandleFunc("POST /api/config/super-admins", serve.JSON(a.setSuperAdmins))
	mux.HandleFunc("POST /api/config/sign-out", serve.JSON(a.signOut))
}

func superActor(m *Model, r *http.Request) access.Actor {
	return m.Directory.Actor(r, m.SuperHeld)
}

func (a configRoutes) settings(r *http.Request, _ serve.None) (*Config, error) {
	return a.store.Model().Config, nil
}

func (a configRoutes) superAdmins(r *http.Request, _ serve.None) (map[string][]string, error) {
	m := a.store.Model()
	if err := require(superActor(m, r), ManageSuperAdmins); err != nil {
		return nil, err
	}
	return map[string][]string{"superAdmins": m.Config.SuperAdmins}, nil
}

func (a configRoutes) setStaleYears(r *http.Request, years StaleYears) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "who")
	ops, err := m.Config.setStaleYears(actor, years)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, ConfigApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set stale-years thresholds", "photo", years.Photo, "facts", years.Facts, "familyPhoto", years.FamilyPhoto)
	return serve.None{}, nil
}

func (a configRoutes) setPrivacyLinks(r *http.Request, body PrivacyLinks) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "who")
	links, ops, err := m.Config.setPrivacyLinks(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, ConfigApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
	return serve.None{}, nil
}

func (a configRoutes) setColor(r *http.Request, body colorBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "who")
	name, ops, err := m.Config.setColor(actor, body.Kind, body.Name, body.Color)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, ConfigApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
	return serve.None{}, nil
}

func (a configRoutes) setSuperAdmins(r *http.Request, body superAdminsBody) (serve.None, error) {
	m := a.store.Model()
	actor := superActor(m, r)
	admins, ops, err := m.Config.setSuperAdmins(actor, body.SuperAdmins)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, ConfigApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: set the super admin list", "admins", admins)
	return serve.None{}, nil
}

func (a configRoutes) signOut(r *http.Request, body signOutBody) (serve.None, error) {
	m := a.store.Model()
	actor := superActor(m, r)
	email, ops, err := m.Config.signOut(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, ConfigApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "config: signed out every session", "email", email, "by", actor.Email)
	return serve.None{}, nil
}
