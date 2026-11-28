package db

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	plainTheme   = "thm00000000001"
	areaTheme    = "thm00000000002"
	skatingTheme = "thm00000000003"
	larpTheme    = "thm00000000004"
)

func TestTheSampleThemesAreTheirPackages(t *testing.T) {
	s := sample(t)
	dirs, err := filepath.Glob("../../sampledata/themes/*/theme.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != len(s.Model().Table("THEME").All()) {
		t.Errorf("%d packages, %d THEME rows", len(dirs), len(s.Model().Table("THEME").All()))
	}
	for _, file := range dirs {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		pkg := map[string]any{}
		if err := json.Unmarshal(raw, &pkg); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		key := pkg["key"].(string)
		row, ok := s.Model().Table("THEME").Find(key, "")
		if !ok {
			t.Errorf("no site theme %s for %s", key, file)
			continue
		}
		stored, err := storedDesign(row["design"])
		if err != nil {
			t.Fatal(err)
		}
		if row["name"] != pkg["name"] || row["blurb"] != pkg["blurb"] {
			t.Errorf("%s is named %q %q, its package %q %q", key, row["name"], row["blurb"], pkg["name"], pkg["blurb"])
		}
		delete(pkg, "key")
		delete(pkg, "name")
		delete(pkg, "blurb")
		if !reflect.DeepEqual(stored, pkg) {
			t.Errorf("%s's THEME row differs from %s", key, file)
		}
		if err := standardDesign(stored); err != nil {
			t.Errorf("%s: %v", key, err)
		}
		names, _ := pictureNames(stored)
		for _, name := range names {
			if _, ok := s.Model().Table("THEME_PICTURE").Find(row["id"], name); !ok {
				t.Errorf("%s names %s, which it has no THEME_PICTURE for", key, name)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(file), name)); err != nil {
				t.Errorf("%s's package is missing %s", key, name)
			}
		}
	}
}

func TestEverySampleThemeDrawsAnEnvelopeForMail(t *testing.T) {
	m := sample(t).Model()
	t.Chdir("../..")
	for _, row := range m.Table("THEME").All() {
		e, err := envelopeOf(m, look{theme: row["id"]}, "Theo Turns Eight")
		if err != nil {
			t.Errorf("%s: %v", row["key"], err)
			continue
		}
		picture, err := sharecard.DrawEnvelope(e)
		if err != nil {
			t.Errorf("%s: %v", row["key"], err)
			continue
		}
		if _, err := png.Decode(bytes.NewReader(picture)); err != nil {
			t.Errorf("%s drew no picture: %v", row["key"], err)
		}
	}
}

func TestAnEnvelopeOfAnotherDesignWrapsTheCard(t *testing.T) {
	m := sample(t).Model()
	draw := func(l look) sharecard.Envelope {
		t.Helper()
		e, err := envelopeOf(m, l, "Theo Turns Eight")
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	own, larp, area := draw(look{theme: skatingTheme}), draw(look{theme: larpTheme}), draw(look{theme: areaTheme})
	mixed := draw(look{theme: skatingTheme, envelope: larpTheme, liner: areaTheme, seal: areaTheme})
	if mixed.Body != larp.Body || mixed.Flap != larp.Flap {
		t.Errorf("skating in a LARP envelope has body %v flap %v, want the LARP's %v %v", mixed.Body, mixed.Flap, larp.Body, larp.Flap)
	}
	if mixed.Liner != area.Liner || mixed.Seal != area.Seal || mixed.Mark != area.Mark {
		t.Errorf("an Area 51 liner and seal are %v, %v %q, want Area 51's %v, %v %q", mixed.Liner, mixed.Seal, mixed.Mark, area.Liner, area.Seal, area.Mark)
	}
	if mixed.Paper != own.Paper || mixed.Kicker != own.Kicker || mixed.Backdrop != own.Backdrop {
		t.Errorf("skating in a LARP envelope has paper %v kicker %q backdrop %v, want skating's %v %q %v", mixed.Paper, mixed.Kicker, mixed.Backdrop, own.Paper, own.Kicker, own.Backdrop)
	}
}

func TestAnInvitationsOwnColorsDrawItsMailEnvelope(t *testing.T) {
	m := sample(t).Model()
	e, err := envelopeOf(m, look{theme: larpTheme, design: `{"card": {"background": "#4a1f5c"}, "page": {"backdrop": "#101010"}}`}, "Winter Solstice Supper")
	if err != nil {
		t.Fatal(err)
	}
	if want := (color.RGBA{0x4a, 0x1f, 0x5c, 0xff}); e.Paper != want {
		t.Errorf("paper is %v, want %v", e.Paper, want)
	}
	if want := (color.RGBA{0x10, 0x10, 0x10, 0xff}); e.Backdrop != want {
		t.Errorf("backdrop is %v, want %v", e.Backdrop, want)
	}
}

func TestADesignTakesOnlyColorsFontsAndWords(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct {
		design string
		ok     bool
	}{
		{``, true},
		{`{"card": {"background": "#4a1f5c", "ink": "#ffffff", "accent": "#e9c46a", "words": false}, "page": {"backdrop": "#101010"}}`, true},
		{`{"fonts": {"display": "Marcellus, Georgia, serif", "body": "Nunito, sans-serif"}}`, true},
		{`{"card": {"background": "url(https://example.com/x.png)"}}`, false},
		{`{"fonts": {"display": "Comic Sans MS"}}`, false},
		{`{"card": {"art": "none"}}`, false},
		{`{"card": "red"}`, false},
		{`not json`, false},
		{`{"text": {"kicker": {"words": "Craft a little fun with us"}}}`, true},
		{`{"text": {"kicker": {"words": "  "}}}`, false},
		{`{"text": {"kicker": {"words": "Hi", "color": "#ff0000"}}}`, false},
		{`{"text": {"title": {"words": "Hi"}}}`, false},
	} {
		_, err := checkedDesign(m, c.design)
		if (err == nil) != c.ok {
			t.Errorf("design %s: err %v, want ok %v", c.design, err, c.ok)
		}
	}
}

func TestAThemeKeepsTheStandard(t *testing.T) {
	for _, c := range []struct {
		design string
		ok     bool
	}{
		{`{}`, true},
		{`{"card": {"art": "url(\"card.jpg\") center / cover"}}`, true},
		{`{"card": {"art": "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E\")"}}`, true},
		{`{"card": {"art": "url(https://example.com/x.png)"}}`, false},
		{`{"card": {"glitter": "lots"}}`, false},
		{`{"card": {"ink": ""}}`, false},
		{`{"seal": {"shape": "heart"}}`, false},
		{`{"seal": {"picture": "url(\"seal.png\") center / cover no-repeat"}}`, true},
		{`{"seal": {"picture": "url(\"seal.png\") center / cover, #ffffff"}}`, false},
		{`{"seal": {"picture": "gold"}}`, false},
		{`{"card": {"align": "right"}}`, false},
		{`{"card": {"details": "side-by-side"}}`, true},
		{`{"card": {"details": "grid"}}`, false},
		{`{"text": {"day": {"format": "{wkd} · {mon} {day}"}}}`, true},
		{`{"text": {"day": {"format": "{weekday} {date}"}}}`, false},
		{`{"words": {"coming": "Who’s beaming down", "yes": "I’m in"}}`, true},
		{`{"words": {"none": ""}}`, false},
		{`{"envelope": {"body": "tomato"}}`, false},
		{`{"text": {"title": {"color": "tomato"}}}`, false},
	} {
		d, err := storedDesign(c.design)
		if err != nil {
			t.Fatal(err)
		}
		if err := standardDesign(d); (err == nil) != c.ok {
			t.Errorf("design %s: err %v, want ok %v", c.design, err, c.ok)
		}
	}
}

func themeFiles(theme map[string]any, pictures map[string][]byte) map[string][]byte {
	raw, _ := json.Marshal(theme)
	files := map[string][]byte{"theme.json": raw}
	for name, content := range pictures {
		files[name] = content
	}
	return files
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(content)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postTheme(t *testing.T, s *Store, pics *Pictures, as string, theme map[string]any, pictures map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	return dropTheme(t, s, pics, as, themeFiles(theme, pictures))
}

func dropTheme(t *testing.T, s *Store, pics *Pictures, as string, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for name, content := range files {
		part, _ := form.CreateFormFile("file", name)
		part.Write(content)
	}
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, pics, testTokens)
	r := httptest.NewRequest(http.MethodPost, "/api/do/theme", body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	serveAs(mux, rec, r, as)
	return rec
}

func TestPicturesWrittenIntoAThemeBecomeItsFiles(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	host := s.Model().addressOf(staff)
	painted := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngOf(t, 8))
	drawn := `data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 10 10'><circle cx='5' cy='5' r='4'/></svg>`
	theme := map[string]any{"key": "brunch", "name": "Brunch", "card": map[string]any{"art": `url("` + painted + `") center / cover`}, "page": map[string]any{"backdrop": `url('` + painted + `') center / cover, #f4e9d8`}, "seal": map[string]any{"picture": `url("` + drawn + `") center / 70% no-repeat`}}
	if rec := postTheme(t, s, pics, host, theme, nil); rec.Code != http.StatusOK {
		t.Fatalf("a theme with its pictures written in: %d %s", rec.Code, rec.Body)
	}
	row, ok := s.Model().Table("THEME").Find("brunch", staff)
	if !ok {
		t.Fatal("no THEME row for the upload")
	}
	if got := s.Model().Table("THEME_PICTURE").Referencing("theme", row["id"]); len(got) != 1 || got[0]["name"] != "inline-1.png" {
		t.Errorf("the theme's pictures are %v, want one inline-1.png for the art and the backdrop", got)
	}
	if strings.Contains(row["design"], "base64") || !strings.Contains(row["design"], `url(\"inline-1.png\") center / cover`) || !strings.Contains(row["design"], "image/svg+xml") {
		t.Errorf("the stored design is %s", row["design"])
	}
	theme["key"] = "huge"
	theme["seal"] = map[string]any{"picture": `url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg'><desc>` + strings.Repeat("x", store.CellLimit) + `</desc></svg>")`}
	if rec := postTheme(t, s, pics, host, theme, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "even with its pictures moved into files") {
		t.Errorf("a design too long for a cell: %d %s", rec.Code, rec.Body)
	}
}

func TestAHostUploadsAThemeOnlyTheirInvitationsUse(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	host, other := s.Model().addressOf(staff), s.Model().addressOf(parent)
	theme := map[string]any{"key": "disco", "name": "Disco", "blurb": "A mirror ball", "card": map[string]any{"art": `url("ball.png") center / 40% no-repeat`, "accent": "#d63384"}}
	if rec := postTheme(t, s, pics, host, theme, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("a theme without the picture it names: %d %s", rec.Code, rec.Body)
	}
	twice := map[string][]byte{"a.zip": zipOf(t, themeFiles(theme, nil)), "b/theme.json": themeFiles(theme, nil)["theme.json"]}
	if rec := dropTheme(t, s, pics, host, twice); rec.Code != http.StatusBadRequest {
		t.Errorf("two theme files dropped together: %d %s", rec.Code, rec.Body)
	}
	folder := map[string][]byte{"disco/": nil, "__MACOSX/disco/._theme.json": []byte("junk"), "disco/.DS_Store": []byte("junk"), "disco/notes.txt": []byte("ideas")}
	for name, content := range themeFiles(theme, map[string][]byte{"ball.png": pngOf(t, 8), "spare.png": pngOf(t, 8)}) {
		folder["disco/"+name] = content
	}
	if rec := dropTheme(t, s, pics, host, map[string][]byte{"disco.zip": zipOf(t, folder)}); rec.Code != http.StatusOK {
		t.Fatalf("the host drops a zip of the theme's folder: %d %s", rec.Code, rec.Body)
	}
	row, ok := s.Model().Table("THEME").Find("disco", staff)
	if !ok {
		t.Fatal("no THEME row for the host's upload")
	}
	if got := s.Model().Table("THEME_PICTURE").Referencing("theme", row["id"]); len(got) != 1 || got[0]["name"] != "ball.png" {
		t.Errorf("the theme's pictures are %v, want only ball.png", got)
	}
	open := http.NewServeMux()
	NewWhenPages(s, pics, nil).RegisterGather(open)
	rec := httptest.NewRecorder()
	open.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/open/themes?ids="+row["id"], nil))
	var shown struct {
		Themes []shownTheme `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &shown); err != nil || len(shown.Themes) != 1 {
		t.Fatalf("the theme straight after upload: %d %s", rec.Code, rec.Body)
	}
	art := cssURL.FindStringSubmatch(designValue(shown.Themes[0].Design, "card.art"))
	if art == nil || !strings.HasPrefix(art[1], themePictureRoute+"tpc") {
		t.Fatalf("the art straight after upload names %v", art)
	}
	rec = httptest.NewRecorder()
	open.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, art[1], nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("the art's picture straight after upload: %d %s %s", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
	}
	made(t, s, "THEME_PICTURE", s.Model().Table("THEME_PICTURE").Referencing("theme", row["id"])[0]["id"], "picture")
	theme["name"] = "Disco Night"
	if rec := postTheme(t, s, pics, host, theme, map[string][]byte{"ball.png": pngOf(t, 12)}); rec.Code != http.StatusOK {
		t.Fatalf("the host uploads it again: %d %s", rec.Code, rec.Body)
	}
	if again, _ := s.Model().Table("THEME").Get(row["id"]); again["name"] != "Disco Night" || len(s.Model().Table("THEME").All()) != 6 {
		t.Errorf("uploading the same key again gives %v, %d themes", again, len(s.Model().Table("THEME").All()))
	}
	for viewer, want := range map[string]int{staff: 6, parent: 5} {
		if got := len(as(t, s, viewer, `(from THEME)`)); got != want {
			t.Errorf("%s reads %d themes, want %d", viewer, got, want)
		}
	}
	set := func(viewer, theme string) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: "grp00000009001", Cells: map[string]any{"theme": theme}}}})
		return err
	}
	if err := set(staff, row["id"]); err != nil {
		t.Fatalf("the host can't use their own theme: %v", err)
	}
	if got := as(t, s, parent, `(from THEME (where (= key "disco")))`); len(got) != 1 {
		t.Errorf("a guest of an invitation in the host's theme reads %v", got)
	}
	if rec := postTheme(t, s, pics, other, map[string]any{"key": "plain", "name": "Mine"}, nil); rec.Code != http.StatusOK {
		t.Errorf("another host uploads a theme whose key the site uses: %d %s", rec.Code, rec.Body)
	}
	if _, err := Write(context.Background(), s, access.Actor{Email: maya}, Env{Viewer: s.Model().SignedIn(maya), Now: testNow}, Batch{Batch: []Edit{{Set: row["id"], Cells: map[string]any{"added_by": ""}}}}); err != nil {
		t.Fatalf("an admin can't make the theme the site's own: %v", err)
	}
	if got := len(as(t, s, parent, `(from THEME (where (= key "disco")))`)); got != 1 {
		t.Errorf("everyone reads the site's own theme: %d", got)
	}
}

func TestGatherAdminsDeleteTheSitesThemesButPlain(t *testing.T) {
	s := sample(t)
	ids, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{
		{Insert: "THEME", As: "t", Row: map[string]any{"key": "spare", "name": "Spare", "design": `{"card": {"art": "url(\"a.jpg\") center / cover"}}`}},
		{Insert: "THEME_PICTURE", Row: map[string]any{"theme": "@t", "name": "a.jpg", "original": "cnt00000009203", "picture": "cnt00000009203"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	spare := ids[0]
	admin := s.Model().SignedIn(maya)
	remove := func(email, viewer, id string) error {
		_, err := Write(context.Background(), s, access.Actor{Email: email}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Delete: id}}})
		return err
	}
	if err := remove(s.Model().addressOf(parent), parent, spare); err == nil {
		t.Error("someone who isn't an admin deleted one of the site's themes")
	}
	type use struct{ group, column, maker string }
	uses := []use{}
	for _, col := range []string{"theme", "envelope", "seal", "liner"} {
		for _, g := range s.Model().Table("GROUP").Referencing(col, skatingTheme) {
			uses = append(uses, use{g["id"], col, g["added_by"]})
		}
	}
	if len(uses) == 0 {
		t.Fatal("no sample invitation uses the skating theme")
	}
	was, _ := s.Model().Table("THEME").Get(skatingTheme)
	pictures := len(s.Model().Table("THEME_PICTURE").Referencing("theme", skatingTheme))
	if err := remove(maya, admin, skatingTheme); err != nil {
		t.Fatalf("an admin can't delete a theme invitations use: %v", err)
	}
	for _, u := range uses {
		g, _ := s.Model().Table("GROUP").Get(u.group)
		copied, ok := s.Model().Table("THEME").Get(g[u.column])
		if !ok || copied["added_by"] != u.maker || copied["name"] != was["name"] || copied["design"] != was["design"] {
			t.Errorf("%s's %s is %v after the delete; want a copy of %s for %s", u.group, u.column, copied, was["name"], u.maker)
			continue
		}
		if got := len(s.Model().Table("THEME_PICTURE").Referencing("theme", copied["id"])); got != pictures {
			t.Errorf("the copy for %s has %d pictures, want %d", u.maker, got, pictures)
		}
	}
	for _, g := range s.Model().Table("GROUP").Referencing("theme", plainTheme) {
		if _, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Set: g["id"], Cells: map[string]any{"theme": larpTheme}}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := remove(maya, admin, plainTheme); err == nil || !strings.Contains(err.Error(), "Create Your Own") {
		t.Errorf("an admin deleting plain, which no invitation uses: %v", err)
	}
	if err := remove(maya, admin, spare); err != nil {
		t.Fatalf("an admin can't delete an unused site theme: %v", err)
	}
	if _, ok := s.Model().Table("THEME").Get(spare); ok || len(s.Model().Table("THEME_PICTURE").Referencing("theme", spare)) != 0 {
		t.Error("the deleted theme or its pictures are still there")
	}
}

func TestDeletingYourOwnThemeInUseKeepsACopyUnderAnotherKey(t *testing.T) {
	s := sample(t)
	const theo = "grp00000009001"
	g, _ := s.Model().Table("GROUP").Get(theo)
	maker := g["added_by"]
	ids, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{
		{Insert: "THEME", As: "t", Row: map[string]any{"key": "mine", "name": "Mine", "design": `{}`, "added_by": maker}},
		{Set: theo, Cells: map[string]any{"theme": "@t"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(context.Background(), s, access.Actor{Email: s.Model().addressOf(maker)}, Env{Viewer: maker, Now: testNow}, Batch{Batch: []Edit{{Delete: ids[0]}}}); err != nil {
		t.Fatalf("the owner can't delete their theme an invitation uses: %v", err)
	}
	g, _ = s.Model().Table("GROUP").Get(theo)
	if copied, ok := s.Model().Table("THEME").Get(g["theme"]); !ok || copied["key"] != "mine-copy" || copied["added_by"] != maker || copied["name"] != "Mine" {
		t.Errorf("the invitation's theme is %v after the delete, want a copy keyed mine-copy", copied)
	}
}

func TestAnAdminsUploadReplacesTheSitesThemeWithItsKey(t *testing.T) {
	const skating = "thm00000000003"
	s := sample(t)
	pics := newPictures(s)
	themes := len(s.Model().Table("THEME").All())
	theme := map[string]any{"key": "skating", "name": "Ice Skating 2", "card": map[string]any{"art": `url("rink.png") center / cover`}}
	if rec := postTheme(t, s, pics, maya, theme, map[string][]byte{"rink.png": pngOf(t, 8)}); rec.Code != http.StatusOK {
		t.Fatalf("an admin uploads a new version of a site theme: %d %s", rec.Code, rec.Body)
	}
	if row, _ := s.Model().Table("THEME").Get(skating); row["name"] != "Ice Skating 2" || row["added_by"] != "" || len(s.Model().Table("THEME").All()) != themes {
		t.Errorf("after an admin's upload the site theme is %v, and there are %d themes, want %d", row, len(s.Model().Table("THEME").All()), themes)
	}
	if got := s.Model().Table("THEME_PICTURE").Referencing("theme", skating); len(got) != 1 || got[0]["name"] != "rink.png" {
		t.Errorf("the site theme's pictures are %v, want rink.png", got)
	}
	theme["name"] = "My Skating"
	if rec := postTheme(t, s, pics, s.Model().addressOf(parent), theme, map[string][]byte{"rink.png": pngOf(t, 8)}); rec.Code != http.StatusOK {
		t.Fatalf("a host uploads a theme with a site theme's key: %d %s", rec.Code, rec.Body)
	}
	if row, _ := s.Model().Table("THEME").Get(skating); row["name"] != "Ice Skating 2" {
		t.Errorf("a host's upload changed the site theme to %v", row)
	}
	if mine, ok := s.Model().Table("THEME").Find("skating", parent); !ok || mine["name"] != "My Skating" {
		t.Errorf("the host's upload is %v, %v; want a private theme of their own", mine, ok)
	}
}

func TestGatherAdminsManageEveryTheme(t *testing.T) {
	s := sample(t)
	ids, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{
		{Insert: "THEME", Row: map[string]any{"key": "mine", "name": "Mine", "design": `{}`, "added_by": parent}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	private := ids[0]
	admin := s.Model().SignedIn(maya)
	change := func(email, viewer string, e Edit) error {
		_, err := Write(context.Background(), s, access.Actor{Email: email}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{e}})
		return err
	}
	host := s.Model().addressOf(parent)
	if err := change(host, parent, Edit{Set: larpTheme, Cells: map[string]any{"name": "Quest"}}); err == nil {
		t.Error("someone who isn't an admin renamed one of the site's themes")
	}
	if err := change(maya, admin, Edit{Set: larpTheme, Cells: map[string]any{"name": "Quest", "blurb": "Swords"}}); err != nil {
		t.Fatalf("an admin can't rename a site theme: %v", err)
	}
	if err := change(maya, admin, Edit{Set: skatingTheme, Cells: map[string]any{"added_by": admin}}); err != nil {
		t.Fatalf("an admin can't make a site theme private to themselves: %v", err)
	}
	if got := len(as(t, s, guest, `(from THEME (where (= key "skating")))`)); got != 0 {
		t.Errorf("someone with no invitation in it reads a theme an admin made private: %d", got)
	}
	if err := change(maya, admin, Edit{Set: skatingTheme, Cells: map[string]any{"added_by": ""}}); err != nil {
		t.Fatalf("an admin can't make the theme public again: %v", err)
	}
	if err := change(maya, admin, Edit{Set: plainTheme, Cells: map[string]any{"added_by": admin}}); err == nil || !strings.Contains(err.Error(), "Create Your Own") {
		t.Errorf("an admin making plain private: %v", err)
	}
	if err := change(maya, admin, Edit{Delete: private}); err != nil {
		t.Fatalf("an admin can't delete someone's private theme: %v", err)
	}
}

func TestTheThemeKitHoldsAPreviewAndTheSitesThemes(t *testing.T) {
	s := sample(t)
	t.Chdir("../..")
	bucket := blob.NewMemoryBucket()
	if err := bucket.FillFrom("sampledata/bucket"); err != nil {
		t.Fatal(err)
	}
	p := NewWhenPages(s, NewPictures(s, bucket), nil)
	mux := http.NewServeMux()
	p.RegisterGather(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/open/theme-kit.zip", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("theme kit: %d %s", rec.Code, rec.Body)
	}
	z, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range z.File {
		r, _ := f.Open()
		files[strings.TrimPrefix(f.Name, "gather-theme-kit/")], _ = io.ReadAll(r)
	}
	for _, want := range []string{"README.md", "standard.json", "preview.html", "my-theme/theme.json", "examples/skating/theme.json", "examples/larp/theme.json", "examples/area/theme.json", "examples/area/aliens.jpg"} {
		if _, ok := files[want]; !ok {
			t.Errorf("the kit has no %s", want)
		}
	}
	if _, ok := files["examples/plain/theme.json"]; ok {
		t.Error("the kit holds Create Your Own's blank theme as an example")
	}
	preview := string(files["preview.html"])
	if strings.Contains(preview, "/*GATHER-") || strings.Count(preview, "</script>") != 2 || !strings.Contains(preview, `modules["/envelope.js"] = (() => {`) {
		t.Errorf("the preview's code is not filled in")
	}
	for _, name := range []string{"my-theme/theme.json", "examples/skating/theme.json"} {
		d, err := storedDesign(string(files[name]))
		if err != nil {
			t.Fatal(err)
		}
		delete(d, "key")
		delete(d, "name")
		delete(d, "blurb")
		if err := standardDesign(d); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTheOpenThemesResolveTheirPictures(t *testing.T) {
	s := sample(t)
	p := NewWhenPages(s, newPictures(s), nil)
	mux := http.NewServeMux()
	p.RegisterGather(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/open/themes?ids="+areaTheme+","+skatingTheme, nil))
	var got struct {
		Themes []shownTheme `json:"themes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Themes) != 2 {
		t.Fatalf("themes: %d %s", rec.Code, rec.Body)
	}
	if liner := designValue(got.Themes[0].Design, "envelope.liner"); liner != `url("/open/theme-picture/tpc00000000001?c=cnt00000009201") center 40% / cover no-repeat, #0c1b22` {
		t.Errorf("Area 51's liner is %s", liner)
	}
	if got.Themes[1].Design["key"] != "skating" || designValue(got.Themes[1].Design, "card.width") != "560px" {
		t.Errorf("skating is not merged over the standard: %v", got.Themes[1].Design["card"])
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func zerosZip(t *testing.T, name string, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(f, zeroReader{}, int64(n)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func filesRequest(t *testing.T, parts map[string][]byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for name, content := range parts {
		w, err := form.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/do/theme", body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	if err := r.ParseMultipartForm(formMemory); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestThemeArchivesShareOneExpandedBudget(t *testing.T) {
	small, err := droppedFiles(filesRequest(t, map[string][]byte{
		"one.zip": zerosZip(t, "a.txt", 32),
		"two.zip": zerosZip(t, "b.txt", 32),
	}))
	if err != nil || len(small) != 2 {
		t.Fatalf("two small archives: %d files, %v", len(small), err)
	}
	n := fileLimit/2 + 1
	one := zerosZip(t, "a.txt", n)
	var used uint64
	got, err := unzipped("one.zip", one, &used)
	if err != nil || len(got) != 1 || len(got[0].content) != n {
		t.Fatalf("one archive of half the budget: %d files, err %v", len(got), err)
	}
	got = nil
	_, err = droppedFiles(filesRequest(t, map[string][]byte{
		"one.zip": one,
		"two.zip": zerosZip(t, "b.txt", n),
	}))
	if err == nil || !strings.Contains(err.Error(), "more than a theme's worth of files") {
		t.Fatalf("two archives over the budget together: %v", err)
	}
}

func TestACorruptThemeEntryIsRefused(t *testing.T) {
	raw := storedZip(t, "note.txt", []byte("hello"))
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := z.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := rc.Read(buf)
	if n != len("hello") || err != nil {
		t.Fatalf("first read %d %v", n, err)
	}
	n, err = rc.Read(buf)
	rc.Close()
	if n != 0 || err != io.EOF {
		t.Fatalf("next read %d %v", n, err)
	}
	var used uint64
	got, err := unzipped("theme.zip", raw, &used)
	if err != nil || len(got) != 1 || string(got[0].content) != "hello" {
		t.Fatalf("honest entry: %v %v", got, err)
	}
	at := bytes.LastIndex(raw, []byte{0x50, 0x4b, 0x01, 0x02})
	if at < 0 || at+20 > len(raw) {
		t.Fatal("no central directory")
	}
	raw[at+16] ^= 0xff
	used = 0
	_, err = unzipped("theme.zip", raw, &used)
	if err == nil || !strings.Contains(err.Error(), "could not read note.txt in theme.zip") {
		t.Fatalf("corrupt entry: %v", err)
	}
}

// A stored entry returns its bytes, then EOF on the next read, which is when
// the checksum runs. Deflate can return both on one read.
func storedZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
