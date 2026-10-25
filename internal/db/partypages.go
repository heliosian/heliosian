package db

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	partiesAbout  = "Fun(d)raiser parties for the Helios community. New parties are on the way."
	partiesListed = 3
	partiesShell  = "web/celebrate/index.html"
)

var partiesPages = []string{"/{$}", "/g/{key}", "/celebrations/{code}", "/my", "/my/{email}", "/hosting", "/admin"}

func PartyCardStyle(name, tagline func() string) *sharecard.Style {
	return &sharecard.Style{
		Palette: sharecard.Standard, Name: name, Tagline: tagline,
		Mark:   "web/public/celebrate/brand/logo-mark.png",
		Lockup: "web/public/celebrate/brand/logo-lockup.png",
	}
}

type PartyPages struct {
	s      *Store
	bucket *blob.Bucket
	style  *sharecard.Style
}

func NewPartyPages(s *Store, bucket *blob.Bucket, style *sharecard.Style) *PartyPages {
	return &PartyPages{s: s, bucket: bucket, style: style}
}

func (p *PartyPages) Register(mux *http.ServeMux) {
	for _, page := range partiesPages {
		mux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, partiesShell)
		})
	}
	mux.HandleFunc("GET /open/share/upcoming.png", p.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", p.shareCard)
}

func (p *PartyPages) Redirected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		m := p.s.Model()
		to, status := m.partyCanonical(r.URL), http.StatusMovedPermanently
		if to == "" {
			to, status = m.partyRedirect(r.URL.Path), http.StatusFound
		}
		if to == "" {
			next.ServeHTTP(w, r)
			return
		}
		redirectTo(w, r, to, status)
	})
}

func (m *Model) partyCanonical(u *url.URL) string {
	kind, key, ok := strings.Cut(strings.Trim(u.Path, "/"), "/")
	if !ok || kind != "g" || strings.Contains(key, "/") {
		return ""
	}
	g := m.party(key)
	if g == nil || g["path"] == u.EscapedPath() {
		return ""
	}
	return g["path"]
}

func (m *Model) partyRedirect(path string) string {
	at := strings.TrimRight(path, "/")
	for _, row := range m.Table("REDIRECT").All() {
		old, to := strings.TrimRight(strings.TrimSpace(row["old"]), "/"), strings.TrimSpace(row["new"])
		if row["app"] != "celebrate" || old == "" || !strings.EqualFold(old, at) {
			continue
		}
		if isURL(to) || onSite(to) {
			return to
		}
	}
	return ""
}

func (m *Model) party(key string) store.Row {
	if g := m.sharedGroup(key); g != nil && g["kind"] == "party" {
		return g
	}
	return nil
}

func (p *PartyPages) PreviewHead(r *http.Request) string {
	m := p.s.Model()
	origin := "https://" + r.Host
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var g store.Row
	if len(parts) == 2 && parts[0] == "g" {
		g = m.party(parts[1])
	}
	if g == nil {
		return p.upcomingHead(m, origin)
	}
	lines := []string{}
	for _, s := range []string{whenText(g), g["location"], blurb(g["description"])} {
		if s != "" {
			lines = append(lines, s)
		}
	}
	return p.style.PreviewTags(g["name"], strings.Join(lines, " — "), origin+r.URL.Path, origin+"/open/share/"+g["id"]+".png")
}

func (m *Model) upcomingParties(at time.Time) []store.Row {
	today := at.Format(DateLayout)
	out := []store.Row{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] != "party" || g["status"] != "open" || g["start"] == "" || g["start"][:min(len(g["start"]), len(DateLayout))] < today || !slices.Contains([]string{"direct", "others"}, g["join"]) {
			continue
		}
		if capacity, _ := strconv.Atoi(g["capacity"]); capacity > 0 && m.held(g["id"]) >= capacity {
			continue
		}
		out = append(out, g)
	}
	slices.SortStableFunc(out, func(x, y store.Row) int { return strings.Compare(x["start"], y["start"]) })
	return out
}

func (m *Model) held(group string) int {
	n := 0
	for _, row := range m.Table("MEMBER").Referencing("group", group) {
		if memberAs(row) == "yes" {
			n++
		}
	}
	return n
}

func (p *PartyPages) upcomingHead(m *Model, origin string) string {
	desc := partiesAbout
	if parties := m.upcomingParties(time.Now().In(School)); len(parties) > 0 {
		next := parties[0]
		desc = "Next up: " + next["name"]
		for _, s := range []string{whenText(next), next["location"]} {
			if s != "" {
				desc += " — " + s
			}
		}
		if rest := parties[1:min(len(parties), 1+partiesListed)]; len(rest) > 0 {
			names := []string{}
			for _, g := range rest {
				day, _ := whenLines(g)
				names = append(names, g["name"]+" ("+day+")")
			}
			desc += ". Also coming: " + strings.Join(names, ", ") + "."
		}
	}
	return p.style.PreviewTags("Upcoming Parties", desc, origin+"/", origin+"/open/share/upcoming.png")
}

func (p *PartyPages) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	m := p.s.Model()
	parties := m.upcomingParties(time.Now().In(School))
	card := sharecard.Card{
		Title:   "New parties are on the way",
		Listing: &sharecard.Listing{Heading: "Upcoming Parties", Empty: "Nothing is on sale just now - check back soon."},
	}
	words := []string{}
	if len(parties) > 0 {
		next := parties[0]
		day, hours := whenLines(next)
		card.Kicker, card.Title, card.Subtitle = "Next party", next["name"], next["subtitle"]
		card.Lines = []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: next["location"]}}
		words = append(words, next["name"], next["subtitle"], day, hours, next["location"])
		for _, g := range parties[1:min(len(parties), 1+partiesListed)] {
			day, _ := whenLines(g)
			card.Listing.Items = append(card.Listing.Items, sharecard.Item{Title: g["name"], Note: day})
			words = append(words, g["name"], g["start"])
		}
	}
	p.style.Serve(w, r, card, words...)
}

func (p *PartyPages) shareCard(w http.ResponseWriter, r *http.Request) {
	m := p.s.Model()
	g := m.party(strings.TrimSuffix(r.PathValue("id"), ".png"))
	if g == nil {
		http.NotFound(w, r)
		return
	}
	name, whole := g["flyer"], true
	if name == "" {
		name, whole = m.groupPhoto(g["id"]), false
	}
	var picture []byte
	if name != "" {
		content, _, err := m.contentBytes(r.Context(), p.bucket, name)
		if err != nil && !errors.Is(err, blob.ErrNotFound) {
			slog.ErrorContext(r.Context(), "party pages: read a share card's picture", "group", g["id"], "name", name, "error", err)
		}
		picture = content
	}
	day, hours := whenLines(g)
	card := sharecard.Card{
		Title: g["name"], Subtitle: g["subtitle"], Picture: picture, Whole: whole,
		Lines: []sharecard.Line{{Icon: "calendar", Text: day}, {Icon: "clock", Text: hours}, {Icon: "pin", Text: g["location"]}},
	}
	p.style.Serve(w, r, card, g["name"], g["subtitle"], day, hours, g["location"], name)
}
