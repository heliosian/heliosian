package db

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"

	"heliosian/internal/claude"
	"heliosian/internal/store"
)

const (
	imageSmallest  = 100
	imageLargest   = 8000
	imageMostBytes = 3_750_000
	imageMaxReply  = 8000
	pdfMaxReply    = 128000
	decoration     = "(decoration)"
)

const pdfSystem = `You are given one PDF a school shared with its families - a flyer, a letter, a schedule, a form, a handbook, a slide deck. A slide deck's slides each become a section headed by the slide's title. Write out everything it says as markdown, so a reader who cannot see it loses nothing. Keep its structure: headings stay headings, lists stay lists, a schedule or grid becomes a markdown table. Transcribe; do not summarise, explain or add anything.

Transcribe only text you can read with certainty, letter by letter. Where something cannot be read - a blurred scan, a table too small to make out - leave it out and write one line in its place saying what it is, e.g. "(a small schedule table, too small to read)". Never reconstruct it from context. Leave out logos, wordmarks and decoration, even when they have words in them; words that only brand the document carry no information.

` + peopleRule

const peopleRule = `Photos of people are never described. Do not say who is in a photo, what anyone looks like, wears or is doing, and never describe a child. A photo gets at most a short generic caption in brackets saying what kind of photo it is, e.g. "(class photo)" or "(photo of a field trip)". Words in the photo - a sign, a banner, a caption - are still written out.`

var claudeImageTypes = []string{"image/jpeg", "image/png", "image/gif", "image/webp"}

const imageSystem = `You are given one image from an email a school sent to its families. Write out everything the image says as markdown, so a reader who cannot see the image loses nothing. Keep its structure: a schedule or grid becomes a markdown table, a list stays a list, headings stay headings. Transcribe; do not summarise, explain or add anything.

Transcribe only text you can read with certainty, letter by letter. Images often hold a small inset - a thumbnail of a table, a screenshot shrunk into a corner - whose text is too small to read. Never transcribe such text, and never reconstruct it from context, from nearby text, or from what it probably says: a table you cannot read cell by cell is left out entirely. Where you leave something out, write one line in its place saying what it is, e.g. "(a small schedule table, too small to read)". If you are unsure whether you can read something, you cannot.

If the image is only a logo, wordmark, banner or decoration, answer with exactly: (decoration) - even when it has words in it, such as the school's name or a program's name. Words that only brand the email carry no information.

` + peopleRule

func (x *Extractor) readBefore(ctx context.Context, m *Model, id string, content store.Row) ([]extracted, bool, error) {
	docs := m.Table("DOCUMENT")
	for _, other := range docs.Referencing("content", content["id"]) {
		if other["id"] == id || other["extracted"] == "" {
			continue
		}
		for _, child := range docs.Referencing("parent", other["id"]) {
			if child["relation"] != "extract" {
				continue
			}
			c, _ := m.Table("CONTENT").Get(child["content"])
			text, _, err := x.bucket.Get(ctx, c["blob"])
			if err != nil {
				return nil, true, err
			}
			return []extracted{{relation: "extract", body: text, mime: "text/markdown"}}, true, nil
		}
		return nil, true, nil
	}
	return nil, false, nil
}

func transcribed(text string, err error) ([]extracted, error) {
	var refused *anthropic.Error
	switch {
	case errors.Is(err, claude.ErrFinal):
		return nil, err
	case errors.As(err, &refused) && refused.StatusCode >= 400 && refused.StatusCode < 500 && refused.StatusCode != http.StatusTooManyRequests:
		return nil, err
	case err != nil:
		return nil, fmt.Errorf("%w: %w", errAskAgain, err)
	}
	text = strings.TrimSpace(text)
	if text == decoration {
		return nil, nil
	}
	return markdownExtract(text), nil
}

func (x *Extractor) readImage(ctx context.Context, m *Model, id string, content store.Row, raw []byte) ([]extracted, error) {
	if before, read, err := x.readBefore(ctx, m, id, content); read || err != nil {
		return before, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		slog.Warn("extract: unreadable image", "document", id, "mime", content["mime"], "error", err)
		return nil, nil
	}
	if config.Width < imageSmallest && config.Height < imageSmallest {
		return nil, nil
	}
	body, mimeType, err := claudeImage(raw, baseType(content["mime"]), config)
	if err != nil {
		slog.Warn("extract: unreadable image", "document", id, "mime", content["mime"], "error", err)
		return nil, nil
	}
	return transcribed(claude.Text(ctx, x.client, anthropic.MessageNewParams{
		Model:        claude.ExtractImageModel,
		MaxTokens:    imageMaxReply,
		System:       []anthropic.TextBlockParam{{Text: imageSystem}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewImageBlockBase64(mimeType, base64.StdEncoding.EncodeToString(body)))},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.ExtractImageEffort},
	}))
}

func (x *Extractor) readPDF(ctx context.Context, m *Model, doc, content store.Row, raw []byte) ([]extracted, error) {
	if before, read, err := x.readBefore(ctx, m, doc["id"], content); read || err != nil {
		return before, err
	}
	pages, err := readPDFPages(ctx, raw)
	if err != nil {
		return nil, err
	}
	if len(raw) > pdfRangeBytes || pages.total > pdfRangePages {
		if pages.total == 1 {
			return nil, fmt.Errorf("a one-page pdf of %d bytes is larger than claude reads, %d", len(raw), pdfRangeBytes)
		}
		return x.splitPDF(ctx, doc, pages, pdfRangePages)
	}
	ask := "Transcribe this document."
	if doc["relation"] == "pages" {
		ask = "This PDF is " + doc["name"] + " of " + pdfTitle(m, doc) + ". Transcribe these pages."
	}
	text, err := claude.Text(ctx, x.client, anthropic.MessageNewParams{
		Model:     claude.ExtractPDFModel,
		MaxTokens: pdfMaxReply,
		System:    []anthropic.TextBlockParam{{Text: pdfSystem}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(
			anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(raw)}),
			anthropic.NewTextBlock(ask),
		)},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.ExtractPDFEffort},
	})
	if errors.Is(err, claude.ErrCutShort) && pages.total > 1 {
		slog.Info("extract: pdf answer cut short, reading it in halves", "document", doc["id"], "pages", pages.total)
		return x.splitPDF(ctx, doc, pages, (pages.total+1)/2)
	}
	return transcribed(text, err)
}

func (x *Extractor) splitPDF(ctx context.Context, doc store.Row, pages pdfPages, most int) ([]extracted, error) {
	ranges, err := pages.ranges(ctx, most)
	if err != nil {
		return nil, err
	}
	return pageChildren(doc, pages, ranges), nil
}

func pdfTitle(m *Model, doc store.Row) string {
	docs := m.Table("DOCUMENT")
	for doc["relation"] == "pages" {
		parent, ok := docs.Get(doc["parent"])
		if !ok {
			break
		}
		doc = parent
	}
	if doc["name"] != "" {
		return "the document titled " + strconv.Quote(doc["name"])
	}
	if doc["filename"] != "" {
		return "the file " + strconv.Quote(doc["filename"])
	}
	return "a longer document"
}

func claudeImage(raw []byte, mimeType string, config image.Config) ([]byte, string, error) {
	if slices.Contains(claudeImageTypes, mimeType) && max(config.Width, config.Height) <= imageLargest && len(raw) <= imageMostBytes {
		return raw, mimeType, nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	for {
		if longest := max(img.Bounds().Dx(), img.Bounds().Dy()); longest > imageLargest {
			img = scaled(img, float64(imageLargest)/float64(longest))
		}
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			return nil, "", err
		}
		if buf.Len() <= imageMostBytes {
			return buf.Bytes(), "image/png", nil
		}
		buf.Reset()
		if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		if buf.Len() <= imageMostBytes {
			return buf.Bytes(), "image/jpeg", nil
		}
		img = scaled(img, 0.75)
	}
}

func scaled(img image.Image, by float64) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*by)), max(1, int(float64(b.Dy())*by))))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
