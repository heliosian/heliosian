package home

import (
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

// CardStyle is the portal's dress for the card: the standard palette with
// the lockup's olive for the tagline and the notes, and the four-petal mark
// beside the wordmark.
func CardStyle(name, tagline func() string) *sharecard.Style {
	palette := sharecard.Standard
	palette.Accent = color.RGBA{0x6f, 0x8f, 0x1a, 0xff}
	return &sharecard.Style{Palette: palette, Name: name, Tagline: tagline, Wordmark: "Heliosian", Mark: "web/public/home/brand/logo-mark.png"}
}

// shareTitle and shareLead are the card's and the tags' words.
const (
	shareTitle = "Tools and resources for the Helios Community"
	shareLead  = "Sign in with your school Google account."
)

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
// or www, heliosiandev.com from home.heliosiandev.com, with a local port
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
func PreviewHead(cache *Cache, style *sharecard.Style) func(r *http.Request) string {
	return func(r *http.Request) string {
		origin := "https://" + r.Host
		tier := tierOf(r.Host)
		names := []string{}
		for _, app := range cache.sharedApps() {
			names = append(names, app.Name+" ("+strings.TrimSuffix(app.Tagline, ".")+", "+hostOf(app, tier)+")")
		}
		desc := style.Tagline() + ". " + shareLead
		if len(names) > 0 {
			desc = style.Tagline() + ": " + strings.Join(names, "; ") + ". " + shareLead
		}
		return style.PreviewTags(shareTitle, desc, origin+"/", origin+"/open/share/apps.png")
	}
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
	parts := []string{}
	listing := &sharecard.Listing{Empty: "The apps are on their way - check back soon."}
	for _, app := range a.cache.sharedApps() {
		listing.Items = append(listing.Items, sharecard.Item{Title: app.Name, Note: app.Tagline, Icon: appMark(app.Key)})
		parts = append(parts, app.Key, app.Name, app.Tagline)
	}
	card := sharecard.Card{Title: shareTitle, Subtitle: shareLead, Button: "Open Heliosian", Listing: listing}
	a.style.Serve(w, r, card, parts...)
}
