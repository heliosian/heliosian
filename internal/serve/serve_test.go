package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
