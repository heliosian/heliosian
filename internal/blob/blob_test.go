package blob

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDiskCacheRoundTrips(t *testing.T) {
	s := &Store{cacheDir: t.TempDir(), entries: map[string]*entry{}}
	photo := &entry{name: "photos/abc.jpg", generation: 7, mimeType: "image/jpeg", data: []byte("photo"), thumb: []byte("photo-thumb")}
	if err := s.cache(photo); err != nil {
		t.Fatal(err)
	}
	got, err := s.cached("photos/abc.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.generation != 7 || got.mimeType != "image/jpeg" || !bytes.Equal(got.data, photo.data) || !bytes.Equal(got.thumb, photo.thumb) {
		t.Fatalf("photo: %+v", got)
	}
	audio := &entry{name: "pronunciation/def.m4a", generation: 3, mimeType: "audio/mp4", data: []byte("say it")}
	if err := s.cache(audio); err != nil {
		t.Fatal(err)
	}
	if got, err = s.cached("pronunciation/def.m4a"); err != nil || got == nil || got.thumb != nil || !bytes.Equal(got.data, audio.data) {
		t.Fatalf("audio: %+v %v", got, err)
	}
	if got, err = s.cached("photos/missing.jpg"); err != nil || got != nil {
		t.Fatalf("missing: %+v %v", got, err)
	}
	tile := &entry{name: "grade-images/grade-k", generation: 9, mimeType: "image/png", data: []byte("tile"), thumb: []byte("tile-thumb")}
	if err := s.cache(tile); err != nil {
		t.Fatal(err)
	}
	if got, err = s.cached("grade-images/grade-k"); err != nil || got != nil {
		t.Fatalf("a named object was cached: %+v %v", got, err)
	}
	if _, err := os.Stat(s.cachePath("grade-images/grade-k")); !os.IsNotExist(err) {
		t.Fatal("a named object was written to the cache")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.cache(photo)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent writers: %v", err)
		}
	}
	if got, err = s.cached("photos/abc.jpg"); err != nil || got == nil || !bytes.Equal(got.data, photo.data) {
		t.Fatalf("after concurrent writers: %+v %v", got, err)
	}
	leftovers, err := filepath.Glob(filepath.Join(s.cacheDir, "photos", "*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v %v", leftovers, err)
	}
	none := &Store{entries: map[string]*entry{}}
	if err := none.cache(photo); err != nil {
		t.Fatal(err)
	}
	if got, err = none.cached("photos/abc.jpg"); err != nil || got != nil {
		t.Fatalf("no cache dir: %+v %v", got, err)
	}
}

func TestServeCaching(t *testing.T) {
	s := &Store{entries: map[string]*entry{
		"photos/abc":           {generation: 7, mimeType: "image/jpeg", data: []byte("photo"), thumb: []byte("photo-thumb")},
		"grade-images/grade-k": {generation: 9, mimeType: "image/png", data: []byte("tile"), thumb: []byte("tile-thumb")},
	}}
	get := func(target, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		s.serve(rec, req)
		return rec
	}

	rec := get("/photos/abc.jpg", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "photo" || rec.Header().Get("ETag") != `"7"` {
		t.Errorf("photo: %d %q etag %q", rec.Code, rec.Body.String(), rec.Header().Get("ETag"))
	}
	if rec.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Errorf("photo cache-control: %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Last-Modified") != "" {
		t.Errorf("photo last-modified sent: %q", rec.Header().Get("Last-Modified"))
	}

	rec = get("/photos/abc.jpg?thumb="+thumbVersion, "")
	if rec.Code != http.StatusOK || rec.Body.String() != "photo-thumb" || rec.Header().Get("Content-Type") != thumbMime {
		t.Errorf("thumb: %d %q type %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
		t.Errorf("thumb cache-control: %q", rec.Header().Get("Cache-Control"))
	}

	if rec = get("/photos/abc.jpg?thumb=0", ""); rec.Code != http.StatusNotFound {
		t.Errorf("stale thumb version: got %d, want 404", rec.Code)
	}

	rec = get("/grade-images/grade-k.png", "")
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") != `"9"` || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("named: %d etag %q cache-control %q", rec.Code, rec.Header().Get("ETag"), rec.Header().Get("Cache-Control"))
	}
	if rec = get("/grade-images/grade-k.png", `"9"`); rec.Code != http.StatusNotModified {
		t.Errorf("named revalidate: got %d, want 304", rec.Code)
	}
	if rec = get("/grade-images/grade-k.png?thumb="+thumbVersion, `"9"`); rec.Code != http.StatusNotModified {
		t.Errorf("named thumb revalidate: got %d, want 304", rec.Code)
	}
	if rec = get("/grade-images/grade-k.png", `"8"`); rec.Code != http.StatusOK || rec.Body.String() != "tile" {
		t.Errorf("named replaced: got %d %q, want 200 with the new bytes", rec.Code, rec.Body.String())
	}
}
