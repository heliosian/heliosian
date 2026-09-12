package events

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

// A shared link to an event is fetched by whatever chat app it lands in, with
// no session, so the preview it shows comes from two public things: Open Graph
// tags slipped into the sign-in page served at the event's address, and a card
// image at /share/{id}.png drawn here - the brand, the title, when it is, and
// the event's own image when it has one. Only what is open or done is
// previewed; a hidden or pending thing shows the plain sign-in page. The tags
// say the title, a sentence of the description and the date - the same things
// a poster on the wall says - and nothing about who has signed up.

const (
	cardWidth  = sharecard.Width
	cardHeight = sharecard.Height
)

// cardStyle is the portal's dress for the card: its palette (the ink is the
// headline's deep teal-black, as the page sets its own), the lockup, and the
// rail's meadow in the corner - the same picture the toolbar has.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
	Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
	Wordmark: "HCA-Team", Tagline: "HCA VOLUNTEER PORTAL",
	Mark: "web/public/team/brand/logo-mark.png", Corner: "web/team/toolbar_background.png",
}

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
	// The flyer is what the card shows when there is one; otherwise the
	// banner, and a thing under an event with neither shows the event's.
	picture, isFlyer := "", false
	for n := act; picture == "" && n != nil; n = model.byID[n.Parent] {
		picture, isFlyer = n.Flyer, true
		if picture == "" {
			picture, isFlyer = n.Image, false
		}
	}
	imageBytes := a.readImage(picture)
	line, under := when(timed(model, act)), lineage(model, act)
	sum := sha256.Sum256([]byte(act.Title + "\x00" + under + "\x00" + line + "\x00" + picture))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	day, hours := whenLines(timed(model, act))
	card, err := cardStyle.Draw(sharecard.Card{
		Kicker: under, Title: act.Title, Picture: imageBytes, Whole: isFlyer,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}},
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
	for _, dir := range []string{"web/team", "web/public/team"} {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}

// whenLines is the card's two lines: the day, and the time - or the timing
// words alone when the thing has those instead of a date.
func whenLines(a *Activity) (string, string) {
	if a.Timing != "" {
		return a.Timing, ""
	}
	start, err := time.ParseInLocation(DateTimeFormat, a.Start, local)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, a.Start, local); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	if end, err := time.ParseInLocation(DateTimeFormat, a.End, local); err == nil && end.After(start) {
		return day, start.Format("3:04") + " – " + end.Format("3:04 PM")
	}
	return day, start.Format("3:04 PM")
}
