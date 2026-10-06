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

	"heliosian/internal/blob"
)

const (
	Width  = 1200
	Height = 630
)

type Palette struct {
	Page, Brand, Accent, Ink, Yellow, Panel color.RGBA
}

var Standard = Palette{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
}

type Style struct {
	Palette
	Name, Tagline        func() string
	Wordmark             string
	Mark, Lockup, Corner string
	marks                sync.Once
	mark, lockup, corner image.Image
}

type Line struct {
	Icon string
	Text string
}

type Card struct {
	Kicker   string
	Title    string
	Subtitle string
	Lines    []Line
	Button   string
	Picture  []byte
	Whole    bool
	Listing  *Listing
}

type Listing struct {
	Heading string
	Items   []Item
	Empty   string
}

type Item struct {
	Title, Note string
	Icon        image.Image
}

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
		s.mark, s.lockup, s.corner = readImage(s.Mark), readImage(s.Lockup), readImage(s.Corner)
	})
	return s.mark, s.corner
}

func face(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

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

func (s *Style) Draw(c Card) ([]byte, error) {
	bold, medium, err := loadFaces()
	if err != nil {
		return nil, fmt.Errorf("share card fonts: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(s.Page), image.Point{}, draw.Src)

	textRight := Width - 72
	if c.Listing != nil {
		panel := image.Rect(Width/2, 0, Width, Height)
		if err := s.drawListing(img, panel, c.Listing, bold, medium); err != nil {
			return nil, err
		}
		textRight = panel.Min.X - 48
	} else if pic, err := blob.Decode(c.Picture); err == nil && c.Picture != nil {
		b := pic.Bounds()
		panelW := 480
		if c.Whole {
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
	if corner != nil {
		b := corner.Bounds()
		h := Height / 4
		w := b.Dx() * h / max(b.Dy(), 1)
		dst := image.Rect(0, Height-h, w, Height)
		draw.CatmullRom.Scale(img, dst, corner, b, draw.Over, nil)
	}

	x, y := 72, 56
	d := &font.Drawer{Dst: img, Src: image.NewUniform(s.Brand)}
	if s.lockup != nil {
		b := s.lockup.Bounds()
		h := 84
		w := b.Dx() * h / max(b.Dy(), 1)
		draw.CatmullRom.Scale(img, image.Rect(x, y, x+w, y+h), s.lockup, b, draw.Over, nil)
	} else {
		if mark != nil {
			dst := image.Rect(x, y, x+60, y+60)
			draw.CatmullRom.Scale(img, dst, mark, mark.Bounds(), draw.Over, nil)
			x += 74
		}
		wordmark, err := face(bold, 36)
		if err != nil {
			return nil, err
		}
		d.Face = wordmark
		d.Dot = fixed.P(x, y+40)
		d.DrawString(s.Wordmark)
		small, err := face(medium, 16)
		if err != nil {
			return nil, err
		}
		d.Face, d.Src = small, image.NewUniform(s.Accent)
		d.Dot = fixed.P(x+2, y+62)
		d.DrawString(strings.ToUpper(s.Tagline()))
	}

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
	last := top + (len(lines)-1)*lineHeight
	swooshY := last + int(size*0.34)
	draw.Draw(img, image.Rect(72, swooshY, textRight, swooshY+6), image.NewUniform(s.Yellow), image.Point{}, draw.Over)

	y = swooshY + 58
	if c.Subtitle != "" {
		var subLines []string
		subSize := 26.0
		for ; subSize >= 18; subSize -= 2 {
			subFace, err := face(medium, subSize)
			if err != nil {
				return nil, err
			}
			d.Face = subFace
			subLines = Wrap(d, c.Subtitle, width)
			if len(subLines) <= 2 {
				break
			}
		}
		if len(subLines) > 2 {
			subLines = subLines[:2]
			subLines[1] += "…"
		}
		d.Src = image.NewUniform(s.Accent)
		subY := swooshY + 20 + int(subSize)
		for i, l := range subLines {
			d.Dot = fixed.P(72, subY+i*int(subSize*1.25))
			d.DrawString(l)
		}
		y = subY + (len(subLines)-1)*int(subSize*1.25) + 52
	}

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
		case "dot":
			size := icon.Dx() * 2 / 5
			at := icon.Min.Add(image.Pt((icon.Dx()-size)/2, (icon.Dy()-size)/2))
			DrawDot(img, image.Rect(at.X, at.Y, at.X+size, at.Y+size), s.Accent)
		default:
			DrawCalendarIcon(img, icon, s.Ink)
		}
		d.Src = image.NewUniform(s.Ink)
		d.Dot = fixed.P(72+int(lineSize*1.45), y)
		d.DrawString(line.Text)
		y += int(lineSize * 1.55)
	}

	if c.Button != "" && y+66 <= Height-14 {
		label, err := face(bold, 26)
		if err != nil {
			return nil, err
		}
		d.Face = label
		textW := d.MeasureString(c.Button).Ceil()
		h := 62
		w := 40 + 30 + 14 + textW + 40
		at := image.Rect(72, y+4, 72+w, y+4+h)
		DrawPill(img, at, s.Brand)
		check := image.Rect(at.Min.X+40, at.Min.Y+(h-30)/2, at.Min.X+70, at.Min.Y+(h-30)/2+30)
		DrawCheckIcon(img, check, color.RGBA{255, 255, 255, 255})
		d.Src = image.NewUniform(color.RGBA{255, 255, 255, 255})
		d.Dot = fixed.P(at.Min.X+40+30+14, at.Min.Y+h/2+9)
		d.DrawString(c.Button)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *Style) drawListing(img *image.RGBA, panel image.Rectangle, l *Listing, bold, medium *opentype.Font) error {
	draw.Draw(img, panel, image.NewUniform(s.Panel), image.Point{}, draw.Src)
	left, right := panel.Min.X+56, panel.Max.X-56
	width := fixed.I(right - left)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(s.Brand)}

	y := 100
	if l.Heading != "" {
		heading, err := face(bold, 40)
		if err != nil {
			return err
		}
		d.Face = heading
		headY := 128
		d.Dot = fixed.P(left, headY)
		d.DrawString(l.Heading)
		swooshY := headY + 14
		draw.Draw(img, image.Rect(left, swooshY, right, swooshY+6), image.NewUniform(s.Yellow), image.Point{}, draw.Over)
		y = swooshY + 84
	}
	if len(l.Items) == 0 && l.Empty != "" {
		empty, err := face(medium, 26)
		if err != nil {
			return err
		}
		d.Face, d.Src = empty, image.NewUniform(s.Accent)
		for i, line := range Wrap(d, l.Empty, width) {
			d.Dot = fixed.P(left, y+i*34)
			d.DrawString(line)
		}
		return nil
	}
	step := 116
	if n := len(l.Items); n > 0 {
		step = min(step, max((panel.Max.Y-40-y)/n, 72))
	}
	titleSize, noteSize := 30.0, 24.0
	if step < 100 {
		titleSize, noteSize = 26, 20
	}
	dot := 10
	indentOf := func(item Item) int {
		if item.Icon != nil {
			return 62
		}
		return dot * 3
	}
	size := titleSize
	for _, item := range l.Items {
		for ; size > 20; size -= 2 {
			title, err := face(bold, size)
			if err != nil {
				return err
			}
			d.Face = title
			if len(Wrap(d, item.Title, width-fixed.I(indentOf(item)))) <= 1 {
				break
			}
		}
	}
	title, err := face(bold, size)
	if err != nil {
		return err
	}
	for _, item := range l.Items {
		indent := indentOf(item)
		d.Face = title
		lines := Wrap(d, item.Title, width-fixed.I(indent))
		text := item.Title
		if len(lines) > 1 {
			text = lines[0] + "…"
		}
		mid := y - int(size*0.36)
		if item.Icon != nil {
			if item.Note != "" {
				mid = y + int(noteSize*0.55) - int(size*0.36)
			}
			at := image.Rect(left, mid-24, left+48, mid+24)
			draw.CatmullRom.Scale(img, at, item.Icon, item.Icon.Bounds(), draw.Over, nil)
		} else {
			DrawDot(img, image.Rect(left, mid-dot, left+dot*2, mid+dot), s.Accent)
		}
		d.Src = image.NewUniform(s.Ink)
		d.Dot = fixed.P(left+indent, y)
		d.DrawString(text)
		if item.Note != "" {
			note, err := face(medium, noteSize)
			if err != nil {
				return err
			}
			d.Face, d.Src = note, image.NewUniform(s.Accent)
			d.Dot = fixed.P(left+indent, y+int(noteSize*1.4))
			d.DrawString(item.Note)
		}
		y += step
	}
	return nil
}
