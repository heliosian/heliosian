package db

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
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

func newPictures(s *Store, queue *store.Queue) *Pictures {
	return NewPictures(s, queue, blob.NewMemoryBucket())
}

func addPhoto(t *testing.T, s *Store, queue *store.Queue, pics *Pictures, as, person string, photo []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postFile(t, s, queue, pics, as, "photo", map[string]string{"person": person}, "photo", photo)
}

func postFile(t *testing.T, s *Store, queue *store.Queue, pics *Pictures, as, verb string, fields map[string]string, file string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if file != "" {
		part, err := form.CreateFormFile(file, file)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(content)
	}
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, queue, pics, []byte(testImportKey), func() time.Time { return testNow })
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

func made(t *testing.T, s *Store, table, id, column string) store.Row {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		row, ok := s.Model().Table(table).Get(id)
		if ok && row[column] != "" {
			return row
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s %s has no %s after two seconds: %v", table, id, column, row)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func thumbOf(t *testing.T, pics *Pictures, name string) string {
	t.Helper()
	content, _, err := pics.bucket.Get(context.Background(), pictureFolder+"/"+name)
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := blob.Thumbnail(content)
	if err != nil {
		t.Fatal(err)
	}
	return blob.Name(thumb, "jpg")
}

func TestPersonPhotoAddsFirst(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	var orders []string
	for i, size := range []int{2, 3} {
		rec := addPhoto(t, s, queue, pics, "bearer:"+testImportKey, staff, pngOf(t, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("photo %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		row := made(t, s, "PHOTO", out.Result[0], "thumbnail")
		if row["person"] != staff || row["photo"] != out.Hash+".png" || row["crop"] != "" {
			t.Fatalf("photo %d answered %+v and reads %v", i, out, row)
		}
		if found, err := pics.bucket.Exists(context.Background(), pictureFolder+"/"+row["photo"]); err != nil || !found {
			t.Fatalf("photo %d's original is not in the bucket: %v", i, err)
		}
		re, mimeType, err := pics.bucket.Get(context.Background(), pictureFolder+"/"+row["reencode"])
		if err != nil || mimeType != "image/jpeg" || blob.Name(re, "jpg") != row["reencode"] {
			t.Fatalf("photo %d's re-encode %q is not its own stored jpeg: %v", i, row["reencode"], err)
		}
		if row["thumbnail"] != thumbOf(t, pics, row["reencode"]) {
			t.Fatalf("photo %d's thumbnail is not made from its re-encode", i)
		}
		orders = append(orders, row["order"])
	}
	if store.CompareKeys(orders[1], orders[0]) >= 0 {
		t.Fatalf("the second photo's order %q is not before the first's %q", orders[1], orders[0])
	}
	if visible := as(t, s, parent, `(from PHOTO)`); len(visible) == 0 || visible[0]["photo"] != "" {
		t.Fatalf("a viewer reads the original: %v", visible)
	}

	photo := pngOf(t, 4)
	rec := addPhoto(t, s, queue, pics, "maya.lindqvist@example.org", staff, photo)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if found, _ := pics.bucket.Exists(context.Background(), pictureFolder+"/"+blob.Name(photo, "png")); found {
		t.Fatal("a refused photo was stored")
	}
	if rec := addPhoto(t, s, queue, pics, "bearer:"+testImportKey, staff, []byte("not a picture")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not an image: %d %s", rec.Code, rec.Body.String())
	}
}

func TestACropBoxMakesTheCropAndThumbnail(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	fields := map[string]string{"person": staff, "crop_left": "1", "crop_top": "2", "crop_width": "4", "crop_height": "3"}
	rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "photo", fields, "photo", pngOf(t, 8))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload with a box: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id := out.Result[0]
	row := made(t, s, "PHOTO", id, "crop")
	content, _, err := pics.bucket.Get(context.Background(), pictureFolder+"/"+row["crop"])
	if err != nil {
		t.Fatal(err)
	}
	if img, _, err := image.Decode(bytes.NewReader(content)); err != nil || img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Fatalf("the crop is %v: %v", img.Bounds(), err)
	}
	if row["thumbnail"] != thumbOf(t, pics, row["crop"]) {
		t.Fatal("the thumbnail is not made from the crop")
	}

	first := row["crop"]
	env := Env{System: importReader, Now: testNow}
	if _, err := Write(context.Background(), s, queue, pics, access.System(importReader), env, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"crop_width": "2"}}}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for row, _ = s.Model().Table("PHOTO").Get(id); row["crop"] == first; row, _ = s.Model().Table("PHOTO").Get(id) {
		if time.Now().After(deadline) {
			t.Fatal("a new box did not make a new crop")
		}
		time.Sleep(10 * time.Millisecond)
	}

	bad := map[string]string{"person": staff, "crop_left": "1", "crop_top": "2", "crop_width": "0", "crop_height": "3"}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "photo", bad, "photo", pngOf(t, 9)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a box with no width: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGroupPhotos(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ids := []string{}
	for i, size := range []int{8, 9} {
		fields := map[string]string{"group": "grp00000000020", "crop_left": "0", "crop_top": "0", "crop_width": "5", "crop_height": "5"}
		rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "photo", fields, "photo", pngOf(t, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("family photo %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		row := made(t, s, "PHOTO", out.Result[0], "thumbnail")
		if row["group"] != "grp00000000020" || row["person"] != "" || row["crop"] == "" || row["thumbnail"] != thumbOf(t, pics, row["crop"]) {
			t.Fatalf("family photo %d reads %v", i, row)
		}
		ids = append(ids, row["id"])
	}
	first, _ := s.Model().Table("PHOTO").Get(ids[0])
	second, _ := s.Model().Table("PHOTO").Get(ids[1])
	if store.CompareKeys(second["order"], first["order"]) >= 0 {
		t.Fatalf("the family's second photo %q is not before its first %q", second["order"], first["order"])
	}
	if visible := as(t, s, parent, `(from PHOTO (where (= group "grp00000000020")))`); len(visible) != 2 {
		t.Fatalf("a family member sees %d of the family's photos", len(visible))
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"visibility": "managers"})); err != nil {
		t.Fatal(err)
	}
	if visible := as(t, s, student, `(from PHOTO (where (= group "grp00000000020")))`); len(visible) != 0 {
		t.Fatalf("someone who may not see the family sees %d of its photos", len(visible))
	}
	both := map[string]string{"group": "grp00000000020", "person": staff}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "photo", both, "photo", pngOf(t, 7)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a photo of a person and a group: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "photo", nil, "photo", pngOf(t, 7)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a photo of nobody: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "maya.lindqvist@example.org", "photo", map[string]string{"group": "grp00000000020"}, "photo", pngOf(t, 6)); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPicturesQueuesWhatIsMissing(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	original := pngOf(t, 7)
	name := blob.Name(original, "png")
	if err := pics.bucket.Put(context.Background(), pictureFolder+"/"+name, "image/png", original); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, PeopleSheet, store.Insert("PHOTO", store.Row{"id": "pho00000000099", "person": staff, "photo": name, "order": "m"})); err != nil {
		t.Fatal(err)
	}
	if rec := postFile(t, s, queue, pics, parent, "pictures", nil, "", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a person queued every picture: %d %s", rec.Code, rec.Body.String())
	}
	rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "pictures", nil, "", nil)
	var out queued
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK || out.Queued != 1 {
		t.Fatalf("queue: %d %s", rec.Code, rec.Body.String())
	}
	made(t, s, "PHOTO", "pho00000000099", "thumbnail")
}

func TestCalendarPDFIsStoredOnce(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	pdf := []byte("%PDF-1.4\n% a year calendar\n")
	fields := map[string]string{"url": "https://www.heliosschool.org/calendar.pdf"}
	ids := []string{}
	for i := range 2 {
		rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "calendar-pdf", fields, "pdf", pdf)
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
		if found, err := pics.bucket.Exists(context.Background(), row["object"]); err != nil || !found {
			t.Fatalf("post %d: the pdf is not in the bucket: %v", i, err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same pdf made two documents: %v", ids)
	}
	if rec := postFile(t, s, queue, pics, "maya.lindqvist@example.org", "calendar-pdf", fields, "pdf", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "calendar-pdf", fields, "pdf", []byte("not a pdf")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not a pdf: %d %s", rec.Code, rec.Body.String())
	}
}
