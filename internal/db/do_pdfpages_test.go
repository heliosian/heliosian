package db

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"

	"heliosian/internal/blob"
)

func TestClientPDFPagesAreStoredInOrderAndResume(t *testing.T) {
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, bucket)
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "pdf", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 3))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@pdf"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "calendar", "content": "@pdf"}},
	})
	first := redPNG(t, 120, 180)
	last := redPNG(t, 180, 120)
	fields := map[string]string{"document": ids[1], "content": ids[0], "page": "1", "pages": "3"}
	post := func(page int, picture []byte, want int) {
		t.Helper()
		fields["page"] = strconv.Itoa(page)
		rec := postFile(t, s, pics, importing, "pdf-page", fields, "picture", picture)
		if rec.Code != want {
			t.Fatalf("page %d: %d %s", page, rec.Code, rec.Body.String())
		}
	}
	if rec := postFile(t, s, pics, "rowan.ashdown@example.org", "pdf-page", fields, "picture", first); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent uploaded a pdf page: %d", rec.Code)
	}
	post(2, last, http.StatusBadRequest)
	post(1, first, http.StatusOK)
	post(1, first, http.StatusOK)
	post(1, last, http.StatusBadRequest)
	post(2, []byte("not a png"), http.StatusBadRequest)
	if rows := children(s, ids[1]); len(rows) != 1 {
		t.Fatalf("duplicate or refused uploads made children: %v", rows)
	}
	fields["pages"] = "2"
	post(2, last, http.StatusBadRequest)
	fields["pages"] = "3"
	post(2, last, http.StatusOK)
	if doc, _ := s.Model().Table("DOCUMENT").Get(ids[1]); doc["rendered"] != "" || doc["page_count"] != "3" {
		t.Fatalf("the partial pdf: %v", doc)
	}
	post(3, first, http.StatusOK)
	post(3, first, http.StatusOK)
	doc, _ := s.Model().Table("DOCUMENT").Get(ids[1])
	if doc["rendered"] == "" {
		t.Fatalf("the complete pdf: %v", doc)
	}
	rows := children(s, ids[1])
	if len(rows) != 3 {
		t.Fatalf("page children: %v", rows)
	}
	for i, row := range rows {
		n := strconv.Itoa(i + 1)
		if row["relation"] != "page" || row["first_page"] != n || row["last_page"] != n || row["source"] != ids[1] || row["extracted"] == "" {
			t.Fatalf("page %d: %v", i+1, row)
		}
		body := []byte(bytesOf(t, s, bucket, row))
		want := first
		if i == 1 {
			want = last
		}
		if !bytes.Equal(body, want) {
			t.Fatalf("page %d lost its pixels", i+1)
		}
	}
	fields["document"] = ids[2]
	post(1, first, http.StatusOK)
	if other := children(s, ids[2]); other[0]["content"] != rows[0]["content"] {
		t.Fatal("identical page bytes have different content rows")
	}
	fields["document"] = ids[1]
	newer := apply(t, s, []Edit{{Insert: "CONTENT", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 1))}})[0]
	apply(t, s, []Edit{{Set: ids[1], Cells: map[string]any{"content": newer}}})
	if rows := children(s, ids[1]); len(rows) != 0 {
		t.Fatalf("stale page images survived: %v", rows)
	}
	doc, _ = s.Model().Table("DOCUMENT").Get(ids[1])
	if doc["rendered"] != "" || doc["page_count"] != "" {
		t.Fatalf("stale completion markers survived: %v", doc)
	}
	post(1, first, http.StatusBadRequest)
}
