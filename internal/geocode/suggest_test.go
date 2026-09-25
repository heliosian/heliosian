package geocode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"heliosian/internal/auth"
)

type counting struct {
	calls int
}

func (c *counting) Suggest(input string) ([]Suggestion, error) {
	c.calls++
	return []Suggestion{{Text: input + " St"}}, nil
}

func suggest(t *testing.T, h http.Handler, q string) []Suggestion {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/address/suggest?q="+url.QueryEscape(q), nil))
	out := []Suggestion{}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return out
}

func TestSuggestCachesAndLimits(t *testing.T) {
	c := &counting{}
	mux := http.NewServeMux()
	NewSuggestions(c).Register(mux)
	h := auth.Fixed("jordan@example.org", mux)
	for range 3 {
		if got := suggest(t, h, "  Waverley  "); len(got) != 1 || got[0].Text != "waverley St" {
			t.Fatalf("got %+v", got)
		}
	}
	if c.calls != 1 {
		t.Fatalf("a repeated query asked %d times", c.calls)
	}
	for i := range suggestPerMinute - 1 {
		suggest(t, h, "street "+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if c.calls != suggestPerMinute {
		t.Fatalf("calls = %d", c.calls)
	}
	if got := suggest(t, h, "one more street"); len(got) != 0 {
		t.Errorf("past the limit got %+v", got)
	}
	if got := suggest(t, h, "waverley"); len(got) != 1 {
		t.Errorf("a cached query past the limit got %+v", got)
	}
	if c.calls != suggestPerMinute {
		t.Errorf("calls past the limit = %d", c.calls)
	}
}
