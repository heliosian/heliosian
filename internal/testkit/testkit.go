package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

func Actors(r *http.Request, held func(string) []access.Allowance) access.Actor {
	email := strings.ToLower(auth.Email(r))
	return access.Actor{Email: email, Allowances: access.Grant(held(email))}
}

type Images func(key string) bool

func (i Images) Has(key string) (bool, error) { return i(key), nil }

func (Images) Prefetch(context.Context, []string) error { return nil }

var (
	None = Images(func(string) bool { return false })
	All  = Images(func(string) bool { return true })
)

func Only(keys ...string) Images {
	present := map[string]bool{}
	for _, key := range keys {
		present[key] = true
	}
	return func(key string) bool { return present[key] }
}

func Files(root string) Images {
	return func(key string) bool {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
		return err == nil && info.Mode().IsRegular()
	}
}

func Call(t *testing.T, handler http.Handler, as, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	auth.Fixed(as, handler).ServeHTTP(rec, r)
	return rec
}

func Form(t *testing.T, handler http.Handler, as, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	auth.Fixed(as, handler).ServeHTTP(rec, r)
	return rec
}

func Status(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return http.StatusOK
	}
	var refusal *access.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("not a refusal: %v", err)
	}
	return refusal.Status
}

func Rows(t *testing.T, dir *data.Dir, queue *store.Queue, app, tab string) []store.Row {
	t.Helper()
	queue.Flush()
	_, rows, err := dir.Table(app, tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func Tables(t *testing.T, dir *data.Dir, queue *store.Queue, app string, names ...string) store.Tables {
	t.Helper()
	out := store.Tables{}
	for _, name := range names {
		out[name] = Rows(t, dir, queue, app, name)
	}
	return out
}

func ChangeLog(t *testing.T, dir *data.Dir, queue *store.Queue, app string) []store.Row {
	t.Helper()
	return Rows(t, dir, queue, app, store.ChangeLogTab)
}

func ChangeLines(t *testing.T, dir *data.Dir, queue *store.Queue, app string) []string {
	t.Helper()
	out := []string{}
	for _, row := range ChangeLog(t, dir, queue, app) {
		out = append(out, row["Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"]+"|"+row["Previous"])
	}
	return out
}

func MustTime(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
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
