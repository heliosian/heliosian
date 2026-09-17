package sharecard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"net/http"
	"strings"
)

// PreviewTags is the markup a sign-in page carries for a chat app: the Open
// Graph and Twitter tags for a site, a title, a description, the page and
// its card, and the plain description tag beside them.
func PreviewTags(site, title, desc, url, image string) string {
	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", site},
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

// ETag is the tag for a card drawn from these parts: a hash, so a chat app
// that fetched the card once need not again until something it says changes.
func ETag(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// Serve draws the card and writes it as the response, or a 304 when the
// caller's ETag is the one the client already holds.
func (s *Style) Serve(w http.ResponseWriter, r *http.Request, c Card, etag string) {
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

// Words is a Listing's titles and notes in a row, for an ETag.
func (l *Listing) Words() []string {
	out := []string{l.Heading, l.Empty}
	for _, it := range l.Items {
		out = append(out, it.Title, it.Note)
	}
	return out
}
