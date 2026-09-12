// Package sharecard draws the 1200x630 picture a chat app shows for a shared
// link - the one image it can fetch without signing in - for every app that
// previews its pages: the brand, a title over the brand's yellow swoosh, a
// few lines behind icons, and the thing's own picture on the right. Each app
// brings its own palette, lockup and corner art; the layout is one.
package sharecard

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "image/jpeg"
)

const (
	Width  = 1200
	Height = 630
)

// Style is one app's dress for the card: its colours, its wordmark and
// tagline, its mark, and the art that sits in the bottom-left corner.
type Style struct {
	Page, Brand, Accent, Ink, Yellow, Panel color.RGBA
	Wordmark, Tagline                       string
	// Mark and Corner are file paths, read once; either may be blank.
	Mark, Corner string
	marks        sync.Once
	mark, corner image.Image
}

// Line is one line under the title: an icon and the words.
type Line struct {
	Icon string // "calendar", "clock", "pin"
	Text string
}

// Card is what to draw: a kicker above the title (what the thing sits under,
// or blank), the title, the lines, and the picture - shown whole, as a flyer
// is, or scaled to cover the panel, as a banner is.
type Card struct {
	Kicker  string
	Title   string
	Lines   []Line
	Picture []byte
	Whole   bool
}

// The two faces the card is set in, parsed once. Montserrat is the sites'
// headline face, so the card reads as the pages do.
var (
	facesOnce sync.Once
	faces     struct {
		bold, medium *opentype.Font
		err          error
	}
)

func loadFaces() (*opentype.Font, *opentype.Font, error) {
	facesOnce.Do(func() {
		load := func(name string) *opentype.Font {
			data, err := os.ReadFile("web/common/fonts/" + name)
			if err != nil {
				faces.err = err
				return nil
			}
			f, err := opentype.Parse(data)
			if err != nil {
				faces.err = err
			}
			return f
		}
		faces.bold = load("Montserrat-ExtraBold.ttf")
		faces.medium = load("Montserrat-Medium.ttf")
	})
	return faces.bold, faces.medium, faces.err
}

func readImage(path string) image.Image {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	img, _, _ := image.Decode(bytes.NewReader(data))
	return img
}

func (s *Style) art() (image.Image, image.Image) {
	s.marks.Do(func() {
		s.mark, s.corner = readImage(s.Mark), readImage(s.Corner)
	})
	return s.mark, s.corner
}

func face(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

// Wrap breaks text into lines no wider than width, on spaces; a single word
// wider than the line stands alone and is clipped by the canvas.
func Wrap(d *font.Drawer, text string, width fixed.Int26_6) []string {
	lines := []string{}
	line := ""
	for _, word := range strings.Fields(text) {
		try := word
		if line != "" {
			try = line + " " + word
		}
		if d.MeasureString(try) > width && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		line = try
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// Draw lays the card out the way the sites' own heroes do: on the left, on
// the pale wash, the lockup, the kicker, the title with its yellow swoosh,
// then the lines each behind its icon, with the corner art growing out of
// the bottom-left; on the right, the picture. A flyer is shown whole, the
// panel taking the flyer's own proportions so it fills edge to edge; a
// banner is scaled to cover a fixed panel and cropped to it.
func (s *Style) Draw(c Card) ([]byte, error) {
	bold, medium, err := loadFaces()
	if err != nil {
		return nil, fmt.Errorf("share card fonts: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(s.Page), image.Point{}, draw.Src)

	// The picture's panel on the right, and where the text column ends.
	textRight := Width - 72
	if pic, _, err := image.Decode(bytes.NewReader(c.Picture)); err == nil && c.Picture != nil {
		b := pic.Bounds()
		panelW := 480
		if c.Whole {
			// The flyer's own proportions, within reason, so it fills the panel.
			panelW = min(max(Height*b.Dx()/max(b.Dy(), 1), 380), 540)
		}
		panel := image.Rect(Width-panelW, 0, Width, Height)
		if c.Whole {
			draw.Draw(img, panel, image.NewUniform(s.Panel), image.Point{}, draw.Src)
			scale := min(float64(panel.Dx())/float64(b.Dx()), float64(panel.Dy())/float64(b.Dy()))
			w, h := int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)
			dst := image.Rect(0, 0, w, h).Add(image.Pt(panel.Min.X+(panel.Dx()-w)/2, panel.Min.Y+(panel.Dy()-h)/2))
			draw.CatmullRom.Scale(img, dst, pic, b, draw.Over, nil)
		} else {
			scale := max(float64(panel.Dx())/float64(b.Dx()), float64(panel.Dy())/float64(b.Dy()))
			w, h := int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)
			dst := image.Rect(0, 0, w, h).Add(image.Pt(panel.Min.X-(w-panel.Dx())/2, panel.Min.Y-(h-panel.Dy())/2))
			fitted := image.NewRGBA(dst)
			draw.CatmullRom.Scale(fitted, dst, pic, b, draw.Src, nil)
			draw.Draw(img, panel, fitted, panel.Min, draw.Src)
		}
		textRight = panel.Min.X - 48
	}

	mark, corner := s.art()
	// The corner art, whole and small - a quarter of the card's height, clear
	// of the text above - so nothing in it is cropped or faded.
	if corner != nil {
		b := corner.Bounds()
		h := Height / 4
		w := b.Dx() * h / max(b.Dy(), 1)
		dst := image.Rect(0, Height-h, w, Height)
		draw.CatmullRom.Scale(img, dst, corner, b, draw.Over, nil)
	}

	// Mark and wordmark, top left.
	x, y := 72, 56
	if mark != nil {
		dst := image.Rect(x, y, x+60, y+60)
		draw.CatmullRom.Scale(img, dst, mark, mark.Bounds(), draw.Over, nil)
		x += 74
	}
	wordmark, err := face(bold, 36)
	if err != nil {
		return nil, err
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(s.Brand), Face: wordmark}
	d.Dot = fixed.P(x, y+40)
	d.DrawString(s.Wordmark)
	small, err := face(medium, 16)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = small, image.NewUniform(s.Accent)
	d.Dot = fixed.P(x+2, y+62)
	d.DrawString(s.Tagline)

	// The kicker above the title, so a thing under another plainly belongs
	// to it.
	width := fixed.I(textRight - 72)
	top := 226
	if c.Kicker != "" {
		kicker, err := face(medium, 22)
		if err != nil {
			return nil, err
		}
		d.Face, d.Src = kicker, image.NewUniform(s.Accent)
		kickerLines := Wrap(d, strings.ToUpper(c.Kicker), width)
		if len(kickerLines) > 1 {
			kickerLines = kickerLines[:1]
			kickerLines[0] += "…"
		}
		d.Dot = fixed.P(72, 176)
		d.DrawString(kickerLines[0])
		top = 238
	}

	// The title, shrunk until it fits three lines, in the deep ink the pages'
	// own headlines use.
	var lines []string
	size := 66.0
	if c.Kicker != "" {
		size = 58
	}
	for ; size >= 34; size -= 4 {
		titleFace, err := face(bold, size)
		if err != nil {
			return nil, err
		}
		d.Face = titleFace
		lines = Wrap(d, c.Title, width)
		if len(lines) <= 3 {
			break
		}
	}
	d.Src = image.NewUniform(s.Ink)
	lineHeight := int(size * 1.12)
	for i, l := range lines {
		d.Dot = fixed.P(72, top+i*lineHeight)
		d.DrawString(l)
	}
	// The swoosh: a yellow stroke as wide as the text column, under the last
	// line's descenders, like the one under the sites' headlines.
	last := top + (len(lines)-1)*lineHeight
	swooshY := last + int(size*0.34)
	draw.Draw(img, image.Rect(72, swooshY, textRight, swooshY+6), image.NewUniform(s.Yellow), image.Point{}, draw.Over)

	// The lines, each behind its icon - the day behind a calendar, the time
	// behind a clock, the place behind a pin. Each shrinks to fit if it must.
	y = swooshY + 58
	for _, line := range c.Lines {
		if line.Text == "" {
			continue
		}
		var lineFace font.Face
		lineSize := 34.0
		for ; lineSize >= 22; lineSize -= 2 {
			if lineFace, err = face(bold, lineSize); err != nil {
				return nil, err
			}
			d.Face = lineFace
			if len(Wrap(d, line.Text, fixed.I(textRight-136))) == 1 {
				break
			}
		}
		icon := image.Rect(72, y-int(lineSize*0.78), 72+int(lineSize*0.95), y-int(lineSize*0.78)+int(lineSize*0.95))
		switch line.Icon {
		case "clock":
			DrawClockIcon(img, icon, s.Ink)
		case "pin":
			DrawPinIcon(img, icon, s.Ink)
		default:
			DrawCalendarIcon(img, icon, s.Ink)
		}
		d.Src = image.NewUniform(s.Ink)
		d.Dot = fixed.P(72+int(lineSize*1.45), y)
		d.DrawString(line.Text)
		y += int(lineSize * 1.55)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
