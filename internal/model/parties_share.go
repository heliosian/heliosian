package model

import (
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/sharecard"
)

func PartiesCardStyle(name, tagline func() string) *sharecard.Style {
	return &sharecard.Style{
		Palette: sharecard.Standard, Name: name, Tagline: tagline,
		Mark:   "web/public/celebrate/brand/logo-mark.png",
		Lockup: "web/public/celebrate/brand/logo-lockup.png",
	}
}

func partyPreviewable(p *Party) bool {
	return p != nil && p.VisibleTo(access.Actor{})
}

func partyWhenLines(p *Party) (string, string) {
	start, err := time.ParseInLocation(DateTimeFormat, p.Start, Location)
	if err != nil {
		if day, err := time.ParseInLocation(DateFormat, p.Start, Location); err == nil {
			return day.Format("Monday, January 2"), ""
		}
		return "", ""
	}
	day := start.Format("Monday, January 2")
	end, err := time.ParseInLocation(DateTimeFormat, p.End, Location)
	if err != nil {
		return day, start.Format("3:04 PM")
	}
	return day, sharecard.Hours(start, end)
}

func whenLine(p *Party) string {
	day, hours := partyWhenLines(p)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

func upcoming(m *Parties) []*Party {
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
	day, _ := partyWhenLines(p)
	return day
}

func partyBlurb(p *Party) string {
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

func PartiesPreviewHead(s *Store, style *sharecard.Style) func(r *http.Request) string {
	return func(r *http.Request) string {
		m := s.Model().Parties
		origin := "https://" + r.Host
		first := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
		var p *Party
		if first == "p" || first == "parties" {
			p = m.Resolve(r.URL.Path)
		}
		if !partyPreviewable(p) {
			return partiesUpcomingHead(style, m, origin)
		}
		parts := []string{}
		if line := whenLine(p); line != "" {
			parts = append(parts, line)
		}
		if p.Location != "" {
			parts = append(parts, p.Location)
		}
		if b := partyBlurb(p); b != "" {
			parts = append(parts, b)
		}
		desc := strings.Join(parts, " — ")
		if desc == "" {
			desc = "A fun(d)raiser party for the Helios community."
		}
		return style.PreviewTags(p.Title, desc, origin+m.PathOf(p), origin+"/open/share/"+p.ID+".png")
	}
}

func partiesUpcomingHead(style *sharecard.Style, m *Parties, origin string) string {
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

func (a partiesApp) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	parties := upcoming(a.parties())
	card := sharecard.Card{
		Title:   "New parties are on the way",
		Listing: &sharecard.Listing{Heading: "Upcoming Parties", Empty: "Nothing is on sale just now - check back soon."},
	}
	parts := []string{}
	if len(parties) > 0 {
		next := parties[0]
		day, hours := partyWhenLines(next)
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

func (a partiesApp) shareCard(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), ".png")
	p := a.parties().Party(id)
	if !partyPreviewable(p) {
		http.NotFound(w, r)
		return
	}
	picture, whole := p.Flyer, true
	if picture == "" {
		picture, whole = p.Image, false
	}
	day, hours := partyWhenLines(p)
	card := sharecard.Card{
		Title: p.Title, Subtitle: p.Subtitle, Picture: a.images.Read(picture), Whole: whole,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: p.Location}},
	}
	a.style.Serve(w, r, card, p.Title, p.Subtitle, day, hours, p.Location, picture)
}
