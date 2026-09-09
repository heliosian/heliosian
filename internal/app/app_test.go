package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func serve(t *testing.T, handler http.Handler, host, path string) *httptest.ResponseRecorder {
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
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/admin", "/../go.mod", "/missing.js", "/manifest.webmanifest", "/fonts/fonts.css"} {
		before := fallthroughs
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
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
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/style.css", "/app.js", "/brand/logo-wordmark.png", "/brand/classrooms/grade-k.jpg"} {
		before := fallthroughs
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
			t.Errorf("%s: got %d, want fallthrough", path, rec.Code)
		}
	}
}

func TestAppFor(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":                       "who",
		"who.lab.heliosian.com":                   "who",
		"who.local.heliosian.com":                 "who",
		"heliosian-489539474126.us-west1.run.app": "who",
		"home.heliosian.com":                      "home",
		"home.lab.heliosian.com":                  "home",
		"home.local.heliosian.com":                "home",
		"hca.heliosian.com":                       "hca",
		"hca.lab.heliosian.com":                   "hca",
		"hca.local.heliosian.com":                 "hca",
		"heliosian.com":                           "home",
		"www.heliosian.com":                       "home",
		"localhost":                               "",
		"lab.heliosian.com":                       "lab",
		"who.staging.heliosian.com":               "",
		"who.lab.local.heliosian.com":             "",
		"who.heliosian.com.evil.example":          "",
	}
	for host, want := range cases {
		if got := appFor(host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
}

func TestRoute(t *testing.T) {
	handler := route(map[string]http.Handler{"who": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})})
	for _, host := range []string{"who.local.heliosian.com:8080", "who.lab.heliosian.com", "who.heliosian.com"} {
		if rec := serve(t, handler, host, "/people"); rec.Code != http.StatusTeapot {
			t.Errorf("%s: got %d, want routed", host, rec.Code)
		}
	}
	for _, host := range []string{"localhost:8080", "heliosian.com", "home.local.heliosian.com", "lab.heliosian.com"} {
		if rec := serve(t, handler, host, "/people"); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", host, rec.Code)
		}
	}
}
