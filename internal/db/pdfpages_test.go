package db

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"heliosian/internal/store"
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

func spans(t *testing.T, doc store.Row, raw []byte, most int) []string {
	t.Helper()
	p, err := readPDFPages(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	ranges, err := p.ranges(t.Context(), most)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, child := range pageChildren(doc, p, ranges) {
		pages, err := readPDFPages(t.Context(), child.body)
		if err != nil {
			t.Fatalf("%s: %v", child.title, err)
		}
		first, total := pageSpan(child.title)
		if child.relation != "pages" || child.mime != pdfType || total == 0 || first < 1 {
			t.Fatalf("child %+v", child)
		}
		out = append(out, child.title+" has "+spanName(1, pages.total, pages.total))
	}
	return out
}

func TestALongPDFSplitsIntoRunsOfPages(t *testing.T) {
	raw := pdfOfPages(t, 25)
	equalLines(t, "runs", spans(t, store.Row{"relation": "part"}, raw, pdfRangePages), []string{
		"pages 1–20 of 25 has pages 1–20 of 20",
		"pages 21–25 of 25 has pages 1–5 of 5",
	})
	equalLines(t, "halves", spans(t, store.Row{"relation": "part"}, raw, 13), []string{
		"pages 1–13 of 25 has pages 1–13 of 13",
		"pages 14–25 of 25 has pages 1–12 of 12",
	})
}

func TestARunSplitAgainKeepsTheWholeDocumentsPageNumbers(t *testing.T) {
	raw := pdfOfPages(t, 5)
	equalLines(t, "runs", spans(t, store.Row{"relation": "pages", "name": "pages 21–25 of 25"}, raw, 3), []string{
		"pages 21–23 of 25 has pages 1–3 of 3",
		"pages 24–25 of 25 has pages 1–2 of 2",
	})
	equalLines(t, "single pages", spans(t, store.Row{"relation": "pages", "name": "pages 24–25 of 25"}, pdfOfPages(t, 2), 1), []string{
		"page 24 of 25 has page 1 of 1",
		"page 25 of 25 has page 1 of 1",
	})
}
