package db

import (
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

const bodyLimit = 256 << 10

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

func body(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	kind, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if kind != "application/json" && kind != "text/plain" {
		http.Error(w, "send application/json, or text/plain for the query language", http.StatusUnsupportedMediaType)
		return "", nil, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyLimit))
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

func Register(mux *http.ServeMux, s *Store, queue *store.Queue, now func() time.Time) {
	mux.HandleFunc("QUERY /api/q", func(w http.ResponseWriter, r *http.Request) {
		kind, raw, ok := body(w, r)
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
		m := s.Model()
		viewer := m.PersonOf(auth.Email(r))
		at := now()
		slog.InfoContext(r.Context(), "query", "viewer", viewer, "query", q.tree.flat())
		result := m.Run(q, Env{Viewer: viewer, Now: at})
		out := answer{
			Now:       at.Format("2006-01-02 15:04"),
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
		kind, raw, ok := body(w, r)
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
		email := auth.Email(r)
		viewer := s.Model().PersonOf(email)
		slog.InfoContext(r.Context(), "write", "viewer", viewer, "writes", len(b.Batch))
		ids, err := Write(r.Context(), s, queue, access.Actor{Email: email}, Env{Viewer: viewer, Now: now()}, b)
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
