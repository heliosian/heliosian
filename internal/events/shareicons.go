package events

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// The two glyphs on the share card, drawn with the vector rasterizer so they
// stay crisp at any size: a calendar and a clock in the site's icon style -
// round-cornered strokes, nothing filled but the strokes themselves.

// iconPath collects an outline; strokes are built as an outer shape wound one
// way and an inner shape wound the other, which the rasterizer's winding
// leaves as a ring.
type iconPath struct{ r *vector.Rasterizer }

func (p iconPath) roundRect(x, y, w, h, rad float32, clockwise bool) {
	k := rad * 0.5523 // cubic approximation of a quarter circle
	if clockwise {
		p.r.MoveTo(x+rad, y)
		p.r.LineTo(x+w-rad, y)
		p.r.CubeTo(x+w-rad+k, y, x+w, y+rad-k, x+w, y+rad)
		p.r.LineTo(x+w, y+h-rad)
		p.r.CubeTo(x+w, y+h-rad+k, x+w-rad+k, y+h, x+w-rad, y+h)
		p.r.LineTo(x+rad, y+h)
		p.r.CubeTo(x+rad-k, y+h, x, y+h-rad+k, x, y+h-rad)
		p.r.LineTo(x, y+rad)
		p.r.CubeTo(x, y+rad-k, x+rad-k, y, x+rad, y)
	} else {
		p.r.MoveTo(x+rad, y)
		p.r.CubeTo(x+rad-k, y, x, y+rad-k, x, y+rad)
		p.r.LineTo(x, y+h-rad)
		p.r.CubeTo(x, y+h-rad+k, x+rad-k, y+h, x+rad, y+h)
		p.r.LineTo(x+w-rad, y+h)
		p.r.CubeTo(x+w-rad+k, y+h, x+w, y+h-rad+k, x+w, y+h-rad)
		p.r.LineTo(x+w, y+rad)
		p.r.CubeTo(x+w, y+rad-k, x+w-rad+k, y, x+w-rad, y)
	}
	p.r.ClosePath()
}

func (p iconPath) circle(cx, cy, rad float32, clockwise bool) {
	k := rad * 0.5523
	if clockwise {
		p.r.MoveTo(cx, cy-rad)
		p.r.CubeTo(cx+k, cy-rad, cx+rad, cy-k, cx+rad, cy)
		p.r.CubeTo(cx+rad, cy+k, cx+k, cy+rad, cx, cy+rad)
		p.r.CubeTo(cx-k, cy+rad, cx-rad, cy+k, cx-rad, cy)
		p.r.CubeTo(cx-rad, cy-k, cx-k, cy-rad, cx, cy-rad)
	} else {
		p.r.MoveTo(cx, cy-rad)
		p.r.CubeTo(cx-k, cy-rad, cx-rad, cy-k, cx-rad, cy)
		p.r.CubeTo(cx-rad, cy+k, cx-k, cy+rad, cx, cy+rad)
		p.r.CubeTo(cx+k, cy+rad, cx+rad, cy+k, cx+rad, cy)
		p.r.CubeTo(cx+rad, cy-k, cx+k, cy-rad, cx, cy-rad)
	}
	p.r.ClosePath()
}

// line is a stroke from one point to another with round ends.
func (p iconPath) line(x1, y1, x2, y2, t float32) {
	dx, dy := x2-x1, y2-y1
	n := float32(math.Hypot(float64(dx), float64(dy)))
	if n == 0 {
		return
	}
	nx, ny := -dy/n*t/2, dx/n*t/2
	p.r.MoveTo(x1+nx, y1+ny)
	p.r.LineTo(x2+nx, y2+ny)
	p.r.LineTo(x2-nx, y2-ny)
	p.r.LineTo(x1-nx, y1-ny)
	p.r.ClosePath()
	p.circle(x1, y1, t/2, true)
	p.circle(x2, y2, t/2, true)
}

func paintIcon(dst draw.Image, r *vector.Rasterizer, at image.Rectangle, c color.Color) {
	mask := image.NewAlpha(image.Rect(0, 0, at.Dx(), at.Dy()))
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	draw.DrawMask(dst, at, image.NewUniform(c), image.Point{}, mask, image.Point{}, draw.Over)
}

// drawCalendarIcon is the site's calendar glyph: a round-cornered frame, a
// rule under its header, two pegs on top.
func drawCalendarIcon(dst draw.Image, at image.Rectangle, c color.Color) {
	s := float32(at.Dx())
	t := s * 0.1
	r := vector.NewRasterizer(at.Dx(), at.Dy())
	p := iconPath{r}
	p.roundRect(t/2, s*0.16, s-t, s*0.84-t/2, s*0.14, true)
	p.roundRect(t/2+t, s*0.16+t, s-3*t, s*0.84-t/2-2*t, s*0.08, false)
	p.line(t, s*0.42, s-t, s*0.42, t)
	p.line(s*0.3, s*0.04, s*0.3, s*0.26, t)
	p.line(s*0.7, s*0.04, s*0.7, s*0.26, t)
	paintIcon(dst, r, at, c)
}

// drawClockIcon is the clock glyph: a ring with the hands at four o'clock.
func drawClockIcon(dst draw.Image, at image.Rectangle, c color.Color) {
	s := float32(at.Dx())
	t := s * 0.1
	r := vector.NewRasterizer(at.Dx(), at.Dy())
	p := iconPath{r}
	p.circle(s/2, s/2, s/2-t/2, true)
	p.circle(s/2, s/2, s/2-t/2-t, false)
	p.line(s/2, s/2, s/2, s*0.24, t)
	p.line(s/2, s/2, s*0.7, s*0.62, t)
	paintIcon(dst, r, at, c)
}
