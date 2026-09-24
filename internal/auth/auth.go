// Package auth gates the server behind google sign-in restricted to the school domain.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/idtoken"

	"heliosian/internal/serve"
)

const (
	Domain        = "heliosschool.org"
	cookieName    = "session"
	sessionLength = 30 * 24 * time.Hour
)

type contextKey struct{}

// Auth gates one app behind Google sign-in and directory membership.
// domain is the one the server answers under - its session and spoof
// cookies are scoped to it, so one sign-in covers every app there and
// reaches no other domain. loginPage is the app's static splash page under
// web/public, the one page anyone without a session sees; it fetches
// clientID from /auth/client rather than carrying it.
type Sessions interface {
	SignedOut(email string) (time.Time, bool)
	SignOut(ctx context.Context, email string) error
}

type Auth struct {
	domain    string
	clientID  string
	key       []byte
	loginPage string
	member    func(email string) bool
	sessions  Sessions
	// Preview, when set, supplies extra <head> markup for the login page served
	// at a given path - the Open Graph tags a chat app reads to preview a link
	// that leads to sign-in. Empty for a path with nothing to preview.
	Preview func(r *http.Request) string
	// Spoof, when set, lets a super admin view every app as someone else
	// (spoof.go); nil leaves Email the signed-in address always.
	Spoof *Spoof
}

func New(domain, clientID string, key []byte, loginPage string, member func(email string) bool, sessions Sessions) *Auth {
	return &Auth{domain: domain, clientID: clientID, key: key, loginPage: loginPage, member: member, sessions: sessions}
}

// Fixed signs every request in as email, with no session at all - for tests.
func Fixed(email string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, identity{real: email, effective: email})))
	})
}

// Fixed signs every request in as email with no session, the way the sample
// server does, but still honours a spoof, so Spoof Mode can be tried there.
func (a *Auth) Fixed(email string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Public(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		a.admit(w, r, a.resolve(r, email), next)
	})
}

const noAccessPage = "web/public/common/no-access.html"

// The check is on the effective identity: an admin viewing as someone the
// directory has dropped is refused as that person would be.
func (a *Auth) admit(w http.ResponseWriter, r *http.Request, id identity, next http.Handler) {
	if !a.member(id.effective) && r.URL.Path != "/auth/logout" && r.URL.Path != "/optin" {
		a.deny(w, r)
		return
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, id)))
}

// fetched is a request a page's script makes, which wants a status, not a page.
func fetched(path string) bool {
	return strings.Contains(path, "/api/") || strings.HasPrefix(path, "/blob/")
}

func (a *Auth) deny(w http.ResponseWriter, r *http.Request) {
	if fetched(r.URL.Path) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	page, err := os.ReadFile(noAccessPage)
	if err != nil {
		slog.ErrorContext(r.Context(), "read the no-access page", "error", err)
		http.Error(w, "no-access page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	if _, err := w.Write(page); err != nil {
		slog.ErrorContext(r.Context(), "write the no-access page", "error", err)
	}
}

// Public is what serves without a session: the sign-in exchange itself,
// everything under /hooks/, the callbacks the service asked other services
// for, everything under /open/, the addresses it hands out - share cards,
// the calendar's personal feeds, Loop's unsubscribe links - that a crawler,
// a calendar app or a mail client follows, each proving its caller its own
// way, and everything under /ext/, the calendar's pages for people outside
// the community it has invited, each found by its own secret.
func Public(path string) bool {
	return path == "/auth/login" || path == "/auth/client" || strings.HasPrefix(path, "/hooks/") || strings.HasPrefix(path, "/open/") || strings.HasPrefix(path, "/ext/")
}

type session struct {
	Email  string `json:"email"`
	Issued int64  `json:"issued"`
}

type token struct {
	Session   json.RawMessage `json:"session"`
	Signature string          `json:"signature"`
}

func Token(key []byte, email string, issued time.Time) string {
	payload, err := json.Marshal(session{Email: email, Issued: issued.Unix()})
	if err != nil {
		panic(err)
	}
	value, err := json.Marshal(token{Session: payload, Signature: sign(key, string(payload))})
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func sign(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/client", a.client)
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.logout)
	a.RegisterSpoof(mux)
}

func (a *Auth) client(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"clientId": a.clientID}); err != nil {
		slog.ErrorContext(r.Context(), "encode client id", "error", err)
	}
}

func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Public(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		email := a.sessionEmail(r)
		if email == "" {
			w.Header().Set("Cache-Control", "no-store")
			if !page(r) {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			a.splash(w, r)
			return
		}
		a.admit(w, r, a.resolve(r, email), next)
	})
}

// page is a request for a page to show: a browser's navigation, or a fetcher
// that names no destination - a chat app reading a link's preview. A
// stylesheet, script or image asked for while signed out must never be
// answered with the login page, which a browser would keep under that
// address and use again once signed in.
func page(r *http.Request) bool {
	if fetched(r.URL.Path) {
		return false
	}
	dest := r.Header.Get("Sec-Fetch-Dest")
	return dest == "" || dest == "document"
}

// cookieDomain is the domain the session is scoped to, so one sign-in covers
// every app under it: the server's own, for the domain itself or an app's
// name one label under it. A host outside that shape gets a host-only cookie.
func (a *Auth) cookieDomain(host string) string {
	host, _, _ = strings.Cut(host, ":")
	if host == a.domain {
		return a.domain
	}
	label, ok := strings.CutSuffix(host, "."+a.domain)
	if !ok || strings.Contains(label, ".") {
		return ""
	}
	return a.domain
}

// splash serves the login page at whatever URL was asked for, so signing in
// lands back on it. The file name is fixed, never the request path.
func (a *Auth) splash(w http.ResponseWriter, r *http.Request) {
	extra := ""
	if a.Preview != nil {
		extra = a.Preview(r)
	}
	if extra == "" {
		serve.File(w, r, a.loginPage)
		return
	}
	body, err := os.ReadFile(a.loginPage)
	if err != nil {
		slog.ErrorContext(r.Context(), "read login page", "error", err)
		http.Error(w, "login page unavailable", http.StatusInternalServerError)
		return
	}
	html := strings.Replace(string(body), "</head>", extra+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	csrf, err := r.Cookie("g_csrf_token")
	if err != nil || csrf.Value == "" || csrf.Value != r.FormValue("g_csrf_token") {
		http.Error(w, "csrf check failed", http.StatusBadRequest)
		return
	}
	payload, err := idtoken.Validate(r.Context(), r.FormValue("credential"), a.clientID)
	if err != nil {
		slog.ErrorContext(r.Context(), "validate id token", "error", err)
		http.Error(w, "invalid credential", http.StatusUnauthorized)
		return
	}
	email, _ := payload.Claims["email"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	hd, _ := payload.Claims["hd"].(string)
	if !verified || hd != Domain || !strings.HasSuffix(email, "@"+Domain) {
		http.Error(w, "account is not in the school domain", http.StatusForbidden)
		return
	}
	a.setSession(w, r, email)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// setSession sets the session cookie for the domain and deletes the copy
// under any other domain a cookie of the name could sit at for this host,
// since the browser sends every copy and the server reads the first: a
// stale one, host-only from before the cookie was scoped to the domain,
// would otherwise shadow the one just set on every request.
func (a *Auth) setSession(w http.ResponseWriter, r *http.Request, email string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	domain := a.cookieDomain(r.Host)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    Token(a.key, email, time.Now()),
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionLength.Seconds()),
	})
	for _, other := range a.logoutDomains(r.Host) {
		if other == domain {
			continue
		}
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: "", Path: "/", Domain: other,
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
}

// logoutDomains lists every domain a session cookie reaching this host could
// have been set for: host-only, and the server's domain. Sessions issued
// before the cookie was scoped to the domain were host-only; signing out has
// to end all of them.
func (a *Auth) logoutDomains(host string) []string {
	domains := []string{""}
	if domain := a.cookieDomain(host); domain != "" {
		domains = append(domains, domain)
	}
	return domains
}

// logout ends every session of the signed-in address, this browser's copy
// and any other, and any spoof with it: the next person to sign in on this
// browser must not inherit a view as someone else.
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.sessions.SignOut(r.Context(), RealEmail(r)); err != nil {
		slog.ErrorContext(r.Context(), "record sign-out", "error", err)
		http.Error(w, "sign-out not recorded", http.StatusInternalServerError)
		return
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	for _, domain := range a.logoutDomains(r.Host) {
		for _, name := range []string{cookieName, spoofCookie} {
			http.SetCookie(w, &http.Cookie{
				Name: name, Value: "", Path: "/", Domain: domain,
				HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
			})
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) sessionEmail(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return ""
	}
	var t token
	if err := json.Unmarshal(decoded, &t); err != nil {
		return ""
	}
	if !hmac.Equal([]byte(sign(a.key, string(t.Session))), []byte(t.Signature)) {
		return ""
	}
	var s session
	if err := json.Unmarshal(t.Session, &s); err != nil {
		return ""
	}
	if time.Now().Unix() > s.Issued+int64(sessionLength.Seconds()) {
		return ""
	}
	if !strings.HasSuffix(s.Email, "@"+Domain) {
		return ""
	}
	if out, ok := a.sessions.SignedOut(s.Email); ok && s.Issued <= out.Unix() {
		return ""
	}
	return s.Email
}
