package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogoutDomains(t *testing.T) {
	cases := map[string][]string{
		"who.heliosian.com":            {"", "heliosian.com"},
		"hca.lab.heliosian.com:443":    {"", "lab.heliosian.com", "heliosian.com"},
		"heliosian.com":                {"", "heliosian.com"},
		"www.heliosian.com":            {"", "heliosian.com"},
		"who.local.heliosian.com:8080": {"", "local.heliosian.com", "heliosian.com"},
		"localhost:8080":               {""},
	}
	for host, want := range cases {
		got := logoutDomains(host)
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", host, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", host, got, want)
				break
			}
		}
	}
}

func TestLogoutClearsEveryDomain(t *testing.T) {
	a := New("client", []byte("key"), "web/public/who/login.html")
	req := httptest.NewRequest(http.MethodPost, "https://hca.lab.heliosian.com/auth/logout", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	a.logout(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want redirect", rec.Code)
	}
	domains := map[string]bool{}
	for _, c := range rec.Result().Cookies() {
		if c.Name != cookieName || c.MaxAge != -1 || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Errorf("unexpected deletion cookie %+v", c)
		}
		domains[c.Domain] = true
	}
	if !domains["heliosian.com"] || !domains["lab.heliosian.com"] || !domains[""] || len(domains) != 3 {
		t.Errorf("cleared domains %v, want host-only, the tier, and the apex", domains)
	}
}

func TestCookieDomain(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":            "heliosian.com",
		"who.lab.heliosian.com":        "lab.heliosian.com",
		"who.local.heliosian.com:8080": "local.heliosian.com",
		"heliosian.com":                "heliosian.com",
		"www.heliosian.com":            "heliosian.com",
		"localhost:8080":               "",
	}
	for host, want := range cases {
		if got := cookieDomain(host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
}
