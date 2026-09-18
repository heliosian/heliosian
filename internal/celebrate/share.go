package celebrate

import (
	"crypto/sha256"
	"encoding/hex"
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
// a card image at /open/share/{id}.png drawn here - the brand, the title, when and
// where (in words, never the street), and the party's flyer or picture. Only
// an open party is previewed that way; a link to anything else - the site
// itself, a page, a pending or hidden party - previews what is on sale: the
// next party with tickets left on the left, and a list of the three after it
// on the right, at /open/share/upcoming.png. The tags say what a poster on the
// wall says and nothing about who is coming.

// cardStyle is the site's dress for the card: the palette sampled from the
// logo art and the logo itself; no corner art.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios Celebrate", Tagline: "FUN(D)RAISER PARTIES",
	Mark: "web/public/celebrate/brand/logo-mark.png",
	// The card wears the logo as drawn, not a lockup set here.
	Lockup: "web/public/celebrate/brand/logo-lockup.png",
}

// ShareTagline gives the card the app's tagline as the registry has it
// now, in place of the one written here.
func ShareTagline(now func() string) {
	cardStyle.TaglineNow = now
}

// previewable is what may be shown to someone who has not signed in.
func previewable(p *Party) bool {
	return p != nil && p.VisibleTo("", false)
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

// upcoming is every party someone could buy a ticket to right now - open,
// selling, not full, not over - soonest first. Undated parties are left out;
// they cannot be next.
func upcoming(m *Model) []*Party {
	at := now()
	out := []*Party{}
	for _, p := range m.SortedParties("") {
		if p.Start != "" && p.Availability(at) == Available {
			out = append(out, p)
		}
	}
	return out
}

// upcomingCount is how many parties the list beside the next one names.
const upcomingCount = 3

// shortDay is a party's day as the list gives it: "Saturday, September 26".
func shortDay(p *Party) string {
	day, _ := whenLines(p)
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

// PreviewHead is the Open Graph markup for a request's path: the party there
// when it is one a stranger may see, else the site's own preview of what is
// on sale. Wired into the sign-in page, which is what an unauthenticated
// fetch of the address gets.
func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		model := cache.Model()
		if model == nil {
			return ""
		}
		origin := "https://" + r.Host
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		var p *Party
		if first == "p" || first == "parties" {
			p = model.Resolve(r.URL.Path)
		}
		if !previewable(p) {
			return upcomingHead(model, origin)
		}
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
		return previewTags(p.Title, desc, origin+model.PathOf(p), origin+"/open/share/"+p.ID+".png")
	}
}

// upcomingHead is the site's own preview: the next party with tickets and
// the few after it, in words, with the card that draws them.
func upcomingHead(m *Model, origin string) string {
	parties := upcoming(m)
	desc := "Fun(d)raiser parties for the Helios community. New parties are on the way."
	if len(parties) > 0 {
		next := parties[0]
		desc = "Next up: " + next.Title
		if line := when(next); line != "" {
			desc += " — " + line
		}
		if next.Location != "" {
			desc += " — " + next.Location
		}
		if rest := parties[1:min(len(parties), 1+upcomingCount)]; len(rest) > 0 {
			names := []string{}
			for _, p := range rest {
				names = append(names, p.Title+" ("+p.StartTime().Format("Jan 2")+")")
			}
			desc += ". Also coming: " + strings.Join(names, ", ") + "."
		}
	}
	return previewTags("Upcoming Parties", desc, origin+"/", origin+"/open/share/upcoming.png")
}

// previewTags is the markup itself, as every app's sign-in page carries it.
func previewTags(title, desc, url, image string) string {
	return sharecard.PreviewTags("Helios Celebrate", title, desc, url, image)
}

// shareUpcoming serves /open/share/upcoming.png: the card for the site itself,
// which is the next party with tickets left, dressed as its own card would
// be, beside a list of the three after it. It changes as parties sell out
// and pass, so its ETag hashes what it names.
func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	parties := upcoming(a.cache.Model())
	card := sharecard.Card{
		Title:   "New parties are on the way",
		Listing: &sharecard.Listing{Heading: "Upcoming Parties", Empty: "Nothing is on sale just now - check back soon."},
	}
	parts := []string{cardStyle.TaglineText()}
	if len(parties) > 0 {
		next := parties[0]
		day, hours := whenLines(next)
		card.Kicker, card.Title, card.Subtitle = "Next party", next.Title, next.Subtitle
		card.Lines = []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: next.Location}}
		parts = append(parts, next.Title, next.Subtitle, day, hours, next.Location)
		for _, p := range parties[1:min(len(parties), 1+upcomingCount)] {
			card.Listing.Items = append(card.Listing.Items, sharecard.Item{Title: p.Title, Note: shortDay(p)})
			parts = append(parts, p.Title, p.Start)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	png, err := cardStyle.Draw(card)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(png)
}

// shareCard serves /open/share/{id}.png: the card for one previewable party. The
// card depends only on the title, the subtitle, the lines and the picture,
// so its ETag is a hash of those and a chat app that fetched it once need not again.
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
	sum := sha256.Sum256([]byte(strings.Join([]string{p.Title, p.Subtitle, day, hours, p.Location, picture}, "\x00")))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	card, err := cardStyle.Draw(sharecard.Card{
		Title: p.Title, Subtitle: p.Subtitle, Picture: a.readImage(picture), Whole: whole,
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
