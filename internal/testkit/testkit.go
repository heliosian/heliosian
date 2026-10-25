package testkit

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

func Rows(t *testing.T, dir *data.Dir, s interface{ Flush() }, app, tab string) []store.Row {
	t.Helper()
	s.Flush()
	_, rows, err := dir.Table(app, tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

type Preview struct {
	URL   string
	Want  []string
	Never []string
}

func Previews(t *testing.T, head func(*http.Request) string, previews ...Preview) {
	t.Helper()
	for _, p := range previews {
		got := head(httptest.NewRequest(http.MethodGet, p.URL, nil))
		for _, want := range p.Want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: preview lacks %s:\n%s", p.URL, want, got)
			}
		}
		for _, never := range p.Never {
			if strings.Contains(got, never) {
				t.Errorf("%s: preview carries %s:\n%s", p.URL, never, got)
			}
		}
	}
}

func Cards(t *testing.T, handler http.Handler, found []string, missing ...string) {
	t.Helper()
	for _, path := range found {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Header().Get("Content-Type"))
			continue
		}
		img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != 1200 || b.Dy() != 630 {
			t.Errorf("%s: card is %v, want 1200x630", path, b)
		}
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
		again := httptest.NewRecorder()
		handler.ServeHTTP(again, req)
		if again.Code != http.StatusNotModified {
			t.Errorf("%s again with its ETag: %d", path, again.Code)
		}
	}
	for _, path := range missing {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
}
