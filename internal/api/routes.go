package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type me struct {
	Email      string   `json:"email"`
	Allowances []string `json:"allowances"`
}

type entry struct {
	Path string `json:"path"`
}

type created struct {
	ID string `json:"id,omitempty"`
}

type step struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type acted struct {
	Results []created `json:"results"`
}

const bodyLimit = 1 << 20

func (reg *Registry[S]) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me", serve.JSON(reg.me))
	mux.HandleFunc("GET /api/openapi.json", serve.JSON(reg.openapi))
	mux.HandleFunc("GET /api/docs", docs)
	mux.HandleFunc("GET /api/r/{id}", serve.JSON(reg.get))
	mux.HandleFunc("GET /api/{type}", serve.JSON(reg.get))
	mux.HandleFunc("GET /api/{type}/{id}", serve.JSON(reg.get))
	mux.HandleFunc("POST /api/query", serve.JSON(reg.batch))
	mux.HandleFunc("POST /api/act", serve.JSON(reg.acts))
	mux.HandleFunc("POST /api/{type}", serve.JSON(reg.create))
	mux.HandleFunc("POST /api/{type}/{id}/{action}", serve.JSON(reg.act))
	mux.HandleFunc("DELETE /api/{type}/{id}", serve.JSON(reg.act))
}

func (reg *Registry[S]) me(r *http.Request, _ serve.None) (me, error) {
	w := reg.current.Load()
	actor := reg.config.Actor(r, w.s)
	out := me{Email: actor.Email, Allowances: []string{}}
	for _, a := range reg.config.Held(actor.Email) {
		out.Allowances = append(out.Allowances, a.Name)
	}
	return out, nil
}

func (reg *Registry[S]) get(r *http.Request, _ serve.None) (envelope, error) {
	w := reg.current.Load()
	rd := reg.reader(w, reg.query(r, w))
	result, err := rd.read(r.URL)
	if err != nil {
		return envelope{}, err
	}
	return rd.envelope(result), nil
}

func (reg *Registry[S]) batch(r *http.Request, entries map[string]entry) (envelope, error) {
	w := reg.current.Load()
	rd := reg.reader(w, reg.query(r, w))
	result := map[string]any{}
	for name, e := range entries {
		target, err := url.Parse(e.Path)
		if err != nil {
			return envelope{}, access.Invalid("%s: %v", name, err)
		}
		out, err := rd.read(target)
		if err != nil {
			return envelope{}, named(name, err)
		}
		result[name] = out
	}
	return rd.envelope(result), nil
}

func named(name string, err error) error {
	var refusal *access.Refusal
	if !errors.As(err, &refusal) {
		return err
	}
	return access.Refuse(refusal.Status, "%s: %s", name, refusal.Message)
}

func (reg *Registry[S]) create(r *http.Request, _ serve.None) (created, error) {
	body, err := readBody(r)
	if err != nil {
		return created{}, err
	}
	results, err := reg.write(r, []step{{Method: r.Method, Path: r.URL.Path, Body: body}})
	if err != nil {
		return created{}, err
	}
	return results[0], nil
}

func (reg *Registry[S]) act(r *http.Request, _ serve.None) (serve.None, error) {
	body, err := readBody(r)
	if err != nil {
		return serve.None{}, err
	}
	_, err = reg.write(r, []step{{Method: r.Method, Path: r.URL.Path, Body: body}})
	return serve.None{}, err
}

func (reg *Registry[S]) acts(r *http.Request, steps []step) (acted, error) {
	if len(steps) == 0 {
		return acted{}, access.Invalid("no writes")
	}
	results, err := reg.write(r, steps)
	if err != nil {
		return acted{}, err
	}
	return acted{Results: results}, nil
}

func readBody(r *http.Request) (json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil {
		return nil, access.Invalid("bad request body: %v", err)
	}
	if len(raw) > bodyLimit {
		return nil, access.Refuse(http.StatusRequestEntityTooLarge, "request body too large")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if !json.Valid(raw) {
		return nil, access.Invalid("bad request body")
	}
	return raw, nil
}

func (reg *Registry[S]) write(r *http.Request, steps []step) ([]created, error) {
	w := reg.current.Load()
	q := reg.query(r, w)
	out := []created{}
	_, err := reg.config.Queue.Transact(r.Context(), q.Actor, func(tx *store.Tx) error {
		for i, st := range steps {
			s := reg.config.Scope(reg.config.Staged(tx), q)
			wr := Write[S]{Request: r, Tx: tx, S: s, Query: q, Body: st.Body, Taken: reg.takenIn(w, s)}
			result, err := reg.step(w, wr, st)
			if err != nil && len(steps) > 1 {
				return named(fmt.Sprintf("write %d", i+1), err)
			}
			if err != nil {
				return err
			}
			out = append(out, result)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (reg *Registry[S]) step(w *world[S], wr Write[S], st step) (created, error) {
	rest, ok := strings.CutPrefix(st.Path, "/api/")
	if !ok {
		return created{}, access.Invalid("%s is not a resource path", st.Path)
	}
	parts := strings.Split(rest, "/")
	t, err := reg.typeNamed(parts[0])
	if err != nil {
		return created{}, err
	}
	switch {
	case st.Method == http.MethodPost && len(parts) == 1:
		if t.Create == nil {
			return created{}, access.Refuse(http.StatusMethodNotAllowed, "%s can't be created", t.Name)
		}
		key, err := t.Create(wr)
		return created{ID: key}, err
	case st.Method == http.MethodPost && len(parts) == 3:
		return created{}, reg.run(w, t, wr, parts[1], parts[2])
	case st.Method == http.MethodDelete && len(parts) == 2:
		return created{}, reg.run(w, t, wr, parts[1], "delete")
	}
	return created{}, access.Invalid("%s %s is not a write", st.Method, st.Path)
}

func (reg *Registry[S]) run(w *world[S], t *Type[S], wr Write[S], segment, name string) error {
	action, ok := t.Actions[name]
	if !ok {
		return access.Missing("%s has no action %s", t.Name, name)
	}
	key, ok := w.resolveIn(wr.S, t, segment)
	if !ok {
		return access.Missing("no %s %s", t.Name, segment)
	}
	if _, visible := t.Get(wr.S, wr.Query, key); !visible {
		return access.Missing("no %s %s", t.Name, segment)
	}
	wr.ID = key
	return action.Do(wr)
}
