package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/bmp"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
)

func TestAnEmailsPDFsAreTranscribed(t *testing.T) {
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
	pdf := pdfOfPages(t, 2)
	if err := bucket.Put(t.Context(), "content/p1", pdfType, pdf); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "p1", "blob": "content/p1", "mime": pdfType, "size": fmt.Sprint(len(pdf))}),
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
	mu.Lock()
	defer mu.Unlock()
	if asked != 1 {
		t.Fatalf("claude was asked %d times, want once", asked)
	}
}

func gifOf(t *testing.T, size int) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := gif.Encode(buf, image.NewRGBA(image.Rect(0, 0, size, size)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func bmpOf(t *testing.T, size int) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := bmp.Encode(buf, image.NewRGBA(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T, size int) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, image.NewRGBA(image.Rect(0, 0, size, size)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestACopyOfASplitPDFTakesItsPagesText(t *testing.T) {
	intercept.Install(intercept.ClaudeHost, claudeReplying(func(request string) string {
		t.Errorf("claude was asked about a pdf already read: %.200s", request)
		return ""
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	for name, body := range map[string]string{"content/m1": "# Slides 1–20", "content/m2": "# Slides 21–25"} {
		if err := bucket.Put(t.Context(), name, "text/markdown", []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := bucket.Put(t.Context(), "content/p0", pdfType, pdfOfPages(t, 25)); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "p0", "blob": "content/p0", "mime": pdfType, "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "p1", "blob": "content/p1", "mime": pdfType, "size": "60"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "p2", "blob": "content/p2", "mime": pdfType, "size": "40"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000004", "hash": "m1", "blob": "content/m1", "mime": "text/markdown", "size": "13"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000005", "hash": "m2", "blob": "content/m2", "mime": "text/markdown", "size": "14"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "file", "content": "cnt00000000001", "name": "Updates", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "pages", "parent": "doc00000000010", "content": "cnt00000000003", "name": "pages 21–25 of 25", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "pages", "parent": "doc00000000010", "content": "cnt00000000002", "name": "pages 1–20 of 25", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000013", "relation": "extract", "parent": "doc00000000011", "content": "cnt00000000004"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000014", "relation": "extract", "parent": "doc00000000012", "content": "cnt00000000005"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "kind": "file", "content": "cnt00000000001", "name": "Updates again"}),
	); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	made(t, s, "DOCUMENT", "doc00000000020", "extracted")
	under := children(s, "doc00000000020")
	if len(under) != 1 || under[0]["relation"] != "extract" || bytesOf(t, s, bucket, under[0]) != "# Slides 1–20\n\n# Slides 21–25" {
		t.Fatalf("the copy's children: %v", under)
	}
}

func TestABlockedAnswerLeavesTheImageUnread(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	intercept.Install(intercept.ClaudeHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"type":"error","error":{"type":"invalid_request_error","message":"Output blocked by content filtering policy"}}`)
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	photo := jpegOf(t, 300)
	if err := bucket.Put(t.Context(), "content/b1", "image/jpeg", photo); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "b1", "blob": "content/b1", "mime": "image/jpeg", "size": "1"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Clubs", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/photo.jpg", "content": "cnt00000000003"}),
	); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	made(t, s, "DOCUMENT", "doc00000000020", "extracted")
	if under := children(s, "doc00000000020"); len(under) != 0 {
		t.Fatalf("a blocked answer was written: %v", under)
	}
	mu.Lock()
	defer mu.Unlock()
	if asked != 1 {
		t.Fatalf("claude was asked %d times, want once", asked)
	}
}

func TestAnOversizeJPEGIsSentAsASmallerJPEG(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, image.NewRGBA(image.Rect(0, 0, imageLargest+1, 100)), nil); err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	body, mimeType, err := claudeImage(buf.Bytes(), "image/jpeg", config)
	if err != nil {
		t.Fatal(err)
	}
	sent, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if mimeType != "image/jpeg" || sent.Width != imageSentLongest || len(body) > imageMostBytes {
		t.Fatalf("sent as %s at %dx%d in %d bytes, want a JPEG %d wide", mimeType, sent.Width, sent.Height, len(body), imageSentLongest)
	}
}

func TestImagesAreTranscribed(t *testing.T) {
	schedule, logo, icon, flyer, chart, sign := gifOf(t, 200), gifOf(t, 150), gifOf(t, 50), bmpOf(t, 120), pngOf(t, 130), jpegOf(t, 140)
	poster, err := base64.StdEncoding.DecodeString("UklGRiQAAABXRUJQVlA4TBcAAAAvd8AdAAfQ//73v/9hABLC//9KRP9TgwA=")
	if err != nil {
		t.Fatal(err)
	}
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
		case strings.Contains(request, base64.StdEncoding.EncodeToString(poster)):
			asked["poster"]++
			return "Art show Thursday"
		case strings.Contains(request, base64.StdEncoding.EncodeToString(chart)):
			asked["chart"]++
			return "Reading log: 20 minutes a night"
		case strings.Contains(request, base64.StdEncoding.EncodeToString(sign)):
			asked["sign"]++
			return "Pick-up moves to the north gate"
		case strings.Contains(request, `"media_type":"image/png"`) || strings.Contains(request, `"media_type":"image/jpeg"`):
			asked["flyer"]++
			return "Book fair Friday"
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
	for i, img := range [][]byte{schedule, logo, icon, flyer, poster, chart, sign} {
		name := []string{"s1", "l1", "i1", "f1", "p1", "c1", "j1"}[i]
		mime := []string{"image/gif", "image/gif", "image/gif", "image/bmp", "image/webp", "image/png", "image/jpeg"}[i]
		if err := bucket.Put(t.Context(), "content/"+name, mime, img); err != nil {
			t.Fatal(err)
		}
		ops = append(ops, store.Insert("CONTENT", store.Row{"id": "cnt0000000020" + string(rune('0'+i)), "hash": name, "blob": "content/" + name, "mime": mime, "size": "1"}))
	}
	ops = append(ops,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/schedule.gif", "content": "cnt00000000200"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000021", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/logo.gif", "content": "cnt00000000201"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000022", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/icon.gif", "content": "cnt00000000202"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000024", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/flyer.bmp", "content": "cnt00000000203"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000025", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/poster.webp", "content": "cnt00000000204"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000026", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/chart.png", "content": "cnt00000000205"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000027", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/sign.jpg", "content": "cnt00000000206"}),
	)
	if err := commit(s, DocumentsSheet, ops...); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	for _, id := range []string{"doc00000000020", "doc00000000021", "doc00000000022", "doc00000000024", "doc00000000025", "doc00000000026", "doc00000000027"} {
		made(t, s, "DOCUMENT", id, "extracted")
	}
	if flyerText := children(s, "doc00000000024"); len(flyerText) != 1 || bytesOf(t, s, bucket, flyerText[0]) != "Book fair Friday" {
		t.Fatalf("the BMP flyer's children: %v", flyerText)
	}
	if posterText := children(s, "doc00000000025"); len(posterText) != 1 || bytesOf(t, s, bucket, posterText[0]) != "Art show Thursday" {
		t.Fatalf("the WebP poster's children: %v", posterText)
	}
	if chartText := children(s, "doc00000000026"); len(chartText) != 1 || bytesOf(t, s, bucket, chartText[0]) != "Reading log: 20 minutes a night" {
		t.Fatalf("the PNG chart's children: %v", chartText)
	}
	if signText := children(s, "doc00000000027"); len(signText) != 1 || bytesOf(t, s, bucket, signText[0]) != "Pick-up moves to the north gate" {
		t.Fatalf("the JPEG sign's children: %v", signText)
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
	if err := commit(s, DocumentsSheet, store.Insert("DOCUMENT", store.Row{"id": "doc00000000023", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/again.gif", "content": "cnt00000000200"})); err != nil {
		t.Fatal(err)
	}
	made(t, s, "DOCUMENT", "doc00000000023", "extracted")
	again := children(s, "doc00000000023")
	if len(again) != 1 || again[0]["content"] != under[0]["content"] {
		t.Fatalf("the same image again: %v, want the first one's transcription %s", again, under[0]["content"])
	}
	mu.Lock()
	defer mu.Unlock()
	if asked["schedule"] != 1 || asked["logo"] != 1 || asked["flyer"] != 1 || asked["poster"] != 1 || asked["chart"] != 1 || asked["sign"] != 1 || asked["other"] != 0 {
		t.Fatalf("claude was asked %v, want the schedule, the logo, the flyer, the poster, the chart and the sign once each and nothing else", asked)
	}
}
