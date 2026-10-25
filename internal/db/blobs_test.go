package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func fetchBlob(t *testing.T, s *Store, pics *Pictures, as, path, etag string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, pics, testTokens)
	r := httptest.NewRequest(http.MethodGet, blobPath+path, nil)
	if etag != "" {
		r.Header.Set("If-None-Match", etag)
	}
	rec := httptest.NewRecorder()
	serveAs(mux, rec, r, as)
	return rec
}

func TestContentIsServedAsItsMimeInASandbox(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	if _, err := pics.bucket.Put(context.Background(), "content/h1", "application/octet-stream", []byte("<script>alert(1)</script>"), 0); err != nil {
		t.Fatal(err)
	}
	content := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "page", Row: map[string]any{"hash": "h1", "blob": "content/h1", "mime": "text/html", "size": "25"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@page"}},
	})[0]
	got := fetchBlob(t, s, pics, "rowan.ashdown@example.org", content+"/blob", "")
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "text/html" || !slices.Contains(got.Header().Values("Content-Security-Policy"), "sandbox") {
		t.Fatalf("a stored page: %d %v", got.Code, got.Header())
	}
	if cache := got.Header().Get("Cache-Control"); cache != "private, max-age=31536000, immutable" {
		t.Fatalf("content, whose bytes never change, is cached as %q", cache)
	}
}

func TestABlobIsServedToWhoeverMayReadItsCell(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	rec := addPhoto(t, s, pics, importing, staff, pngOf(t, 5))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id := out.Result[0]
	row := made(t, s, "PHOTO", id, "thumbnail")
	const reader = "rowan.ashdown@example.org"

	got := fetchBlob(t, s, pics, reader, id+"/thumbnail", "")
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/jpeg" || got.Header().Get("ETag") != `"content/`+hashOf(s, row["thumbnail"])+`"` || got.Body.Len() == 0 {
		t.Fatalf("the thumbnail: %d %v", got.Code, got.Header())
	}
	if cache := got.Header().Get("Cache-Control"); cache != "private, no-cache" {
		t.Fatalf("a thumbnail, remade under the same address, is cached as %q", cache)
	}
	if again := fetchBlob(t, s, pics, reader, id+"/thumbnail", got.Header().Get("ETag")); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("the thumbnail again with its etag: %d", again.Code)
	}
	if original := fetchBlob(t, s, pics, reader, id+"/original", ""); original.Code != http.StatusNotFound {
		t.Fatalf("a person fetching the private original: %d", original.Code)
	}
	if original := fetchBlob(t, s, pics, importing, id+"/original", ""); original.Code != http.StatusOK || original.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("the import fetching the original: %d %v", original.Code, original.Header())
	}
	if thumb := fetchBlob(t, s, pics, reader, row["thumbnail"]+"/blob", ""); thumb.Code != http.StatusOK || thumb.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("the thumbnail's own content: %d %v", thumb.Code, thumb.Header())
	}
	if rec := fetchBlob(t, s, pics, reader, row["original"]+"/blob", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("a person fetching the photo's original by its content: %d", rec.Code)
	}
	for _, path := range []string{id + "/order", id + "/nothing", "pho00000000098/thumbnail", "nonsense/thumbnail"} {
		if rec := fetchBlob(t, s, pics, reader, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
}
