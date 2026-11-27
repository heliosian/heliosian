package db

import (
	"bytes"
	"image"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/intercept"
	"heliosian/internal/sharecard"
	"heliosian/internal/testkit"
)

func googleFonts() *atomic.Int32 {
	fetched := &atomic.Int32{}
	css, files := testkit.GoogleFonts(fetched)
	intercept.Install(testkit.GoogleFontsHost, css)
	intercept.Install(testkit.GoogleFontsFilesHost, files)
	return fetched
}

func whenStyle() *sharecard.Style {
	return WhenCardStyle(func() string { return "Helios When" }, func() string { return "Helios calendar of everything" })
}

func shareImage(t *testing.T, mux *http.ServeMux, id string) image.Config {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/open/share/"+id+".png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("the share image answered %d %s", rec.Code, rec.Body)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestAnInvitationWithACustomURLSharesItsOwnLook(t *testing.T) {
	fetched := googleFonts()
	s, other := sample(t), sample(t)
	t.Chdir("../..")
	pages := NewWhenPages(s, newPictures(s), whenStyle())
	mux := http.NewServeMux()
	pages.RegisterGather(mux)
	if cfg := shareImage(t, mux, theoParty); cfg.Width != sharecard.InvitationWidth || cfg.Height != sharecard.InvitationHeight {
		t.Errorf("Theo's share image is %dx%d", cfg.Width, cfg.Height)
	}
	if fetched.Load() == 0 {
		t.Error("Theo's share image was drawn without fetching LARP Quest's fonts")
	}
	if exists, err := pages.bucket.Exists(t.Context(), "fonts/cinzel-decorative-700.ttf"); err != nil || !exists {
		t.Errorf("the title font wasn't kept in the bucket: %v %v", exists, err)
	}

	before := fetched.Load()
	if _, err := newThemeFonts(pages.bucket).font(t.Context(), fontFace{family: "Cinzel Decorative", weight: 700}); err != nil {
		t.Fatal(err)
	}
	if fetched.Load() != before {
		t.Error("a font already in the bucket was fetched again")
	}

	if _, err := Write(t.Context(), other, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Set: theoParty, Cells: map[string]any{"slug": ""}}}}); err != nil {
		t.Fatal(err)
	}
	before = fetched.Load()
	mux = http.NewServeMux()
	NewWhenPages(other, newPictures(other), whenStyle()).RegisterGather(mux)
	shareImage(t, mux, theoParty)
	if fetched.Load() != before {
		t.Error("an invitation without a Custom URL was drawn in its theme")
	}
}

func TestATextRoleTakesTheNearestFaceItsThemeLoads(t *testing.T) {
	design := map[string]any{
		"fonts": map[string]any{"display": "'Cinzel Decorative', serif", "body": "'EB Garamond', serif", "load": []any{"Cinzel Decorative:wght@700", "EB Garamond:ital,wght@0,400;0,600;1,400"}},
		"text": map[string]any{
			"title":  map[string]any{"font": "display", "weight": "400", "style": "normal"},
			"kicker": map[string]any{"font": "body", "weight": "400", "style": "italic"},
			"day":    map[string]any{"font": "body", "weight": "600", "style": "normal"},
			"hours":  map[string]any{"font": "body", "weight": "500", "style": "normal"},
		},
	}
	for role, want := range map[string]fontFace{
		"title":  {family: "Cinzel Decorative", weight: 700},
		"kicker": {family: "EB Garamond", weight: 400, italic: true},
		"day":    {family: "EB Garamond", weight: 600},
		"hours":  {family: "EB Garamond", weight: 400},
	} {
		if got, ok := themeFace(design, role); !ok || got != want {
			t.Errorf("%s took %+v, want %+v", role, got, want)
		}
	}
	design["fonts"] = map[string]any{"display": "Georgia, serif", "load": []any{}}
	if got, ok := themeFace(design, "title"); ok {
		t.Errorf("a stack the theme loads nothing for took %+v", got)
	}
}
