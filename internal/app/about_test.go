package app

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/sharecard"
)

func TestAboutCards(t *testing.T) {
	t.Chdir("../..")
	for _, c := range []struct {
		about *sharecard.About
		host  string
		desc  string
	}{
		{directoryAbout(func() string { return "Renamed" }, func() string { return "A new line" }), "who.heliosian.com", `content="A new line. Students, parents and staff`},
		{listAbout(func() string { return "Renamed" }, func() string { return "A new line" }), "loop.heliosian.com", `content="A new line. Each email list - a class`},
	} {
		tags := c.about.PreviewHead(httptest.NewRequest("GET", "https://"+c.host+"/some/page", nil))
		for _, want := range []string{`og:site_name" content="Renamed"`, `og:title" content="Renamed"`, c.desc, `og:image" content="https://` + c.host + `/open/share/about.png"`} {
			if !strings.Contains(tags, want) {
				t.Fatalf("%s's preview lacks %s:\n%s", c.host, want, tags)
			}
		}
		rec := httptest.NewRecorder()
		c.about.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/about.png", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s's card: %d", c.host, rec.Code)
		}
		img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
		if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 630 {
			t.Fatalf("%s's card: %v %v", c.host, err, img)
		}
	}
}
