package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/trace"
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

type answerAll struct {
	Now       string                          `json:"now"`
	Results   map[string][]string             `json:"results"`
	Resources map[string]map[string]store.Row `json:"resources"`
}

func parseAll(kind string, raw []byte) ([]string, []*Query, bool, error) {
	var probe map[string]json.RawMessage
	if kind != "application/json" || json.Unmarshal(raw, &probe) != nil || probe["queries"] == nil {
		return nil, nil, false, nil
	}
	if len(probe) != 1 {
		return nil, nil, true, errors.New("a body with queries has nothing else")
	}
	var named map[string]json.RawMessage
	if err := json.Unmarshal(probe["queries"], &named); err != nil || len(named) == 0 {
		return nil, nil, true, errors.New("queries is an object naming at least one query")
	}
	names := slices.Sorted(maps.Keys(named))
	qs := []*Query{}
	for _, name := range names {
		var text string
		var q *Query
		var err error
		if json.Unmarshal(named[name], &text) == nil {
			q, err = Parse(text)
		} else {
			q, err = ParseJSON(named[name])
		}
		if err != nil {
			return nil, nil, true, fmt.Errorf("queries.%s: %v", name, err)
		}
		qs = append(qs, q)
	}
	return names, qs, true, nil
}

func respond(ctx context.Context, w http.ResponseWriter, r *http.Request, root *trace.Span, out any) {
	_, encoding := trace.Start(ctx, "encode")
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(out); err != nil {
		serve.Error(w, r, err)
		return
	}
	encoding.Set("bytes", buf.Len())
	encoding.End()
	root.End()
	w.Header().Set("Trace", root.Header())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.ErrorContext(r.Context(), "write response", "path", r.URL.Path, "error", err)
	}
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

const (
	ModeHeader  = "Helios-Mode"
	SpoofHeader = "Helios-Spoof"
)

func caller(w http.ResponseWriter, r *http.Request, m *Model, tokens auth.Tokens, at time.Time) (Env, access.Actor, bool) {
	mode := r.Header.Get(ModeHeader)
	token, bearer := auth.Bearer(r)
	if !bearer {
		email := auth.Email(r)
		viewer := m.SignedIn(email)
		if viewer == "" {
			http.Error(w, "not in the directory", http.StatusForbidden)
			return Env{}, access.Actor{}, false
		}
		return Env{Viewer: viewer, Mode: mode, Now: at}, access.Actor{Email: email}, true
	}
	email, _, err := tokens.Verify(token)
	if err != nil {
		http.Error(w, "unusable token: "+err.Error(), http.StatusUnauthorized)
		return Env{}, access.Actor{}, false
	}
	if target := r.Header.Get(SpoofHeader); target != "" {
		if !m.SuperAdmin(email) {
			http.Error(w, "only a super admin may spoof", http.StatusForbidden)
			return Env{}, access.Actor{}, false
		}
		address, _, ok := m.SignedInAs(target)
		if !ok {
			http.Error(w, "nobody in the directory has the address "+target, http.StatusForbidden)
			return Env{}, access.Actor{}, false
		}
		slog.InfoContext(r.Context(), "spoof", "admin", email, "as", address)
		email = address
	}
	return Env{Viewer: m.SignedIn(email), Mode: mode, Now: at}, access.Actor{Email: email}, true
}

func Register(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, tokens auth.Tokens, now func() time.Time) {
	registerDo(mux, s, queue, pics, tokens, now)
	registerBlobs(mux, s, pics, tokens, now)
	registerExplain(mux, s, tokens, now)
	mux.HandleFunc("GET /api/openapi.json", openapi)
	mux.HandleFunc("QUERY /api/q", func(w http.ResponseWriter, r *http.Request) {
		root := trace.New("request")
		ctx := trace.With(r.Context(), root)
		m := s.Model()
		env, _, ok := caller(w, r, m, tokens, now())
		if !ok {
			return
		}
		kind, raw, ok := body(w, r, queryLimit)
		if !ok {
			return
		}
		_, parsing := trace.Start(ctx, "parse")
		if names, qs, ok, err := parseAll(kind, raw); ok {
			parsing.End()
			if err != nil {
				serve.Error(w, r, access.Invalid("%v", err))
				return
			}
			out := answerAll{Now: env.Now.Format(cells.StampFormat), Results: map[string][]string{}, Resources: map[string]map[string]store.Row{}}
			for i, result := range m.RunAll(ctx, qs, env) {
				slog.InfoContext(r.Context(), "query", "viewer", env.Viewer, "system", env.System, "name", names[i], "query", qs[i].tree.flat())
				out.Results[names[i]] = result.IDs
				for table, rows := range result.Resources {
					for _, row := range rows {
						merge(out.Resources, table, row)
					}
				}
			}
			respond(ctx, w, r, root, out)
			return
		}
		var q *Query
		var err error
		if kind == "text/plain" {
			q, err = Parse(string(raw))
		} else {
			q, err = ParseJSON(raw)
		}
		parsing.End()
		if err != nil {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		slog.InfoContext(r.Context(), "query", "viewer", env.Viewer, "system", env.System, "query", q.tree.flat())
		result := m.Run(ctx, q, env)
		out := answer{
			Now:       env.Now.Format(cells.StampFormat),
			Query:     q.String(),
			Result:    result.IDs,
			Resources: result.Resources,
		}
		if kind == "text/plain" {
			out.Tree = q.Tree()
		}
		respond(ctx, w, r, root, out)
	})
	mux.HandleFunc("POST /api/q", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens, now())
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
		root := trace.New("request")
		ids, err := Write(trace.With(r.Context(), root), s, queue, pics, actor, env, b)
		root.End()
		w.Header().Set("Trace", root.Header())
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, written{Result: ids})
	})
}

func (m *Model) PersonOf(email string) string {
	row, ok := m.Table("PERSON_EMAIL").Find(strings.ToLower(strings.TrimSpace(email)), "No")
	if !ok {
		return ""
	}
	return row["person"]
}

func (m *Model) SignedIn(email string) string {
	address := strings.ToLower(strings.TrimSpace(email))
	row, ok := m.Table("PERSON_EMAIL").Find(address, "No")
	if !ok {
		row, ok = m.Table("PERSON_EMAIL").Find(address, "Yes")
	}
	if !ok {
		return ""
	}
	p, ok := m.Shown("PERSON").Get(row["person"])
	if !ok || p["deactivated"] != "" {
		return ""
	}
	if hidden, _ := cells.YesNo(p["hidden"], false); hidden {
		return ""
	}
	return row["person"]
}

func (m *Model) SignedInAs(email string) (string, string, bool) {
	person := m.SignedIn(email)
	if person == "" {
		return "", "", false
	}
	p, _ := m.Shown("PERSON").Get(person)
	for _, e := range m.Table("PERSON_EMAIL").Referencing("person", person) {
		if primary, _ := cells.YesNo(e["primary"], false); primary {
			return e["address"], p["name_show"], true
		}
	}
	return "", "", false
}

func (m *Model) SuperAdmin(email string) bool {
	return m.superAdmin(m.SignedIn(email))
}
