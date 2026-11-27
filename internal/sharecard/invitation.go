package sharecard

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	InvitationWidth  = 1200
	InvitationHeight = 630
)

type Invitation struct {
	Backdrop, Paper, Accent                          color.RGBA
	KickerInk, TitleInk, DayInk, PlaceInk, HostsInk  color.RGBA
	BackdropPicture, CardPicture                     image.Image
	Wordless, KickerUpper                            bool
	Kicker, Title, Day, Hours, Place, Address, Hosts string
	DisplayFont, BodyFont, KickerFont, PlaceFont     *opentype.Font
}

func SiteFonts() (*opentype.Font, *opentype.Font, error) {
	return loadFaces()
}

func InvitationAnswered(w http.ResponseWriter, r *http.Request, parts ...string) bool {
	return answer(w, r, ETag(append(parts, "invitation")...))
}

func ServeInvitation(w http.ResponseWriter, r *http.Request, v Invitation, parts ...string) {
	etag := ETag(append(parts, "invitation")...)
	png, err := DrawInvitation(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cardCache.Put(etag, png)
	writeCard(w, etag, png)
}

func cover(dst draw.Image, at image.Rectangle, src image.Image) {
	b := src.Bounds()
	want := float64(at.Dx()) / float64(at.Dy())
	crop := b
	if float64(b.Dx())/float64(b.Dy()) > want {
		w := int(float64(b.Dy()) * want)
		crop = image.Rect(b.Min.X+(b.Dx()-w)/2, b.Min.Y, b.Min.X+(b.Dx()-w)/2+w, b.Max.Y)
	} else {
		h := int(float64(b.Dx()) / want)
		crop = image.Rect(b.Min.X, b.Min.Y+(b.Dy()-h)/2, b.Max.X, b.Min.Y+(b.Dy()-h)/2+h)
	}
	draw.CatmullRom.Scale(dst, at, src, crop, draw.Over, nil)
}

type textLine struct {
	face   font.Face
	ink    color.RGBA
	text   string
	height int
}

func DrawInvitation(v Invitation) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, InvitationWidth, InvitationHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(v.Backdrop), image.Point{}, draw.Src)
	if v.BackdropPicture != nil {
		cover(img, img.Bounds(), v.BackdropPicture)
	}
	card := image.Rect(370, 30, 830, 600)
	shadow := color.RGBA{0, 0, 0, 0x44}
	fill(img, shadow, func(p iconPath) {
		p.roundRect(float32(card.Min.X+6), float32(card.Min.Y+12), float32(card.Dx()), float32(card.Dy()), 6, true)
	})
	fill(img, v.Paper, func(p iconPath) {
		p.roundRect(float32(card.Min.X), float32(card.Min.Y), float32(card.Dx()), float32(card.Dy()), 6, true)
	})
	if v.CardPicture != nil {
		cover(img, card, v.CardPicture)
	}
	if !v.Wordless {
		if err := drawWords(img, card, v); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func drawWords(img *image.RGBA, card image.Rectangle, v Invitation) error {
	width := fixed.I(card.Dx() - 72)
	d := &font.Drawer{Dst: img}
	lines := []textLine{}
	add := func(f *opentype.Font, size float64, ink color.RGBA, text string, gap int, wrap bool) error {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		fc, err := face(f, size)
		if err != nil {
			return err
		}
		d.Face = fc
		parts := []string{text}
		if wrap {
			parts = Wrap(d, text, width)
		}
		for i, part := range parts {
			height := int(size * 1.25)
			if i == 0 {
				height += gap
			}
			lines = append(lines, textLine{face: fc, ink: ink, text: part, height: height})
		}
		return nil
	}
	kicker := v.Kicker
	if v.KickerUpper {
		kicker = strings.ToUpper(kicker)
	}
	if err := add(v.KickerFont, 20, v.KickerInk, kicker, 0, true); err != nil {
		return err
	}
	titleSize := 56.0
	for ; titleSize > 30; titleSize -= 2 {
		fc, err := face(v.DisplayFont, titleSize)
		if err != nil {
			return err
		}
		d.Face = fc
		if len(Wrap(d, v.Title, width)) <= 3 {
			break
		}
	}
	if err := add(v.DisplayFont, titleSize, v.TitleInk, v.Title, 12, true); err != nil {
		return err
	}
	rule := len(lines)
	for _, l := range []struct {
		f    *opentype.Font
		size float64
		ink  color.RGBA
		text string
		gap  int
	}{
		{v.BodyFont, 26, v.DayInk, v.Day, 40},
		{v.BodyFont, 22, v.DayInk, v.Hours, 0},
		{v.PlaceFont, 22, v.PlaceInk, v.Place, 14},
		{v.PlaceFont, 20, v.PlaceInk, v.Address, 0},
		{v.BodyFont, 18, v.HostsInk, v.Hosts, 22},
	} {
		if err := add(l.f, l.size, l.ink, l.text, l.gap, true); err != nil {
			return err
		}
	}
	total := 0
	for _, l := range lines {
		total += l.height
	}
	cx := (card.Min.X + card.Max.X) / 2
	y := card.Min.Y + (card.Dy()-total)/2
	for i, l := range lines {
		y += l.height
		if i == rule {
			ry := float32(y - l.height + 8)
			fill(img, v.Accent, func(p iconPath) { p.roundRect(float32(cx-40), ry, 80, 3, 1.5, true) })
		}
		d.Face, d.Src = l.face, image.NewUniform(l.ink)
		centered(d, l.text, cx, y-int(float64(l.height)*0.25))
	}
	return nil
}
