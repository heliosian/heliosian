package model

import (
	"net/http"
	"testing"

	"heliosian/internal/testkit"
)

func TestPortalPreview(t *testing.T) {
	c, _ := sampleHomeCache(t)
	t.Chdir("../..")
	style := HomeCardStyle(func() string { return HomeApp.Name }, func() string { return HomeApp.Tagline })
	testkit.Previews(t, HomePreviewHead(c, style), testkit.Preview{
		URL: "https://home.heliosiandev.com/",
		Want: []string{`og:title" content="Tools and resources for the Helios Community"`, `og:url" content="https://home.heliosiandev.com/"`,
			`og:image" content="https://home.heliosiandev.com/open/share/apps.png"`, "Helios Who? (A visual directory, who.heliosiandev.com)", "Helios Calendar (The school year, day by day, when.heliosiandev.com)"},
		Never: []string{"Celebrate", "Birthday"},
	})
	if got := tierOf("heliosian.com"); got != "heliosian.com" {
		t.Errorf("tier of heliosian.com = %q", got)
	}
	if got := tierOf("home.heliosiandev.com:8080"); got != "heliosiandev.com:8080" {
		t.Errorf("tier of the local host = %q", got)
	}

	a := homeApp{store: c, style: style}
	testkit.Cards(t, http.HandlerFunc(a.shareApps), []string{"https://heliosian.com/open/share/apps.png"})
}
