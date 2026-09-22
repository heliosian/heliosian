package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
		if rec := get(t, handler, "who.heliosiandev.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/admin", "/../go.mod", "/missing.js", "/manifest.webmanifest", "/fonts/fonts.css"} {
		before := fallthroughs
		if rec := get(t, handler, "who.heliosiandev.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
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
	for _, path := range []string{"/theme.js", "/manifest.webmanifest", "/brand/icon-192.png", "/brand/login-art.jpg", "/brand/splash/splash-390x844-3x-portrait.png", "/fonts/fonts.css", "/fonts/Roboto-Regular.woff2"} {
		if rec := get(t, handler, "who.heliosiandev.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/style.css", "/app.js", "/brand/logo-wordmark.png", "/brand/classrooms/grade-k.jpg"} {
		before := fallthroughs
		if rec := get(t, handler, "who.heliosiandev.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
			t.Errorf("%s: got %d, want fallthrough", path, rec.Code)
		}
	}
}

// A server answers its own domain's names and nothing else: production's
// never a developer's, a developer's never production's.
func TestAppFor(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":              "who",
		"home.heliosian.com":             "home",
		"team.heliosian.com":             "team",
		"hca.heliosian.com":              "team",
		"calendar.heliosian.com":         "calendar",
		"cal.heliosian.com":              "calendar",
		"when.heliosian.com":             "calendar",
		"heliosian.com":                  "home",
		"www.heliosian.com":              "home",
		"localhost":                      "",
		"who.heliosiandev.com":           "",
		"heliosiandev.com":               "",
		"who.lab.heliosian.com":          "",
		"hca.lab.heliosian.com":          "",
		"who.staging.heliosian.com":      "",
		"who.heliosian.com.evil.example": "",
	}
	for host, want := range cases {
		if got := appFor(Domain, host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
	dev := map[string]string{
		"who.heliosiandev.com":      "who",
		"hca.heliosiandev.com":      "team",
		"when.heliosiandev.com":     "calendar",
		"heliosiandev.com":          "home",
		"www.heliosiandev.com":      "home",
		"who.heliosian.com":         "",
		"heliosian.com":             "",
		"who.lab.heliosiandev.com":  "",
		"who.heliosiandev.com.evil": "",
	}
	for host, want := range dev {
		if got := appFor(DevDomain, host); got != want {
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
		if appFor(Domain, host) == "" {
			t.Errorf("%s is listed but routes nowhere", host)
		}
	}
	for _, want := range []string{"heliosian.com", "www.heliosian.com", "home.heliosian.com", "who.heliosian.com", "hca.heliosian.com", "when.heliosian.com", "cal.heliosian.com", "loop.heliosian.com"} {
		if !slices.Contains(hosts, want) {
			t.Errorf("%s is not listed", want)
		}
	}
	for _, never := range []string{"who.heliosiandev.com", "who.lab.heliosian.com"} {
		if slices.Contains(hosts, never) {
			t.Errorf("%s is listed", never)
		}
	}
}

func TestRoute(t *testing.T) {
	handler := route(Domain, map[string]http.Handler{"who": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})})
	for _, host := range []string{"who.heliosian.com:8080", "who.heliosian.com"} {
		if rec := get(t, handler, host, "/people"); rec.Code != http.StatusTeapot {
			t.Errorf("%s: got %d, want routed", host, rec.Code)
		}
	}
	for _, host := range []string{"localhost:8080", "heliosian.com", "who.heliosiandev.com", "who.lab.heliosian.com"} {
		if rec := get(t, handler, host, "/people"); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", host, rec.Code)
		}
	}
}

func TestSecureHeaders(t *testing.T) {
	t.Chdir("../..")
	apps := map[string]http.Handler{"who": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}
	for _, c := range []struct {
		domain, host string
		hsts         string
	}{
		{Domain, "who.heliosian.com", "max-age=31536000; includeSubDomains"},
		{Domain, "localhost:8080", "max-age=31536000; includeSubDomains"},
		{DevDomain, "who.heliosiandev.com", ""},
	} {
		rec := get(t, Server(c.domain, apps).Handler, c.host, "/people")
		h := rec.Header()
		if got := h.Get("Strict-Transport-Security"); got != c.hsts {
			t.Errorf("%s %s: hsts %q, want %q", c.domain, c.host, got, c.hsts)
		}
		for name, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
			"Permissions-Policy":     "camera=(), microphone=(self), geolocation=(), payment=(), usb=()",
		} {
			if got := h.Get(name); got != want {
				t.Errorf("%s %s: %s %q, want %q", c.domain, c.host, name, got, want)
			}
		}
		if got := h.Get("Content-Security-Policy"); !strings.Contains(got, "https://*."+c.domain+":*") || !strings.Contains(got, "report-uri /csp-report") {
			t.Errorf("%s %s: csp %q", c.domain, c.host, got)
		}
	}
}

func TestNoInlineScripts(t *testing.T) {
	t.Chdir("../..")
	csp := policy(Domain)
	pages, err := filepath.Glob("web/*/*.html")
	if err != nil {
		t.Fatal(err)
	}
	public, err := filepath.Glob("web/public/*/*.html")
	if err != nil {
		t.Fatal(err)
	}
	pages = append(pages, public...)
	if len(pages) < 10 {
		t.Fatalf("found only %d shells", len(pages))
	}
	scripts := 0
	for _, name := range pages {
		page, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, tag := range regexp.MustCompile(`<script[^>]*>`).FindAllString(string(page), -1) {
			scripts++
			if !strings.Contains(tag, " src=") {
				t.Errorf("%s: inline script %s", name, tag)
			}
		}
	}
	if scripts == 0 {
		t.Fatal("no scripts found")
	}
	if strings.Contains(csp, "'sha256-") || strings.Contains(csp, "'nonce-") {
		t.Errorf("script hash or nonce in policy: %s", csp)
	}
	if strings.Contains(csp, "'unsafe-inline'") && !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") {
		t.Errorf("unsafe-inline outside style-src: %s", csp)
	}
	if strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("unsafe-eval in policy: %s", csp)
	}
}

func TestReportSink(t *testing.T) {
	t.Chdir("../..")
	handler := Server(Domain, map[string]http.Handler{}).Handler
	req := httptest.NewRequest(http.MethodPost, "https://who.heliosian.com/csp-report", strings.NewReader(`{"csp-report":{"blocked-uri":"https://evil.example"}}`))
	req.Host = "who.heliosian.com"
	req.Header.Set("Content-Type", "application/csp-report")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("report: got %d, want 204", rec.Code)
	}
	if rec := get(t, handler, "who.heliosian.com", "/csp-report"); rec.Code != http.StatusNotFound {
		t.Errorf("GET report path: got %d, want 404", rec.Code)
	}
}

func TestCanonicalHost(t *testing.T) {
	cases := map[string]string{
		"calendar.heliosian.com":      "when.heliosian.com",
		"cal.heliosian.com":           "when.heliosian.com",
		"cal.heliosiandev.com":        "",
		"when.heliosian.com":          "",
		"who.heliosian.com":           "",
		"hca.heliosian.com":           "",
		"heliosian.com":               "",
		"calendar.heliosian.com.evil": "",
	}
	for host, want := range cases {
		if got := canonicalHost(Domain, host); got != want {
			t.Errorf("canonicalHost(%q) = %q, want %q", host, got, want)
		}
	}
	if got := canonicalHost(DevDomain, "cal.heliosiandev.com"); got != "when.heliosiandev.com" {
		t.Errorf("canonicalHost(dev, cal) = %q, want when.heliosiandev.com", got)
	}
}

func TestRouteSendsAliasesToWhen(t *testing.T) {
	served := false
	apps := map[string]http.Handler{"calendar": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { served = true })}
	handler := route(Domain, apps)
	for _, c := range []struct {
		method, host, path string
		want               int
		location           string
	}{
		{"GET", "calendar.heliosian.com", "/day/2026-09-13?x=1", 301, "https://when.heliosian.com/day/2026-09-13?x=1"},
		{"GET", "cal.heliosian.com:8080", "/", 301, "https://when.heliosian.com:8080/"},
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
