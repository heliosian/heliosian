package model

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

func HomeCardStyle(name, tagline func() string) *sharecard.Style {
	palette := sharecard.Standard
	palette.Accent = color.RGBA{0x6f, 0x8f, 0x1a, 0xff}
	return &sharecard.Style{Palette: palette, Name: name, Tagline: tagline, Wordmark: "Heliosian", Mark: "web/public/home/brand/logo-mark.png"}
}

const (
	homeShareTitle = "Tools and resources for the Helios Community"
	homeShareLead  = "Sign in with your school Google account."
)

func (c *HomeCache) sharedApps() []App {
	m := c.Model()
	out := []App{}
	for _, app := range orderedApps(m) {
		v := appVisibilityOf(m, app)
		if v.Mode != VisibleToEveryone {
			continue
		}
		app.Name, app.Tagline = v.Name, v.Tagline
		out = append(out, app)
	}
	return out
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

func hostOf(app App, tier string) string {
	return Qualify(app.Hosts[0], tier)
}

func HomePreviewHead(cache *HomeCache, style *sharecard.Style) func(r *http.Request) string {
	return func(r *http.Request) string {
		origin := "https://" + r.Host
		tier := tierOf(r.Host)
		names := []string{}
		for _, app := range cache.sharedApps() {
			names = append(names, app.Name+" ("+strings.TrimSuffix(app.Tagline, ".")+", "+hostOf(app, tier)+")")
		}
		desc := style.Tagline() + ". " + homeShareLead
		if len(names) > 0 {
			desc = style.Tagline() + ": " + strings.Join(names, "; ") + ". " + homeShareLead
		}
		return style.PreviewTags(homeShareTitle, desc, origin+"/", origin+"/open/share/apps.png")
	}
}

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

func (a homeApp) shareApps(w http.ResponseWriter, r *http.Request) {
	parts := []string{}
	listing := &sharecard.Listing{Empty: "The apps are on their way - check back soon."}
	for _, app := range a.cache.sharedApps() {
		listing.Items = append(listing.Items, sharecard.Item{Title: app.Name, Note: app.Tagline, Icon: appMark(app.Key)})
		parts = append(parts, app.Key, app.Name, app.Tagline)
	}
	card := sharecard.Card{Title: homeShareTitle, Subtitle: homeShareLead, Button: "Open Heliosian", Listing: listing}
	a.style.Serve(w, r, card, parts...)
}
