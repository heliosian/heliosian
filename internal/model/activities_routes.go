package model

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const activitiesShell = "web/team/index.html"

var activitiesPages = []string{
	"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
}

type activitiesApp struct {
	cache     *ActivitiesCache
	images    blob.Images
	directory func() *Directory
	settings  func() *Config
	calendar  calendarApp
	search    imagesearch.Search
	mailer    *mail.Mailgun
	lists     *EmailListsCache
	style     *sharecard.Style
	describer *describe.Describer
}

type ActivitiesDeps struct {
	Cache      *ActivitiesCache
	Images     blob.Images
	Directory  func() *Directory
	Settings   func() *Config
	Calendar   CalendarHooks
	Search     imagesearch.Search
	Mailer     *mail.Mailgun
	EmailLists *EmailListsCache
	Style      *sharecard.Style
	Describer  *describe.Describer
}

func RegisterActivities(mux *http.ServeMux, d ActivitiesDeps) {
	d.Search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	a := activitiesApp{cache: d.Cache, images: d.Images, directory: d.Directory, settings: d.Settings, calendar: d.Calendar.app, search: d.Search, mailer: d.Mailer, lists: d.EmailLists, style: d.Style, describer: d.Describer}
	for _, page := range activitiesPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/team/model", serve.JSON(a.model))
	a.search.Register(mux, "/api/team", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("POST /api/team/volunteer", serve.JSON(a.saveVolunteer))
	mux.HandleFunc("DELETE /api/team/volunteer", serve.JSON(a.removeVolunteer))
	mux.HandleFunc("POST /api/team/activity", serve.JSON(a.saveActivity))
	mux.HandleFunc("POST /api/team/order", serve.JSON(a.orderChildren))
	mux.HandleFunc("DELETE /api/team/activity", serve.JSON(a.deleteActivity))
	mux.HandleFunc("POST /api/team/link", serve.JSON(a.saveLink))
	mux.HandleFunc("DELETE /api/team/link", serve.JSON(a.deleteLink))
	mux.HandleFunc("POST /api/team/category", serve.JSON(a.saveCategory))
	mux.HandleFunc("DELETE /api/team/category", serve.JSON(a.deleteCategory))
	mux.HandleFunc("POST /api/team/category/settings", serve.JSON(a.saveCategoryFlags))
	mux.HandleFunc("POST /api/team/categories/order", serve.JSON(a.reorderCategories))
	mux.HandleFunc("POST /api/team/copy", serve.JSON(a.copyActivity))
	mux.HandleFunc("POST /api/team/describe", serve.JSON(a.describe))
	mux.HandleFunc("POST /api/team/settings", serve.JSON(a.saveSettings))
	mux.HandleFunc("POST /api/team/notify", serve.JSON(a.saveNotify))
	RegisterAdmins(mux, a.cache.AdminList, a.actor, a.adminState)
	mux.HandleFunc("POST /api/team/redirect", serve.JSON(a.saveRedirect))
	mux.HandleFunc("DELETE /api/team/redirect", serve.JSON(a.deleteRedirect))
}

func ActivitiesRedirected(cache *ActivitiesCache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		m := cache.Model()
		to, status := m.Aliased(r.URL.Path), http.StatusMovedPermanently
		if to == "" {
			to, status = m.Destination(r.URL.Path), http.StatusFound
		}
		if to == "" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(to, "/") {
			to = "https://" + r.Host + to
		}
		if r.URL.RawQuery != "" && !strings.Contains(to, "?") {
			to += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, status)
	})
}

func (a activitiesApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, activitiesShell)
}

func (a activitiesApp) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

func (a activitiesApp) model(r *http.Request, _ serve.None) (ActivitiesView, error) {
	actor := a.actor(r)
	view := RenderActivities(a.cache.Model(), a.directory(), a.settings(), a.calendar.linkedRSVPs, a.lists.Model(), actor, time.Now().In(Location))
	view.ImageSearch = a.search.On()
	return view, nil
}

type activityRef struct {
	ID string `json:"id"`
}

type describeActivityBody struct {
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Category string `json:"category"`
	When     string `json:"when"`
	Notes    string `json:"notes"`
}

func (a activitiesApp) describe(r *http.Request, body describeActivityBody) (map[string]string, error) {
	email := a.actor(r).Email
	facts := describe.ActivityFacts{Title: body.Title, Parent: body.Parent, Category: body.Category, When: body.When, Notes: body.Notes}
	description, err := a.describer.Activity(r.Context(), email, facts)
	if errors.Is(err, claude.ErrTooMany) {
		return nil, access.Refuse(http.StatusTooManyRequests, "%v", err)
	}
	if errors.Is(err, describe.ErrTooLong) {
		return nil, access.Invalid("%v", err)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "team:describe", "actor", email, "title", body.Title, "error", err)
		return nil, access.Refuse(http.StatusBadGateway, "could not write a description right now")
	}
	slog.InfoContext(r.Context(), "team:described", "actor", email, "title", body.Title)
	return map[string]string{"description": description}, nil
}

func (a activitiesApp) saveVolunteer(r *http.Request, body volunteerBody) (serve.None, error) {
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

func (a activitiesApp) removeVolunteer(r *http.Request, body removeVolunteerBody) (serve.None, error) {
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
	VolunteersHidden   string     `json:"volunteersHidden"`
	VolunteersComplete bool       `json:"volunteersComplete"`
	DirectSignUp       string     `json:"directSignUp"`
	Priority           bool       `json:"priority"`
	SignUp             string     `json:"signUp"`
	PrettyID           string     `json:"prettyId"`
	AllowAdding        string     `json:"allowAdding"`
	TakeOver           bool       `json:"takeOver"`
}

type activityPatch map[string]json.RawMessage

func (p activityPatch) into(body *activityBody) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, body)
}

func (p activityPatch) fields() []string {
	out := []string{}
	for field := range p {
		if field != "id" {
			out = append(out, field)
		}
	}
	slices.Sort(out)
	return out
}

func (a activitiesApp) saveActivity(r *http.Request, patch activityPatch) (activityRef, error) {
	actor := a.actor(r)
	s, err := a.cache.Model().saveActivity(actor, patch)
	if err != nil {
		return activityRef{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, s.ops...); err != nil {
		return activityRef{}, err
	}
	slog.InfoContext(r.Context(), "team:saved activity", "actor", actor.Email, "action", s.action, "activity", s.title, "id", s.id, "year", s.year, "status", s.status)
	if s.adding {
		a.mailNewActivity(r, a.cache.Model().Activity(s.id), actor.Email)
	}
	return activityRef{ID: s.id}, nil
}

func (a activitiesApp) deleteActivity(r *http.Request, body activityRef) (serve.None, error) {
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

func (a activitiesApp) saveLink(r *http.Request, body linkBody) (serve.None, error) {
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

func (a activitiesApp) orderChildren(r *http.Request, body orderBody) (serve.None, error) {
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

type linkRef struct {
	Link string `json:"link"`
}

func (a activitiesApp) deleteLink(r *http.Request, body linkRef) (serve.None, error) {
	actor := a.actor(r)
	act, ops, err := a.cache.Model().deleteLink(actor, body.Link)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:deleted link", "actor", actor.Email, "link", body.Link, "activity", act.Title, "year", act.Year)
	return serve.None{}, nil
}

func (a activitiesApp) saveCategory(r *http.Request, body categoryBody) (serve.None, error) {
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

func (a activitiesApp) saveCategoryFlags(r *http.Request, body categoryFlagsBody) (serve.None, error) {
	actor := a.actor(r)
	op, c, err := a.cache.Model().saveCategoryFlags(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, op); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:saved category settings", "actor", actor.Email, "category", c.Title, "id", c.ID, "flags", body.Flags)
	return serve.None{}, nil
}

type categoryOrderBody struct {
	EventID string   `json:"eventId"`
	IDs     []string `json:"ids"`
}

func (a activitiesApp) reorderCategories(r *http.Request, body categoryOrderBody) (serve.None, error) {
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

func (a activitiesApp) deleteCategory(r *http.Request, body activityRef) (serve.None, error) {
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

func (a activitiesApp) copyActivity(r *http.Request, body activityRef) (serve.None, error) {
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

type activitySettingsBody struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

func (a activitiesApp) saveSettings(r *http.Request, body activitySettingsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := activitySettingsOps(actor, body.ExpenseFormURL, body.Intro)
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

func (a activitiesApp) saveNotify(r *http.Request, body notifyBody) (serve.None, error) {
	actor := a.actor(r)
	value, ops, err := activityNotifyOps(actor, body.Kinds)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "team:set notifications", "actor", actor.Email, "kinds", value)
	return serve.None{}, nil
}

func (a activitiesApp) adminState(_ *http.Request, actor access.Actor) map[string]any {
	prefs := a.cache.Model().notifyPrefs(actor.Email)
	notify := []string{}
	for _, k := range NotifyKinds {
		if prefs[k] {
			notify = append(notify, k)
		}
	}
	return map[string]any{"notify": notify}
}

type redirectBody struct {
	Original string `json:"original"`
	Old      string `json:"old"`
	New      string `json:"new"`
}

func (a activitiesApp) saveRedirect(r *http.Request, body redirectBody) (serve.None, error) {
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

func (a activitiesApp) deleteRedirect(r *http.Request, body redirectRef) (serve.None, error) {
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
