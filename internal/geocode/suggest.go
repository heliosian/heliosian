package geocode

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// Suggester offers whole addresses for a few typed characters: Google's
// Places behind the real client, a handful of made-up ones behind the
// fake, so the sample server's boxes suggest too.
type Suggester interface {
	Suggest(input string) ([]Suggestion, error)
}

// Suggest on the fake is a few Bay Area addresses that contain what was
// typed, or the first few when nothing does, so a sample page shows the
// picker at work.
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

// RegisterSuggest serves GET /api/address/suggest?q= on an app: the
// suggestions for what was typed, five at most, an empty list for fewer
// than three characters or when the service is not set up. Behind
// sign-in, as every app's API is.
func RegisterSuggest(mux *http.ServeMux, s Suggester) {
	mux.HandleFunc("GET /api/address/suggest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if s == nil || len(q) < 3 || len(q) > 200 {
			w.Write([]byte("[]"))
			return
		}
		out, err := s.Suggest(q)
		if err != nil {
			slog.WarnContext(r.Context(), "address suggestions", "error", err)
			w.Write([]byte("[]"))
			return
		}
		if len(out) > 5 {
			out = out[:5]
		}
		json.NewEncoder(w).Encode(out)
	})
}
