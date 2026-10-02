package blob

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnlyWhatARefreshNamesIsKept(t *testing.T) {
	bucket := NewMemoryBucket()
	s := New(bucket)
	for _, name := range []string{"pronunciation/kept.m4a", "pronunciation/dropped.m4a"} {
		if err := bucket.Put(context.Background(), name, "audio/mp4", []byte(name)); err != nil {
			t.Fatal(err)
		}
		if found, err := s.Has(name); err != nil || !found {
			t.Fatalf("%s: %v %v", name, found, err)
		}
	}
	mux := http.NewServeMux()
	Register(mux, s, "pronunciation")
	swap, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Prefetch(context.Background(), []string{"pronunciation/kept.m4a"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pronunciation/dropped.m4a", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("serve before the swap: %d", rec.Code)
	}
	swap()
	if _, _, ok := s.Bytes("pronunciation/kept.m4a"); !ok {
		t.Fatal("a named object was dropped")
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/pronunciation/dropped.m4a", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an object no sheet names, fetched during the refresh, still served: %d", rec.Code)
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
	bucket := NewMemoryBucket()
	if err := bucket.Put(context.Background(), "photos/abc.jpg", "image/jpeg", []byte("photo")); err != nil {
		t.Fatal(err)
	}
	s := New(bucket)
	s.entries["photos/abc"] = &entry{name: "photos/abc.jpg", generation: 7, mimeType: "image/jpeg", thumb: []byte("photo-thumb")}
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

func TestReencodeShrinksAndCropCuts(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4096, 1024))); err != nil {
		t.Fatal(err)
	}
	re, err := Reencode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(re))
	if err != nil || format != "jpeg" || img.Bounds().Dx() != 2048 || img.Bounds().Dy() != 512 {
		t.Fatalf("re-encode is %s %v: %v", format, img.Bounds(), err)
	}
	cropped, err := Crop(re, image.Rect(100, 50, 400, 250))
	if err != nil {
		t.Fatal(err)
	}
	if img, _, err := image.Decode(bytes.NewReader(cropped)); err != nil || img.Bounds().Dx() != 300 || img.Bounds().Dy() != 200 {
		t.Fatalf("crop is %v: %v", img.Bounds(), err)
	}
	if _, err := Crop(re, image.Rect(2000, 0, 2100, 100)); err == nil {
		t.Fatal("a crop past the edge was cut")
	}
}

func TestFullBytesAreCachedThumbnailsHeld(t *testing.T) {
	bucket := NewMemoryBucket()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	s := New(bucket)
	if err := s.Put("photos", "a.png", "image/png", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.data.Get("photos/a"); !ok {
		t.Fatal("the bytes just written are not cached")
	}
	s.data = New(bucket).data
	if e, ok := s.held("photos/a"); !ok || e.thumb == nil {
		t.Fatal("the thumbnail is not held")
	}
	data, mimeType, ok := s.Bytes("photos/a.png")
	if !ok || mimeType != "image/png" || !bytes.Equal(data, buf.Bytes()) {
		t.Fatalf("bytes no longer cached were not read again from the bucket: %v %q", ok, mimeType)
	}
}
