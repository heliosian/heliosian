package sharecard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
