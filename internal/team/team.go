package team

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	imageFolder = "activity-images"
	shell       = "web/team/index.html"
)

var pages = []string{
	"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
}

var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

type app struct {
	cache       *Cache
	media       *blob.Store
	directory   Directory
	superAdmins func() []string
	search      ImageSearch
	mailer      mail.Sender
	from        string
	rsvps       RSVPLookup
	lists       EmailListLookup
}

type ImageSearch = imagesearch.Search

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, directory Directory, superAdmins func() []string, search ImageSearch, mailer mail.Sender, from string, rsvps RSVPLookup, lists EmailListLookup) {
	if search.UserAgent == "" {
		search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	}
	a := app{cache: cache, media: media, directory: directory, superAdmins: superAdmins, search: search, mailer: mailer, from: from, rsvps: rsvps, lists: lists}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/team/model", a.model)
	a.search.Register(mux, "/api/team", imageFolder, imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("GET /api/team/people", a.people)
	mux.HandleFunc("POST /api/team/volunteer", a.saveVolunteer)
	mux.HandleFunc("DELETE /api/team/volunteer", a.removeVolunteer)
	mux.HandleFunc("POST /api/team/activity", a.saveActivity)
	mux.HandleFunc("POST /api/team/order", a.orderChildren)
	mux.HandleFunc("DELETE /api/team/activity", a.deleteActivity)
	mux.HandleFunc("POST /api/team/link", a.saveLink)
	mux.HandleFunc("DELETE /api/team/link", a.deleteLink)
	mux.HandleFunc("POST /api/team/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/team/category", a.deleteCategory)
	mux.HandleFunc("POST /api/team/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/team/copy", a.copyActivity)
	mux.HandleFunc("POST /api/team/settings", a.saveSettings)
	mux.HandleFunc("POST /api/team/notify", a.saveNotify)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/team/redirect", a.saveRedirect)
	mux.HandleFunc("DELETE /api/team/redirect", a.deleteRedirect)
}

func Redirected(cache *Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if to := cache.Model().Destination(r.URL.Path); to != "" {
				if r.URL.RawQuery != "" && !strings.Contains(to, "?") {
					to += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, to, http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email), Household: a.directory.Family(email)}
}

func today() string {
	return time.Now().In(local).Format(DateFormat)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	view := RenderWith(a.cache.Model(), a.directory, a.rsvps, a.lists, actor, time.Now().In(local))
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(actor.Email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode events model", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		refuse(w, err)
		return false
	}
	return true
}

type activityRef struct {
	ID string `json:"id"`
}

func newID() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func (a app) people(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.directory.People()); err != nil {
		slog.ErrorContext(r.Context(), "events: encode people", "error", err)
	}
}

func (a app) saveVolunteer(w http.ResponseWriter, r *http.Request) {
	var body volunteerBody
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	s, err := a.cache.Model().saveVolunteer(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, s.ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved volunteer", "actor", actor.Email, "action", s.action, "email", s.email, "activity", s.act.Title, "year", s.act.Year)
	a.mailSignUp(r, s.act, s.email, s.position, s.note, actor.Email, s.existed, s.was)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) removeVolunteer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	act, email, ops, err := a.cache.Model().removeVolunteer(actor, a.directory, body.ID, body.Email)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: removed volunteer", "actor", actor.Email, "email", email, "activity", act.Title, "year", act.Year)
	a.mailRemoved(r, act, email, actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

type activityBody struct {
	ID                 string     `json:"id"`
	Year               string     `json:"year"`
	Title              string     `json:"title"`
	Parent             string     `json:"parent"`
	Category           string     `json:"category"`
	Status             string     `json:"status"`
	Description        string     `json:"description"`
	Image              string     `json:"image"`
	Flyer              string     `json:"flyer"`
	Highlight          *Highlight `json:"highlight"`
	Timing             string     `json:"timing"`
	Start              string     `json:"start"`
	End                string     `json:"end"`
	Location           string     `json:"location"`
	Spots              int        `json:"spots"`
	CoLeaderNeeded     bool       `json:"coLeaderNeeded"`
	VolunteersHidden   bool       `json:"volunteersHidden"`
	VolunteersComplete bool       `json:"volunteersComplete"`
	DirectSignUp       bool       `json:"directSignUp"`
	Priority           bool       `json:"priority"`
	SignUp             string     `json:"signUp"`
	PrettyID           string     `json:"prettyId"`
	AllowAdding        string     `json:"allowAdding"`
	TakeOver           bool       `json:"takeOver"`
}

func (a app) saveActivity(w http.ResponseWriter, r *http.Request) {
	var body activityBody
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	s, err := a.cache.Model().saveActivity(actor, body)
	var conflict *prettyConflict
	if errors.As(err, &conflict) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(conflict)
		return
	}
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, s.ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved activity", "actor", actor.Email, "action", s.action, "activity", s.title, "id", s.id, "year", s.year, "status", s.status)
	if s.adding {
		a.mailNewActivity(r, a.cache.Model().Activity(s.id), actor.Email)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteActivity(w http.ResponseWriter, r *http.Request) {
	var body activityRef
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	act, ops, err := a.cache.Model().deleteActivity(actor, body.ID)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted activity", "actor", actor.Email, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	var body linkBody
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	act, action, ops, err := a.cache.Model().saveLink(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved link", "actor", actor.Email, "action", action, "link", strings.TrimSpace(body.Title), "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) orderChildren(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parent string   `json:"parent"`
		IDs    []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	parent, ops, err := a.cache.Model().orderChildren(actor, body.Parent, body.IDs)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: reordered", "actor", actor.Email, "parent", parent.Title, "year", parent.Year, "changed", len(ops))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	act, ops, err := a.cache.Model().deleteLink(actor, body.ID, body.Title)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted link", "actor", actor.Email, "link", body.Title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	var body categoryBody
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	s, err := a.cache.Model().saveCategory(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, s.op) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved category", "actor", actor.Email, "action", s.action, "category", s.title, "id", s.id, "event", s.eventID)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EventID string   `json:"eventId"`
		IDs     []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	eventID := strings.TrimSpace(body.EventID)
	ops, err := a.cache.Model().reorderCategories(actor, eventID, body.IDs)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: reordered categories", "actor", actor.Email, "event", eventID, "count", len(body.IDs))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	cat, ops, err := a.cache.deleteCategory(actor, body.ID)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted category", "actor", actor.Email, "category", cat.Title, "id", cat.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) copyActivity(w http.ResponseWriter, r *http.Request) {
	var body activityRef
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	act, id, year, ops, err := a.cache.Model().copyActivity(actor, body.ID)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: copied activity", "actor", actor.Email, "activity", act.Title, "from", act.Year, "to", year, "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExpenseFormURL string `json:"expenseFormUrl"`
		Intro          string `json:"intro"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := saveSettings(actor, body.ExpenseFormURL, body.Intro)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: changed the settings", "actor", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveNotify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kinds []string `json:"kinds"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	value, ops, err := saveNotify(actor, body.Kinds)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: set notifications", "actor", actor.Email, "kinds", value)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		refuse(w, err)
		return
	}
	prefs := a.cache.Model().notifyPrefs(actor.Email)
	notify := []string{}
	for _, k := range NotifyKinds {
		if prefs[k] {
			notify = append(notify, k)
		}
	}
	view := struct {
		Email  string   `json:"email"`
		Admins []string `json:"admins"`
		Notify []string `json:"notify"`
		Mail   bool     `json:"mail"`
	}{Email: actor.Email, Admins: a.cache.Admins(a.superAdmins()), Notify: notify, Mail: a.mailer != nil}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode events admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	admins, ops, err := a.cache.Model().setAdmins(actor, a.superAdmins(), body.Admins)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: set the admin list", "actor", actor.Email, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveRedirect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Original string `json:"original"`
		Old      string `json:"old"`
		New      string `json:"new"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	s, err := a.cache.Model().saveRedirect(actor, body.Original, body.Old, body.New)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, s.op) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved redirect", "actor", actor.Email, "action", s.action, "old", s.old, "new", s.to)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteRedirect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Old string `json:"old"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	redirect, ops, err := a.cache.Model().deleteRedirect(actor, body.Old)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted redirect", "actor", actor.Email, "old", redirect.Old)
	w.WriteHeader(http.StatusNoContent)
}
