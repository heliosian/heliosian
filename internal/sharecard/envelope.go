package sharecard

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	EnvelopeWidth  = 1072
	EnvelopeHeight = 760
)

type Envelope struct {
	Backdrop, Body, Side, Front, Flap, Liner, Seal, SealInk, Paper, Ink, Accent color.RGBA
	Kicker, Title, Mark                                                         string
}

func fill(dst draw.Image, c color.Color, shape func(p iconPath)) {
	b := dst.Bounds()
	r := vector.NewRasterizer(b.Dx(), b.Dy())
	shape(iconPath{r})
	paintIcon(dst, r, b, c)
}

func polygon(points ...float32) func(p iconPath) {
	return func(p iconPath) {
		p.r.MoveTo(points[0], points[1])
		for i := 2; i < len(points); i += 2 {
			p.r.LineTo(points[i], points[i+1])
		}
		p.r.ClosePath()
	}
}

func centered(d *font.Drawer, text string, cx, baseline int) {
	w := d.MeasureString(text).Round()
	d.Dot = fixed.P(cx-w/2, baseline)
	d.DrawString(text)
}

func DrawEnvelope(e Envelope) ([]byte, error) {
	bold, medium, err := loadFaces()
	if err != nil {
		return nil, fmt.Errorf("envelope fonts: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, EnvelopeWidth, EnvelopeHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(e.Backdrop), image.Point{}, draw.Src)

	const (
		x0, x1, y0, y1 = 136, 936, 330, 730
		cx             = EnvelopeWidth / 2
	)
	shadow := color.RGBA{0, 0, 0, 0x22}
	fill(img, shadow, func(p iconPath) { p.roundRect(x0+6, y0+14, x1-x0, y1-y0, 14, true) })
	fill(img, e.Body, func(p iconPath) { p.roundRect(x0, y0, x1-x0, y1-y0, 14, true) })
	fill(img, e.Flap, polygon(x0, y0, x1, y0, cx, y0-210))
	fill(img, e.Liner, polygon(x0+26, y0, x1-26, y0, cx, y0-180))

	card := image.Rect(206, 110, 866, 640)
	fill(img, shadow, func(p iconPath) {
		p.roundRect(float32(card.Min.X+4), float32(card.Min.Y+8), float32(card.Dx()), float32(card.Dy()), 8, true)
	})
	fill(img, e.Paper, func(p iconPath) {
		p.roundRect(float32(card.Min.X), float32(card.Min.Y), float32(card.Dx()), float32(card.Dy()), 8, true)
	})
	d := &font.Drawer{Dst: img}
	kicker, err := face(medium, 24)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = kicker, image.NewUniform(e.Accent)
	centered(d, strings.ToUpper(e.Kicker), cx, 172)
	title, err := face(bold, 54)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = title, image.NewUniform(e.Ink)
	lines := Wrap(d, e.Title, fixed.I(card.Dx()-96))
	if len(lines) > 2 {
		lines = append(lines[:1], strings.Join(lines[1:], " "))
		for d.MeasureString(lines[1]) > fixed.I(card.Dx()-96) && len(lines[1]) > 1 {
			lines[1] = strings.TrimSpace(lines[1][:len(lines[1])-2]) + "…"
		}
	}
	for i, line := range lines {
		centered(d, line, cx, 246+i*62)
	}
	fill(img, e.Accent, func(p iconPath) { p.roundRect(cx-42, float32(270+(len(lines)-1)*62), 84, 4, 2, true) })

	mid := float32(y0+y1) / 2
	fill(img, e.Side, polygon(x0, y0+6, x0, y1, cx-10, mid+40))
	fill(img, e.Side, polygon(x1, y0+6, x1, y1, cx+10, mid+40))
	fill(img, e.Front, polygon(x0, y1, x1, y1, cx, y0+150))

	sealY := float32(y0 + 170)
	fill(img, shadow, func(p iconPath) { p.circle(cx+2, sealY+5, 50, true) })
	fill(img, e.Seal, func(p iconPath) { p.circle(cx, sealY, 50, true) })
	mark, err := face(bold, 44)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = mark, image.NewUniform(e.SealInk)
	centered(d, e.Mark, cx, int(sealY)+16)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
