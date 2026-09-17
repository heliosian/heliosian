package birthday

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A link to any address previews as what the app is - the same tags and the
// same card wherever it leads - and the card is drawn without a session.
func TestSharePreview(t *testing.T) {
	t.Chdir("../..")
	head := PreviewHead()
	for _, path := range []string{"/", "/some/page"} {
		tags := head(httptest.NewRequest("GET", "https://birthday.heliosian.com"+path, nil))
		for _, want := range []string{`og:site_name" content="Helios Birthday"`, `og:title" content="Help celebrate our staff`, `og:url" content="https://birthday.heliosian.com/"`, `og:image" content="https://birthday.heliosian.com/open/share/about.png"`} {
			if !strings.Contains(tags, want) {
				t.Fatalf("%s preview lacks %s:\n%s", path, want, tags)
			}
		}
	}
	rec := httptest.NewRecorder()
	app{}.shareCard(rec, httptest.NewRequest("GET", "/open/share/about.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("card: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
		t.Fatalf("card: %v %v", err, img)
	}
	// Fetched again with the tag it was given, the card is not drawn twice.
	again := httptest.NewRequest("GET", "/open/share/about.png", nil)
	again.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec = httptest.NewRecorder()
	app{}.shareCard(rec, again)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("second fetch: %d", rec.Code)
	}
}
