package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogoutDomains(t *testing.T) {
	cases := map[string][]string{
		"who.heliosian.com":            {"", "heliosian.com"},
		"hca.local.heliosian.com:443":  {"", "local.heliosian.com", "heliosian.com"},
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
	req := httptest.NewRequest(http.MethodPost, "https://hca.local.heliosian.com/auth/logout", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	a.logout(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want redirect", rec.Code)
	}
	// The session and the spoof both end, on every domain.
	domains := map[string]map[string]bool{cookieName: {}, spoofCookie: {}}
	for _, c := range rec.Result().Cookies() {
		if domains[c.Name] == nil || c.MaxAge != -1 || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Errorf("unexpected deletion cookie %+v", c)
			continue
		}
		domains[c.Name][c.Domain] = true
	}
	for name, cleared := range domains {
		if !cleared["heliosian.com"] || !cleared["local.heliosian.com"] || !cleared[""] || len(cleared) != 3 {
			t.Errorf("%s: cleared domains %v, want host-only, the tier, and the apex", name, cleared)
		}
	}
}

// A spoof cookie signed with the key, for the admin the session names,
// makes Email the target and keeps RealEmail the admin; one for another
// admin, one for someone the community does not list, or one the admin may
// no longer use is ignored, and Email stays the signed-in address.
func TestSpoofResolvesTheTargetOnlyWhenItHolds(t *testing.T) {
	key := []byte("key")
	a := New("client", key, "web/public/who/login.html")
	allowed := map[string]bool{"admin@heliosschool.org": true}
	a.Spoof = &Spoof{
		Allowed: func(email string) bool { return allowed[email] },
		Person: func(email string) (Person, bool) {
			if email == "parent@heliosschool.org" || email == "alias@heliosschool.org" {
				return Person{Email: "parent@heliosschool.org", Name: "A Parent"}, true
			}
			return Person{}, false
		},
	}
	var got identity
	handler := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = identity{real: RealEmail(r), effective: Email(r)}
	}))
	expiry := time.Now().Add(time.Hour)
	cases := []struct {
		name    string
		session string
		spoof   string
		wantAs  string
	}{
		{"a valid spoof", "admin@heliosschool.org", spoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", expiry), "parent@heliosschool.org"},
		{"an alias resolves", "admin@heliosschool.org", spoofToken(key, "admin@heliosschool.org", "alias@heliosschool.org", expiry), "parent@heliosschool.org"},
		{"another admin's spoof", "other@heliosschool.org", spoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", expiry), "other@heliosschool.org"},
		{"an unlisted target", "admin@heliosschool.org", spoofToken(key, "admin@heliosschool.org", "nobody@heliosschool.org", expiry), "admin@heliosschool.org"},
		{"a run-out spoof", "admin@heliosschool.org", spoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", time.Now().Add(-time.Minute)), "admin@heliosschool.org"},
		{"a forged spoof", "admin@heliosschool.org", spoofToken([]byte("other"), "admin@heliosschool.org", "parent@heliosschool.org", expiry), "admin@heliosschool.org"},
		{"no spoof", "admin@heliosschool.org", "", "admin@heliosschool.org"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com/people", nil)
			req.AddCookie(&http.Cookie{Name: cookieName, Value: Token(key, c.session, expiry)})
			if c.spoof != "" {
				req.AddCookie(&http.Cookie{Name: spoofCookie, Value: c.spoof})
			}
			got = identity{}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if got.real != c.session || got.effective != c.wantAs {
				t.Errorf("got real %q as %q, want real %q as %q", got.real, got.effective, c.session, c.wantAs)
			}
		})
	}
}

// Starting a spoof sets the signed cookie for the tier and notes the person
// at the head of the recent list, five at most and never twice; stopping
// clears the spoof and leaves the list; and someone who is not allowed gets
// nothing.
func TestSetSpoofKeepsTheRecentFive(t *testing.T) {
	key := []byte("key")
	a := New("client", key, "web/public/who/login.html")
	a.Spoof = &Spoof{
		Allowed: func(email string) bool { return email == "admin@heliosschool.org" },
		Person: func(email string) (Person, bool) {
			if strings.HasPrefix(email, "p") {
				return Person{Email: email, Name: "Person " + email}, true
			}
			return Person{}, false
		},
	}
	post := func(as, body, recent string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "https://who.local.heliosian.com/auth/spoof", strings.NewReader(body))
		req.Header.Set("X-Forwarded-Proto", "https")
		req = req.WithContext(context.WithValue(req.Context(), contextKey{}, identity{real: as, effective: as}))
		if recent != "" {
			req.AddCookie(&http.Cookie{Name: recentCookie, Value: recent})
		}
		rec := httptest.NewRecorder()
		a.setSpoof(rec, req)
		return rec
	}
	cookies := func(rec *httptest.ResponseRecorder) map[string]*http.Cookie {
		out := map[string]*http.Cookie{}
		for _, c := range rec.Result().Cookies() {
			out[c.Name] = c
		}
		return out
	}
	rec := post("admin@heliosschool.org", `{"email":"p3@heliosschool.org"}`, "p1@heliosschool.org|p2@heliosschool.org|p3@heliosschool.org|p4@heliosschool.org|p5@heliosschool.org")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("start: got %d %s", rec.Code, rec.Body)
	}
	set := cookies(rec)
	if real, target, ok := a.spoofFields(set[spoofCookie].Value); !ok || real != "admin@heliosschool.org" || target != "p3@heliosschool.org" || set[spoofCookie].Domain != "local.heliosian.com" {
		t.Errorf("spoof cookie %+v reads %q as %q %v", set[spoofCookie], real, target, ok)
	}
	if got := set[recentCookie].Value; got != "p3@heliosschool.org|p1@heliosschool.org|p2@heliosschool.org|p4@heliosschool.org|p5@heliosschool.org" {
		t.Errorf("recent = %q, want p3 moved to the front", got)
	}
	rec = post("admin@heliosschool.org", `{"email":"p6@heliosschool.org"}`, set[recentCookie].Value)
	if got := cookies(rec)[recentCookie].Value; got != "p6@heliosschool.org|p3@heliosschool.org|p1@heliosschool.org|p2@heliosschool.org|p4@heliosschool.org" {
		t.Errorf("recent = %q, want p6 in front and p5 dropped", got)
	}
	rec = post("admin@heliosschool.org", `{"email":""}`, "")
	if c := cookies(rec)[spoofCookie]; rec.Code != http.StatusNoContent || c == nil || c.MaxAge != -1 {
		t.Errorf("stop: got %d, spoof cookie %+v, want it cleared", rec.Code, c)
	}
	if rec := post("admin@heliosschool.org", `{"email":"nobody@heliosschool.org"}`, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("unlisted target: got %d, want 400", rec.Code)
	}
	if rec := post("parent@heliosschool.org", `{"email":"p1@heliosschool.org"}`, ""); rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
		t.Errorf("not allowed: got %d with %d cookies, want 403 and none", rec.Code, len(rec.Result().Cookies()))
	}
}

func TestCookieDomain(t *testing.T) {
	cases := map[string]string{
		"who.heliosian.com":            "heliosian.com",
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

// Quan mode is offered to the super admins and to anyone with "quan" in
// their address, and to nobody else.
func TestQuanFor(t *testing.T) {
	a := New("client", []byte("key"), "web/public/who/login.html")
	a.Spoof = &Spoof{Allowed: func(email string) bool { return email == "admin@heliosschool.org" }}
	for email, want := range map[string]bool{
		"admin@heliosschool.org":            true,
		"quan.tran@heliosschool.org":        true,
		"Marquand@example.org":              true,
		"jordan.whitfield@heliosschool.org": false,
	} {
		if got := a.QuanFor(email); got != want {
			t.Errorf("QuanFor(%q) = %v, want %v", email, got, want)
		}
	}
}
