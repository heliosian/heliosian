package feedback

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/when"
)

const (
	titleLimit = 200
	bodyLimit  = 60000
)

type IssueFiler interface {
	File(ctx context.Context, title, body, issueType string, labels []string) (string, error)
}

type admin struct {
	cache      *Cache
	filer      IssueFiler
	superAdmin func(string) bool
}

func RegisterAdmin(mux *http.ServeMux, cache *Cache, filer IssueFiler, superAdmin func(string) bool) {
	a := admin{cache: cache, filer: filer, superAdmin: superAdmin}
	mux.HandleFunc("GET /api/admin/feedback", serve.JSON(a.list))
	mux.HandleFunc("GET /api/admin/feedback/{id}", serve.JSON(a.one))
	mux.HandleFunc("POST /api/admin/feedback/{id}/file", serve.JSON(a.file))
	mux.HandleFunc("POST /api/admin/feedback/{id}/dismiss", serve.JSON(a.dismiss))
}

type listView struct {
	Reports []summary `json:"reports"`
	CanFile bool      `json:"canFile"`
	Repo    string    `json:"repo"`
}

type summary struct {
	ID        string `json:"id"`
	Received  string `json:"received"`
	App       string `json:"app"`
	AppName   string `json:"appName"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
	Issue     string `json:"issue,omitempty"`
	HandledBy string `json:"handledBy,omitempty"`
}

type detail struct {
	summary
	Details  string   `json:"details"`
	Email    string   `json:"email"`
	Role     string   `json:"role"`
	URL      string   `json:"url"`
	Page     string   `json:"page"`
	Browser  string   `json:"browser"`
	Viewport string   `json:"viewport"`
	Screen   string   `json:"screen"`
	Language string   `json:"language"`
	Timezone string   `json:"timezone"`
	Errors   []string `json:"errors"`
	Draft    draft    `json:"draft"`
}

type draft struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
	Repo   string   `json:"repo"`
}

func summaryOf(r Report) summary {
	return summary{
		ID:        r.ID,
		Received:  r.At.In(when.Location).Format("2006-01-02 15:04"),
		App:       r.App,
		AppName:   r.AppName,
		Kind:      r.Kind,
		Status:    r.Status,
		Summary:   r.Summary,
		Issue:     r.Issue,
		HandledBy: r.HandledBy,
	}
}

func (a admin) list(r *http.Request, _ serve.None) (listView, error) {
	if err := requireSuperAdmin(actorOf(r, a.superAdmin)); err != nil {
		return listView{}, err
	}
	out := []summary{}
	for _, report := range a.cache.Reports() {
		out = append(out, summaryOf(report))
	}
	return listView{Reports: out, CanFile: a.filer != nil, Repo: Repo}, nil
}

func (a admin) one(r *http.Request, _ serve.None) (detail, error) {
	if err := requireSuperAdmin(actorOf(r, a.superAdmin)); err != nil {
		return detail{}, err
	}
	report, ok := a.cache.Report(r.PathValue("id"))
	if !ok {
		return detail{}, access.Missing("no such report")
	}
	title, body, issueType, labels := Strip(report)
	return detail{
		summary:  summaryOf(report),
		Details:  report.Details,
		Email:    report.Email,
		Role:     role(report.SuperAdmin),
		URL:      report.URL,
		Page:     report.Page,
		Browser:  report.UserAgent,
		Viewport: report.Viewport,
		Screen:   report.Screen,
		Language: report.Language,
		Timezone: report.Timezone,
		Errors:   report.Errors,
		Draft:    draft{Title: title, Body: body, Type: issueType, Labels: labels, Repo: Repo},
	}, nil
}

func (a admin) file(r *http.Request, in draft) (map[string]string, error) {
	actor := actorOf(r, a.superAdmin)
	report, err := a.cache.Model().filing(actor, r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if a.filer == nil {
		return nil, access.Refuse(http.StatusServiceUnavailable, "filing on GitHub is not set up on this server")
	}
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	issueType := strings.TrimSpace(in.Type)
	if title == "" {
		return nil, access.Invalid("an issue needs a title")
	}
	if issueType == "" {
		return nil, access.Invalid("an issue needs a type")
	}
	if len([]rune(title)) > titleLimit || len([]rune(body)) > bodyLimit {
		return nil, access.Invalid("that's longer than an issue can be")
	}
	if found := emailPattern.FindString(title + "\n" + body); found != "" {
		return nil, access.Invalid("that still has an email address in it (%s); take it out before filing", found)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	issue, err := a.filer.File(ctx, title, body, issueType, in.Labels)
	if err != nil {
		slog.ErrorContext(r.Context(), "feedback: filing failed", "error", err, "id", report.ID)
		return nil, access.Refuse(http.StatusBadGateway, "GitHub would not take the issue: %s", err)
	}
	if err := a.markFiled(r.Context(), actor, report.ID, issue); err != nil {
		slog.ErrorContext(r.Context(), "feedback: mark filed", "error", err, "id", report.ID, "issue", issue)
		return nil, access.Refuse(http.StatusInternalServerError, "the issue is filed at %s but the report could not be marked: %s", issue, err)
	}
	return map[string]string{"issue": issue}, nil
}

func (a admin) markFiled(ctx context.Context, actor access.Actor, id, issue string) error {
	ops, err := a.cache.Model().filed(actor, id, issue, time.Now())
	if err != nil {
		return err
	}
	return a.cache.Commit(ctx, actor, ops...)
}

func (a admin) dismiss(r *http.Request, _ serve.None) (serve.None, error) {
	actor := actorOf(r, a.superAdmin)
	ops, err := a.cache.Model().dismissed(actor, r.PathValue("id"), time.Now())
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	return serve.None{}, nil
}
