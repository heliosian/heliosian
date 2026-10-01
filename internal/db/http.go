package db

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const queryLimit = 64 << 10

type answer struct {
	Now       string                          `json:"now"`
	Query     string                          `json:"query"`
	Result    []string                        `json:"result"`
	Resources map[string]map[string]store.Row `json:"resources"`
}

func Register(mux *http.ServeMux, s *Store, now func() time.Time) {
	mux.HandleFunc("QUERY /api/q", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, queryLimit))
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "query too long", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		q, err := Parse(string(raw))
		if err != nil {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		m := s.Model()
		viewer := m.PersonOf(auth.Email(r))
		at := now()
		slog.InfoContext(r.Context(), "query", "viewer", viewer, "query", q.tree.flat())
		result := m.Run(q, Env{Viewer: viewer, Now: at})
		serve.Write(w, r, http.StatusOK, answer{
			Now:       at.Format("2006-01-02 15:04"),
			Query:     q.String(),
			Result:    result.IDs,
			Resources: result.Resources,
		})
	})
}

func (m *Model) PersonOf(email string) string {
	row, ok := m.Table("PERSON_EMAIL").Find(strings.ToLower(strings.TrimSpace(email)))
	if !ok {
		return ""
	}
	return row["person"]
}
