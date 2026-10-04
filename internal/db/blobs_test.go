package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/store"
)

func fetchBlob(t *testing.T, s *Store, pics *Pictures, as, path, etag string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, nil, pics, []byte(testImportKey), func() time.Time { return testNow })
	r := httptest.NewRequest(http.MethodGet, blobPath+path, nil)
	if etag != "" {
		r.Header.Set("If-None-Match", etag)
	}
	rec := httptest.NewRecorder()
	if key, ok := strings.CutPrefix(as, "bearer:"); ok {
		r.Header.Set("Authorization", "Bearer "+key)
		mux.ServeHTTP(rec, r)
		return rec
	}
	auth.Fixed(as, mux).ServeHTTP(rec, r)
	return rec
}

func TestContentIsServedAsItsMimeInASandbox(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	if err := pics.bucket.Put(context.Background(), "content/h1", "application/octet-stream", []byte("<script>alert(1)</script>")); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "h1", "blob": "content/h1", "mime": "text/html", "size": "25"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "page", "content": "cnt00000000001"}),
	); err != nil {
		t.Fatal(err)
	}
	got := fetchBlob(t, s, pics, "rowan.ashdown@example.org", "cnt00000000001/blob", "")
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "text/html" || !slices.Contains(got.Header().Values("Content-Security-Policy"), "sandbox") {
		t.Fatalf("a stored page: %d %v", got.Code, got.Header())
	}
}

func TestABlobIsServedToWhoeverMayReadItsCell(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	rec := addPhoto(t, s, queue, pics, "bearer:"+testImportKey, staff, pngOf(t, 5))
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
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "image/jpeg" || got.Header().Get("ETag") != `"`+row["thumbnail"]+`"` || got.Body.Len() == 0 {
		t.Fatalf("the thumbnail: %d %v", got.Code, got.Header())
	}
	if again := fetchBlob(t, s, pics, reader, id+"/thumbnail", got.Header().Get("ETag")); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("the thumbnail again with its etag: %d", again.Code)
	}
	if original := fetchBlob(t, s, pics, reader, id+"/photo", ""); original.Code != http.StatusNotFound {
		t.Fatalf("a person fetching the private original: %d", original.Code)
	}
	if original := fetchBlob(t, s, pics, "bearer:"+testImportKey, id+"/photo", ""); original.Code != http.StatusOK || original.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("the import fetching the original: %d %v", original.Code, original.Header())
	}
	for _, path := range []string{id + "/order", id + "/nothing", "pho00000000098/thumbnail", "nonsense/thumbnail"} {
		if rec := fetchBlob(t, s, pics, reader, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
}
