package db

import (
	"image/color"
	"net/http"
	"os"
	"slices"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	whenHeader = "web/public/when/brand/default-header.jpg"
	whenAbout  = "One calendar for everything at Helios: every school day, early dismissal and break for your classrooms, every event from the school, the HCA and Helios Celebrate, and calendars you save and subscribe to in your own app."
)

var sharedKinds = []string{"event", "party", "activity"}

func WhenCardStyle(name, tagline func() string) *sharecard.Style {
	palette := sharecard.Standard
	palette.Brand = color.RGBA{0x0e, 0x4d, 0x54, 0xff}
	palette.Yellow = color.RGBA{0xe8, 0xa3, 0x3d, 0xff}
	return &sharecard.Style{Palette: palette, Name: name, Tagline: tagline, Wordmark: "Helios When", Mark: "web/public/when/brand/logo-mark.png"}
}

type WhenPages struct {
	s      *Store
	pics   *Pictures
	bucket *blob.Bucket
	style  *sharecard.Style
	fonts  *themeFonts
}

func NewWhenPages(s *Store, pics *Pictures, style *sharecard.Style) *WhenPages {
	return &WhenPages{s: s, pics: pics, bucket: pics.bucket, style: style, fonts: newThemeFonts(pics.bucket)}
}

func (p *WhenPages) Register(mux *http.ServeMux) {
	p.registerCards(mux)
	mux.HandleFunc("GET /ext/{token}", p.extPage("web/public/when/ext.html"))
}

func (p *WhenPages) RegisterGather(mux *http.ServeMux) {
	p.registerCards(mux)
	mux.HandleFunc("GET /ext/{token}", p.extPage("web/public/gather/ext.html"))
	mux.HandleFunc("GET /preview/{token}", p.extPage("web/public/gather/ext.html"))
	mux.HandleFunc("GET /open/envelope/{id}", p.envelopePicture)
	mux.HandleFunc("GET /open/themes", p.themes)
	mux.HandleFunc("GET /open/theme-kit.zip", p.themeKit)
	mux.HandleFunc("GET "+themePictureRoute+"{id}", p.themePicture)
}

func (p *WhenPages) registerCards(mux *http.ServeMux) {
	mux.HandleFunc("GET /open/share/upcoming.png", p.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", p.shareCard)
}

func (m *Model) sharedGroup(key string) store.Row {
	groups := m.Table("GROUP")
	g, ok := groups.Get(key)
	if !ok {
		g, ok = groups.Distinct("slug", cells.NormalizePretty(key))
	}
	if !ok {
		if a, found := m.Table("ALIAS").Find(key); found {
			g, ok = groups.Get(a["target"])
		}
	}
	if !ok || !slices.Contains(sharedKinds, g["kind"]) {
		return nil
	}
	return g
}

func (p *WhenPages) PreviewHead(r *http.Request) string {
	origin := "https://" + r.Host
	key, ok := strings.CutPrefix(r.URL.Path, "/g/")
	if g := p.s.Model().sharedGroup(key); ok && g != nil {
		return p.head(g, origin)
	}
	return p.style.PreviewTags(p.style.Name(), whenAbout, origin+"/", origin+"/open/share/upcoming.png")
}

func (p *WhenPages) head(g store.Row, origin string) string {
	return p.style.PreviewTags(g["name"], eventBlurb(g), origin+g["path"], origin+"/open/share/"+g["id"]+".png")
}

func (p *WhenPages) extPage(shell string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := os.ReadFile(shell)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		m := p.s.Model()
		head := ""
		if _, g, ok := m.invitedGroup(r.PathValue("token")); ok {
			head = p.head(g, "https://"+r.Host)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write([]byte(strings.Replace(string(page), "<!--preview-->", head, 1)))
	}
}

func (p *WhenPages) shareUpcoming(w http.ResponseWriter, r *http.Request) {
	picture, err := os.ReadFile(whenHeader)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	card := sharecard.Card{
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
	p.style.Serve(w, r, card, card.Kicker, card.Title, card.Button)
}

func (p *WhenPages) shareCard(w http.ResponseWriter, r *http.Request) {
	m := p.s.Model()
	g := m.sharedGroup(strings.TrimSuffix(r.PathValue("id"), ".png"))
	if g == nil {
		http.NotFound(w, r)
		return
	}
	if g["slug"] != "" && g["theme"] != "" {
		p.invitationCard(w, r, m, g)
		return
	}
	name, whole := m.sharedPicture(g)
	kicker := m.topCategory(g)
	day, hours := whenLines(g)
	button := map[string]string{"event": "RSVP", "party": "Get Tickets", "activity": "Sign Up"}[g["kind"]]
	load := func() ([]byte, error) {
		if name == whenHeader {
			return os.ReadFile(whenHeader)
		}
		return m.cardPicture(r.Context(), p.bucket, "when pages", g, name), nil
	}
	card := sharecard.Card{Kicker: kicker, Title: g["name"], Lines: whenCardLines(day, hours, whereWords(g)), Button: button}
	serveGroupCard(w, r, p.style, card, name, whole, load, g["name"], kicker, day, hours, whereWords(g), button)
}

func (m *Model) sharedPicture(g store.Row) (string, bool) {
	if g["flyer"] != "" {
		return g["flyer"], true
	}
	groups := m.Table("GROUP")
	for n, ok := g, true; ok; n, ok = groups.Get(n["parent"]) {
		if image := m.groupPhoto(n["id"]); image != "" {
			return image, false
		}
	}
	return whenHeader, false
}

func (m *Model) topCategory(g store.Row) string {
	top := ""
	groups := m.Table("GROUP")
	for n, ok := groups.Get(g["parent"]); ok; n, ok = groups.Get(n["parent"]) {
		if n["kind"] == "category" {
			top = n["name"]
		}
	}
	return top
}
