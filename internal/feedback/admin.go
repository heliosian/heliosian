package feedback

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/auth"
)

const (
	titleLimit = 200
	bodyLimit  = 60000
)

// IssueFiler opens an issue and answers with its address. In production it is
// the GitHub App; without one the admin page says filing is not set up.
type IssueFiler interface {
	File(ctx context.Context, title, body string, labels []string) (string, error)
}

type admin struct {
	store      *Store
	filer      IssueFiler
	superAdmin func(string) bool
}

// RegisterAdmin wires the triage queue onto Heliosian's own admin, the one
// place every app's reports are read: the reports themselves carry the
// reporter's address and their page, so this is the super admins' tier alone,
// not every app admin's.
func RegisterAdmin(mux *http.ServeMux, store *Store, filer IssueFiler, superAdmin func(string) bool) {
	a := admin{store: store, filer: filer, superAdmin: superAdmin}
	mux.HandleFunc("GET /api/admin/feedback", a.list)
	mux.HandleFunc("GET /api/admin/feedback/{id}", a.one)
	mux.HandleFunc("POST /api/admin/feedback/{id}/file", a.file)
	mux.HandleFunc("POST /api/admin/feedback/{id}/dismiss", a.dismiss)
}

func (a admin) require(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.superAdmin(email) {
		http.Error(w, "super admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// summary is one report as the queue lists it - enough to sort and pick by,
// without the details or the context.
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

// detail is one report opened: everything it carries, and the draft issue the
// editor starts from.
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

// draft is the issue as Strip writes it, which the admin edits before filing.
type draft struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
	Repo   string   `json:"repo"`
}

func summaryOf(r Report) summary {
	return summary{
		ID:        r.ID,
		Received:  r.At.In(school).Format("2006-01-02 15:04"),
		App:       r.App,
		AppName:   r.AppName,
		Kind:      r.Kind,
		Status:    r.Status,
		Summary:   r.Summary,
		Issue:     r.Issue,
		HandledBy: r.HandledBy,
	}
}

func (a admin) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(w, r); !ok {
		return
	}
	reports := a.store.Reports()
	out := make([]summary, 0, len(reports))
	for _, report := range reports {
		out = append(out, summaryOf(report))
	}
	view := struct {
		Reports []summary `json:"reports"`
		CanFile bool      `json:"canFile"`
		Repo    string    `json:"repo"`
	}{Reports: out, CanFile: a.filer != nil, Repo: Repo}
	writeJSON(w, r, view)
}

func (a admin) one(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(w, r); !ok {
		return
	}
	report, ok := a.store.Report(r.PathValue("id"))
	if !ok {
		http.Error(w, "no such report", http.StatusNotFound)
		return
	}
	title, body, labels := Strip(report)
	writeJSON(w, r, detail{
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
		Draft:    draft{Title: title, Body: body, Labels: labels, Repo: Repo},
	})
}

func (a admin) file(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.require(w, r)
	if !ok {
		return
	}
	if a.filer == nil {
		http.Error(w, "filing on GitHub is not set up on this server", http.StatusServiceUnavailable)
		return
	}
	report, ok := a.store.Report(r.PathValue("id"))
	if !ok {
		http.Error(w, "no such report", http.StatusNotFound)
		return
	}
	if report.Status == StatusFiled {
		http.Error(w, "that report is already filed", http.StatusConflict)
		return
	}
	var in draft
	if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&in); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	if title == "" {
		http.Error(w, "an issue needs a title", http.StatusBadRequest)
		return
	}
	if len([]rune(title)) > titleLimit || len([]rune(body)) > bodyLimit {
		http.Error(w, "that's longer than an issue can be", http.StatusBadRequest)
		return
	}
	if found := emailPattern.FindString(title + "\n" + body); found != "" {
		http.Error(w, "that still has an email address in it ("+found+"); take it out before filing", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	issue, err := a.filer.File(ctx, title, body, in.Labels)
	if err != nil {
		slog.ErrorContext(r.Context(), "feedback: filing failed", "error", err, "id", report.ID)
		http.Error(w, "GitHub would not take the issue: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.store.Filed(report.ID, issue, actor, time.Now()); err != nil {
		slog.ErrorContext(r.Context(), "feedback: mark filed", "error", err, "id", report.ID, "issue", issue)
		http.Error(w, "the issue is filed at "+issue+" but the report could not be marked: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, r, map[string]string{"issue": issue})
}

func (a admin) dismiss(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.require(w, r)
	if !ok {
		return
	}
	report, ok := a.store.Report(r.PathValue("id"))
	if !ok {
		http.Error(w, "no such report", http.StatusNotFound)
		return
	}
	if err := a.store.Dismissed(report.ID, actor, time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, r *http.Request, view any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "feedback: encode", "error", err)
	}
}
