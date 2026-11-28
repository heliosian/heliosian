package db

import (
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

var partiesPages = []string{"/{$}", "/g/{key}", "/celebrations/{code}", "/celebrations/{code}/{section}", "/my", "/my/{email}", "/hosting", "/guests", "/parties", "/admin"}

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
	art    *InvitationArt
}

func NewPartyPages(s *Store, bucket *blob.Bucket, style *sharecard.Style, art *InvitationArt) *PartyPages {
	return &PartyPages{s: s, bucket: bucket, style: style, art: art}
}

func (m *Model) celebrations() []store.Row {
	out := []store.Row{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] == "celebration" && g["status"] != "closed" {
			out = append(out, g)
		}
	}
	slices.SortStableFunc(out, func(x, y store.Row) int { return strings.Compare(y["start"], x["start"]) })
	return out
}

func (m *Model) celebrationsForInvitation(id string) []store.Row {
	return slices.DeleteFunc(m.Table("GROUP").Referencing("invitation", id), func(g store.Row) bool {
		return g["kind"] != "celebration"
	})
}

func (m *Model) celebrationAt(path string) store.Row {
	all := m.celebrations()
	if len(all) == 0 {
		return nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "celebrations" {
		return all[0]
	}
	for _, c := range all {
		if strings.EqualFold(c["slug"], parts[1]) || c["id"] == parts[1] {
			return c
		}
	}
	return nil
}

func (m *Model) celebrationInvitation(c store.Row) store.Row {
	if c == nil || c["invitation"] == "" {
		return nil
	}
	inv, ok := m.Table("GROUP").Get(c["invitation"])
	if !ok || inv["kind"] != "event" || inv["theme"] == "" {
		return nil
	}
	return inv
}

func (p *PartyPages) Register(mux *http.ServeMux) {
	serve.Pages(mux, partiesShell, partiesPages...)
	mux.HandleFunc("GET /open/share/upcoming.png", p.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", p.shareCard)
}

func (p *PartyPages) Redirected(next http.Handler) http.Handler {
	return redirected(next, func(r *http.Request) (string, int) {
		m := p.s.Model()
		if to := m.partyCanonical(r.URL); to != "" {
			return to, http.StatusMovedPermanently
		}
		return m.partyRedirect(r.URL.Path), http.StatusFound
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
	if g != nil {
		return p.style.PreviewTags(g["name"], eventBlurb(g), origin+r.URL.Path, origin+"/open/share/"+g["id"]+".png")
	}
	if inv := m.celebrationInvitation(m.celebrationAt(r.URL.Path)); inv != nil {
		return p.style.PreviewTags(inv["name"], eventBlurb(inv), origin+r.URL.Path, origin+"/open/share/"+inv["id"]+".png")
	}
	return p.upcomingHead(m, origin)
}

func (m *Model) upcomingParties(at time.Time) []store.Row {
	today := at.Format(DateLayout)
	out := []store.Row{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] != "party" || g["status"] != "open" || g["start"] == "" || g["start"][:min(len(g["start"]), len(DateLayout))] < today || !slices.Contains([]string{"direct", "others"}, g["join"]) {
			continue
		}
		if capacity, _ := strconv.Atoi(g["capacity"]); capacity > 0 && m.directMemberCount(g["id"]) >= capacity {
			continue
		}
		out = append(out, g)
	}
	slices.SortStableFunc(out, func(x, y store.Row) int { return strings.Compare(x["start"], y["start"]) })
	return out
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
	card := sharecard.Card{Title: "New parties are on the way", Subtitle: "Tickets go on sale soon."}
	words := []string{}
	if len(parties) > 0 {
		next := parties[0]
		day, hours := whenLines(next)
		card.Kicker, card.Title, card.Subtitle = "Next party", next["name"], next["subtitle"]
		card.Lines = whenCardLines(day, hours, next["location"])
		words = append(words, next["name"], next["subtitle"], day, hours, next["location"])
		if rest := parties[1:min(len(parties), 1+partiesListed)]; len(rest) > 0 {
			card.Listing = &sharecard.Listing{Heading: "Also coming"}
			for _, g := range rest {
				day, _ := whenLines(g)
				card.Listing.Items = append(card.Listing.Items, sharecard.Item{Title: g["name"], Note: day})
				words = append(words, g["name"], g["start"])
			}
		}
	}
	p.style.Serve(w, r, card, words...)
}

func (p *PartyPages) shareCard(w http.ResponseWriter, r *http.Request) {
	m := p.s.Model()
	key := strings.TrimSuffix(r.PathValue("id"), ".png")
	for _, c := range m.celebrations() {
		if inv := m.celebrationInvitation(c); inv != nil && inv["id"] == key {
			l := lookOf(inv)
			if c["theme"] != "" {
				l = look{theme: c["theme"]}
			}
			p.art.invitationCard(w, r, m, inv, l, false)
			return
		}
	}
	g := m.party(key)
	if g == nil {
		http.NotFound(w, r)
		return
	}
	name, whole := g["flyer"], true
	if name == "" {
		name, whole = m.groupPhoto(g["id"]), false
	}
	day, hours := whenLines(g)
	load := func() ([]byte, error) { return m.cardPicture(r.Context(), p.bucket, "party pages", g, name), nil }
	card := sharecard.Card{Title: g["name"], Subtitle: g["subtitle"], Lines: whenCardLines(day, hours, g["location"])}
	serveGroupCard(w, r, p.style, card, name, whole, load, g["name"], g["subtitle"], day, hours, g["location"])
}
