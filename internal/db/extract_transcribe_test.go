package db

import (
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
)

func TestAnEmailsPDFsAreTranscribed(t *testing.T) {
	t.Skip("PDF extraction is off")
	var mu sync.Mutex
	asked := 0
	intercept.Install(intercept.ClaudeHost, claudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		if !strings.Contains(request, "application/pdf") {
			t.Errorf("claude was asked something other than a pdf: %.200s", request)
		}
		asked++
		return "# Supply list\n\n- Pencils\n- Glue"
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	if err := bucket.Put(t.Context(), "content/p1", pdfType, []byte("%PDF-1.4\n% a supply list\n")); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "p1", "blob": "content/p1", "mime": pdfType, "size": "26"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Supplies", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002", "filename": "supplies.pdf"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "kind": "calendar", "content": "cnt00000000002"}),
	); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	made(t, s, "DOCUMENT", "doc00000000011", "extracted")
	under := children(s, "doc00000000011")
	if len(under) != 1 || under[0]["relation"] != "extract" || bytesOf(t, s, bucket, under[0]) != "# Supply list\n\n- Pencils\n- Glue" {
		t.Fatalf("the pdf's children: %v", under)
	}
	if calendar, _ := s.Model().Table("DOCUMENT").Get("doc00000000020"); calendar["extracted"] != "" || s.Model().readable(calendar, pdfType) {
		t.Fatalf("the year calendar's pdf is read: %v", calendar)
	}
	mu.Lock()
	defer mu.Unlock()
	if asked != 1 {
		t.Fatalf("claude was asked %d times, want once", asked)
	}
}

func TestImagesAreTranscribed(t *testing.T) {
	t.Skip("image extraction is off")
	schedule, logo, icon := pngOf(t, 200), pngOf(t, 150), pngOf(t, 50)
	var mu sync.Mutex
	asked := map[string]int{}
	intercept.Install(intercept.ClaudeHost, claudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(request, base64.StdEncoding.EncodeToString(schedule)):
			asked["schedule"]++
			return "| Day | Club |\n|---|---|\n| Monday | Chess |"
		case strings.Contains(request, base64.StdEncoding.EncodeToString(logo)):
			asked["logo"]++
			return "(decoration)"
		}
		asked["other"]++
		return "(decoration)"
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ops := []store.Op{
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Clubs", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002", "extracted": "2026-02-12 01:48:03"}),
	}
	for i, img := range [][]byte{schedule, logo, icon} {
		name := []string{"s1", "l1", "i1"}[i]
		if err := bucket.Put(t.Context(), "content/"+name, "image/png", img); err != nil {
			t.Fatal(err)
		}
		ops = append(ops, store.Insert("CONTENT", store.Row{"id": "cnt0000000010" + string(rune('0'+i)), "hash": name, "blob": "content/" + name, "mime": "image/png", "size": "1"}))
	}
	ops = append(ops,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/schedule.png", "content": "cnt00000000100"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000021", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/logo.png", "content": "cnt00000000101"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000022", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/icon.png", "content": "cnt00000000102"}),
	)
	if err := commit(s, DocumentsSheet, ops...); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	for _, id := range []string{"doc00000000020", "doc00000000021", "doc00000000022"} {
		made(t, s, "DOCUMENT", id, "extracted")
	}
	under := children(s, "doc00000000020")
	if len(under) != 1 || under[0]["relation"] != "extract" || bytesOf(t, s, bucket, under[0]) != "| Day | Club |\n|---|---|\n| Monday | Chess |" {
		t.Fatalf("the schedule's children: %v", under)
	}
	if under := children(s, "doc00000000021"); len(under) != 0 {
		t.Fatalf("the logo was transcribed: %v", under)
	}
	if under := children(s, "doc00000000022"); len(under) != 0 {
		t.Fatalf("the icon was transcribed: %v", under)
	}
	if err := commit(s, DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000023", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/again.png", "content": "cnt00000000100"})); err != nil {
		t.Fatal(err)
	}
	made(t, s, "DOCUMENT", "doc00000000023", "extracted")
	again := children(s, "doc00000000023")
	if len(again) != 1 || again[0]["content"] != under[0]["content"] {
		t.Fatalf("the same image again: %v, want the first one's transcription %s", again, under[0]["content"])
	}
	mu.Lock()
	defer mu.Unlock()
	if asked["schedule"] != 1 || asked["logo"] != 1 || asked["other"] != 0 {
		t.Fatalf("claude was asked %v, want the schedule and the logo once each and nothing else", asked)
	}
}
