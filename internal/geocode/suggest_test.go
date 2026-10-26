package geocode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"heliosian/internal/intercept"
	"heliosian/internal/testkit/authtest"
)

type counting struct {
	mu    sync.Mutex
	calls int
}

func (c *counting) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{map[string]any{"placePrediction": map[string]any{"text": map[string]string{"text": req.Input + " St"}}}}})
}

func (c *counting) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
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
	intercept.Install(intercept.PlacesHost, c)
	mux := http.NewServeMux()
	NewSuggestions(New("test")).Register(mux)
	h := authtest.Fixed("jordan@example.org", mux)
	for range 3 {
		if got := suggest(t, h, "  Waverley  "); len(got) != 1 || got[0].Text != "waverley St" {
			t.Fatalf("got %+v", got)
		}
	}
	if c.count() != 1 {
		t.Fatalf("a repeated query asked %d times", c.count())
	}
	for i := range suggestPerMinute - 1 {
		suggest(t, h, "street "+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if c.count() != suggestPerMinute {
		t.Fatalf("calls = %d", c.count())
	}
	if got := suggest(t, h, "one more street"); len(got) != 0 {
		t.Errorf("past the limit got %+v", got)
	}
	if got := suggest(t, h, "waverley"); len(got) != 1 {
		t.Errorf("a cached query past the limit got %+v", got)
	}
	if c.count() != suggestPerMinute {
		t.Errorf("calls past the limit = %d", c.count())
	}
}

func TestCanceledSuggestionReturns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	intercept.Install(intercept.PlacesHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		close(started)
		<-release
		json.NewEncoder(w).Encode(map[string]any{"suggestions": []any{}})
	}))
	mux := http.NewServeMux()
	NewSuggestions(New("test")).Register(mux)
	h := authtest.Fixed("jordan@example.org", mux)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/address/suggest?q=waverley", nil).WithContext(ctx))
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the Places request never started")
	}
	at := time.Now()
	cancel()
	select {
	case <-done:
		t.Logf("canceled suggestion returned in %s", time.Since(at))
	case <-time.After(time.Second):
		t.Fatalf("canceled suggestion still waiting on Places after %s", time.Since(at))
	}
}
