package db

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/testkit"
)

func TestEveryEventIsShared(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{{Set: "grp00000000040", Cells: map[string]any{"visible_to": "", "status": "pending"}}})
	pages := NewWhenPages(s, blob.NewMemoryBucket(), WhenCardStyle(func() string { return "Helios When" }, func() string { return "The school year, day by day" }))
	mux := http.NewServeMux()
	pages.Register(mux)
	t.Chdir("../..")
	picnic := []string{`og:title" content="Fall Picnic"`, `og:url" content="https://when.heliosian.com/g/fall-picnic"`, `og:image" content="https://when.heliosian.com/open/share/grp00000000040.png"`}
	testkit.Previews(t, pages.PreviewHead,
		testkit.Preview{URL: "https://when.heliosian.com/g/fall-picnic", Want: picnic},
		testkit.Preview{URL: "https://when.heliosian.com/g/grp00000000040", Want: picnic},
		testkit.Preview{URL: "https://when.heliosian.com/mine", Want: []string{`og:title" content="Helios When"`, "One calendar", `og:image" content="https://when.heliosian.com/open/share/upcoming.png"`}},
	)
	testkit.Cards(t, mux, []string{"/open/share/grp00000000040.png", "/open/share/fall-picnic.png", "/open/share/grp00000000521.png", "/open/share/grp00000000512.png", "/open/share/upcoming.png"}, "/open/share/grp00000000010.png", "/open/share/nope.png")
	for token, previewed := range map[string]bool{"tok00000000001": true, "nope": false} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://when.heliosian.com/ext/"+token, nil))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `og:title" content="Fall Picnic"`) != previewed {
			t.Errorf("/ext/%s: %d, previewed %v: %s", token, rec.Code, previewed, rec.Body)
		}
	}
}
