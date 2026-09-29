package model

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/feedback"
	"heliosian/internal/serve"
)

const (
	issueTitleLimit = 200
	issueBodyLimit  = 60000
)

type feedbackAdmin struct {
	store  *Store
	bucket *blob.Bucket
	filer  *feedback.GitHubApp
}

func RegisterFeedbackAdmin(mux *http.ServeMux, s *Store, bucket *blob.Bucket, filer *feedback.GitHubApp) {
	a := feedbackAdmin{store: s, bucket: bucket, filer: filer}
	mux.HandleFunc("GET /api/admin/feedback", serve.JSON(a.list))
	mux.HandleFunc("GET /api/admin/feedback/{id}", serve.JSON(a.one))
	mux.HandleFunc("GET /api/admin/feedback/{id}/screenshot", a.screenshot)
	mux.HandleFunc("POST /api/admin/feedback/{id}/file", serve.JSON(a.file))
	mux.HandleFunc("POST /api/admin/feedback/{id}/dismiss", serve.JSON(a.dismiss))
}

type reportListView struct {
	Reports []reportSummary `json:"reports"`
	CanFile bool            `json:"canFile"`
	Repo    string          `json:"repo"`
}

type reportSummary struct {
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

type reportDetail struct {
	reportSummary
	Details    string     `json:"details"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	URL        string     `json:"url"`
	Page       string     `json:"page"`
	Browser    string     `json:"browser"`
	Viewport   string     `json:"viewport"`
	Screen     string     `json:"screen"`
	Language   string     `json:"language"`
	Timezone   string     `json:"timezone"`
	Errors     []string   `json:"errors"`
	Screenshot bool       `json:"screenshot"`
	Draft      issueDraft `json:"draft"`
}

type issueDraft struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
	Repo   string   `json:"repo"`
}

func reportSummaryOf(r Report) reportSummary {
	return reportSummary{
		ID:        r.ID,
		Received:  r.At.In(Location).Format("2006-01-02 15:04"),
		App:       r.App,
		AppName:   r.AppName,
		Kind:      r.Kind,
		Status:    r.Status,
		Summary:   r.Summary,
		Issue:     r.Issue,
		HandledBy: r.HandledBy,
	}
}

func (a feedbackAdmin) list(r *http.Request, _ serve.None) (reportListView, error) {
	m := a.store.Model()
	if err := requireSuperAdmin(superActor(m, r)); err != nil {
		return reportListView{}, err
	}
	out := []reportSummary{}
	for _, report := range m.Feedback.Reports() {
		out = append(out, reportSummaryOf(report))
	}
	return reportListView{Reports: out, CanFile: a.filer != nil, Repo: feedback.Repo}, nil
}

func (a feedbackAdmin) one(r *http.Request, _ serve.None) (reportDetail, error) {
	m := a.store.Model()
	if err := requireSuperAdmin(superActor(m, r)); err != nil {
		return reportDetail{}, err
	}
	report, ok := m.Feedback.Report(r.PathValue("id"))
	if !ok {
		return reportDetail{}, access.Missing("no such report")
	}
	title, body, issueType, labels := StripReport(report)
	return reportDetail{
		reportSummary: reportSummaryOf(report),
		Details:       report.Details,
		Email:         report.Email,
		Role:          reportRole(report.SuperAdmin),
		URL:           report.URL,
		Page:          report.Page,
		Browser:       report.UserAgent,
		Viewport:      report.Viewport,
		Screen:        report.Screen,
		Language:      report.Language,
		Timezone:      report.Timezone,
		Errors:        report.Errors,
		Screenshot:    report.Screenshot != "",
		Draft:         issueDraft{Title: title, Body: body, Type: issueType, Labels: labels, Repo: feedback.Repo},
	}, nil
}

func (a feedbackAdmin) screenshot(w http.ResponseWriter, r *http.Request) {
	m := a.store.Model()
	if err := requireSuperAdmin(superActor(m, r)); err != nil {
		serve.Error(w, r, err)
		return
	}
	report, ok := m.Feedback.Report(r.PathValue("id"))
	if !ok || report.Screenshot == "" {
		http.NotFound(w, r)
		return
	}
	content, mimeType, err := a.bucket.Get(r.Context(), report.Screenshot)
	if errors.Is(err, blob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	serve.Content(w, r, report.Screenshot, content)
}

func (a feedbackAdmin) file(r *http.Request, in issueDraft) (map[string]string, error) {
	m := a.store.Model()
	actor := superActor(m, r)
	report, err := m.Feedback.filing(actor, r.PathValue("id"))
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
	if len([]rune(title)) > issueTitleLimit || len([]rune(body)) > issueBodyLimit {
		return nil, access.Invalid("that's longer than an issue can be")
	}
	if found := reportEmailPattern.FindString(title + "\n" + body); found != "" {
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

func (a feedbackAdmin) markFiled(ctx context.Context, actor access.Actor, id, issue string) error {
	ops, err := a.store.Model().Feedback.filed(actor, id, issue, time.Now())
	if err != nil {
		return err
	}
	return a.store.Commit(ctx, actor, feedbackAppName, ops...)
}

func (a feedbackAdmin) dismiss(r *http.Request, _ serve.None) (serve.None, error) {
	m := a.store.Model()
	actor := superActor(m, r)
	ops, err := m.Feedback.dismissed(actor, r.PathValue("id"), time.Now())
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, feedbackAppName, ops...); err != nil {
		return serve.None{}, err
	}
	return serve.None{}, nil
}
