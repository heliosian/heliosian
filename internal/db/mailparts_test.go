package db

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func children(s *Store, parent string) []store.Row {
	out := []store.Row{}
	for _, row := range s.Model().Table("DOCUMENT").All() {
		if row["parent"] == parent {
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
	return out
}

func TestMailIsSplitIntoItsParts(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewMailParts(s, queue, bucket)
	logo := pngOf(t, 3)
	eml := strings.Join([]string{
		"From: Maya Lindqvist <maya.lindqvist@example.org>",
		"Date: Thu, 12 Feb 2026 01:48:03 +0000",
		"Subject: Spring Camping Trip Follow Up",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="outer"`,
		"",
		"--outer",
		`Content-Type: multipart/related; boundary="related"`,
		"",
		"--related",
		`Content-Type: multipart/alternative; boundary="alt"`,
		"",
		"--alt",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Bring a sleeping bag.",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		`<html><body><p>Bring a sleeping bag.</p><img src="cid:logo-1"></body></html>`,
		"--alt--",
		"--related",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-Disposition: inline; filename=logo.png",
		"Content-Id: <logo-1>",
		"",
		base64.StdEncoding.EncodeToString(logo),
		"--related--",
		"--outer",
		"Content-Type: message/rfc822",
		`Content-Disposition: attachment; filename="=?UTF-8?Q?Packing_list?=.eml"`,
		"",
		"From: Rowan Ashdown <rowan@example.org>",
		"Date: Wed, 11 Feb 2026 09:00:00 +0000",
		"Subject: Packing list",
		"",
		"Tent, stove, water.",
		"--outer--",
		"",
	}, "\r\n")
	rec := postFile(t, s, queue, pics, "bearer:"+testImportKey, "mail", nil, "eml", []byte(eml))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	root := out.Result[0]
	made(t, s, "DOCUMENT", root, "extracted")
	parts := children(s, root)
	got := []string{}
	for _, part := range parts {
		content, _ := s.Model().Table("CONTENT").Get(part["content"])
		got = append(got, part["relation"]+"|"+content["mime"]+"|"+part["filename"]+"|"+part["content_id"])
	}
	equalLines(t, "parts", got, []string{
		"part|text/plain; charset=utf-8||",
		"part|text/html; charset=utf-8||",
		"part|image/png|logo.png|logo-1",
		"part|message/rfc822|Packing list.eml|",
	})
	content, _ := s.Model().Table("CONTENT").Get(parts[2]["content"])
	if stored, _, err := bucket.Get(t.Context(), content["blob"]); err != nil || string(stored) != string(logo) {
		t.Fatalf("the logo's bytes in the bucket: %v", err)
	}
	made(t, s, "DOCUMENT", parts[3]["id"], "extracted")
	inner := children(s, parts[3]["id"])
	if len(inner) != 1 {
		t.Fatalf("the forwarded message's parts: %v", inner)
	}
	text, _ := s.Model().Table("CONTENT").Get(inner[0]["content"])
	if body, _, err := bucket.Get(t.Context(), text["blob"]); err != nil || strings.TrimSpace(string(body)) != "Tent, stove, water." {
		t.Fatalf("the forwarded message's text reads %q: %v", body, err)
	}
	for _, part := range parts[:3] {
		if part["extracted"] != "" {
			t.Fatalf("a part that is not a message was marked extracted: %v", part)
		}
	}
}

func equalLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}
