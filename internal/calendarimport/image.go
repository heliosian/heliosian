package calendarimport

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/anthropics/anthropic-sdk-go"
)

func renderPage(pdf []byte) (image.Image, error) {
	dir, err := os.MkdirTemp("", "calendarimport")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("pdftoppm", "-png", "-r", "300", "-singlefile", "-f", "1", "-l", "1", in, filepath.Join(dir, "page"))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w", err)
	}
	f, err := os.Open(filepath.Join(dir, "page.png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func hsl(r, g, b float64) (float64, float64, float64) {
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func rgb(h, s, l float64) (float64, float64, float64) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

func enhance(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	out := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			fr, fg, fb := float64(r)/65535, float64(g)/65535, float64(b)/65535
			h, s, l := hsl(fr, fg, fb)
			if s > 0.15 && l > 0.2 && l < 0.97 {
				fr, fg, fb = rgb(h, 1, 0.55)
			}
			out.Set(x, y, color.RGBA{uint8(fr*255 + 0.5), uint8(fg*255 + 0.5), uint8(fb*255 + 0.5), 255})
		}
	}
	return out
}

func imageBlock(img image.Image) (anthropic.ContentBlockParamUnion, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return anthropic.ContentBlockParamUnion{}, err
	}
	return anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(buf.Bytes())), nil
}

func titleBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	h, s, l := hsl(float64(r)/65535, float64(g)/65535, float64(b)/65535)
	return s > 0.8 && h > 195 && h < 235 && l > 0.4 && l < 0.7
}

func titleBars(page *image.RGBA) []image.Rectangle {
	bounds := page.Bounds()
	minRun := bounds.Dx() / 10
	bars := []image.Rectangle{}
	place := func(run image.Rectangle) {
		for i, bar := range bars {
			if run.Min.Y-bar.Max.Y <= 3 && run.Min.X < bar.Max.X && bar.Min.X < run.Max.X {
				bars[i] = bar.Union(run)
				return
			}
		}
		bars = append(bars, run)
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		start := -1
		for x := bounds.Min.X; x <= bounds.Max.X; x++ {
			blue := x < bounds.Max.X && titleBlue(page.At(x, y))
			if blue && start < 0 {
				start = x
			}
			if !blue && start >= 0 {
				if x-start >= minRun {
					place(image.Rect(start, y, x, y+1))
				}
				start = -1
			}
		}
	}
	tall := []image.Rectangle{}
	for _, bar := range bars {
		if bar.Dy() >= bounds.Dy()/150 {
			tall = append(tall, bar)
		}
	}
	return tall
}

func monthCrops(page *image.RGBA) ([]anthropic.ContentBlockParamUnion, []image.Rectangle, error) {
	bars := titleBars(page)
	if len(bars) != 12 {
		return nil, nil, fmt.Errorf("found %d month title bars on the page, want 12: %v", len(bars), bars)
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Min.X < bars[j].Min.X })
	columns := [][]image.Rectangle{{bars[0]}}
	for _, bar := range bars[1:] {
		last := columns[len(columns)-1]
		if bar.Min.X-last[0].Min.X > page.Bounds().Dx()/4 {
			columns = append(columns, []image.Rectangle{})
		}
		columns[len(columns)-1] = append(columns[len(columns)-1], bar)
	}
	if len(columns) != 2 || len(columns[0]) != 6 || len(columns[1]) != 6 {
		return nil, nil, fmt.Errorf("the month title bars are not two columns of six")
	}
	margin := page.Bounds().Dx() / 250
	crops := []anthropic.ContentBlockParamUnion{}
	rects := []image.Rectangle{}
	for _, column := range columns {
		sort.Slice(column, func(i, j int) bool { return column[i].Min.Y < column[j].Min.Y })
		for i, bar := range column {
			top := bar.Min.Y - margin
			bottom := 0
			if i+1 < len(column) {
				bottom = column[i+1].Min.Y - margin
			} else {
				bottom = top + (bar.Min.Y - column[i-1].Min.Y)
			}
			rect := image.Rect(bar.Min.X-margin, top, bar.Max.X+margin, bottom).Intersect(page.Bounds())
			out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
			draw.Draw(out, out.Bounds(), page, rect.Min, draw.Src)
			block, err := imageBlock(out)
			if err != nil {
				return nil, nil, err
			}
			crops = append(crops, block)
			rects = append(rects, rect)
		}
	}
	return crops, rects, nil
}
