package blob

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func headerPNG(w, h int) []byte {
	ihdr := []byte("IHDR")
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(w))
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h))
	ihdr = append(ihdr, 8, 0, 0, 0, 0)
	out := []byte("\x89PNG\r\n\x1a\n")
	out = binary.BigEndian.AppendUint32(out, 13)
	out = append(out, ihdr...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(ihdr))
}

func TestDecodeRefusesOversizeHeader(t *testing.T) {
	bomb := headerPNG(50000, 50000)
	if len(bomb) > 64 {
		t.Fatalf("the crafted file is %d bytes, which is not the point", len(bomb))
	}
	if _, err := Decode(bomb); err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("a 50000x50000 header: %v, want the pixel limit", err)
	}
	if _, err := Thumbnail(bomb); err == nil {
		t.Error("Thumbnail decoded a 50000x50000 header")
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Errorf("an 8x8 png was refused: %v", err)
	}
	if _, err := Thumbnail(buf.Bytes()); err != nil {
		t.Errorf("Thumbnail refused an 8x8 png: %v", err)
	}
}

func TestServeCaching(t *testing.T) {
	s := &Store{entries: map[string]*entry{
		"photos/abc": {generation: 7, mimeType: "image/jpeg", data: []byte("photo"), thumb: []byte("photo-thumb")},
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

	if rec = get("/photos/abc.jpg", `"7"`); rec.Code != http.StatusNotModified {
		t.Errorf("revalidate: got %d, want 304", rec.Code)
	}
}
