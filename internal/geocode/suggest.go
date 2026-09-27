package geocode

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/lru"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
)

const (
	suggestPerMinute = 60
	suggestCached    = 2000
)

type Suggestions struct {
	client *Client
	recent *ratelimit.Limiter
	cache  *lru.Cache[string, []Suggestion]
}

func NewSuggestions(c *Client) *Suggestions {
	return &Suggestions{client: c, recent: ratelimit.New(suggestPerMinute, time.Minute), cache: lru.New[string, []Suggestion](suggestCached)}
}

func (s *Suggestions) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/address/suggest", s.serve)
}

func (s *Suggestions) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, http.StatusOK, s.suggest(r))
}

func (s *Suggestions) suggest(r *http.Request) []Suggestion {
	q := strings.ToLower(strings.Join(strings.Fields(r.URL.Query().Get("q")), " "))
	if len(q) < 3 || len(q) > 200 {
		return []Suggestion{}
	}
	if out, ok := s.cache.Get(q); ok {
		return out
	}
	email := strings.ToLower(auth.RealEmail(r))
	if !s.recent.Allow(email, time.Now()) {
		slog.WarnContext(r.Context(), "address suggestions: over the limit", "email", email)
		return []Suggestion{}
	}
	out, err := s.client.Suggest(q)
	if err != nil {
		slog.WarnContext(r.Context(), "address suggestions", "error", err)
		return []Suggestion{}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	s.cache.Put(q, out)
	return out
}
