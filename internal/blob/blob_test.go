package blob

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"
)

func headerPNG(w, h int) []byte {
	ihdr := []byte("IHDR")
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(w))
	ihdr = binary.BigEndian.AppendUint32(ihdr, uint32(h))
	ihdr = append(ihdr, 8, 0, 0, 0, 0)
	out := []byte("\x89PNG\r\n\x1a\n")
	out = binary.BigEndian.AppendUint32(out, 13)
	out = append(out, ihdr...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(ihdr))
}

func TestDecodeRefusesOversizeHeader(t *testing.T) {
	bomb := headerPNG(50000, 50000)
	if len(bomb) > 64 {
		t.Fatalf("the crafted file is %d bytes, which is not the point", len(bomb))
	}
	if _, err := Decode(bomb); err == nil || !strings.Contains(err.Error(), "over the limit") {
		t.Errorf("a 50000x50000 header: %v, want the pixel limit", err)
	}
	if _, err := Thumbnail(bomb); err == nil {
		t.Error("Thumbnail decoded a 50000x50000 header")
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Errorf("an 8x8 png was refused: %v", err)
	}
	if _, err := Thumbnail(buf.Bytes()); err != nil {
		t.Errorf("Thumbnail refused an 8x8 png: %v", err)
	}
}

func TestReencodeShrinksAndCropCuts(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4096, 1024))); err != nil {
		t.Fatal(err)
	}
	re, err := Reencode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(re))
	if err != nil || format != "jpeg" || img.Bounds().Dx() != 2048 || img.Bounds().Dy() != 512 {
		t.Fatalf("re-encode is %s %v: %v", format, img.Bounds(), err)
	}
	cropped, err := Crop(re, image.Rect(100, 50, 400, 250))
	if err != nil {
		t.Fatal(err)
	}
	if img, _, err := image.Decode(bytes.NewReader(cropped)); err != nil || img.Bounds().Dx() != 300 || img.Bounds().Dy() != 200 {
		t.Fatalf("crop is %v: %v", img.Bounds(), err)
	}
	if _, err := Crop(re, image.Rect(2000, 0, 2100, 100)); err == nil {
		t.Fatal("a crop past the edge was cut")
	}
}
