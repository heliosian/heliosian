package db

import (
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	queryLimit = 256 << 10
	batchLimit = 16 << 20
)

type answer struct {
	Now       string                          `json:"now"`
	Query     string                          `json:"query"`
	Tree      map[string]any                  `json:"tree,omitempty"`
	Result    []string                        `json:"result"`
	Resources map[string]map[string]store.Row `json:"resources"`
}

type written struct {
	Result []string `json:"result"`
}

func body(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, bool) {
	kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if kind != "application/json" && kind != "text/plain" {
		http.Error(w, "send application/json, or text/plain for the query language", http.StatusUnsupportedMediaType)
		return "", nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return "", nil, false
	}
	if err != nil {
		serve.Error(w, r, err)
		return "", nil, false
	}
	return kind, raw, true
}

const importReader = "import"

func caller(w http.ResponseWriter, r *http.Request, m *Model, importKey []byte, at time.Time) (Env, access.Actor, bool) {
	key, bearer := auth.Bearer(r)
	if !bearer {
		email := auth.Email(r)
		return Env{Viewer: m.PersonOf(email), Now: at}, access.Actor{Email: email}, true
	}
	if len(importKey) == 0 || subtle.ConstantTimeCompare([]byte(key), importKey) != 1 {
		http.Error(w, "unknown key", http.StatusUnauthorized)
		return Env{}, access.Actor{}, false
	}
	return Env{System: importReader, Now: at}, access.System(importReader), true
}

func Register(mux *http.ServeMux, s *Store, queue *store.Queue, importKey []byte, now func() time.Time) {
	mux.HandleFunc("QUERY /api/q", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, importKey, now())
		if !ok {
			return
		}
		kind, raw, ok := body(w, r, queryLimit)
		if !ok {
			return
		}
		var q *Query
		var err error
		if kind == "text/plain" {
			q, err = Parse(string(raw))
		} else {
			q, err = ParseJSON(raw)
		}
		if err != nil {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		slog.InfoContext(r.Context(), "query", "viewer", env.Viewer, "system", env.System, "query", q.tree.flat())
		result := m.Run(q, env)
		out := answer{
			Now:       env.Now.Format("2006-01-02 15:04"),
			Query:     q.String(),
			Result:    result.IDs,
			Resources: result.Resources,
		}
		if kind == "text/plain" {
			out.Tree = q.Tree()
		}
		serve.Write(w, r, http.StatusOK, out)
	})
	mux.HandleFunc("POST /api/q", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		kind, raw, ok := body(w, r, batchLimit)
		if !ok {
			return
		}
		if kind != "application/json" {
			http.Error(w, "a write is an application/json batch", http.StatusUnsupportedMediaType)
			return
		}
		b, err := ParseBatch(raw)
		if err != nil {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		slog.InfoContext(r.Context(), "write", "viewer", env.Viewer, "system", env.System, "writes", len(b.Batch))
		ids, err := Write(r.Context(), s, queue, actor, env, b)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, written{Result: ids})
	})
}

func (m *Model) PersonOf(email string) string {
	row, ok := m.Table("PERSON_EMAIL").Find(strings.ToLower(strings.TrimSpace(email)))
	if !ok {
		return ""
	}
	return row["person"]
}
