package api

import (
	"errors"
	"net/http"
	"net/url"

	"heliosian/internal/access"
	"heliosian/internal/serve"
)

type me struct {
	Email      string          `json:"email"`
	Allowances map[string]bool `json:"allowances"`
}

type entry struct {
	Path string `json:"path"`
}

type created struct {
	ID string `json:"id"`
}

func (reg *Registry[S]) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me", serve.JSON(reg.me))
	mux.HandleFunc("GET /api/r/{id}", serve.JSON(reg.get))
	mux.HandleFunc("GET /api/{type}", serve.JSON(reg.get))
	mux.HandleFunc("GET /api/{type}/{id}", serve.JSON(reg.get))
	mux.HandleFunc("POST /api/query", serve.JSON(reg.batch))
	mux.HandleFunc("POST /api/{type}", serve.JSON(reg.create))
	mux.HandleFunc("POST /api/{type}/{id}/{action}", serve.JSON(reg.act))
	mux.HandleFunc("DELETE /api/{type}/{id}", serve.JSON(reg.act))
}

func (reg *Registry[S]) me(r *http.Request, _ serve.None) (me, error) {
	w := reg.current.Load()
	actor := reg.config.Actor(r, w.s)
	out := me{Email: actor.Email, Allowances: map[string]bool{}}
	for _, a := range reg.config.Held(actor.Email) {
		out.Allowances[a.Name] = actor.May(a)
	}
	return out, nil
}

func (reg *Registry[S]) get(r *http.Request, _ serve.None) (envelope, error) {
	w := reg.current.Load()
	rd := reg.reader(w, reg.query(r, w))
	data, err := rd.read(r.URL)
	if err != nil {
		return envelope{}, err
	}
	return rd.envelope(data), nil
}

func (reg *Registry[S]) batch(r *http.Request, entries map[string]entry) (envelope, error) {
	w := reg.current.Load()
	rd := reg.reader(w, reg.query(r, w))
	data := map[string]any{}
	for name, e := range entries {
		target, err := url.Parse(e.Path)
		if err != nil {
			return envelope{}, access.Invalid("%s: %v", name, err)
		}
		out, err := rd.read(target)
		if err != nil {
			return envelope{}, named(name, err)
		}
		data[name] = out
	}
	return rd.envelope(data), nil
}

func named(name string, err error) error {
	var refusal *access.Refusal
	if !errors.As(err, &refusal) {
		return err
	}
	return access.Refuse(refusal.Status, "%s: %s", name, refusal.Message)
}

func (reg *Registry[S]) create(r *http.Request, _ serve.None) (created, error) {
	w := reg.current.Load()
	t, err := reg.typeNamed(r.PathValue("type"))
	if err != nil {
		return created{}, err
	}
	if t.Create == nil {
		return created{}, access.Refuse(http.StatusMethodNotAllowed, "%s can't be created", t.Name)
	}
	key, err := t.Create(r, w.s, reg.query(r, w))
	if err != nil {
		return created{}, err
	}
	return created{ID: key}, nil
}

func (reg *Registry[S]) act(r *http.Request, _ serve.None) (serve.None, error) {
	w := reg.current.Load()
	t, err := reg.typeNamed(r.PathValue("type"))
	if err != nil {
		return serve.None{}, err
	}
	name := r.PathValue("action")
	if r.Method == http.MethodDelete {
		name = "delete"
	}
	action, ok := t.Actions[name]
	if !ok {
		return serve.None{}, access.Missing("%s has no action %s", t.Name, name)
	}
	q := reg.query(r, w)
	key, ok := w.resolve(t, r.PathValue("id"))
	if !ok {
		return serve.None{}, access.Missing("no %s %s", t.Name, r.PathValue("id"))
	}
	if _, visible := t.Get(w.s, q, key); !visible {
		return serve.None{}, access.Missing("no %s %s", t.Name, r.PathValue("id"))
	}
	return serve.None{}, action.Do(r, w.s, q, key)
}
