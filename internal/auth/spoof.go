package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
)

const (
	spoofCookie  = "spoof"
	recentCookie = "spoofed"
	spoofLength  = 24 * time.Hour
	recentLength = 365 * 24 * time.Hour
	recentKept   = 5
)

type Spoof struct {
	Allowed func(email string) bool
	Person  func(email string) (Person, bool)
}

type Person struct {
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Words    string `json:"words,omitempty"`
}

type identity struct {
	real      string
	effective string
}

func Email(r *http.Request) string {
	id, _ := r.Context().Value(contextKey{}).(identity)
	return id.effective
}

func RealEmail(r *http.Request) string {
	return RealEmailFrom(r.Context())
}

func RealEmailFrom(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(identity)
	return id.real
}

func Spoofing(r *http.Request) string {
	id, _ := r.Context().Value(contextKey{}).(identity)
	if id.effective == id.real {
		return ""
	}
	return id.effective
}

func (a *Auth) resolve(r *http.Request, real string) identity {
	id := identity{real: real, effective: real}
	if a.Spoof == nil {
		return id
	}
	cookie, err := r.Cookie(spoofCookie)
	if err != nil {
		return id
	}
	admin, target, ok := a.spoofFields(cookie.Value)
	if !ok || !strings.EqualFold(admin, real) || !a.Spoof.Allowed(real) {
		return id
	}
	p, ok := a.Spoof.Person(target)
	if !ok {
		return id
	}
	id.effective = p.Email
	return id
}

func SpoofToken(key []byte, real, target string, expiry time.Time) string {
	payload := fmt.Sprintf("%s|%s|%d", real, target, expiry.Unix())
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sign(key, payload)
}

func (a *Auth) spoofFields(value string) (real, target string, ok bool) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", false
	}
	payload := string(decoded)
	if !verify(a.key, payload, parts[1]) {
		return "", "", false
	}
	fields := strings.Split(payload, "|")
	if len(fields) != 3 {
		return "", "", false
	}
	expiry, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return "", "", false
	}
	return fields[0], fields[1], true
}

func (a *Auth) RegisterSpoof(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/spoof", a.spoofState)
	mux.HandleFunc("POST /auth/spoof", a.setSpoof)
}

func (a *Auth) canSpoof(r *http.Request) bool {
	return a.Spoof != nil && a.Spoof.Allowed(RealEmail(r))
}

func (a *Auth) person(email string) *Person {
	p, ok := a.Spoof.Person(email)
	if !ok {
		return nil
	}
	return &p
}

func (a *Auth) QuanFor(email string) bool {
	return (a.Spoof != nil && a.Spoof.Allowed(email)) || strings.Contains(strings.ToLower(email), "quan")
}

func (a *Auth) spoofState(w http.ResponseWriter, r *http.Request) {
	view := struct {
		CanSpoof bool     `json:"canSpoof"`
		Quan     bool     `json:"quan"`
		Real     *Person  `json:"real,omitempty"`
		Spoofing *Person  `json:"spoofing,omitempty"`
		Recent   []Person `json:"recent"`
	}{Recent: []Person{}, Quan: a.QuanFor(RealEmail(r))}
	if a.canSpoof(r) {
		view.CanSpoof = true
		view.Real = a.person(RealEmail(r))
		if target := Spoofing(r); target != "" {
			view.Spoofing = a.person(target)
		}
		for _, email := range a.recent(r) {
			if p := a.person(email); p != nil {
				view.Recent = append(view.Recent, *p)
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, http.StatusOK, view)
}

func (a *Auth) setSpoof(w http.ResponseWriter, r *http.Request) {
	if !a.canSpoof(r) {
		serve.Error(w, r, access.Forbidden("super admin access required"))
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !serve.Decode(w, r, &body) {
		return
	}
	real := RealEmail(r)
	target := strings.ToLower(strings.TrimSpace(body.Email))
	if target != "" {
		p, ok := a.Spoof.Person(target)
		if !ok {
			serve.Error(w, r, access.Invalid("nobody in the directory has that address"))
			return
		}
		target = p.Email
	}
	domain := a.cookieDomain(r.Host)
	if target == "" || strings.EqualFold(target, real) {
		setCookie(w, spoofCookie, "", domain, -1)
		slog.InfoContext(r.Context(), "spoof: stopped", "admin", real)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	setCookie(w, spoofCookie, SpoofToken(a.key, real, target, time.Now().Add(spoofLength)), domain, int(spoofLength.Seconds()))
	recent := slices.DeleteFunc(a.recent(r), func(e string) bool { return e == target })
	recent = append([]string{target}, recent...)
	if len(recent) > recentKept {
		recent = recent[:recentKept]
	}
	setCookie(w, recentCookie, strings.Join(recent, "|"), domain, int(recentLength.Seconds()))
	slog.InfoContext(r.Context(), "spoof: started", "admin", real, "as", target)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) recent(r *http.Request) []string {
	cookie, err := r.Cookie(recentCookie)
	if err != nil || cookie.Value == "" {
		return nil
	}
	return strings.Split(cookie.Value, "|")
}
