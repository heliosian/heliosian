package sharecard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strings"
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

func (s *Style) Serve(w http.ResponseWriter, r *http.Request, c Card, parts ...string) {
	etag := ETag(slices.Concat(parts, []string{s.Name(), s.Tagline()})...)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	png, err := s.Draw(c)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(png)
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
