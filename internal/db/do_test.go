package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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
	content, _, err := pics.bucket.Get(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := blob.Thumbnail(content)
	if err != nil {
		t.Fatal(err)
	}
	return pictureFolder + "/" + blob.Name(thumb, "jpg")
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
		row := made(t, s, "PHOTO", out.Result[0], "ready")
		if row["person"] != staff || row["original"] != pictureFolder+"/"+out.Hash+".png" || row["crop"] != "" || row["thumbnail"] == "" {
			t.Fatalf("photo %d answered %+v and reads %v", i, out, row)
		}
		if found, err := pics.bucket.Exists(context.Background(), row["original"]); err != nil || !found {
			t.Fatalf("photo %d's original is not in the bucket: %v", i, err)
		}
		re, mimeType, err := pics.bucket.Get(context.Background(), row["reencode"])
		if err != nil || mimeType != "image/jpeg" || pictureFolder+"/"+blob.Name(re, "jpg") != row["reencode"] {
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
	if visible := as(t, s, parent, `(from PHOTO)`); len(visible) == 0 || visible[0]["original"] != "" {
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
	content, _, err := pics.bucket.Get(context.Background(), row["crop"])
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
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"visible_to": ""})); err != nil {
		t.Fatal(err)
	}
	if visible := as(t, s, guest, `(from PHOTO (where (= group "grp00000000020")))`); len(visible) != 0 {
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

func TestStartupMakesPhotosNotReady(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	original := pngOf(t, 7)
	name := pictureFolder + "/" + blob.Name(original, "png")
	if err := bucket.Put(context.Background(), name, "image/png", original); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, PeopleSheet, store.Insert("PHOTO", store.Row{"id": "pho00000000099", "person": staff, "original": name, "order": "m"})); err != nil {
		t.Fatal(err)
	}
	NewPictures(s, queue, bucket)
	if row := made(t, s, "PHOTO", "pho00000000099", "ready"); row["ready"] != "Yes" || row["thumbnail"] == "" || row["reencode"] == "" {
		t.Fatalf("a photo left unmade before a restart reads %v", row)
	}
}

func TestChangingWhatAPhotoIsMadeFromClearsReady(t *testing.T) {
	old := store.Row{"id": "pho00000000099", "original": "a.png", "ready": "Yes"}
	moved := maps.Clone(old)
	moved["crop_left"] = "3"
	if edits, _ := photoNotReady(nil, Change{Table: "PHOTO", Old: old, New: moved}); len(edits) != 1 || edits[0].Cells["ready"] != "" {
		t.Fatalf("a new box left the photo ready: %v", edits)
	}
	reordered := maps.Clone(old)
	reordered["order"] = "x"
	if edits, _ := photoNotReady(nil, Change{Table: "PHOTO", Old: old, New: reordered}); len(edits) != 0 {
		t.Fatalf("a new order cleared ready: %v", edits)
	}
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
		if !ok || row["kind"] != "calendar" || row["url"] != fields["url"] {
			t.Fatalf("post %d answered %+v and reads %v", i, out, row)
		}
		content, ok := s.Model().Table("CONTENT").Get(row["content"])
		if !ok || content["hash"] != out.Hash || content["blob"] != "content/"+strings.TrimSuffix(blob.Name(pdf, "pdf"), ".pdf") || content["mime"] != "application/pdf" || content["size"] != strconv.Itoa(len(pdf)) {
			t.Fatalf("post %d: the document's content reads %v", i, content)
		}
		if found, err := pics.bucket.Exists(context.Background(), content["blob"]); err != nil || !found {
			t.Fatalf("post %d: the pdf is not in the bucket: %v", i, err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same pdf made two documents: %v", ids)
	}
	if n := s.Model().Table("CONTENT").Len(); n != 1 {
		t.Fatalf("the same pdf stored %d times", n)
	}
	if rec := postFile(t, s, queue, pics, "maya.lindqvist@example.org", "calendar-pdf", fields, "pdf", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "calendar-pdf", fields, "pdf", []byte("not a pdf")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not a pdf: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMailIsStoredOnceWithItsHeaders(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	eml := []byte("From: Maya Lindqvist <Maya.Lindqvist@example.org>\r\n" +
		"Date: Thu, 12 Feb 2026 01:48:03 +0000\r\n" +
		"Subject: =?UTF-8?Q?Spring_Camping_Trip_=E2=80=94_Follow_Up?=\r\n" +
		"List-Id: Hummingbirds Parents <Hummingbirds.Parents.heliosns.org>\r\n" +
		"Message-Id: <one@example.org>\r\n" +
		"\r\n" +
		"Bring a sleeping bag.\r\n")
	ids := []string{}
	for i := range 2 {
		rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "mail", nil, "eml", eml)
		if rec.Code != http.StatusOK {
			t.Fatalf("post %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, out.Result[0])
		row, ok := s.Model().Table("DOCUMENT").Get(out.Result[0])
		if !ok || row["kind"] != "list" || row["name"] != "Spring Camping Trip — Follow Up" || row["published"] != "2026-02-11 17:48:03" || row["author"] != s.Model().PersonOf("maya.lindqvist@example.org") || row["author"] == "" {
			t.Fatalf("post %d answered %+v and reads %v", i, out, row)
		}
		content, ok := s.Model().Table("CONTENT").Get(row["content"])
		if !ok || content["hash"] != out.Hash || content["mime"] != "message/rfc822" || content["size"] != strconv.Itoa(len(eml)) {
			t.Fatalf("post %d: the document's content reads %v", i, content)
		}
		if found, err := pics.bucket.Exists(context.Background(), content["blob"]); err != nil || !found {
			t.Fatalf("post %d: the message is not in the bucket: %v", i, err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same message made two documents: %v", ids)
	}
	links := s.Model().Table("DOCUMENT_GROUP").Referencing("document", ids[0])
	if len(links) != 1 || links[0]["group"] != "grp00000000030" || links[0]["relation"] != "sent_to" {
		t.Fatalf("the message's groups: %v", links)
	}
	if rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "mail", nil, "eml", []byte("Subject: no date\r\n\r\nhello\r\n")); rec.Code != http.StatusBadRequest {
		t.Fatalf("no date: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "maya.lindqvist@example.org", "mail", nil, "eml", eml); rec.Code != http.StatusForbidden {
		t.Fatalf("a person: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMailIsSentToItsListsGroups(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Insert("GROUP", store.Row{"id": "grp00000000031", "kind": "group", "status": "open", "slug": "jayvens-parents", "name": "Jayvens Parents", "visible_to": "grp00000000004"})); err != nil {
		t.Fatal(err)
	}
	m := s.Model()
	for listID, want := range map[string]string{
		"":                                   "newsletter",
		"<parentsandstaff.heliosschool.org>": "list grp00000000002 grp00000000003",
		"<parentsonly.heliosschool.org>":     "list grp00000000002",
		"<community.heliosns.org>":           "list grp00000000004",
		"Hummingbirds Parents <Hummingbirds.Parents.heliosns.org>": "list grp00000000030",
		"<2020-21.hummingbirds.parents.heliosns.org>":              "list grp00000000030",
		"<hummingbirds.students.heliosns.org>":                     "list grp00000000010",
		"<jaysandravens.heliosns.org>":                             "list grp00000000031",
		"<chat.heliosschool.org>":                                  "list grp00000000004",
		"<michelle-level3math.parents.heliosschool.org>":           "list grp00000000004",
		"<3064358178.560896@benchmarkemail.com>":                   "newsletter grp00000000004",
	} {
		root, sentTo, err := m.mailRoot([]byte("Date: Thu, 12 Feb 2026 01:48:03 +0000\r\nList-Id: " + listID + "\r\n\r\nhello\r\n"))
		if err != nil {
			t.Errorf("%q: %v", listID, err)
			continue
		}
		slices.Sort(sentTo)
		if got := strings.Join(append([]string{root["kind"].(string)}, sentTo...), " "); got != want {
			t.Errorf("%q: %s, want %s", listID, got, want)
		}
	}
	if _, _, err := m.mailRoot([]byte("Date: Thu, 12 Feb 2026 01:48:03 +0000\r\nList-Id: <team.loop.heliosian.com>\r\n\r\nhello\r\n")); !errors.Is(err, errLoopMail) {
		t.Fatalf("loop's own mail: %v", err)
	}
}
