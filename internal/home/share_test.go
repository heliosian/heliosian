package home

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A link to the portal previews the apps everyone has - never one narrowed
// to a list or one with no row yet - each at its address on the link's own
// tier, and the card draws them.
func TestPortalPreview(t *testing.T) {
	c := sampleCache(t)
	t.Chdir("../..")
	head := PreviewHead(c)(httptest.NewRequest("GET", "https://home.lab.heliosian.com/", nil))
	for _, want := range []string{`og:title" content="Tools and resources for the Helios Community"`, `og:url" content="https://home.lab.heliosian.com/"`,
		`og:image" content="https://home.lab.heliosian.com/open/share/apps.png"`, "Helios Who? (A visual directory, who.lab.heliosian.com)", "Helios Calendar (The school year, day by day, when.lab.heliosian.com)"} {
		if !strings.Contains(head, want) {
			t.Errorf("preview head lacks %s:\n%s", want, head)
		}
	}
	for _, never := range []string{"Celebrate", "Birthday"} {
		if strings.Contains(head, never) {
			t.Errorf("preview head names %s, which is not everyone's:\n%s", never, head)
		}
	}
	if got := tierOf("heliosian.com"); got != "heliosian.com" {
		t.Errorf("tier of heliosian.com = %q", got)
	}
	if got := tierOf("home.local.heliosian.com:8080"); got != "local.heliosian.com:8080" {
		t.Errorf("tier of the local host = %q", got)
	}

	a := app{cache: c}
	rec := httptest.NewRecorder()
	a.shareApps(rec, httptest.NewRequest("GET", "https://heliosian.com/open/share/apps.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("card: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1200 || b.Dy() != 630 {
		t.Errorf("card is %v, want 1200x630", b)
	}
	etag := rec.Header().Get("ETag")
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://heliosian.com/open/share/apps.png", nil)
	req.Header.Set("If-None-Match", etag)
	a.shareApps(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("same card again = %d, want 304", rec.Code)
	}
}
