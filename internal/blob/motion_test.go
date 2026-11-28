package blob

import (
	"bytes"
	"fmt"
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

func TestGIFScreenMatchesTheDecoder(t *testing.T) {
	src := animatedGIF(t, 8, 8, 4)
	if err := screenMatchesDecoder(src); err != nil {
		t.Fatal(err)
	}
	pal := color.Palette{color.Black, color.White}
	shared := &gif.GIF{Config: image.Config{ColorModel: pal, Width: 4, Height: 4}}
	for range 2 {
		f := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
		f.Set(1, 1, color.White)
		shared.Image = append(shared.Image, f)
		shared.Delay = append(shared.Delay, 5)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, shared); err != nil {
		t.Fatal(err)
	}
	if err := screenMatchesDecoder(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, block := range [][]byte{
		{0x21, 0xFE, 0x03, 'a', 'b', 'c', 0x00},
		append(append([]byte{0x21, 0x01}, bytes.Repeat([]byte{0}, 13)...), 0x00),
		append(append([]byte{0x21, 0xFF, 0x0B}, []byte("ANIMEXTS1.0")...), 0x00),
	} {
		with := append(append([]byte{}, src[:len(src)-1]...), block...)
		with = append(with, 0x3B)
		if err := screenMatchesDecoder(with); err != nil {
			t.Fatal(err)
		}
	}
}

func screenMatchesDecoder(src []byte) error {
	g, err := gif.DecodeAll(bytes.NewReader(src))
	if err != nil {
		return err
	}
	w, h, frames, err := gifScreen(src)
	if err != nil {
		return err
	}
	if w != g.Config.Width || h != g.Config.Height || frames != len(g.Image) {
		return fmt.Errorf("screen %dx%d over %d frames, decoder %dx%d over %d", w, h, frames, g.Config.Width, g.Config.Height, len(g.Image))
	}
	return nil
}

func TestAnOverBudgetGIFIsRefusedBeforeDecode(t *testing.T) {
	const frames = 5
	w, h := 6000, 6000
	var b []byte
	b = append(b, 'G', 'I', 'F', '8', '9', 'a')
	b = append(b, byte(w), byte(w>>8), byte(h), byte(h>>8), 0x80, 0x00, 0x00)
	b = append(b, 0, 0, 0, 255, 255, 255)
	for range frames {
		b = append(b, 0x21, 0xF9, 0x04, 0x00, 0x0A, 0x00, 0x00, 0x00)
		b = append(b, 0x2C, 0, 0, 0, 0, byte(w), byte(w>>8), byte(h), byte(h>>8), 0x00)
		b = append(b, 0x02, 0x01, 0x00, 0x00)
	}
	b = append(b, 0x3B)
	if err := Check(b); err != nil {
		t.Fatal(err)
	}
	_, err := ScaledGIF(b)
	want := "the animation is 6000x6000 pixels over 5 frames, too much to show"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestManyTinyFramesStillScale(t *testing.T) {
	const frames = 1000
	src := animatedGIF(t, 1, 1, frames)
	out, err := ScaledGIF(src)
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != frames || g.Config.Width != 1 || g.Config.Height != 1 {
		t.Fatalf("came back as %d frames of %dx%d", len(g.Image), g.Config.Width, g.Config.Height)
	}
}

func TestTooManyTinyFramesAreRefusedBeforeDecode(t *testing.T) {
	frames := int(int64(4*maxPixels)/(1+frameHeader)) + 1
	if frames > 4*maxPixels {
		t.Fatalf("%d frames still fits the pixel-only budget", frames)
	}
	frame := []byte{
		0x21, 0xF9, 0x04, 0x00, 0x0A, 0x00, 0x00, 0x00,
		0x2C, 0, 0, 0, 0, 1, 0, 1, 0, 0x00,
		0x02, 0x01, 0x00, 0x00,
	}
	b := make([]byte, 0, 13+len(frame)*frames+1)
	b = append(b, 'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0x80, 0x00, 0x00, 0, 0, 0, 255, 255, 255)
	for range frames {
		b = append(b, frame...)
	}
	b = append(b, 0x3B)
	if err := Check(b); err != nil {
		t.Fatal(err)
	}
	_, err := ScaledGIF(b)
	want := fmt.Sprintf("the animation is 1x1 pixels over %d frames, too much to show", frames)
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestATransparentGIFStillScales(t *testing.T) {
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.RGBA{R: uint8(i), A: 255}
	}
	pal[0] = color.RGBA{}
	const frames = 4
	g := &gif.GIF{Config: image.Config{ColorModel: pal, Width: 8, Height: 8}}
	for range frames {
		f := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
		f.Set(1, 1, pal[1])
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 5)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	src := buf.Bytes()
	if !bytes.Contains(src, []byte{0x21, 0xF9, 0x04, 0x01}) {
		t.Fatal("encoder did not mark a transparent frame")
	}
	out, err := ScaledGIF(src)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image) != frames || decoded.Config.Width != 8 || decoded.Config.Height != 8 {
		t.Fatalf("came back as %d frames of %dx%d", len(decoded.Image), decoded.Config.Width, decoded.Config.Height)
	}
}

func TestTooManyTransparentFramesAreRefusedBeforeDecode(t *testing.T) {
	const oldHeader = 256
	frames := int(int64(4*maxPixels) / (1 + oldHeader))
	if int64(frames) <= int64(4*maxPixels)/(1+frameHeader) {
		t.Fatalf("%d transparent frames still fit the budget", frames)
	}
	frame := []byte{
		0x21, 0xF9, 0x04, 0x01, 0x0A, 0x00, 0x00, 0x00,
		0x2C, 0, 0, 0, 0, 1, 0, 1, 0, 0x00,
		0x02, 0x01, 0x00, 0x00,
	}
	b := make([]byte, 0, 13+256*3+len(frame)*frames+1)
	b = append(b, 'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0xF7, 0x00, 0x00)
	for i := range 256 {
		b = append(b, byte(i), 0, 0)
	}
	for range frames {
		b = append(b, frame...)
	}
	b = append(b, 0x3B)
	if err := Check(b); err != nil {
		t.Fatal(err)
	}
	_, err := ScaledGIF(b)
	want := fmt.Sprintf("the animation is 1x1 pixels over %d frames, too much to show", frames)
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}
