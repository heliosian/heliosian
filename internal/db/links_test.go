package db

import (
	"strings"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
)

func TestAFetchedPageGetsNoImagesOfItsOwn(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	page := []byte(`<html><body><p>Hot lunch starts Monday.</p><img src="https://example.org/menu.png"></body></html>`)
	if err := bucket.Put(t.Context(), "content/p1", "text/html; charset=utf-8", page); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "p1", "blob": "content/p1", "mime": "text/html; charset=utf-8", "size": "90"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Hot lunch", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000003", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "linked", "parent": "doc00000000011", "url": "https://example.org/lunch", "content": "cnt00000000003"}),
	); err != nil {
		t.Fatal(err)
	}
	NewExtractor(s, queue, bucket, "test")
	queue.Refresh()
	made(t, s, "DOCUMENT", "doc00000000012", "extracted")
	under := children(s, "doc00000000012")
	if len(under) != 1 || under[0]["relation"] != "extract" {
		t.Fatalf("the fetched page's children: %v", under)
	}
}

func TestExtractingAnEmailChoosesItsLinks(t *testing.T) {
	intercept.Install(intercept.ClaudeHost, claudeReplying(func(request string) string {
		if !strings.Contains(request, "Handbook week") {
			return `{"links": []}`
		}
		return `{"links": [{"n": 1, "url": "https://docs.example.org/handbook", "decision": "fetch"}, {"n": 2, "url": "https://forms.example.org/signup", "decision": "form"}]}`
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	root := uploadMail(t, s, pics, "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nDate: Thu, 12 Feb 2026 01:48:03 +0000\r\nSubject: Handbook week\r\nList-Id: <community.heliosschool.org>\r\nContent-Type: text/html; charset=utf-8\r\n\r\n"+
		`<p>Read the <a href="https://docs.example.org/handbook">handbook</a>, then <a href="https://forms.example.org/signup">sign up</a>.</p><img src="https://example.org/banner.png">`+"\r\n")
	made(t, s, "DOCUMENT", root, "extracted")
	body := children(s, root)[0]["id"]
	made(t, s, "DOCUMENT", body, "extracted")
	got := []string{}
	for _, child := range children(s, body) {
		got = append(got, child["relation"]+"|"+child["url"]+"|"+child["name"]+"|"+child["fetch"]+"|"+child["link"])
	}
	equalLines(t, "the email's children", got, []string{
		"extract||||",
		"image|https://example.org/banner.png|||",
		"linked|https://docs.example.org/handbook|handbook||",
		"linked|https://forms.example.org/signup|sign up|skipped|form",
	})
}

func TestAnEmailsLinksAreCollected(t *testing.T) {
	raw := []byte(`<html><body>
<p>Our <a href="https://docs.google.com/document/d/abc/edit">supply list</a> is ready, and so is the <a href="https://docs.google.com/document/d/abc/edit">same list again</a>.</p>
<ul><li>Sign up <a href="https://forms.gle/x1Yu5c8i">here</a> by Friday.</li></ul>
<p><a href="mailto:office@example.org">Write to us</a> or <a href="#top">go up</a>.</p>
<p><a href="https://mailer.example.com/u?id=1">Unsubscribe</a> from these emails.</p>
<p><a href="https://www.google.com/url?q=https://example.org/menu&amp;sa=D">menu</a></p>
</body></html>`)
	got, err := emailLinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []LinkChoice{
		{URL: "https://docs.google.com/document/d/abc/edit", Text: "supply list", Context: "Our supply list is ready, and so is the same list again ."},
		{URL: "https://forms.gle/x1Yu5c8i", Text: "here", Context: "Sign up here by Friday."},
		{URL: "https://example.org/menu", Text: "menu", Context: "menu"},
	}
	if len(got) != len(want) {
		t.Fatalf("links: got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d: got %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

func TestGoogleLinksFetchTheirExport(t *testing.T) {
	for address, want := range map[string]string{
		"https://docs.google.com/document/d/abc/edit?usp=sharing": "https://docs.google.com/document/d/abc/export?format=pdf",
		"https://docs.google.com/presentation/u/0/d/xyz/view":     "https://docs.google.com/presentation/d/xyz/export?format=pdf",
		"https://docs.google.com/spreadsheets/d/s1/edit#gid=0":    "https://docs.google.com/spreadsheets/d/s1/export?format=pdf",
		"https://drive.google.com/file/d/f1/view?usp=sharing":     "https://drive.google.com/uc?id=f1&export=download",
		"https://docs.google.com/forms/d/e/form/viewform":         "https://docs.google.com/forms/d/e/form/viewform",
		"https://example.org/document/d/abc/edit":                 "https://example.org/document/d/abc/edit",
		"https://drive.google.com/uc?id=f2&export=download":       "https://drive.google.com/uc?id=f2&export=download",
	} {
		if got := ExportURL(address); got != want {
			t.Errorf("%s: got %s, want %s", address, got, want)
		}
	}
}

func TestWhatALinkedDocumentKeeps(t *testing.T) {
	png, pixel, pdf, page := pngOf(t, 3), pngOf(t, 1), []byte("%PDF-1.4\n% a pdf\n"), []byte("<html><body>a page</body></html>")
	for _, c := range []struct {
		name     string
		relation string
		body     []byte
		ok       bool
	}{
		{"an image for an image", "image", png, true},
		{"a pixel for an image", "image", pixel, false},
		{"a page for an image", "image", page, false},
		{"a pdf for an image", "image", pdf, false},
		{"an image for a link", "linked", png, true},
		{"a pixel for a link", "linked", pixel, false},
		{"a pdf for a link", "linked", pdf, true},
		{"a page for a link", "linked", page, true},
		{"anything else for a link", "linked", []byte{0, 1, 2, 3}, false},
	} {
		if _, err := keptBody(c.relation, c.body); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
