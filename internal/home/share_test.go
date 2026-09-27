package home

import (
	"net/http"
	"testing"

	"heliosian/internal/testkit"
)

func TestPortalPreview(t *testing.T) {
	c, _ := sampleCache(t)
	t.Chdir("../..")
	style := CardStyle(func() string { return Home.Name }, func() string { return Home.Tagline })
	testkit.Previews(t, PreviewHead(c, style), testkit.Preview{
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

	a := app{cache: c, style: style}
	testkit.Cards(t, http.HandlerFunc(a.shareApps), []string{"https://heliosian.com/open/share/apps.png"})
}
