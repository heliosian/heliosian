package db

import (
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const activityNight = "grp00000000512"

func activityPagesSample(t *testing.T) (*Store, *store.Queue, *ActivityPages, *http.ServeMux) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	style := &sharecard.Style{
		Palette: sharecard.Palette{
			Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
			Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
		},
		Name: func() string { return "HCA-Team" }, Tagline: func() string { return "HCA Volunteer Portal" },
		Mark: "web/public/team/brand/logo-mark.png", Lockup: "web/public/team/brand/logo-lockup.png", Corner: "web/team/toolbar_background.png",
	}
	pages := NewActivityPages(s, blob.NewMemoryBucket(), style)
	mux := http.NewServeMux()
	pages.Register(mux)
	return s, queue, pages, mux
}

func TestActivityPreviews(t *testing.T) {
	_, _, pages, mux := activityPagesSample(t)
	type unlisted struct {
		pages *ActivityPages
		mux   *http.ServeMux
	}
	unlistedPages := []unlisted{}
	for _, group := range []string{"grp00000000511", activityNight} {
		for _, cells := range []map[string]any{{"visible_to": ""}, {"status": "pending"}, {"status": "closed"}} {
			s, queue, p, m := activityPagesSample(t)
			apply(t, s, queue, []Edit{{Set: group, Cells: cells}})
			unlistedPages = append(unlistedPages, unlisted{p, m})
		}
	}
	t.Chdir("../..")
	page := []string{`og:title" content="International Night"`, `og:url" content="https://team.heliosian.com/g/international-night"`,
		`og:image" content="https://team.heliosian.com/open/share/` + activityNight + `.png"`, `Friday, February 5 · 5:30 – 9:00 PM — An evening of food`}
	testkit.Previews(t, pages.PreviewHead,
		testkit.Preview{URL: "https://team.heliosian.com/g/international-night", Want: page},
		testkit.Preview{URL: "https://team.heliosian.com/g/" + activityNight, Want: page},
		testkit.Preview{URL: "https://team.heliosian.com/my", Want: []string{`og:title" content="HCA-Team"`, `og:image" content="https://team.heliosian.com/open/share/upcoming.png"`}},
	)
	testkit.Cards(t, mux, []string{"/open/share/" + activityNight + ".png", "/open/share/upcoming.png"}, "/open/share/grp00000000510.png", "/open/share/grp00000000040.png")
	for _, p := range unlistedPages {
		testkit.Previews(t, p.pages.PreviewHead, testkit.Preview{URL: "https://team.heliosian.com/g/international-night", Want: page})
		testkit.Cards(t, p.mux, []string{"/open/share/" + activityNight + ".png"})
	}
}

func TestActivityNeeds(t *testing.T) {
	s, queue, pages, _ := activityPagesSample(t)
	at := time.Date(2026, time.October, 1, 9, 0, 0, 0, School)
	tr := pages.tree()
	list := tr.needs(at)
	if len(list) != 1 || list[0]["id"] != activityNight {
		t.Fatalf("needs: %v", list)
	}
	if got := tr.needs(time.Date(2027, time.March, 1, 9, 0, 0, 0, School)); len(got) != 0 {
		t.Fatalf("a past event is a need: %v", got)
	}
	apply(t, s, queue, []Edit{{Set: activityNight, Cells: map[string]any{"capacity": "3"}}})
	tr = pages.tree()
	if note := tr.needNote(tr.group(activityNight)); note != "Friday, February 5 · 2 spots left" {
		t.Fatalf("note: %q", note)
	}
	apply(t, s, queue, []Edit{{Set: activityNight, Cells: map[string]any{"capacity": "1"}}})
	if got := pages.tree().needs(at); len(got) != 0 {
		t.Fatalf("a full event is a need: %v", got)
	}
}

func TestActivityRedirects(t *testing.T) {
	s, queue, pages, _ := activityPagesSample(t)
	apply(t, s, queue, []Edit{
		{Insert: "ALIAS", Row: map[string]any{"alias": "E001", "target": activityNight}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "team", "old": "/old-booth", "new": "/g/international-night"}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "team", "old": "/activities/nsSPomxFcPrSfoRCAgzI", "new": "/v/international-night"}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "team", "old": "https://hca.heliosian.com/fair", "new": "/"}},
		{Insert: "REDIRECT", Row: map[string]any{"app": "team", "old": "/elsewhere", "new": "https://celebrate.heliosian.com/g/taco-night"}},
	})
	handler := pages.Redirected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for _, c := range []struct {
		path   string
		status int
		to     string
	}{
		{"/g/E001", http.StatusMovedPermanently, "https://example.com/g/international-night"},
		{"/g/E001?x=1", http.StatusMovedPermanently, "https://example.com/g/international-night?x=1"},
		{"/g/" + activityNight, http.StatusMovedPermanently, "https://example.com/g/international-night"},
		{"/g/inight/" + activityNight, http.StatusMovedPermanently, "https://example.com/g/international-night"},
		{"/old-booth", http.StatusFound, "https://example.com/g/international-night"},
		{"/activities/nsSPomxFcPrSfoRCAgzI", http.StatusFound, "https://example.com/v/international-night"},
		{"/fair", http.StatusFound, "https://example.com/"},
		{"/fair/calendar", http.StatusFound, "https://example.com/calendar"},
		{"/elsewhere/x", http.StatusFound, "https://celebrate.heliosian.com/g/taco-night/x"},
		{"/fair//elsewhere.example", http.StatusTeapot, ""},
		{"/fair/\\elsewhere.example", http.StatusTeapot, ""},
		{"/g/international-night", http.StatusTeapot, ""},
		{"/v/international-night", http.StatusTeapot, ""},
		{"/calendar", http.StatusTeapot, ""},
		{"/nowhere", http.StatusTeapot, ""},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.status || rec.Header().Get("Location") != c.to {
			t.Errorf("%s: %d %q, want %d %q", c.path, rec.Code, rec.Header().Get("Location"), c.status, c.to)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/old-booth", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("a POST is redirected: %d", rec.Code)
	}
}

func TestActivityWhenLines(t *testing.T) {
	for _, tc := range []struct {
		start, end, line string
	}{
		{"2026-09-24", "", "Thursday, September 24"},
		{"2026-09-24 16:00", "2026-09-24 18:00", "Thursday, September 24 · 4:00 – 6:00 PM"},
		{"2026-09-24 11:00", "2026-09-24 13:00", "Thursday, September 24 · 11:00 AM – 1:00 PM"},
		{"2026-10-02", "2026-10-04", "Friday, October 2 – Sunday, October 4"},
		{"2026-10-02 16:00", "2026-10-04 12:00", "Friday, October 2 – Sunday, October 4 · Fri 4:00 PM – Sun 12:00 PM"},
		{"2026-10-30 09:00", "2026-11-01", "Friday, October 30 – Sunday, November 1 · Fri 9:00 AM"},
	} {
		if got := whenText(store.Row{"start": tc.start, "end": tc.end}); got != tc.line {
			t.Errorf("whenText(%q, %q) = %q, want %q", tc.start, tc.end, got, tc.line)
		}
	}
	if got := whenText(store.Row{"timing": "Spring", "start": "2026-09-24"}); got != "Spring" {
		t.Errorf("timing words: %q", got)
	}
}

func TestActivityBlurb(t *testing.T) {
	got := blurb("A **bold** [fair](https://example.com) ![x](y.png)\n\n**Links**\n\n- [Menu](https://example.com/menu): what to bring")
	if got != "A bold fair Links Menu: what to bring" {
		t.Errorf("blurb: %q", got)
	}
}
