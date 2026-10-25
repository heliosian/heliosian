package db

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/testkit"
)

func TestWikiShare(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	save := func(page wikiPage) string {
		t.Helper()
		id, _, err := saveWiki(context.Background(), s, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, page)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	body := "# Field trips\n\n![a bus](/api/blob/pho00000099999/image)\n- **Every grade** goes [somewhere](https://example.org) twice a year. Forms come home a week before.\n\n# Costs\n\nNone.\n"
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
	s := sample(t)
	pics := newPictures(s)
	ctx := context.Background()
	save := func(page wikiPage) string {
		t.Helper()
		id, _, err := saveWiki(ctx, s, pics, access.Actor{Email: "test"}, Env{Viewer: parent, Now: testNow}, page)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	top := save(wikiPage{Name: "Field Trips", Body: "Trips."})
	picture := pngOf(t, 4)
	hash := blob.Hash(picture)
	if _, err := pics.bucket.Put(ctx, "content/"+hash, "image/png", picture, 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "picture", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "image/png", "size": strconv.Itoa(len(picture))}},
		{Insert: "PHOTO", Row: map[string]any{"document": top, "original": "@picture", "reencode": "@picture", "ready": "Yes"}},
	})
	photo, shown := ids[1], ids[0]
	save(wikiPage{Document: top, Name: "Field Trips", Body: "---\nheader_image: /api/blob/" + photo + "/image\n---\nTrips."})
	sub := save(wikiPage{Parent: top, Name: "Museums", Body: "Downtown."})
	plain := save(wikiPage{Name: "Plain", Body: "No picture."})
	t.Chdir("../..")
	share := NewWikiShare(s, pics, func() string { return "Helios Wiki" })
	for id, want := range map[string]string{top: shown, sub: shown, plain: wikiBackgrounds + wikiBackgroundNames[backgroundHash(plain)%uint32(len(wikiBackgroundNames))] + ".jpg"} {
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
	for markdown, want := range map[string]string{
		"---\nheader_image: /api/blob/pho00000020001/image\n---\nHi.":    "pho00000020001",
		"---\nheader_image: https://example.org/x.jpg\n---\nHi.":         "",
		"---\nheader_image: /api/blob/pho00000020001/original\n---\nHi.": "",
		"---\nheader_image: /api/blob/pho00000020001/image?x\n---\nHi.":  "",
		"header_image: /api/blob/pho00000020001/image":                   "",
		"": "",
	} {
		if got := headerImage(markdown); got != want {
			t.Errorf("headerImage(%q) = %q, want %q", markdown, got, want)
		}
	}
}

func TestPageOutline(t *testing.T) {
	markdown := "> [!QUESTION] 30 minutes is not much time. How do you get through it all?\n> Some tips:\n\n### Key Topics\n\nYour goal.\n\n> [!QUESTION] What matters **most**?\n>\n> ### Who We Are\n>\n> - Culture\n\n```\n# not this\n```\n> [!NOTE]\n> Plain."
	if got := strings.Join(pageOutline(markdown), " | "); got != "30 minutes is not much time. How do you get through it all? | Key Topics | What matters most? | Who We Are" {
		t.Errorf("the outline reads %q", got)
	}
	if got := firstSentence(markdown); got != "30 minutes is not much time." {
		t.Errorf("the first sentence reads %q", got)
	}
}

func TestFirstSentence(t *testing.T) {
	for markdown, want := range map[string]string{
		"# Title\n\nOne. Two.":                                        "One.",
		"```\nNot this.\n```\nThis one!":                              "This one!",
		"> Quoted line with no end":                                   "Quoted line with no end",
		"1. *First* step? Then more.":                                 "First step?",
		"# Only headings\n## And more":                                "",
		"Version 2.5 is out. Upgrade.":                                "Version 2.5 is out.",
		"![pic](/api/blob/pho00000020001/image)\nHi.":                 "Hi.",
		"---\nheader_image: /api/blob/pho00000020001/image\n---\nHi.": "Hi.",
	} {
		if got := firstSentence(markdown); got != want {
			t.Errorf("firstSentence(%q) = %q, want %q", markdown, got, want)
		}
	}
}
