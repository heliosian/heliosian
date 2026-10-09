package app

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompressGzipsTextAndLeavesTheRest(t *testing.T) {
	body := strings.Repeat(`{"name":"Juni Ashdown"},`, 200)
	handler := compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
		case "/png":
			w.Header().Set("Content-Type", "image/png")
		case "/stream":
			w.Header().Set("Content-Type", "text/event-stream")
		}
		io.WriteString(w, body)
	}))
	for _, c := range []struct {
		path, accept string
		gzipped      bool
	}{
		{"/json", "gzip, deflate, br", true},
		{"/json", "", false},
		{"/json", "gzip;q=0", false},
		{"/png", "gzip", false},
		{"/stream", "gzip", false},
	} {
		r := httptest.NewRequest(http.MethodGet, c.path, nil)
		if c.accept != "" {
			r.Header.Set("Accept-Encoding", c.accept)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		got := w.Body.String()
		if gzipped := w.Header().Get("Content-Encoding") == "gzip"; gzipped != c.gzipped {
			t.Fatalf("%s with Accept-Encoding %q: gzipped %v, want %v", c.path, c.accept, gzipped, c.gzipped)
		}
		if c.gzipped {
			zr, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(zr)
			if err != nil {
				t.Fatal(err)
			}
			got = string(b)
		}
		if got != body {
			t.Fatalf("%s with Accept-Encoding %q: the body came back changed", c.path, c.accept)
		}
	}
}
