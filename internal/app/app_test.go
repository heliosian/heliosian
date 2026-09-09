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
	handler := files("who", next)
	for _, path := range []string{"/style.css", "/brand/icon-192.png", "/fonts/fonts.css", "/manifest.webmanifest"} {
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/people", "/admin", "/index.html", "/login.html", "/../go.mod", "/missing.js"} {
		before := fallthroughs
		if rec := serve(t, handler, "who.local.heliosian.com", path); rec.Code != http.StatusTeapot || fallthroughs != before+1 {
			t.Errorf("%s: got %d, want fallthrough", path, rec.Code)
		}
	}
}

func TestRoute(t *testing.T) {
	handler := route(map[string]http.Handler{"who": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})})
	for _, host := range []string{"who.local.heliosian.com:8080", "who.heliosian.com", "heliosian-489539474126.us-west1.run.app"} {
		if rec := serve(t, handler, host, "/people"); rec.Code != http.StatusTeapot {
			t.Errorf("%s: got %d, want routed", host, rec.Code)
		}
	}
	for _, host := range []string{"localhost:8080", "heliosian.com", "home.local.heliosian.com"} {
		if rec := serve(t, handler, host, "/people"); rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", host, rec.Code)
		}
	}
}
