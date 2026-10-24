package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/bmp"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/testkit"
)

func TestAnEmailsPDFsAreTranscribed(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
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
	if _, err := bucket.Put(t.Context(), "content/p1", pdfType, pdf, 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "pdf", Row: map[string]any{"hash": "p1", "blob": "content/p1", "mime": pdfType, "size": fmt.Sprint(len(pdf))}},
		{Insert: "DOCUMENT", As: "supplies", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Supplies", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "part", "parent": "@supplies", "content": "@pdf", "filename": "supplies.pdf"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "calendar", "content": "@pdf"}},
	})
	part := ids[3]
	NewExtractor(s, queue, bucket, "test")
	made(t, s, "DOCUMENT", part, "extracted")
	under := children(s, part)
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

func TestACopyOfASplitPDFIsSplitToo(t *testing.T) {
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(string) string { return "# Slides" }))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	if _, err := bucket.Put(t.Context(), "content/p0", pdfType, pdfOfPages(t, 25), 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "whole", Row: map[string]any{"hash": "p0", "blob": "content/p0", "mime": pdfType, "size": "100"}},
		{Insert: "CONTENT", As: "first", Row: map[string]any{"hash": "p1", "blob": "content/p1", "mime": pdfType, "size": "60"}},
		{Insert: "CONTENT", As: "second", Row: map[string]any{"hash": "p2", "blob": "content/p2", "mime": pdfType, "size": "40"}},
		{Insert: "DOCUMENT", As: "updates", Row: map[string]any{"kind": "file", "content": "@whole", "name": "Updates", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "pages", "parent": "@updates", "content": "@first", "name": "pages 1–20 of 25", "first_page": "1", "last_page": "20", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "pages", "parent": "@updates", "content": "@second", "name": "pages 21–25 of 25", "first_page": "21", "last_page": "25", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@whole", "name": "Updates again"}},
	})
	copied := ids[6]
	NewExtractor(s, queue, bucket, "test")
	made(t, s, "DOCUMENT", copied, "extracted")
	names := []string{}
	for _, child := range children(s, copied) {
		names = append(names, child["relation"]+" "+child["name"]+": "+child["first_page"]+" to "+child["last_page"])
	}
	equalLines(t, "the copy's children", names, []string{"pages pages 1–20 of 25: 1 to 20", "pages pages 21–25 of 25: 21 to 25"})
}

func TestABlockedAnswerLeavesTheImageUnread(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	photo := jpegOf(t, 300)
	intercept.Install(intercept.ClaudeHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if bytes.Contains(body, []byte(base64.StdEncoding.EncodeToString(photo))) {
			mu.Lock()
			asked++
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`)
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", `{"type":"error","error":{"type":"invalid_request_error","message":"Output blocked by content filtering policy"}}`)
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	if _, err := bucket.Put(t.Context(), "content/b1", "image/jpeg", photo, 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}},
		{Insert: "CONTENT", As: "photo", Row: map[string]any{"hash": "b1", "blob": "content/b1", "mime": "image/jpeg", "size": "1"}},
		{Insert: "DOCUMENT", As: "clubs", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Clubs", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@clubs", "content": "@html", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://example.org/photo.jpg", "content": "@photo"}},
	})
	pictured := ids[5]
	NewExtractor(s, queue, bucket, "test")
	made(t, s, "DOCUMENT", pictured, "extracted")
	if under := children(s, pictured); len(under) != 0 {
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
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
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
	edits := []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}},
		{Insert: "DOCUMENT", As: "clubs", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Clubs", "extracted": "2026-02-12 01:48:03"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@clubs", "content": "@html", "extracted": "2026-02-12 01:48:03"}},
	}
	files := []string{"schedule.gif", "logo.gif", "icon.gif", "flyer.bmp", "poster.webp", "chart.png", "sign.jpg"}
	blobs := []string{"s1", "l1", "i1", "f1", "p1", "c1", "j1"}
	for i, img := range [][]byte{schedule, logo, icon, flyer, poster, chart, sign} {
		name := blobs[i]
		mime := []string{"image/gif", "image/gif", "image/gif", "image/bmp", "image/webp", "image/png", "image/jpeg"}[i]
		if _, err := bucket.Put(t.Context(), "content/"+name, mime, img, 0); err != nil {
			t.Fatal(err)
		}
		edits = append(edits, Edit{Insert: "CONTENT", As: name, Row: map[string]any{"hash": name, "blob": "content/" + name, "mime": mime, "size": "1"}})
	}
	for i, file := range files {
		edits = append(edits, Edit{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://example.org/" + file, "content": "@" + blobs[i]}})
	}
	ids := apply(t, s, queue, edits)
	images := ids[len(ids)-len(files):]
	scheduleDoc, logoDoc, iconDoc, flyerDoc, posterDoc, chartDoc, signDoc := images[0], images[1], images[2], images[3], images[4], images[5], images[6]
	NewExtractor(s, queue, bucket, "test")
	for _, id := range images {
		made(t, s, "DOCUMENT", id, "extracted")
	}
	if flyerText := children(s, flyerDoc); len(flyerText) != 1 || bytesOf(t, s, bucket, flyerText[0]) != "Book fair Friday" {
		t.Fatalf("the BMP flyer's children: %v", flyerText)
	}
	if posterText := children(s, posterDoc); len(posterText) != 1 || bytesOf(t, s, bucket, posterText[0]) != "Art show Thursday" {
		t.Fatalf("the WebP poster's children: %v", posterText)
	}
	if chartText := children(s, chartDoc); len(chartText) != 1 || bytesOf(t, s, bucket, chartText[0]) != "Reading log: 20 minutes a night" {
		t.Fatalf("the PNG chart's children: %v", chartText)
	}
	if signText := children(s, signDoc); len(signText) != 1 || bytesOf(t, s, bucket, signText[0]) != "Pick-up moves to the north gate" {
		t.Fatalf("the JPEG sign's children: %v", signText)
	}
	under := children(s, scheduleDoc)
	if len(under) != 1 || under[0]["relation"] != "extract" || bytesOf(t, s, bucket, under[0]) != "| Day | Club |\n|---|---|\n| Monday | Chess |" {
		t.Fatalf("the schedule's children: %v", under)
	}
	if under := children(s, logoDoc); len(under) != 0 {
		t.Fatalf("the logo was transcribed: %v", under)
	}
	if under := children(s, iconDoc); len(under) != 0 {
		t.Fatalf("the icon was transcribed: %v", under)
	}
	againDoc := apply(t, s, queue, []Edit{{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": ids[3], "url": "https://example.org/again.gif", "content": ids[4]}}})[0]
	made(t, s, "DOCUMENT", againDoc, "extracted")
	again := children(s, againDoc)
	if len(again) != 1 || again[0]["content"] != under[0]["content"] {
		t.Fatalf("the same image again: %v, want the first one's transcription %s", again, under[0]["content"])
	}
	mu.Lock()
	defer mu.Unlock()
	if asked["schedule"] != 1 || asked["logo"] != 1 || asked["flyer"] != 1 || asked["poster"] != 1 || asked["chart"] != 1 || asked["sign"] != 1 || asked["other"] != 0 {
		t.Fatalf("claude was asked %v, want the schedule, the logo, the flyer, the poster, the chart and the sign once each and nothing else", asked)
	}
}
