package model

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

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
	store     *Store
	images    blob.Images
	calendar  calendarApp
	search    imagesearch.Search
	mailer    *mail.Mailgun
	style     *sharecard.Style
	describer *describe.Describer
}

type ActivitiesDeps struct {
	Store     *Store
	Images    blob.Images
	Calendar  CalendarHooks
	Search    imagesearch.Search
	Mailer    *mail.Mailgun
	Style     *sharecard.Style
	Describer *describe.Describer
}

func RegisterActivities(mux *http.ServeMux, d ActivitiesDeps) ActivitiesHooks {
	d.Search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	a := activitiesApp{store: d.Store, images: d.Images, calendar: d.Calendar.app, search: d.Search, mailer: d.Mailer, style: d.Style, describer: d.Describer}
	for _, page := range activitiesPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	a.search.Register(mux, "/api/team", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("POST /api/team/describe", serve.JSON(a.describe))
	return ActivitiesHooks{app: a}
}

func (a activitiesApp) activities() *Activities {
	return a.store.Model().Activities
}

func (a activitiesApp) directory() *Directory {
	return a.store.Model().Directory
}

func ActivitiesRedirected(s *Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		m := s.Model().Activities
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
	return a.store.Model().actor(r, "team")
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

type activitySettingsBody struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

type notifyBody struct {
	Kinds []string `json:"kinds"`
}
