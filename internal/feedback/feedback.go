// Package feedback files what people report from any app's toolbar as issues in the private triage repository.
package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/logging"
)

const (
	repo         = "heliosian/triage"
	summaryLimit = 120
	detailsLimit = 4000
	errorLimit   = 5
	errorChars   = 300
	perWindow    = 5
	window       = 10 * time.Minute
)

var school = schoolZone()

func schoolZone() *time.Location {
	zone, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		logging.Fatal("load school time zone", "error", err)
	}
	return zone
}

type Report struct {
	App        string
	AppName    string
	Kind       string
	Summary    string
	Details    string
	Email      string
	SuperAdmin bool
	URL        string
	Page       string
	Viewport   string
	Screen     string
	Language   string
	Timezone   string
	UserAgent  string
	Errors     []string
	At         time.Time
}

type Filer interface {
	File(ctx context.Context, r Report) error
}

type Queue struct {
	reports chan Report
	mu      sync.Mutex
	recent  map[string][]time.Time
}

func NewQueue(filer Filer) *Queue {
	q := &Queue{reports: make(chan Report, 64), recent: map[string][]time.Time{}}
	go q.run(filer)
	return q
}

func (q *Queue) run(filer Filer) {
	for r := range q.reports {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := filer.File(ctx, r)
		cancel()
		if err != nil {
			title, body, _ := Render(r)
			slog.Error("feedback: filing failed", "error", err, "title", title, "body", body)
		}
	}
}

func (q *Queue) allow(email string, now time.Time) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	kept := []time.Time{}
	for _, t := range q.recent[email] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perWindow {
		q.recent[email] = kept
		return false
	}
	q.recent[email] = append(kept, now)
	return true
}

func (q *Queue) add(r Report) bool {
	select {
	case q.reports <- r:
		return true
	default:
		return false
	}
}

type api struct {
	app        string
	name       func() string
	superAdmin func(string) bool
	queue      *Queue
}

func Register(mux *http.ServeMux, app string, name func() string, superAdmin func(string) bool, queue *Queue) {
	a := api{app: app, name: name, superAdmin: superAdmin, queue: queue}
	mux.HandleFunc("POST /api/feedback", a.file)
}

type submission struct {
	Kind     string   `json:"kind"`
	Summary  string   `json:"summary"`
	Details  string   `json:"details"`
	URL      string   `json:"url"`
	Page     string   `json:"page"`
	Viewport string   `json:"viewport"`
	Screen   string   `json:"screen"`
	Language string   `json:"language"`
	Timezone string   `json:"timezone"`
	Errors   []string `json:"errors"`
}

func (a api) file(w http.ResponseWriter, r *http.Request) {
	var in submission
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if in.Kind != "bug" && in.Kind != "idea" {
		http.Error(w, "kind must be bug or idea", http.StatusBadRequest)
		return
	}
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		http.Error(w, "say what happened, or what you'd like, in a line", http.StatusBadRequest)
		return
	}
	if len([]rune(summary)) > summaryLimit || len([]rune(in.Details)) > detailsLimit {
		http.Error(w, "that's longer than a report can be", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(auth.Email(r))
	now := time.Now()
	if !a.queue.allow(email, now) {
		http.Error(w, "that's a lot of reports in a few minutes; please wait a little and try again", http.StatusTooManyRequests)
		return
	}
	report := Report{
		App:        a.app,
		AppName:    a.name(),
		Kind:       in.Kind,
		Summary:    summary,
		Details:    strings.TrimSpace(in.Details),
		Email:      email,
		SuperAdmin: a.superAdmin(email),
		URL:        clip(in.URL, 1000),
		Page:       clip(in.Page, 200),
		Viewport:   clip(in.Viewport, 40),
		Screen:     clip(in.Screen, 40),
		Language:   clip(in.Language, 40),
		Timezone:   clip(in.Timezone, 80),
		UserAgent:  clip(r.UserAgent(), 400),
		Errors:     clipAll(in.Errors),
		At:         now,
	}
	if !a.queue.add(report) {
		http.Error(w, "we couldn't take the report just now; please try again in a moment", http.StatusServiceUnavailable)
		return
	}
	slog.InfoContext(r.Context(), "feedback: queued", "kind", in.Kind, "summary", summary)
	w.WriteHeader(http.StatusAccepted)
}

func clip(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

func clipAll(errors []string) []string {
	out := []string{}
	for _, e := range errors {
		if len(out) == errorLimit {
			break
		}
		e = clip(e, errorChars)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

func Render(r Report) (title, body string, labels []string) {
	title = "[" + r.AppName + "] " + r.Summary
	var b strings.Builder
	if r.Details == "" {
		b.WriteString("_No details given._\n")
	} else {
		b.WriteString(r.Details + "\n")
	}
	b.WriteString("\n## Context\n\n| | |\n|---|---|\n")
	row := func(name, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "| %s | %s |\n", name, cell(value))
	}
	row("App", r.AppName+" (`"+r.App+"`)")
	row("Page", r.Page)
	row("Address", address(r.URL))
	row("Role", role(r.SuperAdmin))
	row("Browser", r.UserAgent)
	row("Viewport", r.Viewport)
	row("Screen", r.Screen)
	row("Language", r.Language)
	row("Time zone", r.Timezone)
	row("Reported", r.At.In(school).Format("2006-01-02 15:04 MST"))
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, "\n<details>\n<summary>Recent errors (%d)</summary>\n\n```\n%s\n```\n\n</details>\n", len(r.Errors), strings.Join(r.Errors, "\n"))
	}
	b.WriteString("\n## Reporter\n\n> Remove this section before filing anything publicly.\n\n- Email: " + r.Email + "\n")
	return title, b.String(), []string{r.Kind, "app:" + r.App, "untriaged"}
}

func cell(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "|", "\\|")), " ")
}

func address(u string) string {
	u = strings.Map(func(r rune) rune {
		if r == '<' || r == '>' || r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, u)
	if u == "" {
		return ""
	}
	return "<" + u + ">"
}

func role(superAdmin bool) string {
	if superAdmin {
		return "super admin"
	}
	return "member"
}

type GitHub struct {
	Token    string
	Endpoint string
}

func (g *GitHub) File(ctx context.Context, r Report) error {
	title, body, labels := Render(r)
	payload, err := json.Marshal(map[string]any{"title": title, "body": body, "labels": labels})
	if err != nil {
		return err
	}
	endpoint := g.Endpoint
	if endpoint == "" {
		endpoint = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/repos/"+repo+"/issues", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("feedback: github %d: %s", resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	var created struct {
		URL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return err
	}
	slog.Info("feedback: filed", "issue", created.URL, "title", title)
	return nil
}
