package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/color"
	"net/http"
	"os"
	"strings"
	"sync"

	"heliosian/internal/sharecard"

	_ "image/png"
)

// A link to Heliosian pasted into a chat app is fetched with no session, so
// what it previews comes from two public things: Open Graph tags slipped
// into the sign-in page, and a card at /open/share/apps.png drawn here - the
// mark and wordmark, a headline, and down the right the apps everyone at
// Helios has, each with its mark, its name and what it is. An app narrowed
// to a list is nobody's to preview and is left off; the tags name the same
// apps in words, each with its address on the tier the link was to.

// cardStyle is the portal's dress for the card: the page's white, the deep
// teal the site's headlines use, the lockup's olive for the tagline and the
// notes, and the four-petal mark beside the wordmark.
var cardStyle = &sharecard.Style{
	Page: color.RGBA{0xf4, 0xf8, 0xf8, 0xff}, Brand: color.RGBA{0x0f, 0x4e, 0x54, 0xff}, Accent: color.RGBA{0x6f, 0x8f, 0x1a, 0xff},
	Ink: color.RGBA{0x0a, 0x32, 0x36, 0xff}, Yellow: color.RGBA{0xfa, 0xe1, 0x05, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe8, 0xff},
	Wordmark: "Heliosian", Tagline: "Helios Community Apps",
	Mark: "web/public/home/brand/logo-mark.png",
}

// shareTitle and shareLead are the card's and the tags' words.
const (
	shareTitle = "Tools and resources for the Helios Community"
	shareLead  = "Sign in with your school Google account."
)

// sharedApps is every app the card may name: those the Visibility tab
// gives to everyone, in the switch's order, each with its name and tagline
// as the tab has them.
func (c *Cache) sharedApps() []App {
	model := c.Model()
	out := []App{}
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		if v.Mode != VisibleToEveryone {
			continue
		}
		app.Name, app.Tagline = v.Name, v.Tagline
		out = append(out, app)
	}
	return out
}

// tierOf is the tier a page's host sits on - heliosian.com from heliosian.com
// or www, lab.heliosian.com from home.lab.heliosian.com, with a local port
// carried over - which is what an app's address is built on.
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

// hostOf is an app's address on a tier: who.heliosian.com, when.heliosian.com.
func hostOf(app App, tier string) string {
	label := app.Host
	if label == "" {
		label = app.Key
	}
	return label + "." + tier
}

// PreviewHead is the Open Graph markup for any address on the portal: what
// Heliosian is, the apps everyone has, and the card that draws them. Wired
// into the sign-in page, which is what an unauthenticated fetch gets.
func PreviewHead(cache *Cache) func(r *http.Request) string {
	return func(r *http.Request) string {
		if cache.Model() == nil {
			return ""
		}
		origin := "https://" + r.Host
		tier := tierOf(r.Host)
		names := []string{}
		for _, app := range cache.sharedApps() {
			names = append(names, app.Name+" ("+strings.TrimSuffix(app.Tagline, ".")+", "+hostOf(app, tier)+")")
		}
		desc := cardStyle.TaglineText() + ". " + shareLead
		if len(names) > 0 {
			desc = cardStyle.TaglineText() + ": " + strings.Join(names, "; ") + ". " + shareLead
		}
		return previewTags(shareTitle, desc, origin+"/", origin+"/open/share/apps.png")
	}
}

// previewTags is the markup itself: the Open Graph and Twitter tags for a
// title, a description, the page and its card.
func previewTags(title, desc, url, image string) string {
	tags := [][2]string{
		{"og:type", "website"},
		{"og:site_name", "Heliosian"},
		{"og:title", title},
		{"og:description", desc},
		{"og:url", url},
		{"og:image", image},
		{"og:image:width", fmt.Sprint(sharecard.Width)},
		{"og:image:height", fmt.Sprint(sharecard.Height)},
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

// appMarks holds each app's mark, read once from the shared brand folder.
var appMarks sync.Map

func appMark(key string) image.Image {
	if v, ok := appMarks.Load(key); ok {
		img, _ := v.(image.Image)
		return img
	}
	var img image.Image
	if f, err := os.Open("web/public/common/brand/apps/" + key + ".png"); err == nil {
		defer f.Close()
		img, _, _ = image.Decode(f)
	}
	appMarks.Store(key, img)
	return img
}

// shareApps serves /open/share/apps.png: the portal's card, which names the
// apps everyone has, each with its tagline. It changes as apps are renamed,
// described or let out, so its ETag hashes what it names.
func (a app) shareApps(w http.ResponseWriter, r *http.Request) {
	apps := a.cache.sharedApps()
	parts := []string{cardStyle.TaglineText()}
	listing := &sharecard.Listing{Empty: "The apps are on their way - check back soon."}
	for _, app := range apps {
		listing.Items = append(listing.Items, sharecard.Item{Title: app.Name, Note: app.Tagline, Icon: appMark(app.Key)})
		parts = append(parts, app.Key, app.Name, app.Tagline)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	png, err := cardStyle.Draw(sharecard.Card{
		Title:    shareTitle,
		Subtitle: shareLead,
		Button:   "Open Heliosian",
		Listing:  listing,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(png)
}
