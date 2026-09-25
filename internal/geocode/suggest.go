package geocode

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/lru"
	"heliosian/internal/ratelimit"
)

const (
	suggestPerMinute = 60
	suggestCached    = 2000
)

type Suggester interface {
	Suggest(input string) ([]Suggestion, error)
}

func (Fake) Suggest(input string) ([]Suggestion, error) {
	all := []string{
		"865 Waverley St, Palo Alto, CA 94301, USA",
		"88 Castro St, Mountain View, CA 94041, USA",
		"1420 Alder Ct, Los Altos, CA 94024, USA",
		"600 W Middlefield Rd, Mountain View, CA 94043, USA",
		"Rinconada Park, 777 Embarcadero Rd, Palo Alto, CA 94303, USA",
		"Mitchell Park, 600 E Meadow Dr, Palo Alto, CA 94306, USA",
	}
	out := []Suggestion{}
	q := strings.ToLower(strings.TrimSpace(input))
	for _, a := range all {
		if q == "" || strings.Contains(strings.ToLower(a), q) {
			out = append(out, Suggestion{Text: a})
		}
	}
	if len(out) == 0 {
		for _, a := range all[:3] {
			out = append(out, Suggestion{Text: a})
		}
	}
	return out, nil
}

type Suggestions struct {
	suggester Suggester
	recent    *ratelimit.Limiter
	cache     *lru.Cache[string, []Suggestion]
}

func NewSuggestions(s Suggester) *Suggestions {
	return &Suggestions{suggester: s, recent: ratelimit.New(suggestPerMinute, time.Minute), cache: lru.New[string, []Suggestion](suggestCached)}
}

func (s *Suggestions) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/address/suggest", s.serve)
}

func (s *Suggestions) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	q := strings.ToLower(strings.Join(strings.Fields(r.URL.Query().Get("q")), " "))
	if len(q) < 3 || len(q) > 200 {
		w.Write([]byte("[]"))
		return
	}
	if out, ok := s.cache.Get(q); ok {
		json.NewEncoder(w).Encode(out)
		return
	}
	email := strings.ToLower(auth.RealEmail(r))
	if !s.recent.Allow(email, time.Now()) {
		slog.WarnContext(r.Context(), "address suggestions: over the limit", "email", email)
		w.Write([]byte("[]"))
		return
	}
	out, err := s.suggester.Suggest(q)
	if err != nil {
		slog.WarnContext(r.Context(), "address suggestions", "error", err)
		w.Write([]byte("[]"))
		return
	}
	if len(out) > 5 {
		out = out[:5]
	}
	s.cache.Put(q, out)
	json.NewEncoder(w).Encode(out)
}
