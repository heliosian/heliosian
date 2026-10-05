package db

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

func bytesOf(t *testing.T, s *Store, bucket *blob.Bucket, doc store.Row) string {
	t.Helper()
	content, _ := s.Model().Table("CONTENT").Get(doc["content"])
	raw, _, err := bucket.Get(t.Context(), content["blob"])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestMailIsReadIntoATree(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
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
		"Bring a sleeping",
		"bag.",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		`<html><body><p>Bring a <b>sleeping bag</b>.</p><img src="cid:logo-1"><p>You received this message because you are subscribed to the list.</p></body></html>`,
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
	if bytesOf(t, s, bucket, parts[2]) != string(logo) {
		t.Fatal("the logo's bytes in the bucket differ")
	}
	made(t, s, "DOCUMENT", parts[0]["id"], "extracted")
	if extracts := children(s, parts[0]["id"]); len(extracts) != 0 {
		t.Fatalf("the text part beside the HTML was read: %v", extracts)
	}
	made(t, s, "DOCUMENT", parts[1]["id"], "extracted")
	extracts := children(s, parts[1]["id"])
	if len(extracts) != 1 || extracts[0]["relation"] != "extract" {
		t.Fatalf("the HTML part's extracts: %v", extracts)
	}
	content, _ := s.Model().Table("CONTENT").Get(extracts[0]["content"])
	if got := bytesOf(t, s, bucket, extracts[0]); got != "Bring a **sleeping bag**." || content["mime"] != "text/markdown" {
		t.Fatalf("the HTML part's markdown reads %q as %s", got, content["mime"])
	}
	made(t, s, "DOCUMENT", parts[3]["id"], "extracted")
	inner := children(s, parts[3]["id"])
	if len(inner) != 1 || strings.TrimSpace(bytesOf(t, s, bucket, inner[0])) != "Tent, stove, water." {
		t.Fatalf("the forwarded message's parts: %v", inner)
	}
	made(t, s, "DOCUMENT", inner[0]["id"], "extracted")
	if extracts := children(s, inner[0]["id"]); len(extracts) != 1 || bytesOf(t, s, bucket, extracts[0]) != "Tent, stove, water." {
		t.Fatalf("the forwarded message's text was not read: %v", extracts)
	}
	for _, row := range s.Model().Table("DOCUMENT").All() {
		if row["relation"] == "extract" && len(children(s, row["id"])) > 0 {
			t.Fatalf("an extract was read again: %v", children(s, row["id"]))
		}
	}
	if parts[2]["extracted"] != "" {
		t.Fatalf("an image with no extractor was marked extracted: %v", parts[2])
	}
}

func TestHTMLImagesAreFetched(t *testing.T) {
	logo, pixel, inline, later := pngOf(t, 3), pngOf(t, 1), pngOf(t, 2), pngOf(t, 4)
	asked := map[string]int{}
	mu := sync.Mutex{}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked[r.URL.Path]++
		times := asked[r.URL.Path]
		mu.Unlock()
		switch r.URL.Path {
		case "/logo.png":
			w.Write(logo)
		case "/pixel.png":
			w.Write(pixel)
		case "/page":
			w.Write([]byte("<html><body>not a picture</body></html>"))
		case "/private.png":
			w.WriteHeader(http.StatusForbidden)
		case "/broken.png":
			w.WriteHeader(http.StatusBadGateway)
		case "/busy.png":
			if times == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Write(later)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	StartFetcher(s, queue, bucket)
	body := strings.Join([]string{
		`<html><body><p>See the schedule.</p>`,
		`<img src="` + site.URL + `/logo.png">`,
		`<img src="` + site.URL + `/logo.png">`,
		`<img src="` + site.URL + `/open.gif" width="1" height="1">`,
		`<img src="` + site.URL + `/pixel.png">`,
		`<img src="cid:logo-1">`,
		`<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(inline) + `">`,
		`<img src="` + site.URL + `/gone.png">`,
		`<img src="` + site.URL + `/page">`,
		`<img src="` + site.URL + `/private.png">`,
		`<img src="` + site.URL + `/busy.png">`,
		`<img src="` + site.URL + `/broken.png">`,
		`<img src="/relative.png">`,
		`</body></html>`,
	}, "")
	eml := strings.Join([]string{
		"From: Maya Lindqvist <maya.lindqvist@example.org>",
		"Date: Thu, 12 Feb 2026 01:48:03 +0000",
		"Subject: Clubs this week",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="alt"`,
		"",
		"--alt",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"See the schedule.",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		body,
		"--alt--",
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
	htmlPart := children(s, root)[1]
	made(t, s, "DOCUMENT", htmlPart["id"], "extracted")
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		broken := asked["/broken.png"]
		mu.Unlock()
		settled := !slices.ContainsFunc(children(s, htmlPart["id"]), func(child store.Row) bool {
			return child["content"] == "" && child["fetch"] == "" && !strings.HasSuffix(child["url"], "/broken.png")
		})
		if settled && broken > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the images were not all fetched: %v", children(s, htmlPart["id"]))
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := []string{}
	under := children(s, htmlPart["id"])
	for _, child := range under {
		content, _ := s.Model().Table("CONTENT").Get(child["content"])
		got = append(got, child["relation"]+"|"+content["mime"]+"|"+strings.TrimPrefix(child["url"], site.URL)+"|"+child["fetch"])
	}
	equalLines(t, "the HTML part's children", got, []string{
		"extract|text/markdown||",
		"image|image/png|/logo.png|",
		"image||/pixel.png|refused",
		"image|image/png||",
		"image||/gone.png|gone",
		"image||/page|refused",
		"image||/private.png|sign_in",
		"image|image/png|/busy.png|",
		"image||/broken.png|",
	})
	if bytesOf(t, s, bucket, under[1]) != string(logo) || bytesOf(t, s, bucket, under[3]) != string(inline) || bytesOf(t, s, bucket, under[7]) != string(later) {
		t.Fatal("the images' bytes in the bucket differ")
	}
	mu.Lock()
	defer mu.Unlock()
	if asked["/open.gif"] > 0 {
		t.Fatal("an image sized as a pixel was fetched")
	}
}

func equalLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}
