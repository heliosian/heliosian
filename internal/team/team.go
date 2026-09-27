package team

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

const (
	imageFolder = "activity-images"
	shell       = "web/team/index.html"
)

var pages = []string{
	"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
}

type app struct {
	cache     *Cache
	media     *blob.Store
	directory func() *who.Model
	settings  func() *config.Settings
	search    imagesearch.Search
	mailer    mail.Sender
	from      string
	rsvps     RSVPLookup
	lists     EmailListLookup
	style     *sharecard.Style
}

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, directory func() *who.Model, settings func() *config.Settings, search imagesearch.Search, mailer mail.Sender, from string, rsvps RSVPLookup, lists EmailListLookup, style *sharecard.Style) {
	if search.UserAgent == "" {
		search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	}
	a := app{cache: cache, media: media, directory: directory, settings: settings, search: search, mailer: mailer, from: from, rsvps: rsvps, lists: lists, style: style}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/team/model", serve.JSON(a.model))
	a.search.Register(mux, "/api/team", imageFolder, imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("GET /api/team/people", serve.JSON(a.people))
	mux.HandleFunc("POST /api/team/volunteer", serve.JSON(a.saveVolunteer))
	mux.HandleFunc("DELETE /api/team/volunteer", serve.JSON(a.removeVolunteer))
	mux.HandleFunc("POST /api/team/activity", serve.JSON(a.saveActivity))
	mux.HandleFunc("POST /api/team/order", serve.JSON(a.orderChildren))
	mux.HandleFunc("DELETE /api/team/activity", serve.JSON(a.deleteActivity))
	mux.HandleFunc("POST /api/team/link", serve.JSON(a.saveLink))
	mux.HandleFunc("DELETE /api/team/link", serve.JSON(a.deleteLink))
	mux.HandleFunc("POST /api/team/category", serve.JSON(a.saveCategory))
	mux.HandleFunc("DELETE /api/team/category", serve.JSON(a.deleteCategory))
	mux.HandleFunc("POST /api/team/categories/order", serve.JSON(a.reorderCategories))
	mux.HandleFunc("POST /api/team/copy", serve.JSON(a.copyActivity))
	mux.HandleFunc("POST /api/team/settings", serve.JSON(a.saveSettings))
	mux.HandleFunc("POST /api/team/notify", serve.JSON(a.saveNotify))
	admins.Register(mux, "team", cache.List, a.actor, a.adminState)
	mux.HandleFunc("POST /api/team/redirect", serve.JSON(a.saveRedirect))
	mux.HandleFunc("DELETE /api/team/redirect", serve.JSON(a.deleteRedirect))
}

func Redirected(cache *Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if to := cache.Model().Destination(r.URL.Path); to != "" {
				if strings.HasPrefix(to, "/") {
					to = "https://" + r.Host + to
				}
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
	directory := a.directory()
	email := directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email), Household: directory.Family(email)}
}

func today() string {
	return time.Now().In(when.Location).Format(DateFormat)
}

func (a app) model(r *http.Request, _ serve.None) (View, error) {
	actor := a.actor(r)
	view := RenderWith(a.cache.Model(), a.directory(), a.settings(), a.rsvps, a.lists, actor, time.Now().In(when.Location))
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(actor.Email)
	return view, nil
}

type activityRef struct {
	ID string `json:"id"`
}

func (a app) people(r *http.Request, _ serve.None) ([]DirectoryPerson, error) {
	return directoryPeople(a.directory()), nil
}

func (a app) saveVolunteer(r *http.Request, body volunteerBody) (serve.None, error) {
	actor := a.actor(r)
	s, err := a.cache.Model().saveVolunteer(actor, a.directory(), body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, s.ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved volunteer", "actor", actor.Email, "action", s.action, "email", s.email, "activity", s.act.Title, "year", s.act.Year)
	a.mailSignUp(r, s.act, s.email, s.position, s.note, actor.Email, s.existed, s.was)
	return serve.None{}, nil
}

type removeVolunteerBody struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

func (a app) removeVolunteer(r *http.Request, body removeVolunteerBody) (serve.None, error) {
	actor := a.actor(r)
	act, email, ops, err := a.cache.Model().removeVolunteer(actor, body.ID, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:removed volunteer", "actor", actor.Email, "email", email, "activity", act.Title, "year", act.Year)
	a.mailRemoved(r, act, email, actor.Email)
	return serve.None{}, nil
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

func (a app) saveActivity(r *http.Request, body activityBody) (serve.None, error) {
	actor := a.actor(r)
	s, err := a.cache.Model().saveActivity(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, s.ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved activity", "actor", actor.Email, "action", s.action, "activity", s.title, "id", s.id, "year", s.year, "status", s.status)
	if s.adding {
		a.mailNewActivity(r, a.cache.Model().Activity(s.id), actor.Email)
	}
	return serve.None{}, nil
}

func (a app) deleteActivity(r *http.Request, body activityRef) (serve.None, error) {
	actor := a.actor(r)
	act, ops, err := a.cache.Model().deleteActivity(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:deleted activity", "actor", actor.Email, "activity", act.Title, "year", act.Year)
	return serve.None{}, nil
}

func (a app) saveLink(r *http.Request, body linkBody) (serve.None, error) {
	actor := a.actor(r)
	act, action, ops, err := a.cache.Model().saveLink(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved link", "actor", actor.Email, "action", action, "link", strings.TrimSpace(body.Title), "activity", act.Title, "year", act.Year)
	return serve.None{}, nil
}

type orderBody struct {
	Parent string   `json:"parent"`
	IDs    []string `json:"ids"`
}

func (a app) orderChildren(r *http.Request, body orderBody) (serve.None, error) {
	actor := a.actor(r)
	parent, ops, err := a.cache.Model().orderChildren(actor, body.Parent, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:reordered", "actor", actor.Email, "parent", parent.Title, "year", parent.Year, "changed", len(ops))
	return serve.None{}, nil
}

type deleteLinkBody struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (a app) deleteLink(r *http.Request, body deleteLinkBody) (serve.None, error) {
	actor := a.actor(r)
	act, ops, err := a.cache.Model().deleteLink(actor, body.ID, body.Title)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:deleted link", "actor", actor.Email, "link", body.Title, "activity", act.Title, "year", act.Year)
	return serve.None{}, nil
}

func (a app) saveCategory(r *http.Request, body categoryBody) (serve.None, error) {
	actor := a.actor(r)
	s, err := a.cache.Model().saveCategory(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, s.op); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved category", "actor", actor.Email, "action", s.action, "category", s.title, "id", s.id, "event", s.eventID)
	return serve.None{}, nil
}

type categoryOrderBody struct {
	EventID string   `json:"eventId"`
	IDs     []string `json:"ids"`
}

func (a app) reorderCategories(r *http.Request, body categoryOrderBody) (serve.None, error) {
	actor := a.actor(r)
	eventID := strings.TrimSpace(body.EventID)
	ops, err := a.cache.Model().reorderCategories(actor, eventID, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:reordered categories", "actor", actor.Email, "event", eventID, "count", len(body.IDs))
	return serve.None{}, nil
}

func (a app) deleteCategory(r *http.Request, body activityRef) (serve.None, error) {
	actor := a.actor(r)
	cat, ops, err := a.cache.deleteCategory(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:deleted category", "actor", actor.Email, "category", cat.Title, "id", cat.ID)
	return serve.None{}, nil
}

func (a app) copyActivity(r *http.Request, body activityRef) (serve.None, error) {
	actor := a.actor(r)
	act, id, year, ops, err := a.cache.Model().copyActivity(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:copied activity", "actor", actor.Email, "activity", act.Title, "from", act.Year, "to", year, "id", id)
	return serve.None{}, nil
}

type settingsBody struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

func (a app) saveSettings(r *http.Request, body settingsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := saveSettings(actor, body.ExpenseFormURL, body.Intro)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:changed the settings", "actor", actor.Email)
	return serve.None{}, nil
}

type notifyBody struct {
	Kinds []string `json:"kinds"`
}

func (a app) saveNotify(r *http.Request, body notifyBody) (serve.None, error) {
	actor := a.actor(r)
	value, ops, err := saveNotify(actor, body.Kinds)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:set notifications", "actor", actor.Email, "kinds", value)
	return serve.None{}, nil
}

func (a app) adminState(_ *http.Request, actor access.Actor) map[string]any {
	prefs := a.cache.Model().notifyPrefs(actor.Email)
	notify := []string{}
	for _, k := range NotifyKinds {
		if prefs[k] {
			notify = append(notify, k)
		}
	}
	return map[string]any{"notify": notify, "mail": a.mailer != nil}
}

type redirectBody struct {
	Original string `json:"original"`
	Old      string `json:"old"`
	New      string `json:"new"`
}

func (a app) saveRedirect(r *http.Request, body redirectBody) (serve.None, error) {
	actor := a.actor(r)
	s, err := a.cache.Model().saveRedirect(actor, body.Original, body.Old, body.New)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, s.op); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved redirect", "actor", actor.Email, "action", s.action, "old", s.old, "new", s.to)
	return serve.None{}, nil
}

type redirectRef struct {
	Old string `json:"old"`
}

func (a app) deleteRedirect(r *http.Request, body redirectRef) (serve.None, error) {
	actor := a.actor(r)
	redirect, ops, err := a.cache.Model().deleteRedirect(actor, body.Old)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:deleted redirect", "actor", actor.Email, "old", redirect.Old)
	return serve.None{}, nil
}
