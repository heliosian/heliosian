package team

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/color"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"heliosian/internal/sharecard"
)

// A shared link to an event is fetched by whatever chat app it lands in, with
// no session, so the preview it shows comes from two public things: Open Graph
// tags slipped into the sign-in page served at the event's address, and a card
// image at /open/share/{id}.png drawn here - the brand, the title, when it is, and
// the event's own image when it has one. Only what is open or done is
// previewed that way; a link to anything else - the portal itself, a page, a
// hidden or pending thing - previews what needs hands: the open things this
// school year still short of volunteers, soonest first, at
// /open/share/upcoming.png. The tags say the title, a sentence of the
// description and the date - the same things a poster on the wall says - and
// nothing about who has signed up.

const (
	cardWidth  = sharecard.Width
	cardHeight = sharecard.Height
)

// cardStyle is the portal's dress for the card: its palette (the ink is the
// headline's deep teal-black, as the page sets its own), the horizontal
// lockup as the designer drew it, and the rail's meadow in the corner - the
// same picture the toolbar has. The wordmark and tagline stand in should the
// lockup ever fail to load.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
	Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
	Wordmark: "HCA-Team", Tagline: "HCA VOLUNTEER PORTAL",
	Mark: "web/public/team/brand/logo-mark.png", Lockup: "web/public/team/brand/logo-lockup.png", Corner: "web/team/toolbar_background.png",
}

// ShareWords gives the card the app's name and tagline as the registry has
// them now - the big words and the line under them - in place of the ones
// written here.
func ShareWords(name, tagline func() string) {
	shareName = name
	cardStyle.TaglineNow = tagline
}

// shareName is the app's name as the registry has it, the card's title.
var shareName func() string

// title is the card's big words: the app's name from the registry, else
// the wordmark written here.
func title() string {
	if shareName != nil {
		if now := strings.TrimSpace(shareName()); now != "" {
			return now
		}
	}
	return cardStyle.Wordmark
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
	if days, hours := spanLines(a); days != "" {
		if hours != "" {
			return days + " · " + hours
		}
		return days
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

// PreviewHead is the Open Graph markup for the thing at a request's path,
// or the portal's own preview of what needs hands when there is nothing
// there to show a stranger. Wired into the sign-in page, which is what an
// unauthenticated fetch of the address gets.
func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		model := cache.Model()
		if model == nil {
			return ""
		}
		origin := "https://" + r.Host
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		var a *Activity
		if first == "v" || first == "activities" {
			a = model.Resolve(r.URL.Path)
		}
		if !previewable(a) {
			return upcomingHead(model, origin, time.Now().In(local))
		}
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
		return previewTags(title, desc, origin+model.PathOf(a), origin+"/open/share/"+a.ID+".png")
	}
}

// previewTags is the markup itself, as every app's sign-in page carries it.
func previewTags(title, desc, url, image string) string {
	return sharecard.PreviewTags("HCA-Team", title, desc, url, image)
}

// needs is what the portal's own preview lists: the open roots of the school
// year that still want volunteers - not marked complete, and with a spot
// left when they count spots - the dated ones still to come, soonest first,
// then the ones with no date to sort by, in the page's order.
func needs(m *Model, at time.Time) []*Activity {
	year, today := SchoolYear(at), at.Format(DateFormat)
	dated, undated := []*Activity{}, []*Activity{}
	for _, a := range m.Activities {
		if a.Year != year || a.Status != StatusOpen || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) {
			continue
		}
		if a.Start == "" {
			undated = append(undated, a)
		} else if a.Start[:min(len(a.Start), len(DateFormat))] >= today {
			dated = append(dated, a)
		}
	}
	slices.SortStableFunc(dated, func(x, y *Activity) int { return strings.Compare(x.Start, y.Start) })
	return append(dated, undated...)
}

// needsCount is how many the card lists.
const needsCount = 4

// needNote is the line under a need's title: its day, and what is still
// wanted when it counts spots.
func needNote(a *Activity) string {
	parts := []string{}
	if day, _ := whenLines(a); day != "" {
		parts = append(parts, day)
	}
	if a.Spots > 0 {
		left := a.Spots - len(a.Volunteers)
		if left == 1 {
			parts = append(parts, "1 spot left")
		} else {
			parts = append(parts, fmt.Sprintf("%d spots left", left))
		}
	} else if a.CoLeaderNeeded {
		parts = append(parts, "co-chair wanted")
	}
	return strings.Join(parts, " · ")
}

// The portal's own card and tags: its name over its tagline, as the
// registry has them, and the things still short of hands.
const (
	upcomingLead  = "Sign up for a shift, a booth or a committee."
	upcomingEmpty = "Nothing needs hands just now - check back soon."
)

// upcomingHead is the portal's own preview: the things that still want
// volunteers, in words, with the card that draws them.
func upcomingHead(m *Model, origin string, at time.Time) string {
	desc := cardStyle.TaglineText() + ". " + upcomingLead
	if list := needs(m, at); len(list) > 0 {
		names := []string{}
		for _, a := range list[:min(len(list), needsCount)] {
			name := a.Title
			if note := needNote(a); note != "" {
				name += " (" + note + ")"
			}
			names = append(names, name)
		}
		desc = "Volunteers needed: " + strings.Join(names, "; ") + ". " + upcomingLead
	}
	return previewTags(title(), desc, origin+"/", origin+"/open/share/upcoming.png")
}

// shareUpcoming serves /open/share/upcoming.png: the card for the portal
// itself, which lists what still wants volunteers. It changes as people sign
// up and days pass, so its ETag hashes what it names.
func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	listing := &sharecard.Listing{Heading: "Volunteers needed", Empty: upcomingEmpty}
	for _, act := range needs(a.cache.Model(), time.Now().In(local)) {
		if len(listing.Items) == needsCount {
			break
		}
		listing.Items = append(listing.Items, sharecard.Item{Title: act.Title, Note: needNote(act)})
	}
	card := sharecard.Card{Title: title(), Subtitle: cardStyle.TaglineText(), Button: "See what's open", Listing: listing}
	cardStyle.Serve(w, r, card, sharecard.ETag(append(listing.Words(), title(), cardStyle.TaglineText())...))
}

// shareCard serves /open/share/{id}.png: the card for one previewable thing. The
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
	sum := sha256.Sum256([]byte(act.Title + "\x00" + under + "\x00" + line + "\x00" + picture + "\x00" + cardStyle.TaglineText()))
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
	if days, hours := spanLines(a); days != "" {
		return days, hours
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

// spanLines is the day and time lines of a thing that runs more than one day:
// "Friday, October 2 – Sunday, October 4", and, when both ends carry a
// time, "Fri 4:00 PM – Sun 12:00 PM" - each time with its weekday, since the
// days line no longer says which is which. Both empty for anything else, so
// the one-day forms above apply.
func spanLines(a *Activity) (string, string) {
	start, err := ParseWhen(a.Start)
	if err != nil {
		return "", ""
	}
	end, err := ParseWhen(a.End)
	if err != nil || (end.Year() == start.Year() && end.YearDay() == start.YearDay()) {
		return "", ""
	}
	days := start.Format("Monday, January 2") + " – " + end.Format("Monday, January 2")
	if len(a.Start) > len(DateFormat) && len(a.End) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM") + " – " + end.Format("Mon 3:04 PM")
	}
	if len(a.Start) > len(DateFormat) {
		return days, start.Format("Mon 3:04 PM")
	}
	return days, ""
}
