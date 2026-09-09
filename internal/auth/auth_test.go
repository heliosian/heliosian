package auth

import "testing"

func TestCookieDomain(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":                       "heliosian.com",
		"who.lab.heliosian.com":                   "lab.heliosian.com",
		"who.local.heliosian.com:8080":            "local.heliosian.com",
		"heliosian.com":                           "heliosian.com",
		"www.heliosian.com":                       "heliosian.com",
		"heliosian-489539474126.us-west1.run.app": "",
		"localhost:8080":                          "",
	}
	for host, want := range cases {
		if got := cookieDomain(host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
}
