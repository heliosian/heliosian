package db

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
	"heliosian/internal/trace"
)

func TestContentReferencesCannotPublishUnreadableBytes(t *testing.T) {
	for _, column := range []string{"original", "reencode", "crop", "thumbnail"} {
		t.Run(column, func(t *testing.T) {
			s := sample(t)
			ids := apply(t, s, []Edit{
				{Insert: "CONTENT", As: "private", Row: map[string]any{"hash": "private", "blob": "content/private", "mime": "image/png", "size": "100"}},
				{Insert: "PHOTO", Row: map[string]any{"person": staff, "original": "@private"}},
				{Insert: "CONTENT", As: "public", Row: map[string]any{"hash": "public", "blob": "content/public", "mime": "image/png", "size": "100"}},
				{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@public"}},
			})
			env := Env{Viewer: parent, Now: testNow}
			if _, _, ok := s.Model().BlobCell(trace.New("private"), env, ids[0], "blob"); ok {
				t.Fatal("the other person's original was already readable")
			}
			row := map[string]any{"person": parent, "original": ids[2], column: ids[0]}
			written, err := Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, env, Batch{Batch: []Edit{{Insert: "PHOTO", Row: row}}})
			if err == nil {
				_, _, readable := s.Model().BlobCell(trace.New("published"), env, ids[0], "blob")
				t.Fatalf("published unreadable content as PHOTO.%s: %v (bytes readable: %v)", column, written, readable)
			}
		})
	}
}

func TestReadableContentCanBeReused(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "public", Row: map[string]any{"hash": "public", "blob": "content/public", "mime": "image/png", "size": "100"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@public"}},
	})
	env := Env{Viewer: parent, Now: testNow}
	if _, err := Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, env, Batch{Batch: []Edit{{Insert: "PHOTO", Row: map[string]any{"person": parent, "original": ids[0]}}}}); err != nil {
		t.Fatalf("reuse readable content: %v", err)
	}
	photo, _ := s.Model().Table("PHOTO").Find(parent, "", "", ids[0])
	if err := s.Model().Authorize(env, Change{Table: "PHOTO", Old: photo, New: store.Row{"id": photo["id"], "person": parent, "original": ids[0], "order": "a"}}); err != nil {
		t.Fatalf("reorder a photo: %v", err)
	}
}

func TestContentReferencesAreCheckedOnUpdates(t *testing.T) {
	s := sample(t)
	private := apply(t, s, []Edit{{Insert: "CONTENT", Row: map[string]any{"hash": "private-recording", "blob": "content/private-recording", "mime": "audio/mp4", "size": "100"}}})[0]
	for _, id := range []string{parent, "grp00000000020"} {
		_, err := Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"pronunciation_original": private}}}})
		if err == nil || !strings.Contains(err.Error(), "unreadable content") {
			t.Errorf("attaching private bytes to %s: %v", id, err)
		}
	}
	if _, err := ParseBatch([]byte(`{"batch":[{"set":"` + parent + `","cells":{"pronunciation_original":"` + private + `"},"uploaded":"` + private + `"}]}`)); err == nil {
		t.Fatal("a client supplied its own upload attestation")
	}
}

func TestUploadingBytesCanReusePrivateContent(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	picture := pngOf(t, 7)
	if rec := addPhoto(t, s, pics, importing, staff, picture); rec.Code != http.StatusOK {
		t.Fatalf("the first upload: %d %s", rec.Code, rec.Body.String())
	}
	content, _ := s.Model().Table("CONTENT").Find(blob.Hash(picture), "image/png")
	if _, _, ok := s.Model().BlobCell(trace.New("private"), Env{Viewer: parent, Now: testNow}, content["id"], "blob"); ok {
		t.Fatal("the first upload's original was already readable")
	}
	if rec := addPhoto(t, s, pics, "rowan.ashdown@example.org", parent, picture); rec.Code != http.StatusOK {
		t.Fatalf("uploading the same bytes as a member: %d %s", rec.Code, rec.Body.String())
	}
	if photos := s.Model().Table("PHOTO").Referencing("original", content["id"]); len(photos) != 2 {
		t.Fatalf("the two uploads did not reuse their content: %v", photos)
	}
}
