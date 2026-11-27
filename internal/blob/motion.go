package blob

import (
	"bytes"
	"fmt"
	"image"
	"image/color/palette"
	"image/gif"

	"golang.org/x/image/draw"
)

const motionSide = 1280

func Animated(src []byte) bool {
	return bytes.HasPrefix(src, []byte("GIF8")) && bytes.Count(src, []byte{0x21, 0xf9, 0x04}) > 1
}

func ScaledGIF(src []byte) ([]byte, error) {
	if err := Check(src); err != nil {
		return nil, err
	}
	g, err := gif.DecodeAll(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		w, h = g.Image[0].Bounds().Dx(), g.Image[0].Bounds().Dy()
	}
	if w*h*len(g.Image) > 4*maxPixels {
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
