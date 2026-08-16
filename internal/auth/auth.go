// Package auth gates the server behind google sign-in restricted to the school domain.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/idtoken"
)

const (
	Domain        = "heliosschool.org"
	cookieName    = "session"
	sessionLength = 30 * 24 * time.Hour
)

type contextKey struct{}

type Auth struct {
	clientID string
	key      []byte
}

func New(clientID string, key []byte) *Auth {
	return &Auth{clientID: clientID, key: key}
}

func Email(r *http.Request) string {
	email, _ := r.Context().Value(contextKey{}).(string)
	return email
}

func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.logout)
}

func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		email := a.sessionEmail(r)
		if email == "" {
			if strings.Contains(r.URL.Path, "/api/") {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			a.loginPage(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, email)))
	})
}

func (a *Auth) loginPage(w http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFiles("web/login.html")
	if err != nil {
		log.Printf("[ERROR] parse login page: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	err = t.Execute(w, map[string]string{
		"ClientID": a.clientID,
		"LoginURI": scheme + "://" + r.Host + "/auth/login",
	})
	if err != nil {
		log.Printf("[ERROR] render login page: %v", err)
	}
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	csrf, err := r.Cookie("g_csrf_token")
	if err != nil || csrf.Value == "" || csrf.Value != r.FormValue("g_csrf_token") {
		http.Error(w, "csrf check failed", http.StatusBadRequest)
		return
	}
	payload, err := idtoken.Validate(r.Context(), r.FormValue("credential"), a.clientID)
	if err != nil {
		log.Printf("[ERROR] validate id token: %v", err)
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
	expiry := time.Now().Add(sessionLength).Unix()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    a.token(email, expiry),
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionLength.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Auth) token(email string, expiry int64) string {
	payload := fmt.Sprintf("%s|%d", email, expiry)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + a.sign(payload)
}

func (a *Auth) sign(payload string) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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
	if !hmac.Equal([]byte(a.sign(payload)), []byte(parts[1])) {
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
