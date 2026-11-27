package db

import (
	"bytes"
	"image"
	"image/color/palette"
	"image/gif"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

const theoParty = "grp00000009001"

func animatedGIF(t *testing.T, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for range frames {
		g.Image = append(g.Image, image.NewPaletted(image.Rect(0, 0, 16, 16), palette.Plan9))
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func postDesignPicture(t *testing.T, s *Store, pics *Pictures, picture []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	form.WriteField("group", theoParty)
	form.WriteField("part", "card")
	part, _ := form.CreateFormFile("picture", "picture")
	part.Write(picture)
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, pics, testTokens)
	r := httptest.NewRequest(http.MethodPost, "/api/do/design-picture", body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	serveAs(mux, rec, r, maya)
	return rec
}

func TestAnAnimatedCardPictureKeepsMoving(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	for _, c := range []struct {
		name    string
		picture []byte
		want    string
	}{{"an animated GIF", animatedGIF(t, 3), "image/gif"}, {"a GIF that holds still", animatedGIF(t, 1), "image/jpeg"}, {"a PNG", pngOf(t, 8), "image/jpeg"}} {
		if rec := postDesignPicture(t, s, pics, c.picture); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", c.name, rec.Code, rec.Body)
		}
		if err := pics.make(theoParty); err != nil {
			t.Fatalf("making %s: %v", c.name, err)
		}
		g, _ := s.Model().Table("GROUP").Get(theoParty)
		if made, _ := s.Model().Table("CONTENT").Get(g["card_picture"]); made["mime"] != c.want {
			t.Errorf("%s became %q, want %q", c.name, made["mime"], c.want)
		}
	}
}

func TestAThemesAnimatedPictureKeepsMoving(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	theme := map[string]any{"key": "stars", "name": "Stars", "page": map[string]any{"backdrop": `url("stars.gif") center / cover, #101830`}}
	if rec := postTheme(t, s, pics, s.Model().addressOf(staff), theme, map[string][]byte{"stars.gif": animatedGIF(t, 3)}); rec.Code != http.StatusOK {
		t.Fatalf("a theme with an animated picture: %d %s", rec.Code, rec.Body)
	}
	row, _ := s.Model().Table("THEME").Find("stars", staff)
	pictures := s.Model().Table("THEME_PICTURE").Referencing("theme", row["id"])
	if len(pictures) != 1 {
		t.Fatalf("the theme's pictures are %v", pictures)
	}
	if err := pics.make(pictures[0]["id"]); err != nil {
		t.Fatal(err)
	}
	picture, _ := s.Model().Table("THEME_PICTURE").Get(pictures[0]["id"])
	if made, _ := s.Model().Table("CONTENT").Get(picture["picture"]); made["mime"] != "image/gif" {
		t.Errorf("the theme's animated picture became %q", made["mime"])
	}
}
