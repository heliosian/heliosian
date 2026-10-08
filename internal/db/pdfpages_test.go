package db

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func pdfOfPages(t *testing.T, n int) []byte {
	t.Helper()
	imgs := []io.Reader{}
	for i := range n {
		img := image.NewRGBA(image.Rect(0, 0, 40, 30))
		img.Set(0, 0, color.RGBA{R: uint8(i), A: 255})
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			t.Fatal(err)
		}
		imgs = append(imgs, buf)
	}
	out := &bytes.Buffer{}
	if err := api.ImportImages(t.Context(), nil, out, imgs, nil, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func spans(t *testing.T, raw []byte) []string {
	t.Helper()
	p, err := readPDFPages(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	ranges, err := p.ranges(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, child := range pageChildren(p, ranges) {
		pages, err := readPDFPages(t.Context(), child.body)
		if err != nil {
			t.Fatalf("%s: %v", child.title, err)
		}
		if child.relation != "pages" || child.mime != pdfType {
			t.Fatalf("child %+v", child)
		}
		out = append(out, child.title+" has "+spanName(1, pages.total, pages.total))
	}
	return out
}

func TestALongPDFSplitsIntoRunsOfPages(t *testing.T) {
	equalLines(t, "runs", spans(t, pdfOfPages(t, 25)), []string{
		"pages 1–20 of 25 has pages 1–20 of 20",
		"pages 21–25 of 25 has pages 1–5 of 5",
	})
}
