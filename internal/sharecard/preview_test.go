package sharecard

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func isolateCardCache() func() {
	prev := cardCache
	cardCache = newCardCache()
	return func() { cardCache = prev }
}

func TestADrawnCardIsServedAgainWithoutItsPicture(t *testing.T) {
	t.Cleanup(isolateCardCache())
	t.Chdir("../..")
	style := &Style{Palette: Standard, Name: func() string { return "Helios When" }, Tagline: func() string { return "The school year" }, Wordmark: "Helios When"}
	picture, err := os.ReadFile("web/wiki/backgrounds/nature.jpg")
	if err != nil {
		t.Fatal(err)
	}
	card := Card{Title: "Fall Picnic", Picture: picture, Whole: true, Lines: []Line{{Icon: "pin", Text: "The field"}}}
	parts := []string{"Fall Picnic", "The field", "nature"}
	req := httptest.NewRequest("GET", "/open/share/picnic.png", nil)
	if style.Answered(httptest.NewRecorder(), req, parts...) {
		t.Fatal("answered a card that has not been drawn")
	}
	first := httptest.NewRecorder()
	style.Serve(first, req, card, parts...)
	if first.Code != http.StatusOK || first.Body.Len() == 0 {
		t.Fatalf("first draw: %d, %d bytes", first.Code, first.Body.Len())
	}
	card.Picture = nil
	second := httptest.NewRecorder()
	if !style.Answered(second, req, parts...) {
		t.Fatal("a repeat was not answered from the drawn card")
	}
	if second.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatalf("repeat: %d, same bytes %v (%d vs %d)", second.Code, bytes.Equal(first.Body.Bytes(), second.Body.Bytes()), first.Body.Len(), second.Body.Len())
	}
	if style.Answered(httptest.NewRecorder(), req, "something else") {
		t.Fatal("a different card was answered from this one")
	}
	again := httptest.NewRequest("GET", "/open/share/picnic.png", nil)
	again.Header.Set("If-None-Match", first.Header().Get("ETag"))
	notModified := httptest.NewRecorder()
	if !style.Answered(notModified, again, parts...) || notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("revalidation: %d, %d bytes", notModified.Code, notModified.Body.Len())
	}
}

func TestACardKeepsTheBrandingItWasDrawnWith(t *testing.T) {
	t.Cleanup(isolateCardCache())
	t.Chdir("../..")
	name, tagline := "Helios When", "Before"
	style := &Style{
		Palette: Standard, Wordmark: "Helios When",
		Name: func() string { return name },
		Tagline: func() string {
			line := tagline
			name, tagline = "Renamed", "After"
			return line
		},
	}
	card := Card{Title: "Fall Picnic"}
	parts := []string{"Fall Picnic"}
	req := httptest.NewRequest("GET", "/open/share/picnic.png", nil)
	got := httptest.NewRecorder()
	style.Serve(got, req, card, parts...)
	if got.Code != http.StatusOK || got.Header().Get("ETag") != ETag("Fall Picnic", "Helios When", "Before") {
		t.Fatalf("drawn card: %d %s", got.Code, got.Header().Get("ETag"))
	}
	steady := func(appName, line string) *Style {
		return &Style{Palette: Standard, Wordmark: "Helios When", Name: func() string { return appName }, Tagline: func() string { return line }}
	}
	png, err := steady("Helios When", "Before").Draw(card)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Body.Bytes(), png) {
		t.Fatal("the card was drawn with branding read after it changed")
	}
	if steady("Renamed", "After").Answered(httptest.NewRecorder(), req, parts...) {
		t.Fatal("the card was stored under the branding it changed to")
	}
	kept := httptest.NewRecorder()
	if !steady("Helios When", "Before").Answered(kept, req, parts...) || !bytes.Equal(kept.Body.Bytes(), png) {
		t.Fatal("the card was not kept under the branding it was drawn with")
	}
}

func TestServe(t *testing.T) {
	t.Chdir("../..")
	name := "Helios Test"
	about := &About{
		Style:   &Style{Palette: Standard, Name: func() string { return name }, Tagline: func() string { return "A line" }, Wordmark: "Helios Test"},
		Desc:    "What it is.",
		Listing: Listing{Heading: "Inside", Items: []Item{{Title: "One", Note: "the first"}}},
		Button:  "Open",
	}
	fetch := func(etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/open/share/about.png", nil)
		if etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		about.ServeHTTP(rec, r)
		return rec
	}
	rec := fetch("")
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "public, max-age=3600" || etag == "" || rec.Body.Len() == 0 {
		t.Fatalf("card: %d %v", rec.Code, rec.Header())
	}
	if rec := fetch(etag); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("again with its etag: %d, %d bytes", rec.Code, rec.Body.Len())
	}
	name = "Renamed"
	if rec := fetch(etag); rec.Code != http.StatusOK || rec.Header().Get("ETag") == etag {
		t.Fatalf("after a rename: %d %s", rec.Code, rec.Header().Get("ETag"))
	}
	tags := about.PreviewHead(httptest.NewRequest("GET", "https://test.heliosian.com/any/page", nil))
	for _, want := range []string{`og:site_name" content="Renamed"`, `og:title" content="Renamed"`, `og:description" content="A line. What it is."`, `og:url" content="https://test.heliosian.com/"`, `og:image" content="https://test.heliosian.com/open/share/about.png"`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("preview lacks %s:\n%s", want, tags)
		}
	}
}
