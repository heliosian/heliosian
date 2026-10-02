package db

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func pngOf(t *testing.T, size int) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func addPhoto(t *testing.T, s *Store, queue *store.Queue, media *blob.Store, as, person string, photo []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postFile(t, s, queue, media, as, "person-photo", map[string]string{"person": person}, "photo", photo)
}

func postFile(t *testing.T, s *Store, queue *store.Queue, media *blob.Store, as, verb string, fields map[string]string, file string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := form.CreateFormFile(file, file)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, queue, media, []byte(testImportKey), func() time.Time { return testNow })
	r := httptest.NewRequest(http.MethodPost, doPrefix+verb, body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	if key, ok := strings.CutPrefix(as, "bearer:"); ok {
		r.Header.Set("Authorization", "Bearer "+key)
		mux.ServeHTTP(rec, r)
		return rec
	}
	auth.Fixed(as, mux).ServeHTTP(rec, r)
	return rec
}

func TestPersonPhotoAddsFirst(t *testing.T) {
	s, queue := sampleWithQueue(t)
	media := blob.New(blob.NewMemoryBucket())
	var orders []string
	for i, size := range []int{2, 3} {
		rec := addPhoto(t, s, queue, media, "bearer:"+testImportKey, staff, pngOf(t, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("photo %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		row, ok := s.Model().Table("PERSON_PHOTO").Get(out.Result[0])
		if !ok || row["person"] != staff || row["photo"] != out.Hash+".png" {
			t.Fatalf("photo %d answered %+v and reads %v", i, out, row)
		}
		if found, err := media.Has("photos/" + row["photo"]); err != nil || !found {
			t.Fatalf("photo %d is not in the bucket: %v", i, err)
		}
		thumb, mimeType, ok := media.Bytes("photos/" + row["thumbnail"])
		if !ok || row["thumbnail"] == row["photo"] || mimeType != "image/jpeg" || blob.Name(thumb, "jpg") != row["thumbnail"] {
			t.Fatalf("photo %d's thumbnail %q is not its own stored jpeg", i, row["thumbnail"])
		}
		orders = append(orders, row["order"])
	}
	if store.CompareKeys(orders[1], orders[0]) >= 0 {
		t.Fatalf("the second photo's order %q is not before the first's %q", orders[1], orders[0])
	}

	photo := pngOf(t, 4)
	rec := addPhoto(t, s, queue, media, "maya.lindqvist@example.org", staff, photo)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if found, _ := media.Has("photos/" + blob.Name(photo, "png")); found {
		t.Fatal("a refused photo was stored")
	}
	if rec := addPhoto(t, s, queue, media, "bearer:"+testImportKey, staff, []byte("not a picture")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not an image: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCalendarPDFIsStoredOnce(t *testing.T) {
	s, queue := sampleWithQueue(t)
	media := blob.New(blob.NewMemoryBucket())
	pdf := []byte("%PDF-1.4\n% a year calendar\n")
	fields := map[string]string{"url": "https://www.heliosschool.org/calendar.pdf"}
	ids := []string{}
	for i := range 2 {
		rec := postFile(t, s, queue, media, "bearer:"+testImportKey, "calendar-pdf", fields, "pdf", pdf)
		if rec.Code != http.StatusOK {
			t.Fatalf("post %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, out.Result[0])
		row, ok := s.Model().Table("DOCUMENT").Get(out.Result[0])
		if !ok || row["kind"] != "calendar" || row["hash"] != out.Hash || row["object"] != "calendar/"+blob.Name(pdf, "pdf") || row["url"] != fields["url"] {
			t.Fatalf("post %d answered %+v and reads %v", i, out, row)
		}
		if found, err := media.Has(row["object"]); err != nil || !found {
			t.Fatalf("post %d: the pdf is not in the bucket: %v", i, err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same pdf made two documents: %v", ids)
	}
	if rec := postFile(t, s, queue, media, "maya.lindqvist@example.org", "calendar-pdf", fields, "pdf", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, media, "bearer:"+testImportKey, "calendar-pdf", fields, "pdf", []byte("not a pdf")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not a pdf: %d %s", rec.Code, rec.Body.String())
	}
}
