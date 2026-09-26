package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Spoof Mode lets a super admin view, and act in, every app as someone
// else. It lives beside sign-in because it is a question of identity: the
// spoof is a second signed cookie next to the session's, on the same
// tier-wide domain, naming who is really signed in and who they are viewing
// as, and Wrap resolves it once so Email answers with the person being
// viewed as in every app alike - reads and writes both, since what the admin
// sees and can do should be exactly what that person would. RealEmail keeps
// the signed-in admin for the log and for the switch itself. A spoof lasts
// a day at most, though Stop ends it sooner, and the last few people viewed
// as ride along in a third cookie for the toolbar's menu.
const (
	spoofCookie  = "spoof"
	recentCookie = "spoofed"
	spoofLength  = 24 * time.Hour
	recentLength = 365 * 24 * time.Hour
	recentKept   = 5
)

// Spoof is what the switch needs to know about the community: who may view
// as another, who anyone is, and everyone there is to search.
type Spoof struct {
	// Allowed says whether the signed-in address may view as another.
	Allowed func(email string) bool
	// Person finds someone by any of their addresses - the row for them,
	// under the address they are known by - or false for nobody the
	// community lists.
	Person func(email string) (Person, bool)
	// People lists everyone who may be viewed as, for the search.
	People func() []Person
}

// Person is one row of the switch's menu and search.
type Person struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	// Words places them - a job, a grade, or Parent - beside the name.
	Words string `json:"words,omitempty"`
}

// identity is what a request carries: who is signed in, and who the apps
// act as - the same address unless a spoof is on.
type identity struct {
	real      string
	effective string
}

// Email is who the apps act as: the signed-in address, or the person a super
// admin is viewing as.
func Email(r *http.Request) string {
	id, _ := r.Context().Value(contextKey{}).(identity)
	return id.effective
}

// RealEmail is who is actually signed in, whatever Email says.
func RealEmail(r *http.Request) string {
	return RealEmailFrom(r.Context())
}

func RealEmailFrom(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(identity)
	return id.real
}

// Spoofing is who the signed-in person is viewing as, or "" for themselves.
func Spoofing(r *http.Request) string {
	id, _ := r.Context().Value(contextKey{}).(identity)
	if id.effective == id.real {
		return ""
	}
	return id.effective
}

// resolve is the identity a request runs as: the signed-in address, made
// the spoof target when a valid spoof cookie names this admin - signed with
// the same key, unexpired, and for someone the community still lists - and
// they are still allowed to.
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

// spoofFields reads a spoof cookie back: the admin and the target, when the
// signature holds and the spoof has not run out.
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

// RegisterSpoof wires the switch: what the toolbar shows, the people it
// searches, and the start or stop. All three sit behind sign-in, and all
// three answer for the signed-in admin rather than whoever they are viewing
// as, so the way back is never locked out by the spoof it ends.
func (a *Auth) RegisterSpoof(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/spoof", a.spoofState)
	mux.HandleFunc("GET /auth/spoof/people", a.spoofPeople)
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

// QuanFor says whether the signed-in address is offered Quan mode - the
// pink-and-lime easter egg (web/common/quan.css) - as a choice in its own
// right in the user menu: the super admins, and anyone with "quan" in
// their address. Nobody starts on it.
func (a *Auth) QuanFor(email string) bool {
	return (a.Spoof != nil && a.Spoof.Allowed(email)) || strings.Contains(strings.ToLower(email), "quan")
}

// spoofState is what the toolbar draws: whether the switch shows at all,
// who is viewing as whom, and the last few viewed as - the ones the
// community still lists, most recent first - and, riding along, whether
// the signed-in person is offered Quan mode.
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode spoof state", "error", err)
	}
}

func (a *Auth) spoofPeople(w http.ResponseWriter, r *http.Request) {
	if !a.canSpoof(r) {
		http.Error(w, "super admin access required", http.StatusForbidden)
		return
	}
	people := a.Spoof.People()
	if people == nil {
		people = []Person{}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(people); err != nil {
		slog.ErrorContext(r.Context(), "encode spoof people", "error", err)
	}
}

// setSpoof starts viewing as the address in the body, or stops with a blank
// one. Starting notes the person at the head of the recent list; viewing as
// oneself is the same as stopping.
func (a *Auth) setSpoof(w http.ResponseWriter, r *http.Request) {
	if !a.canSpoof(r) {
		http.Error(w, "super admin access required", http.StatusForbidden)
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	real := RealEmail(r)
	target := strings.ToLower(strings.TrimSpace(body.Email))
	if target != "" {
		p, ok := a.Spoof.Person(target)
		if !ok {
			http.Error(w, "nobody in the directory has that address", http.StatusBadRequest)
			return
		}
		target = p.Email
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	domain := a.cookieDomain(r.Host)
	if target == "" || strings.EqualFold(target, real) {
		http.SetCookie(w, &http.Cookie{
			Name: spoofCookie, Value: "", Path: "/", Domain: domain,
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
		})
		slog.InfoContext(r.Context(), "spoof: stopped", "admin", real)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     spoofCookie,
		Value:    SpoofToken(a.key, real, target, time.Now().Add(spoofLength)),
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(spoofLength.Seconds()),
	})
	recent := slices.DeleteFunc(a.recent(r), func(e string) bool { return e == target })
	recent = append([]string{target}, recent...)
	if len(recent) > recentKept {
		recent = recent[:recentKept]
	}
	http.SetCookie(w, &http.Cookie{
		Name:     recentCookie,
		Value:    strings.Join(recent, "|"),
		Path:     "/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(recentLength.Seconds()),
	})
	slog.InfoContext(r.Context(), "spoof: started", "admin", real, "as", target)
	w.WriteHeader(http.StatusNoContent)
}

// recent is the addresses last viewed as, most recent first, as the cookie
// holds them.
func (a *Auth) recent(r *http.Request) []string {
	cookie, err := r.Cookie(recentCookie)
	if err != nil || cookie.Value == "" {
		return nil
	}
	return strings.Split(cookie.Value, "|")
}
