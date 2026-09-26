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

	"heliosian/internal/access"
	"heliosian/internal/sharecard"
)

var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x1f, 0x83, 0x8a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Helios Celebrate", Tagline: "FUN(D)RAISER PARTIES",
	Mark:   "web/public/celebrate/brand/logo-mark.png",
	Lockup: "web/public/celebrate/brand/logo-lockup.png",
}

func ShareTagline(now func() string) {
	cardStyle.TaglineNow = now
}

func previewable(p *Party) bool {
	return p != nil && p.VisibleTo(access.Viewer{})
}

func whenLines(p *Party) (string, string) {
	start, err := time.ParseInLocation(DateTimeFormat, p.Start, local)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, p.Start, local); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, p.End, local)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func when(p *Party) string {
	day, hours := whenLines(p)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

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

const upcomingCount = 3

func shortDay(p *Party) string {
	day, _ := whenLines(p)
	return day
}

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

func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		model := cache.Model()
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

func previewTags(title, desc, url, image string) string {
	return sharecard.PreviewTags("Helios Celebrate", title, desc, url, image)
}

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

func (a app) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	p := a.cache.Model().Party(id)
	if !previewable(p) {
		http.NotFound(w, r)
		return
	}
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
