package db

import (
	"cmp"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var hexColor = regexp.MustCompile(`#([0-9a-fA-F]{6})\b`)

func mergedDesign(base, over map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		o, isMap := v.(map[string]any)
		b, baseMap := base[k].(map[string]any)
		if isMap && baseMap {
			out[k] = mergedDesign(b, o)
			continue
		}
		out[k] = v
	}
	return out
}

var (
	designColorPaths = []string{"card.background", "card.ink", "card.accent", "page.backdrop"}
	designFontPaths  = []string{"fonts.display", "fonts.body"}
	wholeHex         = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func checkedDesign(m *Model, raw string) (map[string]any, error) {
	d := map[string]any{}
	if raw == "" {
		return d, nil
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil, fmt.Errorf("design is not a JSON object: %v", err)
	}
	fonts := map[string]bool{}
	for _, row := range m.Table("THEME").All() {
		t, err := themeDesign(m, row["id"])
		if err != nil {
			return nil, err
		}
		fonts[designValue(t, "fonts.display")] = true
		fonts[designValue(t, "fonts.body")] = true
	}
	for section, v := range d {
		fields, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("design.%s is not an object", section)
		}
		for field, value := range fields {
			path := section + "." + field
			s, isText := value.(string)
			switch {
			case slices.Contains(designColorPaths, path):
				if !isText || !wholeHex.MatchString(s) {
					return nil, fmt.Errorf("design %s is %v, not a #rrggbb color", path, value)
				}
			case slices.Contains(designFontPaths, path):
				if !isText || !fonts[s] {
					return nil, fmt.Errorf("design %s is %v, not a font a theme uses", path, value)
				}
			case path == "card.words":
				if _, ok := value.(bool); !ok {
					return nil, fmt.Errorf("design card.words is %v, not true or false", value)
				}
			case path == "text.kicker":
				kicker, ok := value.(map[string]any)
				words, isWords := kicker["words"].(string)
				if !ok || len(kicker) != 1 || !isWords || strings.TrimSpace(words) == "" || len([]rune(words)) > 100 {
					return nil, fmt.Errorf("design text.kicker is %v; an invitation sets only its words, up to 100 characters", value)
				}
			default:
				return nil, fmt.Errorf("design has no %s to change", path)
			}
		}
	}
	return d, nil
}

func designChecked(m *Model, c Change) ([]Edit, error) {
	if c.New == nil || c.New["design"] == c.Old["design"] {
		return nil, nil
	}
	if _, err := checkedDesign(m, c.New["design"]); err != nil {
		return nil, access.Invalid("%v", err)
	}
	return nil, nil
}

func designAt(design map[string]any, path string) any {
	var at any = design
	for _, part := range strings.Split(path, ".") {
		m, _ := at.(map[string]any)
		at = m[part]
	}
	return at
}

func pageBackgroundChecked(m *Model, c Change) ([]Edit, error) {
	if c.New == nil || c.New["background"] == "" || wholeHex.MatchString(c.New["background"]) {
		return nil, nil
	}
	return nil, access.Invalid("a page's background is a #rrggbb color, not %q", c.New["background"])
}

func designValue(design map[string]any, path string) string {
	s, _ := designAt(design, path).(string)
	return s
}

func designHex(design map[string]any, path string) (string, error) {
	found := hexColor.FindString(designValue(design, path))
	if found == "" {
		return "", fmt.Errorf("theme %s has no #rrggbb color at %s", designValue(design, "key"), path)
	}
	return found, nil
}

func designColor(design map[string]any, path string) (color.RGBA, error) {
	found, err := designHex(design, path)
	if err != nil {
		return color.RGBA{}, err
	}
	n, _ := strconv.ParseUint(found[1:], 16, 32)
	return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 0xff}, nil
}

func sealLetter(design map[string]any, title string) string {
	mark := designValue(design, "seal.text")
	if mark == "initial" {
		mark = title
	}
	for _, r := range mark {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return strings.ToUpper(string(r))
		}
	}
	return ""
}

type look struct {
	theme, envelope, seal, liner, design string
	cardPicture, backdropPicture         string
}

func lookOf(g map[string]string) look {
	return look{
		theme: g["theme"], envelope: g["envelope"], seal: g["seal"], liner: g["liner"], design: g["design"],
		cardPicture: g["card_picture"], backdropPicture: g["backdrop_picture"],
	}
}

func cardDesign(m *Model, l look) (map[string]any, error) {
	card, err := themeDesign(m, l.theme)
	if err != nil {
		return nil, err
	}
	over, err := checkedDesign(m, l.design)
	if err != nil {
		return nil, err
	}
	return mergedDesign(card, over), nil
}

func envelopeOf(m *Model, l look, title string) (sharecard.Envelope, error) {
	card, err := cardDesign(m, l)
	if err != nil {
		return sharecard.Envelope{}, err
	}
	wrap, err := themeDesign(m, cmp.Or(l.envelope, l.theme))
	if err != nil {
		return sharecard.Envelope{}, err
	}
	lining, err := themeDesign(m, cmp.Or(l.liner, l.envelope, l.theme))
	if err != nil {
		return sharecard.Envelope{}, err
	}
	sticker, err := themeDesign(m, cmp.Or(l.seal, l.envelope, l.theme))
	if err != nil {
		return sharecard.Envelope{}, err
	}
	e := sharecard.Envelope{Kicker: designValue(card, "text.kicker.words"), Title: title, Mark: sealLetter(sticker, title)}
	for _, c := range []struct {
		into   *color.RGBA
		design map[string]any
		path   string
	}{
		{&e.Backdrop, card, "page.backdrop"}, {&e.Body, wrap, "envelope.body"}, {&e.Side, wrap, "envelope.side"}, {&e.Front, wrap, "envelope.front"},
		{&e.Flap, wrap, "envelope.flap"}, {&e.Liner, lining, "envelope.liner"}, {&e.Seal, sticker, "seal.color"}, {&e.SealInk, sticker, "seal.ink"},
		{&e.Paper, card, "card.background"}, {&e.Ink, card, "card.ink"}, {&e.Accent, card, "card.accent"},
	} {
		if *c.into, err = designColor(c.design, c.path); err != nil {
			return sharecard.Envelope{}, err
		}
	}
	return e, nil
}

func (a *InvitationArt) envelopePicture(w http.ResponseWriter, r *http.Request) {
	m := a.pics.s.Model()
	g := m.sharedGroup(strings.TrimSuffix(r.PathValue("id"), ".png"))
	if g == nil || g["theme"] == "" {
		http.NotFound(w, r)
		return
	}
	e, err := envelopeOf(m, lookOf(g), g["name"])
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	e.Subtitle = g["subtitle"]
	picture, err := sharecard.DrawEnvelope(e)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(picture)
}
