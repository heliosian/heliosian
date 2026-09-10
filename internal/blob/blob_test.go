package blob

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
