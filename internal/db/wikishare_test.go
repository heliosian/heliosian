package db

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/testkit"
)

func TestWikiShare(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	save := func(page wikiPage) string {
		t.Helper()
		id, _, err := saveWiki(context.Background(), s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, page)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	body := "# Field trips\n\n![a bus](/api/wiki/picture/x.png)\n- **Every grade** goes [somewhere](https://example.org) twice a year. Forms come home a week before.\n\n# Costs\n\nNone.\n"
	id := save(wikiPage{Name: "Field Trips", Body: body})
	save(wikiPage{Parent: id, Name: "Museums", Body: "Downtown."})
	save(wikiPage{Parent: id, Name: "Farms", Body: "Out of town."})
	empty := save(wikiPage{Name: "Empty", Body: ""})
	t.Chdir("../..")
	share := NewWikiShare(s, pics, func() string { return "Helios Wiki" })
	testkit.Previews(t, share.PreviewHead,
		testkit.Preview{
			URL:   "https://wiki.heliosian.com/p/" + id,
			Want:  []string{`og:title" content="Field Trips"`, `og:description" content="Every grade goes somewhere twice a year. In this section: Museums · Farms"`, `og:url" content="https://wiki.heliosian.com/p/Field-Trips"`, `og:image" content="https://wiki.heliosian.com/open/share/` + id + `.png"`},
			Never: []string{"Forms come home", "None.", "a bus"},
		},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/" + id + "/edit", Want: []string{`og:title" content="Field Trips"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/field-trips/museums", Want: []string{`og:title" content="Museums"`, `og:url" content="https://wiki.heliosian.com/p/Field-Trips/Museums"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/Field-Trips/Museums/edit", Want: []string{`og:title" content="Museums"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/" + empty, Want: []string{`og:title" content="Empty"`, `og:description" content=""`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/getting-started", Want: []string{`og:title" content="Getting Started"`, `og:url" content="https://wiki.heliosian.com/p/getting-started"`, `og:image" content="https://wiki.heliosian.com/open/share/doc00000000102.png"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/doc00000000102", Want: []string{`og:url" content="https://wiki.heliosian.com/p/getting-started"`}},
		testkit.Preview{URL: "https://wiki.heliosian.com/", Never: []string{"og:"}},
		testkit.Preview{URL: "https://wiki.heliosian.com/p/doc00000000001", Never: []string{"og:"}},
	)
	mux := http.NewServeMux()
	share.Register(mux)
	testkit.Cards(t, mux, []string{"/open/share/" + id + ".png", "/open/share/" + empty + ".png"}, "/open/share/doc00000000001.png", "/open/share/nope.png")
}

func TestWikiShareHeader(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	ctx := context.Background()
	save := func(page wikiPage) string {
		t.Helper()
		id, _, err := saveWiki(ctx, s, queue, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, page)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	name := strings.Repeat("c", 64) + ".png"
	if err := pics.bucket.Put(ctx, "wiki-images/"+name, "image/png", pngOf(t, 4)); err != nil {
		t.Fatal(err)
	}
	top := save(wikiPage{Name: "Field Trips", Body: "---\nheader_image: /api/wiki/picture/" + name + "\n---\nTrips."})
	sub := save(wikiPage{Parent: top, Name: "Museums", Body: "Downtown."})
	plain := save(wikiPage{Name: "Plain", Body: "No picture."})
	t.Chdir("../..")
	share := NewWikiShare(s, pics, func() string { return "Helios Wiki" })
	for id, want := range map[string]string{top: "wiki-images/" + name, sub: "wiki-images/" + name, plain: wikiBackgrounds + wikiBackgroundNames[backgroundHash(plain)%uint32(len(wikiBackgroundNames))] + ".jpg"} {
		got, picture, err := share.header(ctx, id)
		if err != nil || got != want || len(picture) == 0 {
			t.Errorf("%s wears %q (%d bytes, %v), want %q", id, got, len(picture), err, want)
		}
	}
	if page, _, _ := share.page(ctx, sub); page.top != "Field Trips" {
		t.Errorf("the sub-page's top page reads %q", page.top)
	}
	if page, _, _ := share.page(ctx, top); page.top != "" {
		t.Errorf("a top page names a page above it: %q", page.top)
	}
}

func TestHeaderImage(t *testing.T) {
	name := strings.Repeat("a", 64) + ".jpg"
	for markdown, want := range map[string]string{
		"---\nheader_image: /api/wiki/picture/" + name + "\n---\nHi.": "wiki-images/" + name,
		"---\nheader_image: https://example.org/x.jpg\n---\nHi.":      "",
		"---\nheader_image: /api/wiki/picture/secret.png\n---\nHi.":   "",
		"header_image: /api/wiki/picture/" + name:                     "",
		"": "",
	} {
		if got := headerImage(markdown); got != want {
			t.Errorf("headerImage(%q) = %q, want %q", markdown, got, want)
		}
	}
}

func TestFirstSentence(t *testing.T) {
	for markdown, want := range map[string]string{
		"# Title\n\nOne. Two.":                                 "One.",
		"```\nNot this.\n```\nThis one!":                       "This one!",
		"> Quoted line with no end":                            "Quoted line with no end",
		"1. *First* step? Then more.":                          "First step?",
		"# Only headings\n## And more":                         "",
		"Version 2.5 is out. Upgrade.":                         "Version 2.5 is out.",
		"![pic](/api/wiki/picture/a.png)\nHi.":                 "Hi.",
		"---\nheader_image: /api/wiki/picture/a.png\n---\nHi.": "Hi.",
	} {
		if got := firstSentence(markdown); got != want {
			t.Errorf("firstSentence(%q) = %q, want %q", markdown, got, want)
		}
	}
}
