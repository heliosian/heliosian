package blob

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/gif"

	"golang.org/x/image/draw"
)

const (
	motionSide = 1280
	// Bytes kept for a frame besides its pixels: a cloned 256-color palette
	// and the encoded color table, rounded up so a growing list still fits.
	frameHeader = 4096 + 1024
)

func Animated(src []byte) bool {
	return bytes.HasPrefix(src, []byte("GIF8")) && bytes.Count(src, []byte{0x21, 0xf9, 0x04}) > 1
}

func ScaledGIF(src []byte) ([]byte, error) {
	if err := Check(src); err != nil {
		return nil, err
	}
	w, h, frames, err := gifScreen(src)
	if err != nil {
		return nil, err
	}
	area := int64(w) * int64(h)
	if area < 1 || int64(frames) > int64(4*maxPixels)/(area+frameHeader) {
		return nil, fmt.Errorf("the animation is %dx%d pixels over %d frames, too much to show", w, h, frames)
	}
	g, err := gif.DecodeAll(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	if int64(len(g.Image)) > int64(4*maxPixels)/(area+frameHeader) {
		return nil, fmt.Errorf("the animation is %dx%d pixels over %d frames, too much to show", w, h, len(g.Image))
	}
	scale := min(1, float64(motionSide)/float64(max(w, h)))
	sw, sh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	out := &gif.GIF{LoopCount: g.LoopCount, Config: image.Config{Width: sw, Height: sh}}
	for i, frame := range g.Image {
		before := image.NewRGBA(canvas.Bounds())
		draw.Draw(before, before.Bounds(), canvas, image.Point{}, draw.Src)
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		pal := frame.Palette
		if len(pal) == 0 {
			pal = palette.Plan9
		}
		shown := image.NewPaletted(image.Rect(0, 0, sw, sh), pal)
		draw.ApproxBiLinear.Scale(shown, shown.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
		out.Image = append(out.Image, shown)
		out.Delay = append(out.Delay, g.Delay[i])
		out.Disposal = append(out.Disposal, gif.DisposalNone)
		switch {
		case i < len(g.Disposal) && g.Disposal[i] == gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case i < len(g.Disposal) && g.Disposal[i] == gif.DisposalPrevious:
			canvas = before
		}
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var errGIF = errors.New("gif: not an animation this server can read")

func gifScreen(src []byte) (w, h, frames int, err error) {
	if len(src) < 13 || (string(src[:6]) != "GIF87a" && string(src[:6]) != "GIF89a") {
		return 0, 0, 0, errGIF
	}
	w = int(src[6]) | int(src[7])<<8
	h = int(src[8]) | int(src[9])<<8
	i := 13
	if src[10]&0x80 != 0 {
		i, err = skipColorTable(src, i, src[10])
		if err != nil {
			return 0, 0, 0, err
		}
	}
	for i < len(src) {
		switch src[i] {
		case 0x3b:
			if frames == 0 {
				return 0, 0, 0, errGIF
			}
			return w, h, frames, nil
		case 0x21:
			i, err = skipExtension(src, i+1)
			if err != nil {
				return 0, 0, 0, err
			}
		case 0x2c:
			i, err = skipImage(src, i+1)
			if err != nil {
				return 0, 0, 0, err
			}
			frames++
		default:
			return 0, 0, 0, errGIF
		}
	}
	return 0, 0, 0, errGIF
}

func skipColorTable(src []byte, i int, fields byte) (int, error) {
	n := 3 * (1 << (1 + uint(fields&7)))
	if i > len(src)-n {
		return 0, errGIF
	}
	return i + n, nil
}

func skipExtension(src []byte, i int) (int, error) {
	if i >= len(src) {
		return 0, errGIF
	}
	label := src[i]
	i++
	switch label {
	case 0xF9:
		if i > len(src)-6 {
			return 0, errGIF
		}
		return i + 6, nil
	case 0x01:
		if i > len(src)-13 {
			return 0, errGIF
		}
		return skipSubBlocks(src, i+13)
	case 0xFF:
		if i >= len(src) {
			return 0, errGIF
		}
		n := int(src[i])
		i++
		if i > len(src)-n {
			return 0, errGIF
		}
		return skipSubBlocks(src, i+n)
	case 0xFE:
		return skipSubBlocks(src, i)
	default:
		return 0, errGIF
	}
}

func skipImage(src []byte, i int) (int, error) {
	if i > len(src)-9 {
		return 0, errGIF
	}
	fields := src[i+8]
	i += 9
	if fields&0x80 != 0 {
		var err error
		i, err = skipColorTable(src, i, fields)
		if err != nil {
			return 0, err
		}
	}
	if i >= len(src) {
		return 0, errGIF
	}
	i++
	return skipSubBlocks(src, i)
}

func skipSubBlocks(src []byte, i int) (int, error) {
	for {
		if i >= len(src) {
			return 0, errGIF
		}
		n := int(src[i])
		i++
		if n == 0 {
			return i, nil
		}
		if i > len(src)-n {
			return 0, errGIF
		}
		i += n
	}
}
