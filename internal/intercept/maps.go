package intercept

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	GeocodeHost = "geocode.googleapis.com"
	PlacesHost  = "places.googleapis.com"
)

var sampleAddresses = []string{
	"865 Waverley St, Palo Alto, CA 94301, USA",
	"88 Castro St, Mountain View, CA 94041, USA",
	"1420 Alder Ct, Los Altos, CA 94024, USA",
	"600 W Middlefield Rd, Mountain View, CA 94043, USA",
	"Rinconada Park, 777 Embarcadero Rd, Palo Alto, CA 94303, USA",
	"Mitchell Park, 600 E Meadow Dr, Palo Alto, CA 94306, USA",
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func Geocode() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v4/geocode/address" {
			http.Error(w, "intercept answers only /v4/geocode/address", http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256([]byte(r.Form.Get("addressQuery")))
		location := map[string]float64{
			"latitude":  37.5 + (float64(sum[0])/255-0.5)*0.12,
			"longitude": -122.45 + (float64(sum[1])/255-0.5)*0.12,
		}
		writeJSON(w, map[string]any{"results": []any{map[string]any{"location": location}}})
	})
}

func Places() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/places:autocomplete" {
			http.Error(w, "intercept answers only /v1/places:autocomplete", http.StatusNotFound)
			return
		}
		var req struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q := strings.ToLower(strings.TrimSpace(req.Input))
		found := []string{}
		for _, a := range sampleAddresses {
			if q == "" || strings.Contains(strings.ToLower(a), q) {
				found = append(found, a)
			}
		}
		if len(found) == 0 {
			found = sampleAddresses[:3]
		}
		suggestions := []any{}
		for _, a := range found {
			suggestions = append(suggestions, map[string]any{"placePrediction": map[string]any{"text": map[string]string{"text": a}}})
		}
		writeJSON(w, map[string]any{"suggestions": suggestions})
	})
}
