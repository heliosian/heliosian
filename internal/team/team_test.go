package team

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

const night = "grp00000000512"

var sampleRoot, _ = filepath.Abs("../../sampledata")

func sample(t *testing.T) (*db.Store, *Team, *http.ServeMux) {
	t.Helper()
	dir := &data.Dir{Root: sampleRoot}
	s, err := db.NewStore(dir, dir, store.NewQueue(), db.NewSearchIndex())
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	mux := http.NewServeMux()
	site := Register(mux, Deps{
		Data: s, Bucket: bucket, Images: blob.NewImages(blob.New(bucket), "team"),
		Style: CardStyle(func() string { return "HCA-Team" }, func() string { return "HCA Volunteer Portal" }),
	})
	return s, site, mux
}

func commit(t *testing.T, s *db.Store, sheet string, ops ...store.Op) {
	t.Helper()
	if err := s.Commit(context.Background(), access.System("test"), sheet, ops...); err != nil {
		t.Fatal(err)
	}
}

func TestPreviews(t *testing.T) {
	t.Chdir("../..")
	_, site, mux := sample(t)
	page := []string{`og:title" content="International Night"`, `og:url" content="https://team.heliosian.com/v/international-night"`,
		`og:image" content="https://team.heliosian.com/open/share/` + night + `.png"`, `Friday, February 5 · 5:30 – 9:00 PM — An evening of food`}
	testkit.Previews(t, site.PreviewHead,
		testkit.Preview{URL: "https://team.heliosian.com/v/international-night", Want: page},
		testkit.Preview{URL: "https://team.heliosian.com/activities/" + night, Want: page},
		testkit.Preview{URL: "https://team.heliosian.com/my", Want: []string{`og:title" content="HCA-Team"`, `og:image" content="https://team.heliosian.com/open/share/upcoming.png"`}},
	)
	testkit.Cards(t, mux, []string{"/open/share/" + night + ".png", "/open/share/upcoming.png"}, "/open/share/grp00000000510.png", "/open/share/grp00000000040.png")
	for _, cells := range []store.Row{{"visible_to": ""}, {"status": "pending"}} {
		s, site, mux := sample(t)
		commit(t, s, db.GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000511"}, cells))
		testkit.Previews(t, site.PreviewHead, testkit.Preview{URL: "https://team.heliosian.com/v/international-night", Want: []string{`og:title" content="HCA-Team"`}, Never: []string{night}})
		testkit.Cards(t, mux, nil, "/open/share/"+night+".png")
	}
}

func TestNeeds(t *testing.T) {
	s, site, _ := sample(t)
	at := time.Date(2026, time.October, 1, 9, 0, 0, 0, db.School)
	tr := site.tree()
	list := tr.needs(at)
	if len(list) != 1 || list[0]["id"] != night {
		t.Fatalf("needs: %v", list)
	}
	if got := tr.needs(time.Date(2027, time.March, 1, 9, 0, 0, 0, db.School)); len(got) != 0 {
		t.Fatalf("a past event is a need: %v", got)
	}
	commit(t, s, db.GroupsSheet, store.Update("GROUP", store.Row{"id": night}, store.Row{"capacity": "3"}))
	tr = site.tree()
	if note := tr.needNote(tr.group(night)); note != "Friday, February 5 · 2 spots left" {
		t.Fatalf("note: %q", note)
	}
	commit(t, s, db.GroupsSheet, store.Update("GROUP", store.Row{"id": night}, store.Row{"capacity": "1"}))
	if got := site.tree().needs(at); len(got) != 0 {
		t.Fatalf("a full event is a need: %v", got)
	}
}

func TestRedirects(t *testing.T) {
	s, site, _ := sample(t)
	commit(t, s, db.ConfigSheet,
		store.Insert("ALIAS", store.Row{"id": "als00000000901", "alias": "E001", "target": night}),
		store.Insert("REDIRECT", store.Row{"id": "rdr00000000901", "app": "team", "old": "/old-booth", "new": "/v/international-night"}),
		store.Insert("REDIRECT", store.Row{"id": "rdr00000000902", "app": "team", "old": "https://hca.heliosian.com/fair", "new": "/"}),
		store.Insert("REDIRECT", store.Row{"id": "rdr00000000903", "app": "team", "old": "/elsewhere", "new": "https://celebrate.heliosian.com/p/taco-night"}),
	)
	handler := site.Redirected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for _, c := range []struct {
		path   string
		status int
		to     string
	}{
		{"/activities/E001", http.StatusMovedPermanently, "https://example.com/v/international-night"},
		{"/activities/E001?x=1", http.StatusMovedPermanently, "https://example.com/v/international-night?x=1"},
		{"/v/" + night, http.StatusMovedPermanently, "https://example.com/v/international-night"},
		{"/v/inight/" + night, http.StatusMovedPermanently, "https://example.com/v/international-night"},
		{"/old-booth", http.StatusFound, "https://example.com/v/international-night"},
		{"/fair", http.StatusFound, "https://example.com/"},
		{"/fair/calendar", http.StatusFound, "https://example.com/calendar"},
		{"/elsewhere/x", http.StatusFound, "https://celebrate.heliosian.com/p/taco-night/x"},
		{"/fair//elsewhere.example", http.StatusTeapot, ""},
		{"/fair/\\elsewhere.example", http.StatusTeapot, ""},
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

func TestWhenLines(t *testing.T) {
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

func TestBlurb(t *testing.T) {
	got := blurb("A **bold** [fair](https://example.com) ![x](y.png)\n\n**Links**\n\n- [Menu](https://example.com/menu): what to bring")
	if got != "A bold fair Links Menu: what to bring" {
		t.Errorf("blurb: %q", got)
	}
}
