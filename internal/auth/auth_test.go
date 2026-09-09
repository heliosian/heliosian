package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogoutClearsBothCookies(t *testing.T) {
	a := New("client", []byte("key"), "web/public/who/login.html")
	req := httptest.NewRequest(http.MethodPost, "https://who.heliosian.com/auth/logout", nil)
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
	if !domains["heliosian.com"] || !domains[""] || len(domains) != 2 {
		t.Errorf("cleared domains %v, want the shared one and the host-only one", domains)
	}
}

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
