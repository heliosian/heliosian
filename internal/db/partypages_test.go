package db

import (
	"net/http"
	"testing"

	"heliosian/internal/blob"
	"heliosian/internal/testkit"
)

func TestPartyPreviewsAndShareCards(t *testing.T) {
	s, _ := sampleWithQueue(t)
	pages := NewPartyPages(s, blob.NewMemoryBucket(), PartyCardStyle(func() string { return "Helios Celebrate" }, func() string { return "Fun(d)raiser Parties" }))
	mux := http.NewServeMux()
	pages.Register(mux)
	t.Chdir("../..")
	testkit.Previews(t, pages.PreviewHead,
		testkit.Preview{URL: "https://celebrate.heliosian.com/p/taco-night", Want: []string{
			`og:title" content="Taco Night"`, `og:url" content="https://celebrate.heliosian.com/p/taco-night"`,
			`og:image" content="https://celebrate.heliosian.com/open/share/` + tacoNight + `.png"`, `Saturday, March 6 · 6:00 – 9:00 PM — Montara — A taco bar`}},
		testkit.Preview{URL: "https://celebrate.heliosian.com/parties/" + tacoNight, Want: []string{`og:title" content="Taco Night"`}},
		testkit.Preview{URL: "https://celebrate.heliosian.com/my", Want: []string{
			`og:title" content="Upcoming Parties"`, `og:image" content="https://celebrate.heliosian.com/open/share/upcoming.png"`,
			`Next up: Taco Night — Saturday, March 6 · 6:00 – 9:00 PM — Montara`}},
	)
	testkit.Cards(t, mux, []string{"/open/share/" + tacoNight + ".png", "/open/share/upcoming.png"}, "/open/share/"+celebration+".png", "/open/share/grp00000000040.png")
}
