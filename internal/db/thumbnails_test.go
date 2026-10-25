package db

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func storedContent(t *testing.T, bucket *blob.Bucket, mimeType string, body []byte) map[string]any {
	t.Helper()
	hash := blob.Hash(body)
	if _, err := bucket.Put(t.Context(), "content/"+hash, mimeType, body, 0); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"hash": hash, "blob": "content/" + hash, "mime": mimeType, "size": strconv.Itoa(len(body))}
}

func redPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func thumbnailed(t *testing.T, s *Store, id string) store.Row {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row, _ := s.Model().Table("DOCUMENT").Get(id)
		if row["thumbnailed"] != "" {
			return row
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was never thumbnailed: %v", id, row)
		}
	}
}

func TestAPagesPictureThatFailsIsTriedAgain(t *testing.T) {
	banner := redPNG(t, 600, 300)
	asked := atomic.Int32{}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write(banner)
	}))
	defer site.Close()
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	page := []byte(`<html><head><meta property="og:image" content="` + site.URL + `/banner.png"></head><body>Spring fair</body></html>`)
	linked := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "page", Row: storedContent(t, bucket, "text/html", page)},
		{Insert: "DOCUMENT", As: "mail", Row: map[string]any{"kind": "mail", "name": "Spring fair"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": site.URL + "/fair", "content": "@page"}},
	})[2]
	StartThumbnailer(s, bucket)
	for deadline := time.Now().Add(5 * time.Second); asked.Load() < 1; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the page's picture was never asked for")
		}
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		s.Model().services.inbox(thumbnailInbox).wake.poke()
		row, _ := s.Model().Table("DOCUMENT").Get(linked)
		if row["thumbnail"] != "" {
			return
		}
		if row["thumbnailed"] != "" || time.Now().After(deadline) {
			t.Fatalf("the page had no thumbnail once its picture answered: %v, asked %d times", row, asked.Load())
		}
	}
}

func TestNewContentRemakesTheThumbnail(t *testing.T) {
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "red", Row: storedContent(t, bucket, "image/png", redPNG(t, 960, 720))},
		{Insert: "CONTENT", As: "black", Row: storedContent(t, bucket, "image/png", pngOf(t, 600))},
		{Insert: "CONTENT", As: "text", Row: storedContent(t, bucket, "text/markdown", []byte("# Flyer"))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Fair flyer", "content": "@red"}},
	})
	black, text, flyer := ids[1], ids[2], ids[3]
	StartThumbnailer(s, bucket)
	red := thumbnailed(t, s, flyer)["thumbnail"]
	apply(t, s, []Edit{{Set: flyer, Cells: map[string]any{"content": black}}})
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		row, _ := s.Model().Table("DOCUMENT").Get(flyer)
		if row["thumbnail"] != "" && row["thumbnail"] != red {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the flyer kept its old thumbnail after its content changed: %v", row)
		}
	}
	apply(t, s, []Edit{{Set: flyer, Cells: map[string]any{"content": text}}})
	if row, _ := s.Model().Table("DOCUMENT").Get(flyer); row["thumbnail"] != "" || row["thumbnailed"] != "" {
		t.Fatalf("the flyer kept a thumbnail after its content became text: %v", row)
	}
}

func TestNewContentIsTriedAfterOldContentGaveUp(t *testing.T) {
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "broken", Row: storedContent(t, bucket, "image/png", []byte("\x89PNG not really"))},
		{Insert: "CONTENT", As: "good", Row: storedContent(t, bucket, "image/png", redPNG(t, 600, 400))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": "Fair flyer", "content": "@broken"}},
	})
	good, flyer := ids[1], ids[2]
	StartThumbnailer(s, bucket)
	for range thumbnailTries + 2 {
		s.Model().services.inbox(thumbnailInbox).wake.poke()
		time.Sleep(50 * time.Millisecond)
	}
	if row, _ := s.Model().Table("DOCUMENT").Get(flyer); row["thumbnailed"] != "" {
		t.Fatalf("a broken image was thumbnailed: %v", row)
	}
	apply(t, s, []Edit{{Set: flyer, Cells: map[string]any{"content": good}}})
	if row := thumbnailed(t, s, flyer); row["thumbnail"] == "" {
		t.Fatalf("the flyer's new image got no thumbnail: %v", row)
	}
}

func TestAPDFsThumbnailIsPostedFromALaptop(t *testing.T) {
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, bucket)
	calendar := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "pdf", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 2))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "calendar", "name": "Year calendar", "content": "@pdf"}},
	})[1]
	page := redPNG(t, 960, 1242)
	pdf, _ := s.Model().Table("DOCUMENT").Get(calendar)
	drawn := map[string]string{"document": calendar, "content": pdf["content"]}
	if rec := postFile(t, s, pics, "rowan.ashdown@example.org", "thumbnail", drawn, "picture", page); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent set a thumbnail: %d %s", rec.Code, rec.Body.String())
	}
	newer := apply(t, s, []Edit{{Insert: "CONTENT", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 3))}})[0]
	apply(t, s, []Edit{{Set: calendar, Cells: map[string]any{"content": newer}}})
	if rec := postFile(t, s, pics, importing, "thumbnail", drawn, "picture", page); rec.Code != http.StatusBadRequest {
		t.Fatalf("a picture of the old pdf was taken after the calendar changed: %d %s", rec.Code, rec.Body.String())
	}
	drawn["content"] = newer
	if rec := postFile(t, s, pics, importing, "thumbnail", drawn, "picture", page); rec.Code != http.StatusOK {
		t.Fatalf("the import could not set a thumbnail: %d %s", rec.Code, rec.Body.String())
	}
	row, _ := s.Model().Table("DOCUMENT").Get(calendar)
	thumb, ok := s.Model().Table("CONTENT").Get(row["thumbnail"])
	if row["thumbnailed"] == "" || !ok || thumb["mime"] != "image/jpeg" {
		t.Fatalf("the calendar after its thumbnail was posted: %v %v", row, thumb)
	}
	raw, _, err := bucket.Get(t.Context(), thumb["blob"])
	if err != nil {
		t.Fatal(err)
	}
	if img, err := jpeg.Decode(bytes.NewReader(raw)); err != nil || img.Bounds().Dx() != 480 {
		t.Fatalf("the stored thumbnail: %v %v", img, err)
	}
	if rec := postFile(t, s, pics, importing, "thumbnail", drawn, "picture", page); rec.Code != http.StatusBadRequest {
		t.Fatalf("a thumbnail was posted twice: %d", rec.Code)
	}
}

func TestDocumentsGetThumbnails(t *testing.T) {
	banner := redPNG(t, 1200, 630)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/news/share/banner.png" {
			http.NotFound(w, r)
			return
		}
		w.Write(banner)
	}))
	defer site.Close()
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, bucket)
	withPicture := []byte(`<html><head><base href="/news/"><meta property="og:image" content="share/banner.png"></head><body>Spring fair</body></html>`)
	without := []byte(`<html><head><title>Bare</title></head><body>Nothing to show</body></html>`)
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "pdf", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 2))},
		{Insert: "CONTENT", As: "photo", Row: storedContent(t, bucket, "image/png", redPNG(t, 960, 720))},
		{Insert: "CONTENT", As: "page", Row: storedContent(t, bucket, "text/html; charset=utf-8", withPicture)},
		{Insert: "CONTENT", As: "bare", Row: storedContent(t, bucket, "text/html; charset=utf-8", without)},
		{Insert: "CONTENT", As: "text", Row: storedContent(t, bucket, "text/markdown", []byte("# Notes"))},
		{Insert: "DOCUMENT", As: "flyer", Row: map[string]any{"kind": "file", "name": "Fair flyer", "content": "@photo"}},
		{Insert: "DOCUMENT", As: "calendar", Row: map[string]any{"kind": "calendar", "name": "Year calendar", "content": "@pdf"}},
		{Insert: "DOCUMENT", As: "mail", Row: map[string]any{"kind": "mail", "name": "Not yet sent anywhere"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "part", "parent": "@mail", "content": "@photo"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": site.URL + "/fair", "content": "@page"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@mail", "url": site.URL + "/bare", "content": "@bare"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": "@calendar", "content": "@text"}},
	})
	flyer, calendar, part, page, bare, extract := ids[5], ids[6], ids[8], ids[9], ids[10], ids[11]
	StartThumbnailer(s, bucket)
	for _, id := range []string{flyer, part, page} {
		row := thumbnailed(t, s, id)
		thumb, ok := s.Model().Table("CONTENT").Get(row["thumbnail"])
		if !ok || thumb["mime"] != "image/jpeg" {
			t.Fatalf("%s has no jpeg thumbnail: %v %v", id, row, thumb)
		}
		raw, _, err := bucket.Get(t.Context(), thumb["blob"])
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if w := img.Bounds().Dx(); w != 480 {
			t.Errorf("%s's thumbnail is %d wide", id, w)
		}
		if r, g, _, _ := img.At(10, 10).RGBA(); r>>8 < 150 || g>>8 > 60 {
			t.Errorf("%s's thumbnail is not the red picture: %v", id, img.At(10, 10))
		}
	}
	if a, b := thumbnailed(t, s, flyer), thumbnailed(t, s, part); a["thumbnail"] != b["thumbnail"] {
		t.Fatalf("two documents over the same bytes hold different thumbnails: %v %v", a, b)
	}
	if row := thumbnailed(t, s, bare); row["thumbnail"] != "" {
		t.Fatalf("a page with no picture got a thumbnail: %v", row)
	}
	time.Sleep(50 * time.Millisecond)
	for _, id := range []string{calendar, extract} {
		if row, _ := s.Model().Table("DOCUMENT").Get(id); row["thumbnailed"] != "" || row["thumbnail"] != "" {
			t.Fatalf("%s was thumbnailed: %v", id, row)
		}
	}
	mux := http.NewServeMux()
	Register(mux, s, pics, testTokens)
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		serveAs(mux, rec, httptest.NewRequest(http.MethodGet, path, nil), "rowan.ashdown@example.org")
		return rec
	}
	if rec := get("/api/blob/" + flyer + "/thumbnail"); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("the shared file's thumbnail answered %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := get("/api/blob/" + page + "/thumbnail"); rec.Code != http.StatusNotFound {
		t.Fatalf("the thumbnail of a page linked from mail sent to nobody answered %d", rec.Code)
	}
}
