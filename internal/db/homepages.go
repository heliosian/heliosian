package db

import (
	"image"
	"image/color"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"

	_ "image/png"
)

const (
	homeShell      = "web/home/index.html"
	homeShareTitle = "Tools and resources for the Helios Community"
	homeShareLead  = "Sign in with your school Google account."
)

func HomeCardStyle(name, tagline func() string) *sharecard.Style {
	palette := sharecard.Standard
	palette.Accent = color.RGBA{0x6f, 0x8f, 0x1a, 0xff}
	return &sharecard.Style{Palette: palette, Name: name, Tagline: tagline, Wordmark: "Heliosian", Mark: "web/public/home/brand/logo-mark.png"}
}

type HomePages struct {
	s     *Store
	style *sharecard.Style
	marks sync.Map
}

func NewHomePages(s *Store, style *sharecard.Style) *HomePages {
	return &HomePages{s: s, style: style}
}

func (p *HomePages) Register(mux *http.ServeMux) {
	for _, page := range []string{"GET /{$}", "GET /admin"} {
		mux.HandleFunc(page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, homeShell)
		})
	}
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", p.shareApps)
}

func (m *Model) App(key string) store.Row {
	row, _ := m.Table("APP").Find(key)
	return row
}

func (m *Model) sharedApps() []store.Row {
	everyone := m.slugGroup("everyone")
	out := []store.Row{}
	for _, a := range m.Table("APP").All() {
		if a["key"] != "home" && a["visible_to"] == everyone {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
	return out
}

func (m *Model) slugGroup(slug string) string {
	g, _ := m.Table("GROUP").Distinct("slug", slug)
	return g["id"]
}

func tierOf(pageHost string) string {
	host, port, _ := strings.Cut(strings.ToLower(pageHost), ":")
	labels := strings.Split(host, ".")
	if len(labels) > 2 {
		labels = labels[1:]
	}
	tier := strings.Join(labels, ".")
	if port != "" {
		tier += ":" + port
	}
	return tier
}

func (p *HomePages) PreviewHead(r *http.Request) string {
	origin := "https://" + r.Host
	tier := tierOf(r.Host)
	names := []string{}
	for _, a := range p.s.Model().sharedApps() {
		names = append(names, a["name"]+" ("+strings.TrimSuffix(a["subtitle"], ".")+", "+a["key"]+"."+tier+")")
	}
	desc := p.style.Tagline() + ". " + homeShareLead
	if len(names) > 0 {
		desc = p.style.Tagline() + ": " + strings.Join(names, "; ") + ". " + homeShareLead
	}
	return p.style.PreviewTags(homeShareTitle, desc, origin+"/", origin+"/open/share/apps.png")
}

func (p *HomePages) appMark(key string) image.Image {
	if v, ok := p.marks.Load(key); ok {
		img, _ := v.(image.Image)
		return img
	}
	var img image.Image
	if f, err := os.Open("web/public/common/brand/apps/" + key + ".png"); err == nil {
		defer f.Close()
		img, _, _ = image.Decode(f)
	}
	p.marks.Store(key, img)
	return img
}

func (p *HomePages) shareApps(w http.ResponseWriter, r *http.Request) {
	parts := []string{}
	listing := &sharecard.Listing{Empty: "The apps are on their way - check back soon."}
	for _, a := range p.s.Model().sharedApps() {
		listing.Items = append(listing.Items, sharecard.Item{Title: a["name"], Note: a["subtitle"], Icon: p.appMark(a["key"])})
		parts = append(parts, a["key"], a["name"], a["subtitle"])
	}
	card := sharecard.Card{Title: homeShareTitle, Subtitle: homeShareLead, Button: "Open Heliosian", Listing: listing}
	p.style.Serve(w, r, card, parts...)
}
