package celebrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"image/color"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"heliosian/internal/sharecard"
)

// A shared link to a party is fetched by whatever chat app it lands in, with
// no session, so the preview it shows comes from two public things: Open
// Graph tags slipped into the sign-in page served at the party's address, and
// a card image at /share/{id}.png drawn here - the brand, the title, when and
// where (in words, never the street), and the party's flyer or picture. Only
// an open party is previewed; a pending or hidden one shows the plain sign-in
// page. The tags say what a poster on the wall says and nothing about who is
// coming.

// cardStyle is the site's dress for the card: the palette sampled from the
// logo art, the lockup, and the mark; no corner art.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios Celebrate", Tagline: "FUN(D)RAISER PARTIES",
	Mark: "web/public/celebrate/brand/logo-mark.png",
}

// previewable is what may be shown to someone who has not signed in.
func previewable(p *Party) bool {
	return p != nil && p.Status == StatusOpen
}

// whenLines is the card's day and time lines.
func whenLines(p *Party) (string, string) {
	start, err := time.ParseInLocation(DateTimeFormat, p.Start, local)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, p.Start, local); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	if end, err := time.ParseInLocation(DateTimeFormat, p.End, local); err == nil && end.After(start) {
		return day, start.Format("3:04") + " – " + end.Format("3:04 PM")
	}
	return day, start.Format("3:04 PM")
}

// when is the one line the tags carry: the day and the hours together.
func when(p *Party) string {
	day, hours := whenLines(p)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

// blurb is the summary, else the description cut to a sentence or two.
func blurb(p *Party) string {
	text := strings.Join(strings.Fields(p.Summary), " ")
	if text == "" {
		text = strings.Join(strings.Fields(p.Description), " ")
	}
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

// PreviewHead is the Open Graph markup for the party at a request's path, or
// nothing when there is nothing there to show a stranger. Wired into the
// sign-in page, which is what an unauthenticated fetch of the address gets.
func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		if first != "p" && first != "parties" {
			return ""
		}
		model := cache.Model()
		if model == nil {
			return ""
		}
		p := model.Resolve(r.URL.Path)
		if !previewable(p) {
			return ""
		}
		origin := "https://" + r.Host
		parts := []string{}
		if line := when(p); line != "" {
			parts = append(parts, line)
		}
		if p.Location != "" {
			parts = append(parts, p.Location)
		}
		if b := blurb(p); b != "" {
			parts = append(parts, b)
		}
		desc := strings.Join(parts, " — ")
		if desc == "" {
			desc = "A fun(d)raiser party for the Helios community."
		}
		tags := [][2]string{
			{"og:type", "website"},
			{"og:site_name", "Helios Celebrate"},
			{"og:title", p.Title},
			{"og:description", desc},
			{"og:url", origin + model.PathOf(p)},
			{"og:image", origin + "/share/" + p.ID + ".png"},
			{"og:image:width", fmt.Sprint(sharecard.Width)},
			{"og:image:height", fmt.Sprint(sharecard.Height)},
			{"twitter:card", "summary_large_image"},
			{"twitter:title", p.Title},
			{"twitter:description", desc},
			{"twitter:image", origin + "/share/" + p.ID + ".png"},
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

// shareCard serves /share/{id}.png: the card for one previewable party. The
// card depends only on the title, the lines and the picture, so its ETag is
// a hash of those and a chat app that fetched it once need not again.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	p := a.cache.Model().Party(id)
	if !previewable(p) {
		http.NotFound(w, r)
		return
	}
	// The flyer is what the card shows when there is one; otherwise the
	// banner.
	picture, whole := p.Flyer, true
	if picture == "" {
		picture, whole = p.Image, false
	}
	day, hours := whenLines(p)
	sum := sha256.Sum256([]byte(strings.Join([]string{p.Title, day, hours, p.Location, picture}, "\x00")))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	card, err := cardStyle.Draw(sharecard.Card{
		Title: p.Title, Picture: a.readImage(picture), Whole: whole,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: p.Location}},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(card)
}

// readImage is a party's picture as bytes: an upload from the blob store, or
// one of the bundled files. Nothing when there is none or it is not held.
func (a app) readImage(key string) []byte {
	if key == "" {
		return nil
	}
	if strings.HasPrefix(key, imageFolder+"/") {
		if a.store == nil {
			return nil
		}
		data, _, ok := a.store.Bytes(key)
		if !ok {
			return nil
		}
		return data
	}
	for _, dir := range []string{"web/celebrate", "web/public/celebrate"} {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}
