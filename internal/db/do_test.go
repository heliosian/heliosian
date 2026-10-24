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
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/access"
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
	p := NewPictures(s, queue, blob.NewMemoryBucket())
	go p.run()
	return p
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
	Register(mux, s, queue, pics, testTokens, func() time.Time { return testNow })
	r := httptest.NewRequest(http.MethodPost, "/api/do/"+verb, body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	serveAs(mux, rec, r, as)
	return rec
}

func postUpload(t *testing.T, s *Store, queue *store.Queue, pics *Pictures, as string, fields map[string]string, mimeType string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	header := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="upload"`}}
	if mimeType != "" {
		header.Set("Content-Type", mimeType)
	}
	part, err := form.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, queue, pics, testTokens, func() time.Time { return testNow })
	r := httptest.NewRequest(http.MethodPost, "/api/do/file", body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	serveAs(mux, rec, r, as)
	return rec
}

func postMail(t *testing.T, s *Store, queue *store.Queue, pics *Pictures, as string, eml []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postUpload(t, s, queue, pics, as, nil, "message/rfc822", eml)
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

func contentOf(t *testing.T, s *Store, pics *Pictures, content string) ([]byte, string) {
	t.Helper()
	data, mimeType, err := s.Model().contentBytes(context.Background(), pics.bucket, content)
	if err != nil {
		t.Fatal(err)
	}
	return data, mimeType
}

func hashOf(s *Store, content string) string {
	row, _ := s.Model().Table("CONTENT").Get(content)
	return row["hash"]
}

func thumbOf(t *testing.T, s *Store, pics *Pictures, content string) string {
	t.Helper()
	data, _ := contentOf(t, s, pics, content)
	thumb, err := blob.Thumbnail(data)
	if err != nil {
		t.Fatal(err)
	}
	return blob.Hash(thumb)
}

func TestPersonPhotoAddsFirst(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	var orders []string
	for i, size := range []int{2, 3} {
		rec := addPhoto(t, s, queue, pics, importing, staff, pngOf(t, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("photo %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		row := made(t, s, "PHOTO", out.Result[0], "ready")
		if row["person"] != staff || hashOf(s, row["original"]) != out.Hash || row["crop"] != "" || row["thumbnail"] == "" {
			t.Fatalf("photo %d answered %+v and reads %v", i, out, row)
		}
		if found, err := pics.bucket.Exists(context.Background(), "content/"+out.Hash); err != nil || !found {
			t.Fatalf("photo %d's original is not in the bucket: %v", i, err)
		}
		re, mimeType := contentOf(t, s, pics, row["reencode"])
		if mimeType != "image/jpeg" || blob.Hash(re) != hashOf(s, row["reencode"]) {
			t.Fatalf("photo %d's re-encode %q is not its own stored jpeg", i, row["reencode"])
		}
		if hashOf(s, row["thumbnail"]) != thumbOf(t, s, pics, row["reencode"]) {
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
	rec := addPhoto(t, s, queue, pics, "rowan.ashdown@example.org", staff, photo)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a photo of someone outside one's family: %d %s", rec.Code, rec.Body.String())
	}
	if found, _ := pics.bucket.Exists(context.Background(), "content/"+blob.Hash(photo)); found {
		t.Fatal("a refused photo was stored")
	}
	if rec := addPhoto(t, s, queue, pics, importing, staff, []byte("not a picture")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not an image: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTheSamePictureUploadedAtOnceIsOneContent(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	picture := pngOf(t, 6)
	people := []string{staff, student, parent, "per00000020001", "per00000020002", "per00000020003"}
	codes := make([]int, len(people))
	var wg sync.WaitGroup
	for i, person := range people {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = addPhoto(t, s, queue, pics, importing, person, picture).Code
		}()
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("upload %d answered %d", i, code)
		}
	}
	content, ok := s.Model().Table("CONTENT").Find(blob.Hash(picture), "image/png")
	if !ok || len(s.Model().Table("PHOTO").Referencing("original", content["id"])) != len(people) {
		t.Fatalf("the uploads do not all name one CONTENT row %v", content)
	}
}

func TestAStaleMakesBytesAreSweptOnceOld(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	rec := addPhoto(t, s, queue, pics, importing, staff, pngOf(t, 8))
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	row := made(t, s, "PHOTO", out.Result[0], "ready")
	before, err := pics.bucket.List(context.Background(), "content/")
	if err != nil {
		t.Fatal(err)
	}
	stale := maps.Clone(row)
	stale["crop_left"], stale["crop_top"], stale["crop_width"], stale["crop_height"] = "1", "1", "3", "3"
	if err := pics.makePhoto(row["id"], stale); err != nil {
		t.Fatal(err)
	}
	x := &sweeper{s: s, queue: queue, bucket: pics.bucket}
	if removed, err := x.sweepObjects(context.Background(), time.Now().Add(-orphanAge)); err != nil || removed != 0 {
		t.Fatalf("the sweep removed %d objects written just now: %v", removed, err)
	}
	if removed, err := x.sweepObjects(context.Background(), time.Now().Add(time.Second)); err != nil || removed == 0 {
		t.Fatalf("the sweep removed %d objects once they were old: %v", removed, err)
	}
	after, err := pics.bucket.List(context.Background(), "content/")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Fatalf("the sweep left objects no content names, or took ones it names: %v, then %v", before, after)
	}
	if now, _ := s.Model().Table("PHOTO").Get(row["id"]); now["crop"] != "" || now["thumbnail"] != row["thumbnail"] {
		t.Fatalf("a stale make changed the photo: %v", now)
	}
}

func TestACropBoxMakesTheCropAndThumbnail(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	fields := map[string]string{"person": staff, "crop_left": "1", "crop_top": "2", "crop_width": "4", "crop_height": "3"}
	rec := postFile(t, s, queue, pics, importing, "photo", fields, "photo", pngOf(t, 8))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload with a box: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	id := out.Result[0]
	row := made(t, s, "PHOTO", id, "crop")
	content, _ := contentOf(t, s, pics, row["crop"])
	if img, _, err := image.Decode(bytes.NewReader(content)); err != nil || img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Fatalf("the crop is %v: %v", img.Bounds(), err)
	}
	if hashOf(s, row["thumbnail"]) != thumbOf(t, s, pics, row["crop"]) {
		t.Fatal("the thumbnail is not made from the crop")
	}

	first := row["crop"]
	env := setupEnv
	if _, err := Write(context.Background(), s, queue, access.System(env.System), env, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"crop_width": "2"}}}}); err != nil {
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
	if rec := postFile(t, s, queue, pics, importing, "photo", bad, "photo", pngOf(t, 9)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a box with no width: %d %s", rec.Code, rec.Body.String())
	}
}

func TestGroupPhotos(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ids := []string{}
	for i, size := range []int{8, 9} {
		fields := map[string]string{"group": "grp00000000020", "crop_left": "0", "crop_top": "0", "crop_width": "5", "crop_height": "5"}
		rec := postFile(t, s, queue, pics, importing, "photo", fields, "photo", pngOf(t, size))
		if rec.Code != http.StatusOK {
			t.Fatalf("family photo %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		row := made(t, s, "PHOTO", out.Result[0], "thumbnail")
		if row["group"] != "grp00000000020" || row["person"] != "" || row["crop"] == "" || hashOf(s, row["thumbnail"]) != thumbOf(t, s, pics, row["crop"]) {
			t.Fatalf("family photo %d reads %v", i, row)
		}
		ids = append(ids, row["id"])
	}
	first, _ := s.Model().Table("PHOTO").Get(ids[0])
	second, _ := s.Model().Table("PHOTO").Get(ids[1])
	if store.CompareKeys(second["order"], first["order"]) >= 0 {
		t.Fatalf("the family's second photo %q is not before its first %q", second["order"], first["order"])
	}
	if visible := as(t, s, parent, `(from PHOTO (where (= group "grp00000000020")))`); len(visible) != 3 {
		t.Fatalf("a family member sees %d of the family's photos", len(visible))
	}
	apply(t, s, queue, []Edit{{Set: "grp00000000020", Cells: map[string]any{"visible_to": ""}}})
	if visible := as(t, s, guest, `(from PHOTO (where (= group "grp00000000020")))`); len(visible) != 0 {
		t.Fatalf("someone who may not see the family sees %d of its photos", len(visible))
	}
	both := map[string]string{"group": "grp00000000020", "person": staff}
	if rec := postFile(t, s, queue, pics, importing, "photo", both, "photo", pngOf(t, 7)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a photo of a person and a group: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, importing, "photo", nil, "photo", pngOf(t, 7)); rec.Code != http.StatusBadRequest {
		t.Fatalf("a photo of nobody: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "rowan.ashdown@example.org", "photo", map[string]string{"group": "grp00000000503"}, "photo", pngOf(t, 6)); rec.Code != http.StatusForbidden {
		t.Fatalf("a photo of a family one doesn't manage: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postFile(t, s, queue, pics, "rowan.ashdown@example.org", "photo", map[string]string{"group": "grp00000000020"}, "photo", pngOf(t, 6)); rec.Code != http.StatusOK {
		t.Fatalf("a photo of one's own family: %d %s", rec.Code, rec.Body.String())
	}
	moved := Edit{Set: ids[0], Cells: map[string]any{"order": "zz", "crop_width": "4"}}
	if _, err := Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{moved}}); err != nil {
		t.Fatalf("a family member moves and re-crops the family's photo: %v", err)
	}
	if row, _ := s.Model().Table("PHOTO").Get(ids[0]); row["order"] != "zz" || row["original"] != first["original"] {
		t.Fatalf("the moved photo reads %v", row)
	}
}

func TestStartupMakesPhotosNotReady(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	original := pngOf(t, 7)
	hash := blob.Hash(original)
	if _, err := bucket.Put(context.Background(), "content/"+hash, "image/png", original, 0); err != nil {
		t.Fatal(err)
	}
	photo := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "original", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "image/png", "size": strconv.Itoa(len(original))}},
		{Insert: "PHOTO", Row: map[string]any{"person": staff, "original": "@original", "order": "m"}},
	})[1]
	NewPictures(s, queue, bucket).Start()
	if row := made(t, s, "PHOTO", photo, "ready"); row["ready"] != "Yes" || row["thumbnail"] == "" || row["reencode"] == "" {
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

func TestChangingAnOriginalBlanksWhatWasMade(t *testing.T) {
	old := store.Row{"id": "per00000000099", "pronunciation_original": "cnt0000000000a", "pronunciation": "cnt0000000000b"}
	for _, original := range []string{"cnt0000000000c", ""} {
		changed := maps.Clone(old)
		changed["pronunciation_original"] = original
		if edits, _ := madeNotMade(nil, Change{Table: "PERSON", Old: old, New: changed}); len(edits) != 1 || edits[0].Cells["pronunciation"] != "" {
			t.Fatalf("an original of %q left the made recording: %v", original, edits)
		}
	}
	renamed := maps.Clone(old)
	renamed["pronouns"] = "they/them"
	if edits, _ := madeNotMade(nil, Change{Table: "PERSON", Old: old, New: renamed}); len(edits) != 0 {
		t.Fatalf("another column blanked the made recording: %v", edits)
	}
	poster := store.Row{"id": "grp00000000099", "flyer_original": "cnt0000000000a", "flyer": "cnt0000000000b", "pronunciation_original": "cnt0000000000c", "pronunciation": "cnt0000000000d"}
	replaced := maps.Clone(poster)
	replaced["flyer_original"] = "cnt0000000000e"
	if edits, _ := madeNotMade(nil, Change{Table: "GROUP", Old: poster, New: replaced}); len(edits) != 1 || len(edits[0].Cells) != 1 || edits[0].Cells["flyer"] != "" {
		t.Fatalf("a new flyer blanked %v, want the made flyer alone", edits)
	}
}

func TestCalendarPDFIsStoredOnce(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	pdf := []byte("%PDF-1.4\n% a year calendar\n")
	fields := map[string]string{"kind": "calendar", "url": "https://www.heliosschool.org/calendar.pdf", "published": "2026-09-14 10:22:05"}
	ids := []string{}
	for i := range 2 {
		rec := postUpload(t, s, queue, pics, importing, fields, "", pdf)
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
		if !ok || content["hash"] != out.Hash || content["blob"] != "content/"+blob.Hash(pdf) || content["mime"] != "application/pdf" || content["size"] != strconv.Itoa(len(pdf)) {
			t.Fatalf("post %d: the document's content reads %v", i, content)
		}
		if found, err := pics.bucket.Exists(context.Background(), content["blob"]); err != nil || !found {
			t.Fatalf("post %d: the pdf is not in the bucket: %v", i, err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same pdf made two documents: %v", ids)
	}
	if n := len(as(t, s, staff, `(from CONTENT (where (= mime "application/pdf")))`)); n != 1 {
		t.Fatalf("the same pdf stored %d times", n)
	}
	if rec := postUpload(t, s, queue, pics, "rowan.ashdown@example.org", fields, "", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postUpload(t, s, queue, pics, importing, fields, "", []byte("not a pdf")); rec.Code != http.StatusBadRequest {
		t.Fatalf("not a pdf: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPDFIsStoredOnceAsAFileWithItsNameAndDate(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	pdf := []byte("%PDF-1.4\n% a slide deck\n")
	fields := map[string]string{"name": "Camping - HELP - Condors", "url": "https://docs.google.com/presentation/d/1BvW", "published": "2026-09-14 10:22:05"}
	ids := []string{}
	for i := range 2 {
		rec := postUpload(t, s, queue, pics, importing, fields, "", pdf)
		if rec.Code != http.StatusOK {
			t.Fatalf("post %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, out.Result[0])
		row, ok := s.Model().Table("DOCUMENT").Get(out.Result[0])
		if !ok || row["kind"] != "file" || row["name"] != fields["name"] || row["url"] != fields["url"] || row["published"] != fields["published"] {
			t.Fatalf("post %d answered %+v and reads %v", i, out, row)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("the same pdf made two documents: %v", ids)
	}
	for field, value := range map[string]string{"published": "last tuesday", "url": "", "kind": "deck"} {
		bad := maps.Clone(fields)
		bad[field] = value
		if rec := postUpload(t, s, queue, pics, importing, bad, "", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %q: %d %s", field, value, rec.Code, rec.Body.String())
		}
	}
	if rec := postUpload(t, s, queue, pics, "rowan.ashdown@example.org", fields, "", []byte("%PDF-1.4\n% another\n")); rec.Code != http.StatusForbidden {
		t.Fatalf("a person with no grant: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAPageIsStoredAsAFile(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	fields := map[string]string{"name": "Aftercare", "url": "https://portals.example.org/parent/pages/Aftercare", "published": "2026-10-06 13:19:38"}
	rec := postUpload(t, s, queue, pics, importing, fields, "text/html", []byte("<!doctype html><html><body><p>Aftercare runs until six.</p></body></html>"))
	if rec.Code != http.StatusOK {
		t.Fatalf("a page: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	row, _ := s.Model().Table("DOCUMENT").Get(out.Result[0])
	content, _ := s.Model().Table("CONTENT").Get(row["content"])
	if row["kind"] != "file" || content["mime"] != "text/html" {
		t.Fatalf("the page reads %v, its content %v", row, content)
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
		rec := postMail(t, s, queue, pics, importing, eml)
		if rec.Code != http.StatusOK {
			t.Fatalf("post %d: %d %s", i, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, out.Result[0])
		row, ok := s.Model().Table("DOCUMENT").Get(out.Result[0])
		if !ok || row["kind"] != "mail" || row["name"] != "Spring Camping Trip — Follow Up" || row["published"] != "2026-02-11 17:48:03" || row["author"] != s.Model().PersonOf("maya.lindqvist@example.org") || row["author"] == "" {
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
	if rec := postMail(t, s, queue, pics, importing, []byte("Subject: no date\r\n\r\nhello\r\n")); rec.Code != http.StatusBadRequest {
		t.Fatalf("no date: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postMail(t, s, queue, pics, "rowan.ashdown@example.org", eml); rec.Code != http.StatusForbidden {
		t.Fatalf("a person: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTheMailHookFilesMailOnceAndLeavesLoopsOwn(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	eml := []byte("From: Maya Lindqvist <maya.lindqvist@example.org>\r\n" +
		"Date: Thu, 12 Feb 2026 01:48:03 +0000\r\n" +
		"Subject: Field trip\r\n" +
		"List-Id: <hummingbirds.parents.heliosschool.org>\r\n" +
		"\r\n" +
		"Bring a lunch.\r\n")
	first, err := FileMail(context.Background(), s, queue, pics, eml)
	if err != nil {
		t.Fatal(err)
	}
	again, err := FileMail(context.Background(), s, queue, pics, eml)
	if err != nil || again != first {
		t.Fatalf("filed again as %q, first %q: %v", again, first, err)
	}
	row, ok := s.Model().Table("DOCUMENT").Get(first)
	links := s.Model().Table("DOCUMENT_GROUP").Referencing("document", first)
	if !ok || row["kind"] != "mail" || row["name"] != "Field trip" || len(links) != 1 || links[0]["group"] != "grp00000000030" {
		t.Fatalf("filed %v sent to %v", row, links)
	}
	loop := []byte("Date: Thu, 12 Feb 2026 01:48:03 +0000\r\nList-Id: <team.loop.heliosian.com>\r\nSubject: Ours\r\n\r\nhello\r\n")
	if id, err := FileMail(context.Background(), s, queue, pics, loop); err != nil || id != "" {
		t.Fatalf("loop's own mail was filed as %q: %v", id, err)
	}
}

func TestEachFamilysCopyOfOneEmailIsFiledOnce(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	mail := func(from, to, id, date, list, subject string) []byte {
		header := "From: " + from + "\r\nTo: " + to + "\r\nDate: " + date + "\r\nSubject: " + subject + "\r\nMessage-Id: <" + id + ">\r\n"
		if list != "" {
			header += "List-Id: <" + list + ">\r\n"
		}
		return []byte(header + "\r\nRead it here: https://example.org/n?for=" + to + "\r\n")
	}
	newsletter := "Helios Weekly Newsletter 2026 Oct 16"
	copyTo := func(from, to, date string) []byte {
		return mail(from, to, to+"@mail3.veracross.com", date, "", newsletter)
	}
	school := "Helios School <m@mail3.veracross.com>"
	file := func(eml []byte) string {
		t.Helper()
		id, err := FileMail(context.Background(), s, queue, pics, eml)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := file(copyTo(school, "rowan@example.org", "Fri, 16 Oct 2026 20:04:26 +0000"))
	if second := file(copyTo(school, "maya@example.org", "Fri, 16 Oct 2026 20:04:28 +0000")); second != first {
		t.Fatalf("a second family's copy two seconds later was filed apart: %s and %s", first, second)
	}
	for name, eml := range map[string][]byte{
		"another sender":                      copyTo("Maya Lindqvist <maya.lindqvist@example.org>", "rowan@example.org", "Fri, 16 Oct 2026 20:04:27 +0000"),
		"an hour later":                       copyTo(school, "juni@example.org", "Fri, 16 Oct 2026 21:04:26 +0000"),
		"the week before":                     copyTo(school, "rowan@example.org", "Fri, 09 Oct 2026 20:04:26 +0000"),
		"another message to the same address": mail(school, "rowan@example.org", "rowan-again@mail3.veracross.com", "Fri, 16 Oct 2026 20:05:26 +0000", "", newsletter),
	} {
		if id := file(eml); id == first {
			t.Errorf("%s: filed as the same email", name)
		}
	}

	parent := "Patti Wilczek <patti@example.org>"
	posted := file(mail(parent, "Parents <parentsonly@heliosschool.org>", "fair@example.org", "Fri, 16 Oct 2026 20:10:00 +0000", "parentsonly.heliosschool.org", "Spring Fair"))
	if again := file(mail(parent, "Parents and Staff <parentsandstaff@heliosschool.org>", "fair@example.org", "Fri, 16 Oct 2026 20:10:00 +0000", "parentsandstaff.heliosschool.org", "Spring Fair")); again != posted {
		t.Fatalf("one message through two lists was filed apart: %s and %s", posted, again)
	}
	if got := strings.Join(sentToSorted(s, posted), " "); got != "grp00000000002 grp00000000003" {
		t.Errorf("the email is sent to %s, want the parents of the first list and the staff of the second", got)
	}
	reply := file(mail(parent, "annemarie@example.org", "reply-1@example.org", "Fri, 16 Oct 2026 20:20:00 +0000", "parentsonly.heliosschool.org", "Re: Annual Fund"))
	for name, eml := range map[string][]byte{
		"a reply on the list to someone else":   mail(parent, "kristine@example.org", "reply-2@example.org", "Fri, 16 Oct 2026 20:26:40 +0000", "parentsonly.heliosschool.org", "Re: Annual Fund"),
		"the same announcement to another list": mail(parent, "Parents and Staff <parentsandstaff@heliosschool.org>", "reply-3@example.org", "Fri, 16 Oct 2026 20:21:00 +0000", "parentsandstaff.heliosschool.org", "Re: Annual Fund"),
	} {
		if id := file(eml); id == reply {
			t.Errorf("%s: filed as the same email", name)
		}
	}
}

func sentToSorted(s *Store, document string) []string {
	out := s.Model().sentTo(document)
	slices.Sort(out)
	return out
}

func TestMailIsSentToItsListsGroups(t *testing.T) {
	s, queue := sampleWithQueue(t)
	jayvens := apply(t, s, queue, []Edit{{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "status": "open", "slug": "jayvens-parents", "name": "Jayvens Parents", "visible_to": "grp00000000004"}}})[0]
	m := s.Model()
	for listID, want := range map[string]string{
		"":                                   "mail",
		"<parentsandstaff.heliosschool.org>": "mail grp00000000002 grp00000000003",
		"<parentsonly.heliosschool.org>":     "mail grp00000000002",
		"<community.heliosns.org>":           "mail grp00000000004",
		"Hummingbirds Parents <Hummingbirds.Parents.heliosns.org>": "mail grp00000000030",
		"<2020-21.hummingbirds.parents.heliosns.org>":              "mail grp00000000030",
		"<hummingbirds.students.heliosns.org>":                     "mail grp00000000010",
		"<jaysandravens.heliosns.org>":                             "mail " + jayvens,
		"<chat.heliosschool.org>":                                  "mail grp00000000004",
		"<chat2.heliosschool.org>":                                 "mail grp00000000004",
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
	for _, listID := range []string{"<boardoftrustees.heliosschool.org>", "<board-governance.heliosschool.org>", "<michelle-level3math.parents.heliosschool.org>", "<3064358178.560896@benchmarkemail.com>"} {
		if _, sentTo, err := m.mailRoot([]byte("Date: Thu, 12 Feb 2026 01:48:03 +0000\r\nList-Id: " + listID + "\r\n\r\nhello\r\n")); err == nil {
			t.Errorf("%q: filed to %v", listID, sentTo)
		}
	}
}
