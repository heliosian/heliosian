package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"slices"
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

type Sessions interface {
	SignedOut(email string) (time.Time, bool)
	SignOut(ctx context.Context, email string) error
}

type Login struct {
	Title string
}

const loginTemplate = "web/templates/login.html"

type Auth struct {
	domain    string
	clientID  string
	key       []byte
	page      Login
	member    func(email string) bool
	nonMember []string
	sessions  Sessions
	Preview   func(r *http.Request) string
	Spoof     *Spoof
}

func New(domain, clientID string, key []byte, login Login, member func(email string) bool, nonMember []string, sessions Sessions) *Auth {
	return &Auth{domain: domain, clientID: clientID, key: key, page: login, member: member, nonMember: nonMember, sessions: sessions}
}

func Fixed(email string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, identity{real: email, effective: email})))
	})
}

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

func (a *Auth) admit(w http.ResponseWriter, r *http.Request, id identity, next http.Handler) {
	if !a.member(id.effective) && r.URL.Path != "/auth/logout" && !slices.Contains(a.nonMember, r.URL.Path) {
		a.deny(w, r)
		return
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, id)))
}

func fetched(path string) bool {
	return strings.Contains(path, "/api/")
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

func verify(key []byte, payload, signature string) bool {
	return hmac.Equal([]byte(sign(key, payload)), []byte(signature))
}

func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/client", serve.JSON(a.client))
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.logout)
	a.RegisterSpoof(mux)
}

func (a *Auth) client(r *http.Request, _ serve.None) (map[string]string, error) {
	return map[string]string{"clientId": a.clientID}, nil
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
			slog.InfoContext(r.Context(), "signed out", "path", r.URL.Path, "page", page(r),
				"dest", r.Header.Get("Sec-Fetch-Dest"), "mode", r.Header.Get("Sec-Fetch-Mode"), "site", r.Header.Get("Sec-Fetch-Site"))
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

var assetDests = map[string]bool{
	"style": true, "script": true, "image": true, "font": true,
	"audio": true, "video": true, "track": true, "manifest": true,
	"object": true, "embed": true,
	"worker": true, "sharedworker": true, "serviceworker": true,
}

func page(r *http.Request) bool {
	if fetched(r.URL.Path) {
		return false
	}
	return !assetDests[r.Header.Get("Sec-Fetch-Dest")]
}

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

func (a *Auth) splash(w http.ResponseWriter, r *http.Request) {
	head := ""
	if a.Preview != nil {
		head = a.Preview(r)
	}
	page, err := template.ParseFiles(loginTemplate)
	if err != nil {
		slog.ErrorContext(r.Context(), "read the login page", "error", err)
		http.Error(w, "login page unavailable", http.StatusInternalServerError)
		return
	}
	var body bytes.Buffer
	if err := page.Execute(&body, struct {
		Login
		Head template.HTML
	}{a.page, template.HTML(head)}); err != nil {
		slog.ErrorContext(r.Context(), "render the login page", "error", err)
		http.Error(w, "login page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	serve.Content(w, r, loginTemplate, body.Bytes())
}

func sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if site == "same-origin" || site == "none" {
		return true
	}
	http.Error(w, "cross-site request refused", http.StatusForbidden)
	return false
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
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

func setCookie(w http.ResponseWriter, name, value, domain string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

func (a *Auth) setSession(w http.ResponseWriter, r *http.Request, email string) {
	domain := a.cookieDomain(r.Host)
	setCookie(w, cookieName, Token(a.key, email, time.Now()), domain, int(sessionLength.Seconds()))
	for _, other := range a.logoutDomains(r.Host) {
		if other == domain {
			continue
		}
		setCookie(w, cookieName, "", other, -1)
	}
}

func (a *Auth) logoutDomains(host string) []string {
	domains := []string{""}
	if domain := a.cookieDomain(host); domain != "" {
		domains = append(domains, domain)
	}
	return domains
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	if err := a.sessions.SignOut(r.Context(), RealEmail(r)); err != nil {
		slog.ErrorContext(r.Context(), "record sign-out", "error", err)
		http.Error(w, "sign-out not recorded", http.StatusInternalServerError)
		return
	}
	for _, domain := range a.logoutDomains(r.Host) {
		for _, name := range []string{cookieName, spoofCookie} {
			setCookie(w, name, "", domain, -1)
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
	if !verify(a.key, string(t.Session), t.Signature) {
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
