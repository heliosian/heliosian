package calendar

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

	"heliosian/internal/sharecard"
)

// A shared link to an event is fetched by whatever chat app it lands in,
// with no session, so the preview it shows comes from two public things:
// Open Graph tags slipped into the sign-in page served at the event's
// address, and a card image at /share/{id}.png drawn here - the brand, the
// title, the day, the hours and the place, over the picture the event's
// page wears. A link to anything else previews the calendar itself: the
// next few events, at /share/upcoming.png. The tags say what the school's
// own calendar says and nothing about who is going.

// cardStyle is the calendar's dress for the card: the brand teal, the
// amber of today, and the mark beside the wordmark - the lockups as drawn
// stand taller than the card's header allows.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0e, 0x4d, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xe8, 0xa3, 0x3d, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios When", Tagline: "HELIOS CALENDAR OF EVERYTHING",
	Mark: "web/public/calendar/brand/logo-mark.png",
}

// ShareTagline gives the card the app's tagline as the registry has it
// now, in place of the one written here.
func ShareTagline(now func() string) {
	cardStyle.TaglineNow = now
}

// defaultHeader is the picture an event wears when neither it nor its
// categories have one.
const defaultHeader = "brand/default-header.jpg"

// upcomingCount is how many events the calendar's own card names.
const upcomingCount = 4

// whenLines is the card's day and time lines: "Thursday, September 24"
// and "4:00 – 6:00 PM"; for days that run across, the days.
func whenLines(e *Event) (string, string) {
	if e.AllDay || spansDays(e) {
		if e.start.Equal(e.end) || e.start.Format(DateFormat) == e.end.Format(DateFormat) {
			return e.start.Format("Monday, January 2"), ""
		}
		return e.start.Format("Monday, January 2") + " – " + e.end.Format("Monday, January 2"), ""
	}
	day := e.start.Format("Monday, January 2")
	if e.end.After(e.start) {
		return day, e.start.Format("3:04") + " – " + e.end.Format("3:04 PM")
	}
	return day, e.start.Format("3:04 PM")
}

func spansDays(e *Event) bool {
	return e.start.Format(DateFormat) != e.end.Format(DateFormat) && !e.AllDay
}

// when is the one line the tags carry: the day and the hours together.
func when(e *Event) string {
	day, hours := whenLines(e)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

// blurb is the description cut to a sentence or two.
func blurb(e *Event) string {
	text := strings.Join(strings.Fields(e.Description), " ")
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

// category is the first of an event's tags that is a category rather than
// a classroom - what the card's kicker says.
func (m *Model) category(e *Event) string {
	for _, t := range e.Tags {
		if !m.Roster.has(t) {
			return t
		}
	}
	return ""
}

// events is every event a stranger may be shown, the sheet's and the
// linked ones, as the page lists them.
func (a app) events() []*Event {
	return withLinked(a.cache.Model().Events, a.linked(""))
}

func (a app) event(id string) *Event {
	for _, e := range a.events() {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// upcoming is every event from today on, soonest first.
func (a app) upcoming() []*Event {
	today := now().Format(DateFormat)
	out := []*Event{}
	for _, e := range a.events() {
		if e.end.Format(DateFormat) >= today {
			out = append(out, e)
		}
	}
	return out
}

// PreviewHead is the Open Graph markup for a request's path: the event
// there when the path names one, else the calendar's own preview of what
// is coming. Wired into the sign-in page, which is what an unauthenticated
// fetch of the address gets.
func PreviewHead(cache *Cache, linked func(email string) []Linked) func(r *http.Request) string {
	return func(r *http.Request) string {
		if cache.Model() == nil {
			return ""
		}
		a := app{cache: cache, linked: linked}
		origin := "https://" + r.Host
		if id, ok := strings.CutPrefix(r.URL.Path, "/events/"); ok {
			if e := a.event(id); e != nil {
				return a.eventHead(e, origin)
			}
		}
		return a.upcomingHead(origin)
	}
}

func (a app) eventHead(e *Event, origin string) string {
	parts := []string{when(e)}
	if e.Location != "" {
		parts = append(parts, e.Location)
	}
	if b := blurb(e); b != "" {
		parts = append(parts, b)
	}
	return previewTags(e.Title, strings.Join(parts, " — "), origin+"/events/"+e.ID, origin+"/share/"+e.ID+".png")
}

// upcomingHead is the calendar's own preview: the next few events, in words,
// with the card that draws them.
func (a app) upcomingHead(origin string) string {
	events := a.upcoming()
	desc := "The school year, day by day: every event, day type and dismissal for the Helios community."
	if len(events) > 0 {
		names := []string{}
		for _, e := range events[:min(len(events), upcomingCount)] {
			names = append(names, e.Title+" ("+e.start.Format("Jan 2")+")")
		}
		desc = "Coming up: " + strings.Join(names, ", ") + "."
	}
	return previewTags("Helios Calendar", desc, origin+"/", origin+"/share/upcoming.png")
}

// previewTags is the markup itself: the Open Graph and Twitter tags for a
// title, a description, the page and its card.
func previewTags(title, desc, url, image string) string {
	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", "Helios Calendar"},
		{"og:title", title},
		{"og:description", desc},
		{"og:url", url},
		{"og:image", image},
		{"og:image:width", fmt.Sprint(sharecard.Width)},
		{"og:image:height", fmt.Sprint(sharecard.Height)},
		{"twitter:card", "summary_large_image"},
		{"twitter:title", title},
		{"twitter:description", desc},
		{"twitter:image", image},
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

// shareUpcoming serves /share/upcoming.png: the card for the calendar
// itself - the next event dressed as its own card would be, beside a list
// of the few after it. It changes as days pass, so its ETag hashes what it
// names.
func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	events := a.upcoming()
	card := sharecard.Card{
		Title:   "The school year, day by day",
		Listing: &sharecard.Listing{Heading: "Coming up", Empty: "Nothing on the calendar just now."},
	}
	parts := []string{cardStyle.TaglineText()}
	if len(events) > 0 {
		next := events[0]
		day, hours := whenLines(next)
		card.Kicker, card.Title = "Next up", next.Title
		card.Lines = []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: next.Location}}
		parts = append(parts, next.Title, day, hours, next.Location)
		for _, e := range events[1:min(len(events), 1+upcomingCount)] {
			card.Listing.Items = append(card.Listing.Items, sharecard.Item{Title: e.Title, Note: e.start.Format("Monday, January 2")})
			parts = append(parts, e.Title, e.Start)
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

// shareButton is the word on the card's button, what the event's page
// asks: tickets for a party, a sign-up for an HCA event, and an RSVP for
// everything else - the same for everyone, since the card is public.
func shareButton(e *Event) string {
	switch e.Source {
	case SourceCelebrate:
		return "Get Tickets"
	case SourceTeam:
		return "Sign Up"
	}
	if e.Link != "" {
		return "Sign Up"
	}
	return "RSVP"
}

// shareCard serves /share/{id}.png: the card for one event. It depends only
// on the title, the lines, the picture and the button's word, so its ETag
// is a hash of those and a chat app that fetched it once need not again.
func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	e := a.event(id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	picture := a.cache.Model().pictureOf(e)
	day, hours := whenLines(e)
	kicker := a.cache.Model().category(e)
	button := shareButton(e)
	sum := sha256.Sum256([]byte(strings.Join([]string{e.Title, kicker, day, hours, e.Location, picture, button}, "\x00")))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	card, err := cardStyle.Draw(sharecard.Card{
		Kicker: kicker, Title: e.Title, Picture: a.readImage(picture),
		Lines:  []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: e.Location}},
		Button: button,
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

// pictureOf is the name of the picture an event's page wears, as the page
// chooses it: the event's own, the first of its tags that has one, else
// the calendar's header. A name is a path the app that owns it serves,
// less its leading slash.
func (m *Model) pictureOf(e *Event) string {
	if e.Image != "" {
		return strings.TrimPrefix(e.Image, "/")
	}
	for _, name := range e.Tags {
		for _, t := range m.Tags {
			if t.Name == name && t.ImageURL != "" {
				return strings.TrimPrefix(t.ImageURL, "/")
			}
		}
	}
	return defaultHeader
}

// readImage is a picture as bytes: an upload from the shared blob store -
// the calendar's own folder or another app's, as a linked event's is - or
// a file bundled with whichever app serves it. Nothing when it is not held.
func (a app) readImage(key string) []byte {
	if key == "" {
		return nil
	}
	if a.store != nil {
		if data, _, ok := a.store.Bytes(key); ok {
			return data
		}
	}
	for _, dir := range []string{"web/calendar", "web/public/calendar", "web/celebrate", "web/public/celebrate", "web/team", "web/public/team"} {
		if data, err := os.ReadFile(path.Join(dir, key)); err == nil {
			return data
		}
	}
	return nil
}
