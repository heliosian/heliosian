package blob

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"testing"
)

func animatedGIF(t *testing.T, w, h, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	for i := range frames {
		f := image.NewPaletted(image.Rect(0, 0, w, h), palette.Plan9)
		f.Set(i, i, color.White)
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnAnimatedGIFIsScaledAndKeepsMoving(t *testing.T) {
	src := animatedGIF(t, 2560, 1280, 3)
	if !Animated(src) || Animated(animatedGIF(t, 40, 40, 1)) {
		t.Fatal("animated GIFs aren't told from still ones")
	}
	out, err := ScaledGIF(src)
	if err != nil {
		t.Fatalf("scaling a GIF: %v", err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 3 || g.Config.Width != motionSide || g.Config.Height != motionSide/2 {
		t.Errorf("came back as %d frames of %dx%d", len(g.Image), g.Config.Width, g.Config.Height)
	}
}
