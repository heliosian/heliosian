package sharecard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/lru"
)

func (s *Style) PreviewTags(title, desc, url, image string) string {
	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", s.Name()},
		{"og:title", title},
		{"og:description", desc},
		{"og:url", url},
		{"og:image", image},
		{"og:image:width", fmt.Sprint(Width)},
		{"og:image:height", fmt.Sprint(Height)},
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

func ETag(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

const cardCacheBudget = 32 << 20

var cardCache = newCardCache()

func newCardCache() *lru.Cache[string, []byte] {
	return lru.New[string, []byte](cardCacheBudget, func(png []byte) int { return len(png) })
}

func (s *Style) cardTag(parts ...string) (string, string) {
	name, tagline := s.Name(), s.Tagline()
	return ETag(slices.Concat(parts, []string{name, tagline})...), tagline
}

func writeCard(w http.ResponseWriter, etag string, png []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(png)
}

func (s *Style) Answered(w http.ResponseWriter, r *http.Request, parts ...string) bool {
	etag, _ := s.cardTag(parts...)
	return s.answer(w, r, etag)
}

func (s *Style) answer(w http.ResponseWriter, r *http.Request, etag string) bool {
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	png, ok := cardCache.Get(etag)
	if !ok {
		return false
	}
	writeCard(w, etag, png)
	return true
}

func (s *Style) Serve(w http.ResponseWriter, r *http.Request, c Card, parts ...string) {
	s.serve(w, r, c, true, parts...)
}

// ServeOnce writes this request's card and leaves it out of the cache. A picture that did not
// load shares its etag with the card that has the picture, so keeping it would hide the picture.
func (s *Style) ServeOnce(w http.ResponseWriter, r *http.Request, c Card, parts ...string) {
	s.serve(w, r, c, false, parts...)
}

func (s *Style) serve(w http.ResponseWriter, r *http.Request, c Card, keep bool, parts ...string) {
	etag, tagline := s.cardTag(parts...)
	if s.answer(w, r, etag) {
		return
	}
	png, err := s.draw(c, tagline)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if keep {
		cardCache.Put(etag, png)
	}
	writeCard(w, etag, png)
}

func (l *Listing) Words() []string {
	out := []string{l.Heading, l.Empty}
	for _, it := range l.Items {
		out = append(out, it.Title, it.Note)
	}
	return out
}

type About struct {
	Style   *Style
	Desc    string
	Listing Listing
	Button  string
}

func (a *About) PreviewHead(r *http.Request) string {
	origin := "https://" + r.Host
	return a.Style.PreviewTags(a.Style.Name(), a.Style.Tagline()+". "+a.Desc, origin+"/", origin+"/open/share/about.png")
}

func (a *About) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	card := Card{Title: a.Style.Name(), Subtitle: a.Style.Tagline(), Button: a.Button, Listing: &a.Listing}
	a.Style.Serve(w, r, card, a.Listing.Words()...)
}
