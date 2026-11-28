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
	Wordless, KickerUpper, SideBySide                bool
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
	dx     int
	beside bool
}

type lineSpec struct {
	f    *opentype.Font
	size float64
	ink  color.RGBA
	text string
	gap  int
}

type lineBuilder func(s lineSpec, within int) ([]textLine, error)

func built(build lineBuilder, within int, specs ...lineSpec) ([]textLine, error) {
	out := []textLine{}
	for _, s := range specs {
		more, err := build(s, within)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func (v Invitation) details(build lineBuilder, width int) ([]textLine, error) {
	day := lineSpec{v.BodyFont, 26, v.DayInk, v.Day, 40}
	hours := lineSpec{v.BodyFont, 22, v.DayInk, v.Hours, 0}
	place := lineSpec{v.PlaceFont, 22, v.PlaceInk, v.Place, 14}
	street, town, _ := strings.Cut(v.Address, ", ")
	address := lineSpec{v.PlaceFont, 20, v.PlaceInk, street, 0}
	city := lineSpec{v.PlaceFont, 20, v.PlaceInk, town, 0}
	if !v.SideBySide || strings.TrimSpace(v.Place+v.Address) == "" {
		return built(build, width, day, hours, place, address, city)
	}
	day.size, hours.size, place.size, address.size, city.size = 21, 18, 19, 17, 17
	place.gap, address.gap = day.gap, 0
	if strings.TrimSpace(v.Place) == "" {
		address.gap = day.gap
	}
	half := (width + 40 - 24) / 2
	left, err := built(build, half, day, hours)
	if err != nil {
		return nil, err
	}
	right, err := built(build, half, place, address, city)
	if err != nil {
		return nil, err
	}
	rows := []textLine{}
	for i := range max(len(left), len(right)) {
		row := []textLine{}
		if i < len(left) {
			row = append(row, left[i])
			row[len(row)-1].dx = -(half + 24) / 2
		}
		if i < len(right) {
			row = append(row, right[i])
			row[len(row)-1].dx = (half + 24) / 2
		}
		height := 0
		for _, l := range row {
			height = max(height, l.height)
		}
		for j := range row {
			row[j].height, row[j].beside = height, j > 0
		}
		rows = append(rows, row...)
	}
	return rows, nil
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
	width := card.Dx() - 72
	d := &font.Drawer{Dst: img}
	build := func(s lineSpec, within int) ([]textLine, error) {
		if strings.TrimSpace(s.text) == "" {
			return nil, nil
		}
		fc, err := face(s.f, s.size)
		if err != nil {
			return nil, err
		}
		d.Face = fc
		out := []textLine{}
		for i, part := range Wrap(d, s.text, fixed.I(within)) {
			height := int(s.size * 1.25)
			if i == 0 {
				height += s.gap
			}
			out = append(out, textLine{face: fc, ink: s.ink, text: part, height: height})
		}
		return out, nil
	}
	kicker := v.Kicker
	if v.KickerUpper {
		kicker = strings.ToUpper(kicker)
	}
	titleSize := 56.0
	for ; titleSize > 30; titleSize -= 2 {
		fc, err := face(v.DisplayFont, titleSize)
		if err != nil {
			return err
		}
		d.Face = fc
		if len(Wrap(d, v.Title, fixed.I(width))) <= 3 {
			break
		}
	}
	lines, err := built(build, width, lineSpec{v.KickerFont, 20, v.KickerInk, kicker, 0}, lineSpec{v.DisplayFont, titleSize, v.TitleInk, v.Title, 12})
	if err != nil {
		return err
	}
	rule := len(lines)
	details, err := v.details(build, width)
	if err != nil {
		return err
	}
	hosts, err := build(lineSpec{v.BodyFont, 18, v.HostsInk, v.Hosts, 22}, width)
	if err != nil {
		return err
	}
	lines = append(append(lines, details...), hosts...)
	total := 0
	for _, l := range lines {
		if !l.beside {
			total += l.height
		}
	}
	cx := (card.Min.X + card.Max.X) / 2
	y := card.Min.Y + (card.Dy()-total)/2
	for i, l := range lines {
		if !l.beside {
			y += l.height
		}
		if i == rule {
			ry := float32(y - l.height + 8)
			fill(img, v.Accent, func(p iconPath) { p.roundRect(float32(cx-40), ry, 80, 3, 1.5, true) })
		}
		d.Face, d.Src = l.face, image.NewUniform(l.ink)
		centered(d, l.text, cx+l.dx, y-int(float64(l.height)*0.25))
	}
	return nil
}
