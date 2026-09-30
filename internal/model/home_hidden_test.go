package model

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAppOfLink(t *testing.T) {
	for link, want := range map[string]string{
		"https://who.heliosian.com/people":         "who",
		"https://who.heliosian.com/":               "who",
		"https://celebrate.heliosiandev.com/":      "celebrate",
		"https://who.heliosiandev.com:8080/":       "who",
		"https://WHO.heliosiandev.com:8080/":       "who",
		"https://hca.heliosian.com/":               "team",
		"https://team.heliosian.com/":              "team",
		"https://cal.heliosiandev.com/e/x":         "when",
		"https://calendar.heliosian.com/":          "when",
		"https://hca.run/optin":                    "",
		"https://calendar.example.org/":            "",
		"https://heliosian.com/":                   "",
		"https://home.heliosian.com/":              "",
		"https://who.heliosian.com.example.org/":   "",
		"https://birthday.heliosiandev.com/staff/": "birthday",
		"not a url": "",
		"":          "",
	} {
		if got := appOfLink(link); got != want {
			t.Errorf("%q points into %q, want %q", link, got, want)
		}
	}
}

func TestLinksIntoAHiddenAppAreLeftOut(t *testing.T) {
	c, _, mux := homeServer(t)
	if app := c.Model().Home.link(directoryID).URL; appOfLink(app) != "who" {
		t.Fatalf("the sample Directory link %s does not point into who", app)
	}
	links := func(as string) []string {
		ids := []string{}
		json.Unmarshal(read(t, mux, as, "/api/links").Result, &ids)
		return ids
	}
	const member = "robin.whitfield@heliosschool.org"
	if !slices.Contains(links(member), directoryID) || !slices.Contains(links(homeAdmin), directoryID) {
		t.Fatalf("the Directory is hidden while who is everyone's")
	}
	setAppVisibility(t, c, "who", AppVisibilityRow{Mode: VisibleToList, Emails: []string{member}, Order: "2"})
	if !slices.Contains(links(member), directoryID) {
		t.Errorf("the Directory is hidden from someone who is listed for who")
	}
	if got := links(homeAdmin); slices.Contains(got, directoryID) || !slices.Contains(got, parentPortalID) {
		t.Errorf("an admin kept from who still sees the Directory, or lost the rest: %v", got)
	}
	if rec := read(t, mux, homeAdmin, "/api/link-categories/"+schoolID+"?include=links"); slices.ContainsFunc(rec.one(t, "link-categories", schoolID)["links"].([]any), func(l any) bool { return l == directoryID }) {
		t.Errorf("the School section still lists the Directory for someone kept from who")
	}
}
