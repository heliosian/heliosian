package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func TestJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	echo := JSON(func(r *http.Request, body in) (map[string]string, error) {
		switch body.Name {
		case "refuse":
			return nil, access.Forbidden("no %s", body.Name)
		case "conflict":
			return nil, &access.Refusal{Status: http.StatusConflict, Body: map[string]string{"id": "x"}}
		case "broken":
			return nil, errors.New("sheet is missing a tab")
		}
		return map[string]string{"hello": body.Name}, nil
	})
	quiet := JSON(func(r *http.Request, body None) (None, error) {
		return None{}, nil
	})
	cases := []struct {
		name    string
		handler http.HandlerFunc
		body    string
		code    int
		want    string
	}{
		{"echo", echo, `{"name":"sam"}`, http.StatusOK, `{"hello":"sam"}` + "\n"},
		{"refusal", echo, `{"name":"refuse"}`, http.StatusForbidden, "no refuse\n"},
		{"conflict body", echo, `{"name":"conflict"}`, http.StatusConflict, `{"id":"x"}` + "\n"},
		{"plain error", echo, `{"name":"broken"}`, http.StatusInternalServerError, "internal error\n"},
		{"bad body", echo, `{`, http.StatusBadRequest, "bad request body\n"},
		{"too large", echo, `{"name":"` + strings.Repeat("a", bodyLimit) + `"}`, http.StatusRequestEntityTooLarge, "request body too large\n"},
		{"no body", quiet, "", http.StatusNoContent, ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		c.handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)))
		if rec.Code != c.code || rec.Body.String() != c.want {
			t.Errorf("%s: got %d %q, want %d %q", c.name, rec.Code, rec.Body.String(), c.code, c.want)
		}
	}
}

func TestFileMatchesOnlyIdenticalBytes(t *testing.T) {
	dir := t.TempDir()
	splash := filepath.Join(dir, "login.html")
	shell := filepath.Join(dir, "index.html")
	if err := os.WriteFile(splash, []byte("<p>sign in</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shell, []byte("<p>welcome</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	File(first, httptest.NewRequest(http.MethodGet, "/", nil), splash)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("first response: %d etag %q", first.Code, etag)
	}
	if first.Header().Get("Last-Modified") != "" {
		t.Errorf("last-modified sent: %q", first.Header().Get("Last-Modified"))
	}

	same := httptest.NewRequest(http.MethodGet, "/", nil)
	same.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	File(rec, same, splash)
	if rec.Code != http.StatusNotModified {
		t.Errorf("same bytes: got %d, want 304", rec.Code)
	}

	other := httptest.NewRequest(http.MethodGet, "/", nil)
	other.Header.Set("If-None-Match", etag)
	other.Header.Set("If-Modified-Since", "Thu, 01 Jan 2099 00:00:00 GMT")
	rec = httptest.NewRecorder()
	File(rec, other, shell)
	if rec.Code != http.StatusOK || rec.Body.String() != "<p>welcome</p>" {
		t.Errorf("different bytes: got %d %q, want 200 with the new body", rec.Code, rec.Body.String())
	}
}
