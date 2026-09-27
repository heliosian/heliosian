package calendar

import (
	"image/color"
	"net/http"
	"os"
	"path"
	"strings"

	"heliosian/internal/sharecard"
)

func CardStyle(name, tagline func() string) *sharecard.Style {
	palette := sharecard.Standard
	palette.Brand = color.RGBA{0x0e, 0x4d, 0x54, 0xff}
	palette.Yellow = color.RGBA{0xe8, 0xa3, 0x3d, 0xff}
	return &sharecard.Style{Palette: palette, Name: name, Tagline: tagline, Wordmark: "Helios When", Mark: "web/public/calendar/brand/logo-mark.png"}
}

const defaultHeader = "brand/default-header.jpg"

func whenLines(e *Event) (string, string) {
	if e.AllDay || spansDays(e) {
		if e.start.Equal(e.end) || e.start.Format(DateFormat) == e.end.Format(DateFormat) {
			return e.start.Format("Monday, January 2"), ""
		}
		return e.start.Format("Monday, January 2") + " – " + e.end.Format("Monday, January 2"), ""
	}
	return e.start.Format("Monday, January 2"), sharecard.Hours(e.start, e.end)
}

func spansDays(e *Event) bool {
	return e.start.Format(DateFormat) != e.end.Format(DateFormat) && !e.AllDay
}

func when(e *Event) string {
	day, hours := whenLines(e)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

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

func (m *Model) category(e *Event) string {
	for _, t := range e.Tags {
		if !m.Roster.has(t) {
			return t
		}
	}
	return ""
}

func (a app) events() []*Event {
	return withLinked(a.cache.Model().Events, a.linked(""))
}

func (a app) event(id string) *Event {
	for _, e := range a.events() {
		if e.ID == id {
			return e
		}
	}
	return a.cache.Model().Event(id)
}

func cutEventPath(path string) (string, bool) {
	if id, ok := strings.CutPrefix(path, "/e/"); ok {
		return id, true
	}
	return strings.CutPrefix(path, "/events/")
}

func PreviewHead(cache *Cache, linked func(email string) []Linked, style *sharecard.Style) func(r *http.Request) string {
	return func(r *http.Request) string {
		a := app{cache: cache, linked: linked, style: style}
		origin := "https://" + r.Host
		if id, ok := cutEventPath(r.URL.Path); ok {
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
	return a.style.PreviewTags(e.Title, strings.Join(parts, " — "), origin+EventPath(e), origin+"/open/share/"+e.ID+".png")
}

func (a app) upcomingHead(origin string) string {
	return a.style.PreviewTags(a.style.Name(), whenWords, origin+"/", origin+"/open/share/upcoming.png")
}

func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	card := whenCard(a.readImage(defaultHeader))
	a.style.Serve(w, r, card, card.Kicker, card.Title, card.Button)
}

const whenWords = "One calendar for everything at Helios: every school day, early dismissal and break for your classrooms, every event from the school, the HCA and Helios Celebrate, and calendars you save and subscribe to in your own app."

func whenCard(picture []byte) sharecard.Card {
	return sharecard.Card{
		Kicker: "One calendar for everything at Helios",
		Title:  "The school year, day by day",
		Lines: []sharecard.Line{
			{Icon: "calendar", Text: "School days, dismissals and breaks"},
			{Icon: "clock", Text: "Events, parties and HCA sign-ups"},
			{Icon: "pin", Text: "Calendars you save and subscribe to"},
		},
		Button:  "Open Helios When",
		Picture: picture,
	}
}

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

func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	e := a.event(id)
	if e == nil {
		http.NotFound(w, r)
		return
	}
	e = a.cache.Model().invitedEvent(e)
	picture := a.cache.Model().pictureOf(e)
	whole := false
	if inv := a.cache.Model().Invitations[e.ID]; inv != nil && inv.Flyer != "" {
		picture, whole = inv.Flyer, true
	}
	day, hours := whenLines(e)
	kicker := a.cache.Model().category(e)
	button := shareButton(e)
	card := sharecard.Card{
		Kicker: kicker, Title: e.Title, Picture: a.readImage(picture), Whole: whole,
		Lines:  []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: e.Location}},
		Button: button,
	}
	a.style.Serve(w, r, card, e.Title, kicker, day, hours, e.Location, picture, button)
}

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
