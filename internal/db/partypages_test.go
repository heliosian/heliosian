package db

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/testkit"
)

func TestPartyPreviewsAndShareCards(t *testing.T) {
	s, _ := sampleWithQueue(t)
	pages := NewPartyPages(s, blob.NewMemoryBucket(), PartyCardStyle(func() string { return "Helios Celebrate" }, func() string { return "Fun(d)raiser Parties" }))
	mux := http.NewServeMux()
	pages.Register(mux)
	t.Chdir("../..")
	testkit.Previews(t, pages.PreviewHead,
		testkit.Preview{URL: "https://celebrate.heliosian.com/g/taco-night", Want: []string{
			`og:title" content="Taco Night"`, `og:url" content="https://celebrate.heliosian.com/g/taco-night"`,
			`og:image" content="https://celebrate.heliosian.com/open/share/` + tacoNight + `.png"`, `Saturday, March 6 · 6:00 – 9:00 PM — Montara — A taco bar`}},
		testkit.Preview{URL: "https://celebrate.heliosian.com/g/" + tacoNight, Want: []string{`og:title" content="Taco Night"`}},
		testkit.Preview{URL: "https://celebrate.heliosian.com/my", Want: []string{
			`og:title" content="Upcoming Parties"`, `og:image" content="https://celebrate.heliosian.com/open/share/upcoming.png"`,
			`Next up: Taco Night — Saturday, March 6 · 6:00 – 9:00 PM — Montara`}},
	)
	testkit.Cards(t, mux, []string{"/open/share/" + tacoNight + ".png", "/open/share/upcoming.png"}, "/open/share/"+celebration+".png", "/open/share/grp00000000040.png")
}

func TestPartyAddressesRedirectAndServeTheShell(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Insert: "ALIAS", Row: map[string]any{"alias": "P001", "target": tacoNight, "hidden": true}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "celebrate", "old": "/parties/ha725m8r", "new": "/g/taco-night"}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "team", "old": "/parties/teamonly", "new": "/g/taco-night"}},
	})
	pages := NewPartyPages(s, blob.NewMemoryBucket(), PartyCardStyle(func() string { return "Helios Celebrate" }, func() string { return "Fun(d)raiser Parties" }))
	handler := pages.Redirected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for _, c := range []struct {
		path   string
		status int
		to     string
	}{
		{"/g/P001", http.StatusMovedPermanently, "https://example.com/g/taco-night"},
		{"/g/P001?x=1", http.StatusMovedPermanently, "https://example.com/g/taco-night?x=1"},
		{"/g/" + tacoNight, http.StatusMovedPermanently, "https://example.com/g/taco-night"},
		{"/g/Taco-Night/", http.StatusMovedPermanently, "https://example.com/g/taco-night"},
		{"/parties/ha725m8r", http.StatusFound, "https://example.com/g/taco-night"},
		{"/parties/HA725M8R/", http.StatusFound, "https://example.com/g/taco-night"},
		{"/g/taco-night", http.StatusTeapot, ""},
		{"/p/taco-night", http.StatusTeapot, ""},
		{"/parties/teamonly", http.StatusTeapot, ""},
		{"/g/" + celebration, http.StatusTeapot, ""},
		{"/g/nothing", http.StatusTeapot, ""},
		{"/celebrations/sc-2026", http.StatusTeapot, ""},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.status || rec.Header().Get("Location") != c.to {
			t.Errorf("%s: %d %q, want %d %q", c.path, rec.Code, rec.Header().Get("Location"), c.status, c.to)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/g/P001", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("a POST is redirected: %d", rec.Code)
	}

	mux := http.NewServeMux()
	pages.Register(mux)
	t.Chdir("../..")
	for _, path := range []string{"/", "/g/taco-night", "/celebrations/sc-2026", "/my", "/my/rowan", "/hosting", "/admin"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s: %d, not the page", path, rec.Code)
		}
	}
}

func TestDescriptionsLoseTheirChipsAndCalloutMarkers(t *testing.T) {
	description := "> [!chip] Parents & Kids\n\nA cozy evening of fondue.\n\n> [!note] 📣 Good to know\n> Drop-off is okay."
	if got, want := withoutMarkers(description), "A cozy evening of fondue.\n\n> 📣 Good to know\n> Drop-off is okay."; got != want {
		t.Errorf("withoutMarkers = %q, want %q", got, want)
	}
	if got, want := blurb(description), "A cozy evening of fondue. 📣 Good to know Drop-off is okay."; got != want {
		t.Errorf("blurb = %q, want %q", got, want)
	}
}
