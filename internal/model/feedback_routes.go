package model

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/blob"
	"heliosian/internal/feedback"
	"heliosian/internal/serve"
	"heliosian/internal/store"
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

type FeedbackHooks struct {
	admin feedbackAdmin
}

func RegisterFeedbackAdmin(mux *http.ServeMux, s *Store, bucket *blob.Bucket, filer *feedback.GitHubApp) FeedbackHooks {
	a := feedbackAdmin{store: s, bucket: bucket, filer: filer}
	mux.HandleFunc("GET /api/admin/feedback/{id}/screenshot", a.screenshot)
	return FeedbackHooks{admin: a}
}

type reportSummary struct {
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

func (h FeedbackHooks) Resources() []api.Type[*Model] {
	a := h.admin
	triages := func(m *Model, q api.Query, key string) bool {
		_, ok := m.Feedback.Report(key)
		return ok && q.Actor.May(Triage)
	}
	return []api.Type[*Model]{{
		Name:  "feedback-reports",
		Shape: reportDetail{},
		Has: func(m *Model, key string) bool {
			_, ok := m.Feedback.Report(key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			report, ok := m.Feedback.Report(key)
			if !ok || !q.Actor.May(Triage) {
				return reportDetail{}, false
			}
			return reportDetailOf(report), true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			if !q.Actor.May(Triage) {
				return out
			}
			for _, report := range m.Feedback.Reports() {
				out = append(out, report.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for old, target := range m.Feedback.aliases {
				out[old] = target
			}
			return out
		},
		Actions: map[string]api.Action[*Model]{
			"file": api.Do(func(m *Model, q api.Query, key string) bool {
				report, ok := m.Feedback.Report(key)
				return ok && a.filer != nil && q.Actor.May(Triage) && report.Status != ReportStatusFiled
			}, func(wr api.Write[*Model], in issueDraft) error {
				ops, err := a.file(wr, in)
				return a.store.stage(wr, feedbackAppName, ops, err)
			}),
			"dismiss": api.Do(triages, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Feedback.dismissed(wr.Query.Actor, wr.ID, time.Now())
				logAfter(wr, "feedback: dismissed", "id", wr.ID)
				return a.store.stage(wr, feedbackAppName, ops, err)
			}),
		},
	}}
}

func reportDetailOf(report Report) reportDetail {
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
	}
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

func (a feedbackAdmin) file(wr api.Write[*Model], in issueDraft) ([]store.Op, error) {
	actor, r := wr.Query.Actor, wr.Request
	report, err := wr.S.Feedback.filing(actor, wr.ID)
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
	logAfter(wr, "feedback: filed", "id", report.ID, "issue", issue)
	return wr.S.Feedback.filed(actor, report.ID, issue, time.Now())
}
