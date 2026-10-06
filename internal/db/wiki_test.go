package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func TestTheLoadRefusesWikiPagesInALoopOrUnderOtherDocuments(t *testing.T) {
	s := sample(t)
	loop := []store.Op{
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000091", "kind": "wiki", "name": "One", "parent": "doc00000000092"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000092", "kind": "wiki", "name": "Two", "parent": "doc00000000091"}),
	}
	if err := commit(s, DocumentsSheet, loop...); err == nil {
		t.Fatal("two wiki pages under each other loaded")
	}
	mail := s.Model().Table("DOCUMENT").All()[0]
	if err := commit(s, DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000093", "kind": "wiki", "name": "Three", "parent": mail["id"]})); err == nil {
		t.Fatalf("a wiki page under %s %s loaded", mail["kind"], mail["id"])
	}
}

func TestWikiSideCards(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	save := func(page wikiPage) string {
		t.Helper()
		id, _, err := saveWiki(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, page)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	cards := func(id string) []store.Row {
		out := []store.Row{}
		for _, row := range s.Model().Table("DOCUMENT").Referencing("parent", id) {
			if row["relation"] == "side" {
				out = append(out, row)
			}
		}
		slices.SortFunc(out, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
		return out
	}
	id := save(wikiPage{Name: "Pickup", Body: "At the gate.", Sides: []wikiSide{{Name: "Contacts", Body: "Ask the office."}, {Name: "Times", Body: "3:15"}}})
	got := cards(id)
	if len(got) != 2 || got[0]["name"] != "Contacts" || got[1]["name"] != "Times" {
		t.Fatalf("the cards read %v", got)
	}
	save(wikiPage{Document: id, Name: "Pickup", Body: "At the gate.", Sides: []wikiSide{{ID: got[1]["id"], Name: "Times", Body: "3:30"}}})
	after := cards(id)
	if len(after) != 1 || after[0]["id"] != got[1]["id"] {
		t.Fatalf("after dropping one the cards read %v", after)
	}
	if content, _ := s.Model().Table("CONTENT").Get(after[0]["content"]); content["size"] != "4" {
		t.Fatalf("the rewritten card's content reads %v", content)
	}
	save(wikiPage{Document: id, Name: "Pickup again", Body: "At the gate."})
	if len(cards(id)) != 1 {
		t.Fatal("a save sending no cards changed them")
	}
	if _, err := Write(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Delete: after[0]["id"]}, {Delete: id}}}); err != nil {
		t.Fatalf("deleting the page and its card: %v", err)
	}
}

func TestTheImportStartsWikiPages(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	save := func(page wikiPage) (string, error) {
		id, _, err := saveWiki(ctx, s, queue, pics, access.System(importReader), Env{System: importReader, Now: testNow}, page)
		return id, err
	}
	top, err := save(wikiPage{Name: "Academics", Body: "How the school teaches."})
	if err != nil {
		t.Fatalf("the import's page: %v", err)
	}
	sub, err := save(wikiPage{Parent: top, Name: "Reading", Body: "Every day.", Sides: []wikiSide{{Name: "Common Questions", Body: "- When does it start?"}}})
	if err != nil {
		t.Fatalf("the import's sub-page with a card: %v", err)
	}
	if row, _ := s.Model().Table("DOCUMENT").Get(sub); row["parent"] != top || row["author"] != "" || len(s.Model().Table("DOCUMENT").Referencing("parent", sub)) != 1 {
		t.Fatalf("the imported sub-page reads %v", row)
	}
}

func TestWikiSlugs(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	save := func(viewer string, page wikiPage) (string, error) {
		id, _, err := saveWiki(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: viewer, Now: testNow}, page)
		return id, err
	}
	slugOf := func(id string) string {
		row, _ := s.Model().Table("DOCUMENT").Get(id)
		return row["slug"]
	}
	id, err := save(student, wikiPage{Name: "Pickup", Slug: "pickup", Body: "At the gate."})
	if err != nil || slugOf(id) != "pickup" {
		t.Fatalf("the author's new page with a slug: %v %q", err, slugOf(id))
	}
	if _, err := save(staff, wikiPage{Document: id, Name: "Pickup", Slug: "pickup", Body: "At the side gate."}); err != nil {
		t.Fatalf("someone else's edit keeping the slug: %v", err)
	}
	for _, bad := range []string{"Pickup", "pick--up", "-pickup", "pick up", "doc00000000101", strings.Repeat("a", 41)} {
		if _, err := save(student, wikiPage{Document: id, Name: "Pickup", Slug: bad, Body: "At the side gate."}); err == nil {
			t.Errorf("the slug %q was taken", bad)
		}
	}
	if _, err := save(student, wikiPage{Name: "Another", Slug: "getting-started", Body: "x"}); err == nil {
		t.Fatal("a second page took a slug another page has")
	}
	if _, err := save(student, wikiPage{Document: id, Name: "Pickup", Slug: "pickup-times", Body: "At the side gate."}); err != nil {
		t.Fatalf("the author's change of slug: %v", err)
	}
	m := s.Model()
	if redirect, ok := m.Table("REDIRECT").Find("wiki", "/p/pickup"); !ok || redirect["new"] != "/p/"+id {
		t.Fatalf("the old slug's redirect reads %v", redirect)
	}
	if seen := as(t, s, student, `(from REDIRECT (where (= app "wiki")))`); len(seen) != 1 {
		t.Fatalf("a student reads %d of the wiki's redirects", len(seen))
	}
	for _, key := range []string{id, "pickup-times", "pickup"} {
		if got := m.wikiPageAt(key); got != id {
			t.Errorf("%s finds %q", key, got)
		}
	}
	if got := m.wikiPageAt("doc00000000001"); got != "" {
		t.Errorf("a newsletter's id finds %q", got)
	}
	if _, err := save(parent, wikiPage{Document: id, Name: "Pickup", Slug: "", Body: "At the side gate."}); err != nil || slugOf(id) != "" {
		t.Fatalf("a wiki admin's removal of the slug: %v %q", err, slugOf(id))
	}
	imported, _, err := saveWiki(ctx, s, queue, pics, access.System(importReader), Env{System: importReader, Now: testNow}, wikiPage{Name: "Imported", Body: "From the doc."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := save(student, wikiPage{Document: imported, Name: "Imported", Slug: "imported", Body: "From the doc."}); err == nil {
		t.Fatal("someone gave a page with no author a slug")
	}
	if _, err := save(parent, wikiPage{Document: imported, Name: "Imported", Slug: "imported", Body: "From the doc."}); err != nil {
		t.Fatalf("a wiki admin's slug on a page with no author: %v", err)
	}
}

func TestWikiHiddenPages(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	id, _, err := saveWiki(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: student, Now: testNow}, wikiPage{Name: "Pickup", Body: "At the gate."})
	if err != nil {
		t.Fatal(err)
	}
	hide := func(viewer string, hidden bool) error {
		_, err := Write(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"hidden": hidden}}}})
		return err
	}
	hiddenOf := func() string {
		row, _ := s.Model().Table("DOCUMENT").Get(id)
		return row["hidden"]
	}
	if err := hide(student, true); err != nil || hiddenOf() != "Yes" {
		t.Fatalf("the author hiding their page: %v %q", err, hiddenOf())
	}
	if err := hide(guest, false); err == nil {
		t.Fatal("someone else showed the author's page again")
	}
	if err := hide(parent, false); err != nil || hiddenOf() != "No" {
		t.Fatalf("a wiki admin showing the page again: %v %q", err, hiddenOf())
	}
	if err := commit(s, DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000094", "kind": "newsletter", "name": "News", "hidden": "Yes"})); err == nil {
		t.Fatal("a hidden newsletter loaded")
	}
}

func TestWikiPicturesAreServed(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	picture := pngOf(t, 4)
	name := blob.Name(picture, "png")
	if err := pics.bucket.Put(context.Background(), "wiki-images/"+name, "image/png", picture); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, s, queue, pics, []byte(testImportKey), func() time.Time { return testNow })
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		auth.Fixed("rowan.ashdown@example.org", mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if rec := get("/api/wiki/picture/" + name); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() != len(picture) {
		t.Fatalf("the picture answered %d %q, %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	for _, path := range []string{"/api/wiki/picture/" + strings.Repeat("b", 64) + ".png", "/api/wiki/picture/secret.png"} {
		if rec := get(path); rec.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, rec.Code)
		}
	}
}

func TestWikiPages(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	save := func(viewer string, page wikiPage) (string, error) {
		id, _, err := saveWiki(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: viewer, Now: testNow}, page)
		return id, err
	}
	written := func(viewer string, e Edit) error {
		_, err := Write(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{e}})
		return err
	}
	bodyOf := func(id string) string {
		t.Helper()
		row, _ := s.Model().Table("DOCUMENT").Get(id)
		content, ok := s.Model().Table("CONTENT").Get(row["content"])
		if !ok || content["mime"] != wikiMime {
			t.Fatalf("the page's content reads %v", content)
		}
		raw, _, err := pics.bucket.Get(ctx, content["blob"])
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	id, err := save(parent, wikiPage{Name: " Aftercare tips ", Body: "# Pickup\nBe early."})
	if err != nil {
		t.Fatal(err)
	}
	row, _ := s.Model().Table("DOCUMENT").Get(id)
	if row["kind"] != "wiki" || row["name"] != "Aftercare tips" || row["author"] != parent || row["published"] == "" || bodyOf(id) != "# Pickup\nBe early." {
		t.Fatalf("a new page reads %v", row)
	}
	if again, err := save(staff, wikiPage{Document: id, Name: "Aftercare", Body: "Be on time."}); err != nil || again != id {
		t.Fatalf("another person's edit: %v %v", again, err)
	}
	if row, _ := s.Model().Table("DOCUMENT").Get(id); row["name"] != "Aftercare" || row["author"] != parent || bodyOf(id) != "Be on time." {
		t.Fatalf("the edited page reads %v", row)
	}
	if pages := as(t, s, student, `(from DOCUMENT (where (= id "`+id+`")))`); len(pages) != 1 {
		t.Fatalf("a student reads %d of the new page", len(pages))
	}

	var refusal *access.Refusal
	if _, err := save(guest, wikiPage{Name: "From outside", Body: "a guest's words"}); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Fatalf("a guest's page: %v", err)
	}
	sum := sha256.Sum256([]byte("a guest's words"))
	if found, _ := pics.bucket.Exists(ctx, contentFolder+"/"+hex.EncodeToString(sum[:])); found {
		t.Fatal("a refused page left its bytes")
	}
	if _, err := save(parent, wikiPage{Name: "  ", Body: "no title"}); err == nil {
		t.Fatal("a page with no title was saved")
	}
	if err := written(parent, Edit{Insert: "CONTENT", Row: map[string]any{"hash": "abc", "blob": "photos/private.jpg", "mime": wikiMime, "size": "3"}}); err == nil {
		t.Fatal("a person inserted content of their own naming")
	}
	if err := written(student, Edit{Delete: id}); err == nil {
		t.Fatal("someone else deleted the page")
	}

	first, err := save(parent, wikiPage{Parent: id, Name: "Pickup", Body: "At the gate."})
	if err != nil {
		t.Fatalf("a sub-page: %v", err)
	}
	second, err := save(parent, wikiPage{Parent: id, Name: "Snacks", Body: "Nut-free."})
	if err != nil {
		t.Fatalf("a second sub-page: %v", err)
	}
	a, _ := s.Model().Table("DOCUMENT").Get(first)
	b, _ := s.Model().Table("DOCUMENT").Get(second)
	if a["parent"] != id || a["kind"] != "wiki" || store.CompareKeys(a["order"], b["order"]) >= 0 {
		t.Fatalf("the sub-pages read %v and %v", a, b)
	}
	if _, err := save(parent, wikiPage{Document: id, Parent: first, Name: "Aftercare", Body: "Be on time."}); err == nil {
		t.Fatal("a page went under its own sub-page")
	}
	if _, err := save(parent, wikiPage{Parent: "doc00000000001", Name: "Under mail", Body: "no"}); err == nil {
		t.Fatal("a page went under a document that is no wiki page")
	}
	if err := written(parent, Edit{Delete: id}); err == nil {
		t.Fatal("a page with sub-pages was deleted")
	}
	if _, err := save(staff, wikiPage{Document: second, Name: "Snacks", Body: "Nut-free."}); err != nil {
		t.Fatalf("moving a sub-page to the top: %v", err)
	}
	if moved, _ := s.Model().Table("DOCUMENT").Get(second); moved["parent"] != "" || moved["order"] == "" {
		t.Fatalf("the moved page reads %v", moved)
	}
	for _, gone := range []string{first, id} {
		if err := written(parent, Edit{Delete: gone}); err != nil {
			t.Fatalf("the author's delete of %s: %v", gone, err)
		}
	}
}
