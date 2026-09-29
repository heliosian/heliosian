package model

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBirthdaysAbout(t *testing.T) {
	t.Chdir("../..")
	about := BirthdaysAbout(func() string { return "Renamed" }, func() string { return "A new line" })
	tags := about.PreviewHead(httptest.NewRequest("GET", "https://birthday.heliosian.com/some/page", nil))
	for _, want := range []string{`og:site_name" content="Renamed"`, `og:title" content="Renamed"`, `content="A new line. Every Helios staff member`, `og:image" content="https://birthday.heliosian.com/open/share/about.png"`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("preview lacks %s:\n%s", want, tags)
		}
	}
	rec := httptest.NewRecorder()
	about.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/about.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("card: %d", rec.Code)
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
		t.Fatalf("card: %v %v", err, img)
	}
}
