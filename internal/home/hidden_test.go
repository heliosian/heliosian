package home

import "testing"

// A link into a hidden app is recognized by its host on the page's own tier,
// with the apex and the portal's older name both understood; a link anywhere
// else, or into an app that is not hidden, stays.
func TestHiddenHosts(t *testing.T) {
	cases := []struct {
		page, link string
		apps       []string
		hidden     bool
	}{
		{"heliosian.com", "https://who.heliosian.com/people", []string{"who"}, true},
		{"www.heliosian.com", "https://who.heliosian.com/", []string{"who"}, true},
		{"home.lab.heliosian.com", "https://celebrate.lab.heliosian.com/", []string{"celebrate"}, true},
		{"home.local.heliosian.com:8080", "https://who.local.heliosian.com:8080/", []string{"who"}, true},
		{"home.local.heliosian.com:8080", "https://WHO.local.heliosian.com:8080/", []string{"who"}, true},
		{"heliosian.com", "https://hca.heliosian.com/", []string{"team"}, true},
		{"heliosian.com", "https://team.heliosian.com/", []string{"team"}, true},
		{"heliosian.com", "https://who.heliosian.com/", []string{"team"}, false},
		{"heliosian.com", "https://who.lab.heliosian.com/", []string{"who"}, false},
		{"heliosian.com", "https://hca.run/optin", []string{"team"}, false},
		{"heliosian.com", "https://calendar.example.org/", []string{"who", "team", "celebrate"}, false},
		{"heliosian.com", "https://who.heliosian.com/", nil, false},
		{"heliosian.com", "not a url", []string{"who"}, false},
	}
	for _, c := range cases {
		if got := linksInto(hiddenHosts(c.page, c.apps), c.link); got != c.hidden {
			t.Errorf("from %s with %v hidden, %s hidden = %v, want %v", c.page, c.apps, c.link, got, c.hidden)
		}
	}
}
