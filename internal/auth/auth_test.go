package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLogoutDomains(t *testing.T) {
	a := New("heliosian.com", "client", []byte("key"), "web/public/who/login.html", everyone, noSessions())
	cases := map[string][]string{
		"who.heliosian.com":         {"", "heliosian.com"},
		"hca.heliosian.com:443":     {"", "heliosian.com"},
		"heliosian.com":             {"", "heliosian.com"},
		"www.heliosian.com":         {"", "heliosian.com"},
		"who.heliosiandev.com:8080": {""},
		"localhost:8080":            {""},
	}
	for host, want := range cases {
		got := a.logoutDomains(host)
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

func everyone(string) bool { return true }

type sessions struct {
	out    map[string]time.Time
	ended  []string
	refuse error
}

func noSessions() *sessions { return &sessions{out: map[string]time.Time{}} }

func (s *sessions) SignedOut(email string) (time.Time, bool) {
	at, ok := s.out[email]
	return at, ok
}

func (s *sessions) SignOut(_ context.Context, email string) error {
	if s.refuse != nil {
		return s.refuse
	}
	s.ended = append(s.ended, email)
	s.out[email] = time.Now()
	return nil
}

func TestLogoutClearsEveryDomain(t *testing.T) {
	ended := noSessions()
	a := New("heliosiandev.com", "client", []byte("key"), "web/public/who/login.html", everyone, ended)
	req := httptest.NewRequest(http.MethodPost, "https://hca.heliosiandev.com/auth/logout", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req = req.WithContext(context.WithValue(req.Context(), contextKey{}, identity{real: "admin@heliosschool.org", effective: "parent@heliosschool.org"}))
	rec := httptest.NewRecorder()
	a.logout(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d, want redirect", rec.Code)
	}
	// The sign-out is recorded against the person really signed in, never
	// the one they are viewing as.
	if len(ended.ended) != 1 || ended.ended[0] != "admin@heliosschool.org" {
		t.Errorf("signed out %v, want the admin alone", ended.ended)
	}
	// The session and the spoof both end, host-only and on the server's
	// domain, and no other domain is touched.
	domains := map[string]map[string]bool{cookieName: {}, spoofCookie: {}}
	for _, c := range rec.Result().Cookies() {
		if domains[c.Name] == nil || c.MaxAge != -1 || !c.Secure || c.SameSite != http.SameSiteLaxMode {
			t.Errorf("unexpected deletion cookie %+v", c)
			continue
		}
		domains[c.Name][c.Domain] = true
	}
	for name, cleared := range domains {
		if !cleared["heliosiandev.com"] || !cleared[""] || len(cleared) != 2 {
			t.Errorf("%s: cleared domains %v, want host-only and the domain", name, cleared)
		}
	}
}

func TestSignInAndOutRefuseOtherSites(t *testing.T) {
	s := noSessions()
	a := New("heliosian.com", "client", []byte("key"), "web/public/who/login.html", everyone, s)
	for _, site := range []string{"cross-site", "same-site", ""} {
		for _, path := range []string{"/auth/login", "/auth/logout"} {
			req := httptest.NewRequest(http.MethodPost, "https://who.heliosian.com"+path, nil)
			if site != "" {
				req.Header.Set("Sec-Fetch-Site", site)
			}
			req.AddCookie(&http.Cookie{Name: "g_csrf_token", Value: "t"})
			req = req.WithContext(context.WithValue(req.Context(), contextKey{}, identity{real: "parent@heliosschool.org", effective: "parent@heliosschool.org"}))
			rec := httptest.NewRecorder()
			if path == "/auth/login" {
				a.login(rec, req)
			} else {
				a.logout(rec, req)
			}
			if rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
				t.Errorf("%s from %q: got %d with %d cookies, want 403 and none", path, site, rec.Code, len(rec.Result().Cookies()))
			}
		}
	}
	if len(s.ended) != 0 {
		t.Errorf("signed out %v, want no one", s.ended)
	}
}

// Signed out, a stylesheet, script, image, worker or API call gets a bare
// 401, so a browser never keeps the login page under an asset's address,
// and anything else - a navigation, a chat app naming no destination, a
// service worker fetching the page for its tab, a destination never seen -
// gets the login page at the address asked for; nothing answered signed
// out may be stored.
func TestSignedOutGetsTheLoginPageOnlyForAPage(t *testing.T) {
	t.Chdir("../..")
	a := New("heliosian.com", "client", []byte("key"), "web/public/who/login.html", everyone, noSessions())
	handler := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("%s reached the app signed out", r.URL.Path)
	}))
	cases := []struct {
		path     string
		dest     string
		wantPage bool
	}{
		{"/people", "document", true},
		{"/people", "", true},
		{"/", "empty", true},
		{"/people", "made-up", true},
		{"/style.css", "style", false},
		{"/app.js", "script", false},
		{"/swoosh.png", "image", false},
		{"/fonts/missing.woff2", "font", false},
		{"/manifest.webmanifest", "manifest", false},
		{"/sw.js", "serviceworker", false},
		{"/api/directory/model", "empty", false},
		{"/api/directory/model", "document", false},
		{"/blob/photo", "document", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com"+c.path, nil)
		if c.dest != "" {
			req.Header.Set("Sec-Fetch-Dest", c.dest)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		gotPage := rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "/login.js")
		if gotPage != c.wantPage || (!c.wantPage && rec.Code != http.StatusUnauthorized) {
			t.Errorf("%s as %q: got %d, want the login page %v", c.path, c.dest, rec.Code, c.wantPage)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s as %q: Cache-Control %q, want no-store", c.path, c.dest, got)
		}
	}
}

// Signing in sets the session for the domain and deletes the host-only
// copy, so a stale one from before the cookie was scoped to the domain -
// sent first by the browser, and read first by the server - cannot shadow
// the session just set; a host outside the domain's shape gets the
// host-only cookie alone and nothing deleted.
func TestSignInDeletesTheHostOnlyCopy(t *testing.T) {
	key := []byte("key")
	a := New("heliosian.com", "client", key, "web/public/who/login.html", everyone, noSessions())
	set := func(host string) map[string]*http.Cookie {
		req := httptest.NewRequest(http.MethodPost, "https://"+host+"/auth/login", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		a.setSession(rec, req, "parent@heliosschool.org")
		out := map[string]*http.Cookie{}
		for _, c := range rec.Result().Cookies() {
			if c.Name != cookieName {
				t.Errorf("%s: unexpected cookie %+v", host, c)
				continue
			}
			out[c.Domain] = c
		}
		return out
	}
	got := set("who.heliosian.com")
	if len(got) != 2 || got["heliosian.com"] == nil || got[""] == nil {
		t.Fatalf("who: set cookies for domains %v, want the domain and a host-only deletion", got)
	}
	if c := got["heliosian.com"]; c.MaxAge != int(sessionLength.Seconds()) || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("who: domain cookie %+v, want the session", c)
	}
	req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com/api/people", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: got["heliosian.com"].Value})
	if email := a.sessionEmail(req); email != "parent@heliosschool.org" {
		t.Errorf("who: the set cookie reads as %q", email)
	}
	if c := got[""]; c.MaxAge != -1 || c.Value != "" {
		t.Errorf("who: host-only cookie %+v, want a deletion", c)
	}
	got = set("who.heliosiandev.com:8080")
	if len(got) != 1 || got[""] == nil || got[""].MaxAge != int(sessionLength.Seconds()) {
		t.Errorf("dev host: set cookies for domains %v, want the host-only session alone", got)
	}
}

// bareToken is a session cookie as it was before the cookie was JSON: the
// address, a pipe and an expiry, base64url, a dot, and their signature.
func bareToken(key []byte, email string, expiry time.Time) string {
	payload := email + "|" + strconv.FormatInt(expiry.Unix(), 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sign(key, payload)
}

// A session is the signed address and the moment it was issued, good for
// sessionLength from then unless the address signs out later: a token
// issued at or before the recorded sign-out is refused, one issued after
// it holds, one of the old shape - every cookie from before the cookie
// was JSON, whose number is an expiry still ahead - is refused, and a
// sign-out the record refuses is answered with a 500 and
// no cleared cookie, so the browser's copy is not dropped while every
// other copy still verifies.
func TestSessionEndsAtSignOut(t *testing.T) {
	key := []byte("key")
	out := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	s := noSessions()
	s.out["parent@heliosschool.org"] = out
	a := New("heliosian.com", "client", key, "web/public/who/login.html", everyone, s)
	handler := a.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	cases := []struct {
		name     string
		token    string
		wantCode int
	}{
		{"issued before the sign-out", Token(key, "parent@heliosschool.org", out.Add(-time.Second)), http.StatusUnauthorized},
		{"issued at the sign-out", Token(key, "parent@heliosschool.org", out), http.StatusUnauthorized},
		{"issued after the sign-out", Token(key, "parent@heliosschool.org", out.Add(time.Second)), http.StatusTeapot},
		{"someone never signed out", Token(key, "other@heliosschool.org", out.Add(-time.Hour)), http.StatusTeapot},
		{"run out", Token(key, "other@heliosschool.org", time.Now().Add(-sessionLength-time.Second)), http.StatusUnauthorized},
		{"another key", Token([]byte("other"), "other@heliosschool.org", time.Now()), http.StatusUnauthorized},
		{"the old shape", bareToken(key, "other@heliosschool.org", time.Now().Add(20*24*time.Hour)), http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com/api/people", nil)
			req.AddCookie(&http.Cookie{Name: cookieName, Value: c.token})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != c.wantCode {
				t.Errorf("got %d, want %d", rec.Code, c.wantCode)
			}
		})
	}
	s.refuse = context.Canceled
	req := httptest.NewRequest(http.MethodPost, "https://who.heliosian.com/auth/logout", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req = req.WithContext(context.WithValue(req.Context(), contextKey{}, identity{real: "parent@heliosschool.org", effective: "parent@heliosschool.org"}))
	rec := httptest.NewRecorder()
	a.logout(rec, req)
	if rec.Code != http.StatusInternalServerError || len(rec.Result().Cookies()) != 0 {
		t.Errorf("refused sign-out: got %d with %d cookies, want 500 and none", rec.Code, len(rec.Result().Cookies()))
	}
}

// A spoof cookie signed with the key, for the admin the session names,
// makes Email the target and keeps RealEmail the admin; one for another
// admin, one for someone the community does not list, or one the admin may
// no longer use is ignored, and Email stays the signed-in address.
func TestSpoofResolvesTheTargetOnlyWhenItHolds(t *testing.T) {
	key := []byte("key")
	a := New("heliosian.com", "client", key, "web/public/who/login.html", everyone, noSessions())
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
	issued := time.Now()
	expiry := issued.Add(time.Hour)
	cases := []struct {
		name    string
		session string
		spoof   string
		wantAs  string
	}{
		{"a valid spoof", "admin@heliosschool.org", SpoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", expiry), "parent@heliosschool.org"},
		{"an alias resolves", "admin@heliosschool.org", SpoofToken(key, "admin@heliosschool.org", "alias@heliosschool.org", expiry), "parent@heliosschool.org"},
		{"another admin's spoof", "other@heliosschool.org", SpoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", expiry), "other@heliosschool.org"},
		{"an unlisted target", "admin@heliosschool.org", SpoofToken(key, "admin@heliosschool.org", "nobody@heliosschool.org", expiry), "admin@heliosschool.org"},
		{"a run-out spoof", "admin@heliosschool.org", SpoofToken(key, "admin@heliosschool.org", "parent@heliosschool.org", time.Now().Add(-time.Minute)), "admin@heliosschool.org"},
		{"a forged spoof", "admin@heliosschool.org", SpoofToken([]byte("other"), "admin@heliosschool.org", "parent@heliosschool.org", expiry), "admin@heliosschool.org"},
		{"no spoof", "admin@heliosschool.org", "", "admin@heliosschool.org"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com/people", nil)
			req.AddCookie(&http.Cookie{Name: cookieName, Value: Token(key, c.session, issued)})
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

// A session alone admits nothing: someone the directory does not list gets
// the no-access page, or a bare 403 from a script's fetch, everywhere but
// signing out and the way to the consent form, and an admin viewing as
// someone the directory has dropped is refused as they would be. The
// sample server's fixed sign-in admits by the same check.
func TestWrapAdmitsOnlyMembers(t *testing.T) {
	t.Chdir("../..")
	key := []byte("key")
	members := map[string]bool{"parent@heliosschool.org": true, "admin@heliosschool.org": true}
	a := New("heliosian.com", "client", key, "web/public/who/login.html", func(email string) bool { return members[email] }, noSessions())
	a.Spoof = &Spoof{
		Allowed: func(email string) bool { return email == "admin@heliosschool.org" },
		Person:  func(email string) (Person, bool) { return Person{Email: email}, true },
	}
	served := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusTeapot)
	})
	issued := time.Now()
	expiry := issued.Add(time.Hour)
	call := func(handler http.Handler, session, spoof, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "https://who.heliosian.com"+path, nil)
		if session != "" {
			req.AddCookie(&http.Cookie{Name: cookieName, Value: Token(key, session, issued)})
		}
		if spoof != "" {
			req.AddCookie(&http.Cookie{Name: spoofCookie, Value: SpoofToken(key, session, spoof, expiry)})
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	cases := []struct {
		name     string
		handler  http.Handler
		session  string
		spoof    string
		path     string
		wantCode int
		wantPage bool
	}{
		{"a member's page", a.Wrap(next), "parent@heliosschool.org", "", "/people", http.StatusTeapot, false},
		{"a member's api", a.Wrap(next), "parent@heliosschool.org", "", "/api/team/people", http.StatusTeapot, false},
		{"an unlisted page", a.Wrap(next), "left@heliosschool.org", "", "/people", http.StatusForbidden, true},
		{"an unlisted static file", a.Wrap(next), "left@heliosschool.org", "", "/app.js", http.StatusForbidden, true},
		{"an unlisted api", a.Wrap(next), "left@heliosschool.org", "", "/api/team/people", http.StatusForbidden, false},
		{"an unlisted blob", a.Wrap(next), "left@heliosschool.org", "", "/blob/photo", http.StatusForbidden, false},
		{"an unlisted admin", a.Wrap(next), "left@heliosschool.org", "", "/api/admin/state", http.StatusForbidden, false},
		{"an unlisted sign-out", a.Wrap(next), "left@heliosschool.org", "", "/auth/logout", http.StatusTeapot, false},
		{"an unlisted opt-in", a.Wrap(next), "left@heliosschool.org", "", "/optin", http.StatusTeapot, false},
		{"an unlisted public path", a.Wrap(next), "left@heliosschool.org", "", "/open/share/about.png", http.StatusTeapot, false},
		{"an admin viewing as a member", a.Wrap(next), "admin@heliosschool.org", "parent@heliosschool.org", "/people", http.StatusTeapot, false},
		{"an admin viewing as someone dropped", a.Wrap(next), "admin@heliosschool.org", "dropped@heliosschool.org", "/people", http.StatusForbidden, true},
		{"a fixed member", a.Fixed("parent@heliosschool.org", next), "", "", "/people", http.StatusTeapot, false},
		{"a fixed stranger", a.Fixed("left@heliosschool.org", next), "", "", "/people", http.StatusForbidden, true},
		{"a fixed stranger's public path", a.Fixed("left@heliosschool.org", next), "", "", "/open/share/about.png", http.StatusTeapot, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := served
			rec := call(c.handler, c.session, c.spoof, c.path)
			if rec.Code != c.wantCode {
				t.Errorf("got %d, want %d", rec.Code, c.wantCode)
			}
			if c.wantCode == http.StatusTeapot && served != before+1 {
				t.Errorf("served %d, want the handler reached", served-before)
			}
			if c.wantCode != http.StatusTeapot && served != before {
				t.Errorf("served %d, want the handler never reached", served-before)
			}
			if gotPage := strings.Contains(rec.Body.String(), "/optin"); gotPage != c.wantPage {
				t.Errorf("no-access page in body = %v, want %v", gotPage, c.wantPage)
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
	a := New("heliosiandev.com", "client", key, "web/public/who/login.html", everyone, noSessions())
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
		req := httptest.NewRequest(http.MethodPost, "https://who.heliosiandev.com/auth/spoof", strings.NewReader(body))
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
	if real, target, ok := a.spoofFields(set[spoofCookie].Value); !ok || real != "admin@heliosschool.org" || target != "p3@heliosschool.org" || set[spoofCookie].Domain != "heliosiandev.com" {
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

// The session is scoped to the server's own domain and never another: a
// production sign-in never reaches a developer's names, nor the reverse.
func TestCookieDomain(t *testing.T) {
	production := New("heliosian.com", "client", []byte("key"), "web/public/who/login.html", everyone, noSessions())
	cases := map[string]string{
		"who.heliosian.com":         "heliosian.com",
		"who.heliosian.com:8080":    "heliosian.com",
		"heliosian.com":             "heliosian.com",
		"www.heliosian.com":         "heliosian.com",
		"who.heliosiandev.com":      "",
		"who.lab.heliosian.com":     "",
		"who.heliosian.com.example": "",
		"localhost:8080":            "",
	}
	for host, want := range cases {
		if got := production.cookieDomain(host); got != want {
			t.Errorf("%s: got %q, want %q", host, got, want)
		}
	}
	dev := New("heliosiandev.com", "client", []byte("key"), "web/public/who/login.html", everyone, noSessions())
	if got := dev.cookieDomain("who.heliosiandev.com:8080"); got != "heliosiandev.com" {
		t.Errorf("dev: got %q, want heliosiandev.com", got)
	}
	if got := dev.cookieDomain("who.heliosian.com"); got != "" {
		t.Errorf("dev on production's name: got %q, want host-only", got)
	}
}

// Quan mode is offered to the super admins and to anyone with "quan" in
// their address, and to nobody else.
func TestQuanFor(t *testing.T) {
	a := New("heliosian.com", "client", []byte("key"), "web/public/who/login.html", everyone, noSessions())
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
