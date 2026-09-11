// Package auth gates the server behind google sign-in restricted to the school domain.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/idtoken"

	"heliosian/internal/serve"
)

const (
	Domain        = "heliosschool.org"
	apex          = "heliosian.com"
	cookieName    = "session"
	sessionLength = 30 * 24 * time.Hour
)

type contextKey struct{}

// Auth gates one app behind Google sign-in. loginPage is the app's static
// splash page under web/public, the one page anyone without a session sees;
// it fetches clientID from /auth/client rather than carrying it.
type Auth struct {
	clientID  string
	key       []byte
	loginPage string
	// Preview, when set, supplies extra <head> markup for the login page served
	// at a given path - the Open Graph tags a chat app reads to preview a link
	// that leads to sign-in. Empty for a path with nothing to preview.
	Preview func(r *http.Request) string
}

func New(clientID string, key []byte, loginPage string) *Auth {
	return &Auth{clientID: clientID, key: key, loginPage: loginPage}
}

func Email(r *http.Request) string {
	email, _ := r.Context().Value(contextKey{}).(string)
	return email
}

func Fixed(email string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, email)))
	})
}

// Public names the two endpoints the splash page needs before anyone has a
// session: the client id it initializes Google sign-in with, and the login POST.
// Public is what serves without a session: the sign-in exchange itself, and an
// app's share cards - the images a chat app fetches to preview a link, which
// no crawler could sign in for.
func Public(path string) bool {
	return path == "/auth/login" || path == "/auth/client" || strings.HasPrefix(path, "/share/")
}

func Token(key []byte, email string, expiry time.Time) string {
	payload := fmt.Sprintf("%s|%d", email, expiry.Unix())
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sign(key, payload)
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
			if strings.Contains(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/blob/") {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			a.splash(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, email)))
	})
}

// cookieDomain follows the hostname convention (<app>.<tier>.heliosian.com):
// the session is scoped one label up, so one sign-in covers every app in the
// same tier. A host outside the convention gets a host-only cookie.
func cookieDomain(host string) string {
	host, _, _ = strings.Cut(host, ":")
	if host == apex || host == "www."+apex {
		return apex
	}
	if !strings.HasSuffix(host, "."+apex) {
		return ""
	}
	_, parent, _ := strings.Cut(host, ".")
	return parent
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
	// The same page, with the preview tags slipped into its head. Varies by
	// path, so it is not cached the way the plain page is.
	page, err := os.ReadFile(a.loginPage)
	if err != nil {
		slog.ErrorContext(r.Context(), "read login page", "error", err)
		serve.File(w, r, a.loginPage)
		return
	}
	html := strings.Replace(string(page), "</head>", extra+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
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
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    Token(a.key, email, time.Now().Add(sessionLength)),
		Path:     "/",
		Domain:   cookieDomain(r.Host),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionLength.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// logoutDomains lists every domain a session cookie reaching this host could
// have been set for: host-only, and each parent up to the apex. A production
// sign-in's apex-domain cookie reaches the lab hosts too, and sessions issued
// before the cookie was scoped to a tier were host-only; signing out has to
// end all of them.
func logoutDomains(host string) []string {
	host, _, _ = strings.Cut(host, ":")
	domains := []string{""}
	if host == apex {
		return append(domains, apex)
	}
	if !strings.HasSuffix(host, "."+apex) {
		return domains
	}
	for host != apex {
		_, host, _ = strings.Cut(host, ".")
		domains = append(domains, host)
	}
	return domains
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	for _, domain := range logoutDomains(r.Host) {
		http.SetCookie(w, &http.Cookie{
			Name: cookieName, Value: "", Path: "/", Domain: domain,
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) sessionEmail(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	payload := string(decoded)
	if !hmac.Equal([]byte(sign(a.key, payload)), []byte(parts[1])) {
		return ""
	}
	fields := strings.Split(payload, "|")
	if len(fields) != 2 {
		return ""
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return ""
	}
	if !strings.HasSuffix(fields[0], "@"+Domain) {
		return ""
	}
	return fields[0]
}
