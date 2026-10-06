package db

import (
	"context"
	"net/http"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/testkit"
)

func TestWikiShare(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	body := "Intro words.\n\n# Getting **there**\n\nBy bus.\n\n```\n# not a heading\n```\n\n## [Pickup](https://example.org) times\n"
	id, _, err := saveWiki(context.Background(), s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, wikiPage{Name: "Field Trips", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	share := NewWikiShare(s, pics, func() string { return "Helios Wiki" }, func() string { return "Parent-to-parent info" })
	testkit.Previews(t, share.PreviewHead,
		testkit.Preview{
			URL:   "https://wiki.heliosian.com/p/" + id,
			Want:  []string{`og:title" content="Field Trips"`, `og:description" content="Getting there · Pickup times"`, `og:url" content="https://wiki.heliosian.com/p/` + id + `"`, `og:image" content="https://wiki.heliosian.com/open/share/` + id + `.png"`},
			Never: []string{"Intro words", "By bus", "not a heading"},
		},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/" + id + "/edit", Want: []string{`og:title" content="Field Trips"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/", Never: []string{"og:"}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/doc00000000001", Never: []string{"og:"}},
	)
	mux := http.NewServeMux()
	share.Register(mux)
	testkit.Cards(t, mux, []string{"/open/share/" + id + ".png"}, "/open/share/doc00000000001.png", "/open/share/nope.png")
}
