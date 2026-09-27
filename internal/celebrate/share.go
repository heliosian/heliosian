package celebrate

import (
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/sharecard"
	"heliosian/internal/when"
)

func CardStyle(name, tagline func() string) *sharecard.Style {
	return &sharecard.Style{
		Palette: sharecard.Standard, Name: name, Tagline: tagline,
		Mark:   "web/public/celebrate/brand/logo-mark.png",
		Lockup: "web/public/celebrate/brand/logo-lockup.png",
	}
}

func previewable(p *Party) bool {
	return p != nil && p.VisibleTo(access.Actor{})
}

func whenLines(p *Party) (string, string) {
	start, err := time.ParseInLocation(DateTimeFormat, p.Start, when.Location)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, p.Start, when.Location); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, p.End, when.Location)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func whenLine(p *Party) string {
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

func PreviewHead(cache *Cache, style *sharecard.Style) func(r *http.Request) string {
	return func(r *http.Request) string {
		model := cache.Model()
		origin := "https://" + r.Host
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		var p *Party
		if first == "p" || first == "parties" {
			p = model.Resolve(r.URL.Path)
		}
		if !previewable(p) {
			return upcomingHead(style, model, origin)
		}
		parts := []string{}
		if line := whenLine(p); line != "" {
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
		return style.PreviewTags(p.Title, desc, origin+model.PathOf(p), origin+"/open/share/"+p.ID+".png")
	}
}

func upcomingHead(style *sharecard.Style, m *Model, origin string) string {
	parties := upcoming(m)
	desc := "Fun(d)raiser parties for the Helios community. New parties are on the way."
	if len(parties) > 0 {
		next := parties[0]
		desc = "Next up: " + next.Title
		if line := whenLine(next); line != "" {
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
	return style.PreviewTags("Upcoming Parties", desc, origin+"/", origin+"/open/share/upcoming.png")
}

func (a app) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	parties := upcoming(a.cache.Model())
	card := sharecard.Card{
		Title:   "New parties are on the way",
		Listing: &sharecard.Listing{Heading: "Upcoming Parties", Empty: "Nothing is on sale just now - check back soon."},
	}
	parts := []string{}
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
	a.style.Serve(w, r, card, parts...)
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
	card := sharecard.Card{
		Title: p.Title, Subtitle: p.Subtitle, Picture: a.images.Read(picture), Whole: whole,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: p.Location}},
	}
	a.style.Serve(w, r, card, p.Title, p.Subtitle, day, hours, p.Location, picture)
}
