package app

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func get(t *testing.T, handler http.Handler, host, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestFiles(t *testing.T) {
	t.Chdir("../..")
	fallthroughs := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallthroughs++
		w.WriteHeader(http.StatusTeapot)
	})
	handler := Files("who", next)
	for _, path := range []string{"/style.css", "/brand/logo-wordmark.png", "/brand/classrooms/grade-k.jpg"} {
		if rec := get(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/admin", "/../go.mod", "/missing.js", "/manifest.webmanifest", "/fonts/fonts.css"} {
		before := fallthroughs
		if rec := get(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
			t.Errorf("%s: got %d, want fallthrough", path, rec.Code)
		}
	}
}

func TestPublic(t *testing.T) {
	t.Chdir("../..")
	fallthroughs := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallthroughs++
		w.WriteHeader(http.StatusTeapot)
	})
	handler := Public("who", next)
	for _, path := range []string{"/manifest.webmanifest", "/brand/icon-192.png", "/brand/login-art.jpg", "/brand/splash/splash-390x844-3x-portrait.png", "/fonts/fonts.css", "/fonts/Roboto-Regular.woff2"} {
		if rec := get(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/style.css", "/app.js", "/brand/logo-wordmark.png", "/brand/classrooms/grade-k.jpg"} {
		before := fallthroughs
		if rec := get(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
			t.Errorf("%s: got %d, want fallthrough", path, rec.Code)
		}
	}
}

func TestAppFor(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":              "who",
		"who.local.heliosian.com":        "who",
		"home.heliosian.com":             "home",
		"home.local.heliosian.com":       "home",
		"team.heliosian.com":             "team",
		"team.local.heliosian.com":       "team",
		"hca.heliosian.com":              "team",
		"hca.local.heliosian.com":        "team",
		"calendar.heliosian.com":         "calendar",
		"cal.heliosian.com":              "calendar",
		"when.local.heliosian.com":       "calendar",
		"heliosian.com":                  "home",
		"www.heliosian.com":              "home",
		"localhost":                      "",
		"who.lab.heliosian.com":          "",
		"hca.lab.heliosian.com":          "",
		"who.staging.heliosian.com":      "",
		"who.lab.local.heliosian.com":    "",
		"who.heliosian.com.evil.example": "",
	}
	for host, want := range cases {
		if got := appFor(host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
}

// TestHostnamesCoverTheRouter pins the mapping list to what appFor answers:
// every hostname listed routes to an app, and every app and alias the router
// knows is listed in production.
func TestHostnamesCoverTheRouter(t *testing.T) {
	hosts := Hostnames()
	for _, host := range hosts {
		if appFor(host) == "" {
			t.Errorf("%s is listed but routes nowhere", host)
		}
	}
	for _, want := range []string{"heliosian.com", "www.heliosian.com", "home.heliosian.com", "who.heliosian.com", "hca.heliosian.com", "when.heliosian.com", "cal.heliosian.com", "loop.heliosian.com"} {
		if !slices.Contains(hosts, want) {
			t.Errorf("%s is not listed", want)
		}
	}
	for _, never := range []string{"who.local.heliosian.com", "who.lab.heliosian.com"} {
		if slices.Contains(hosts, never) {
			t.Errorf("%s is listed", never)
		}
	}
}

func TestRoute(t *testing.T) {
	handler := route(map[string]http.Handler{"who": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})})
	for _, host := range []string{"who.local.heliosian.com:8080", "who.heliosian.com"} {
		if rec := get(t, handler, host, "/people"); rec.Code != http.StatusTeapot {
			t.Errorf("%s: got %d, want routed", host, rec.Code)
		}
	}
	for _, host := range []string{"localhost:8080", "heliosian.com", "home.local.heliosian.com", "who.lab.heliosian.com"} {
		if rec := get(t, handler, host, "/people"); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", host, rec.Code)
		}
	}
}

func TestCanonicalHost(t *testing.T) {
	cases := map[string]string{
		"calendar.heliosian.com":      "when.heliosian.com",
		"cal.heliosian.com":           "when.heliosian.com",
		"cal.local.heliosian.com":     "when.local.heliosian.com",
		"when.heliosian.com":          "",
		"who.heliosian.com":           "",
		"hca.heliosian.com":           "",
		"heliosian.com":               "",
		"calendar.heliosian.com.evil": "",
	}
	for host, want := range cases {
		if got := canonicalHost(host); got != want {
			t.Errorf("canonicalHost(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestRouteSendsAliasesToWhen(t *testing.T) {
	served := false
	apps := map[string]http.Handler{"calendar": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { served = true })}
	handler := route(apps)
	for _, c := range []struct {
		method, host, path string
		want               int
		location           string
	}{
		{"GET", "calendar.heliosian.com", "/day/2026-09-13?x=1", 301, "https://when.heliosian.com/day/2026-09-13?x=1"},
		{"GET", "cal.local.heliosian.com:8080", "/", 301, "https://when.local.heliosian.com:8080/"},
		{"GET", "calendar.heliosian.com", "/open/feed/abc.ics", 200, ""},
		{"GET", "calendar.heliosian.com", "/api/calendar/model", 200, ""},
		{"POST", "calendar.heliosian.com", "/api/calendar/x", 200, ""},
		{"GET", "when.heliosian.com", "/", 200, ""},
	} {
		served = false
		r := httptest.NewRequest(c.method, "https://"+c.host+c.path, nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != c.want || (c.want == 200) != served {
			t.Errorf("%s %s%s: got %d (served %v), want %d", c.method, c.host, c.path, w.Code, served, c.want)
		}
		if got := w.Header().Get("Location"); got != c.location {
			t.Errorf("%s %s%s: location %q, want %q", c.method, c.host, c.path, got, c.location)
		}
	}
}
