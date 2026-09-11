package events

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "image/jpeg"
)

// A shared link to an event is fetched by whatever chat app it lands in, with
// no session, so the preview it shows comes from two public things: Open Graph
// tags slipped into the sign-in page served at the event's address, and a card
// image at /share/{id}.png drawn here - the brand, the title, when it is, and
// the event's own image when it has one. Only what is open or done is
// previewed; a hidden or pending thing shows the plain sign-in page. The tags
// say the title, a sentence of the description and the date - the same things
// a poster on the wall says - and nothing about who has signed up.

const (
	cardWidth  = 1200
	cardHeight = 630
)

var (
	cardPage   = color.RGBA{0xee, 0xf6, 0xea, 0xff}
	cardBrand  = color.RGBA{0x0c, 0x4c, 0x54, 0xff}
	cardAccent = color.RGBA{0x00, 0x74, 0x6f, 0xff}
	cardMuted  = color.RGBA{0x4b, 0x5c, 0x5d, 0xff}
	cardYellow = color.RGBA{0xf8, 0xd9, 0x08, 0xff}
)

// previewable is what may be shown to someone who has not signed in.
func previewable(a *Activity) bool {
	return a != nil && (a.Status == StatusOpen || a.Status == StatusDone)
}

// timed is the thing whose date a preview shows: the thing itself, or when it
// has no date or timing of its own, the nearest thing above it that does - a
// booth happens when its event does.
func timed(m *Model, a *Activity) *Activity {
	for n := a; n != nil; n = m.byID[n.Parent] {
		if n.Start != "" || n.Timing != "" {
			return n
		}
	}
	return a
}

// lineage is what a thing sits under, root first - "International Night ›
// Booths" for a booth's performance slot - so a preview says what it belongs
// to. Empty for an event.
func lineage(m *Model, a *Activity) string {
	names := []string{}
	for n := m.byID[a.Parent]; n != nil; n = m.byID[n.Parent] {
		names = append([]string{n.Title}, names...)
	}
	return strings.Join(names, " › ")
}

// when is the one line under the title: the timing text when there is one -
// it stands in for the date wherever the site shows a when - else the date
// and time.
func when(a *Activity) string {
	if a.Timing != "" {
		return a.Timing
	}
	start, err := time.ParseInLocation(DateTimeFormat, a.Start, local)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, a.Start, local); err == nil {
			return day.Format("Monday, January 2")
		}
		return ""
	}
	line := start.Format("Monday, January 2 · ")
	if end, err := time.ParseInLocation(DateTimeFormat, a.End, local); err == nil && end.After(start) {
		// "4:00–6:00 PM" when both fall on the same side of noon.
		if start.Format("PM") == end.Format("PM") {
			return line + start.Format("3:04") + "–" + end.Format("3:04 PM")
		}
		return line + start.Format("3:04 PM") + "–" + end.Format("3:04 PM")
	}
	return line + start.Format("3:04 PM")
}

// blurb is the description cut to a sentence or two for the preview text.
func blurb(a *Activity) string {
	text := strings.Join(strings.Fields(a.Description), " ")
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

// PreviewHead is the Open Graph markup for the thing at a request's path, or
// nothing when there is nothing there to show a stranger. Wired into the
// sign-in page, which is what an unauthenticated fetch of the address gets.
func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		if first != "v" && first != "activities" {
			return ""
		}
		model := cache.Model()
		if model == nil {
			return ""
		}
		a := model.Resolve(r.URL.Path)
		if !previewable(a) {
			return ""
		}
		origin := "https://" + r.Host
		desc := blurb(a)
		if line := when(timed(model, a)); line != "" {
			if desc != "" {
				desc = line + " — " + desc
			} else {
				desc = line
			}
		}
		if desc == "" {
			desc = "Sign up to help on HCA-Team, the HCA Volunteer Portal."
		}
		// A thing under an event is titled with the event, so the preview says
		// what it is part of: "Poland Booth · International Night".
		title := a.Title
		if under := lineage(model, a); under != "" {
			title = a.Title + " · " + under
		}
		tags := [][2]string{
			{"og:type", "website"},
			{"og:site_name", "HCA-Team"},
			{"og:title", title},
			{"og:description", desc},
			{"og:url", origin + model.PathOf(a)},
			{"og:image", origin + "/share/" + a.ID + ".png"},
			{"og:image:width", fmt.Sprint(cardWidth)},
			{"og:image:height", fmt.Sprint(cardHeight)},
			{"twitter:card", "summary_large_image"},
			{"twitter:title", title},
			{"twitter:description", desc},
			{"twitter:image", origin + "/share/" + a.ID + ".png"},
		}
		var b strings.Builder
		b.WriteString("\n")
		for _, t := range tags {
			attr := "property"
			if strings.HasPrefix(t[0], "twitter:") {
				attr = "name"
			}
			fmt.Fprintf(&b, `<meta %s="%s" content="%s">`+"\n", attr, t[0], html.EscapeString(t[1]))
		}
		fmt.Fprintf(&b, `<meta name="description" content="%s">`+"\n", html.EscapeString(desc))
		return b.String()
	}
}

// shareCard serves /share/{id}.png: the card for one previewable thing. The
// card depends only on the title, the date line and the image, so its ETag is
// a hash of those and a chat app that fetched it once need not again.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	act := a.cache.Model().Activity(id)
	if !previewable(act) {
		http.NotFound(w, r)
		return
	}
	model := a.cache.Model()
	// A thing under an event with no image of its own shows the event's.
	picture := act.Image
	for n := act; picture == "" && n != nil; n = model.byID[n.Parent] {
		picture = n.Image
	}
	imageBytes := a.readImage(picture)
	line, under := when(timed(model, act)), lineage(model, act)
	sum := sha256.Sum256([]byte(act.Title + "\x00" + under + "\x00" + line + "\x00" + picture))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	card, err := drawCard(under, act.Title, line, r.Host+model.PathOf(act), imageBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(card)
}

// readImage is an activity's image as bytes: an upload from the blob store,
// or one of the bundled files. Nothing when there is none or it is not held.
func (a app) readImage(key string) []byte {
	if key == "" {
		return nil
	}
	if strings.HasPrefix(key, "activity-images/") {
		if a.store == nil {
			return nil
		}
		data, _, ok := a.store.Bytes(key)
		if !ok {
			return nil
		}
		return data
	}
	for _, dir := range []string{"web/hca", "web/public/hca"} {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}

// The two faces the card is set in, parsed once. Montserrat is the wordmark's
// face and the site's headline face, so the card reads as the site does.
var (
	facesOnce sync.Once
	faces     struct {
		bold, medium *opentype.Font
		err          error
	}
	markOnce sync.Once
	mark     image.Image
)

func loadFaces() (*opentype.Font, *opentype.Font, error) {
	facesOnce.Do(func() {
		load := func(name string) *opentype.Font {
			data, err := os.ReadFile("web/hca/fonts/" + name)
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

var (
	meadowOnce sync.Once
	meadow     image.Image
)

func loadMeadow() image.Image {
	meadowOnce.Do(func() {
		data, err := os.ReadFile("web/hca/toolbar_background.png")
		if err != nil {
			return
		}
		meadow, _, _ = image.Decode(bytes.NewReader(data))
	})
	return meadow
}

func loadMark() image.Image {
	markOnce.Do(func() {
		data, err := os.ReadFile("web/public/hca/brand/logo-mark.png")
		if err != nil {
			return
		}
		mark, _, _ = image.Decode(bytes.NewReader(data))
	})
	return mark
}

func face(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

// wrap breaks text into lines no wider than width, on spaces; a single word
// wider than the line stands alone and is clipped by the canvas.
func wrap(d *font.Drawer, text string, width fixed.Int26_6) []string {
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

// drawCard composes the 1200x630 image: pale page, the mark and wordmark, the
// title as large as fits in three lines with the brand's yellow swoosh under
// it, the date line, the address, and the event's image filling the right
// side when there is one.
func drawCard(under, title, line, address string, picture []byte) ([]byte, error) {
	bold, medium, err := loadFaces()
	if err != nil {
		return nil, fmt.Errorf("share card fonts: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, cardWidth, cardHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(cardPage), image.Point{}, draw.Src)

	// The text column is the whole card, or the left three fifths beside a
	// picture, which is scaled to cover its panel and cropped to it. Without a
	// picture the rail's meadow grows out of the bottom right corner instead.
	textRight := cardWidth - 72
	if pic, _, err := image.Decode(bytes.NewReader(picture)); err != nil || picture == nil {
		if meadow := loadMeadow(); meadow != nil {
			b := meadow.Bounds()
			w := 420
			h := b.Dy() * w / b.Dx()
			dst := image.Rect(cardWidth-w, cardHeight-h, cardWidth, cardHeight)
			draw.CatmullRom.Scale(img, dst, meadow, b, draw.Over, nil)
			textRight = cardWidth - w - 40
		}
	} else {
		panel := image.Rect(720, 0, cardWidth, cardHeight)
		b := pic.Bounds()
		scale := max(float64(panel.Dx())/float64(b.Dx()), float64(panel.Dy())/float64(b.Dy()))
		w, h := int(float64(b.Dx())*scale+0.5), int(float64(b.Dy())*scale+0.5)
		dst := image.Rect(0, 0, w, h).Add(image.Pt(panel.Min.X-(w-panel.Dx())/2, panel.Min.Y-(h-panel.Dy())/2))
		fitted := image.NewRGBA(dst)
		draw.CatmullRom.Scale(fitted, dst, pic, b, draw.Src, nil)
		draw.Draw(img, panel, fitted, panel.Min, draw.Src)
		textRight = 720 - 48
	}

	// Mark and wordmark, top left.
	x, y := 72, 60
	if m := loadMark(); m != nil {
		dst := image.Rect(x, y, x+64, y+64)
		draw.CatmullRom.Scale(img, dst, m, m.Bounds(), draw.Over, nil)
		x += 80
	}
	wordmark, err := face(bold, 40)
	if err != nil {
		return nil, err
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(cardBrand), Face: wordmark}
	d.Dot = fixed.P(x, y+46)
	d.DrawString("HCA-Team")
	small, err := face(medium, 18)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = small, image.NewUniform(cardAccent)
	d.Dot = fixed.P(x+2, y+70)
	d.DrawString("HCA VOLUNTEER PORTAL")

	// What it is part of, as a kicker above the title, so a booth's card
	// plainly belongs to its event.
	width := fixed.I(textRight - 72)
	top := 250
	if under != "" {
		kicker, err := face(medium, 24)
		if err != nil {
			return nil, err
		}
		d.Face, d.Src = kicker, image.NewUniform(cardAccent)
		kickerLines := wrap(d, strings.ToUpper(under), width)
		if len(kickerLines) > 1 {
			kickerLines = kickerLines[:1]
			kickerLines[0] += "…"
		}
		d.Dot = fixed.P(72, 196)
		d.DrawString(kickerLines[0])
		top = 262
	}

	// The title, shrunk until it fits three lines.
	var lines []string
	size := 72.0
	if under != "" {
		size = 64
	}
	for ; size >= 36; size -= 4 {
		titleFace, err := face(bold, size)
		if err != nil {
			return nil, err
		}
		d.Face = titleFace
		lines = wrap(d, title, width)
		if len(lines) <= 3 {
			break
		}
	}
	d.Src = image.NewUniform(cardBrand)
	lineHeight := int(size * 1.15)
	for i, l := range lines {
		d.Dot = fixed.P(72, top+i*lineHeight)
		d.DrawString(l)
	}
	// The swoosh: a yellow stroke as long as the last line, under its
	// descenders, like the one under the site's headlines.
	last := top + (len(lines)-1)*lineHeight
	lastWidth := 0
	if len(lines) > 0 {
		lastWidth = d.MeasureString(lines[len(lines)-1]).Ceil()
	}
	swoosh := image.Rect(72, last+int(size*0.32), min(textRight, 72+lastWidth), last+int(size*0.32)+int(size/7))
	draw.Draw(img, swoosh, image.NewUniform(cardYellow), image.Point{}, draw.Over)

	// When - large, it is the second thing anyone wants to know - then where
	// to find it. The date line shrinks to fit on one line if it can.
	y = last + int(size*0.32) + 76
	if line != "" {
		var dateLines []string
		dateSize := 40.0
		for ; dateSize >= 26; dateSize -= 2 {
			body, err := face(bold, dateSize)
			if err != nil {
				return nil, err
			}
			d.Face = body
			dateLines = wrap(d, line, width)
			if len(dateLines) == 1 {
				break
			}
		}
		d.Src = image.NewUniform(cardAccent)
		for _, l := range dateLines {
			d.Dot = fixed.P(72, y)
			d.DrawString(l)
			y += int(dateSize * 1.3)
		}
	}
	foot, err := face(medium, 22)
	if err != nil {
		return nil, err
	}
	d.Face, d.Src = foot, image.NewUniform(cardAccent)
	d.Dot = fixed.P(72, cardHeight-56)
	d.DrawString(address)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
