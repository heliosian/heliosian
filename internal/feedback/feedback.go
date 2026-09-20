// Package feedback takes what people report from any app's toolbar into the Reports tab, tells the super admins, and files the ones an admin keeps as issues on GitHub.
package feedback

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/logging"
)

const (
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
	ID         string
	App        string
	AppName    string
	Kind       string
	Status     string
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
	// Issue is the address a filed report became; Handled and HandledBy are
	// when an admin filed or dismissed it, and who.
	Issue     string
	Handled   time.Time
	HandledBy string
}

// Saver takes a submitted report, which in production is the Reports tab.
type Saver interface {
	Save(r Report) (Report, error)
}

type Queue struct {
	reports chan Report
	mu      sync.Mutex
	recent  map[string][]time.Time
}

// NewQueue drains submissions off the request path: each is written to the
// sheet and then announced to the super admins, neither of which anyone waits
// on. notify may be nil, which sends nothing.
func NewQueue(saver Saver, notify func(Report)) *Queue {
	q := &Queue{reports: make(chan Report, 64), recent: map[string][]time.Time{}}
	go q.run(saver, notify)
	return q
}

func (q *Queue) run(saver Saver, notify func(Report)) {
	for r := range q.reports {
		saved, err := saver.Save(r)
		if err != nil {
			slog.Error("feedback: saving failed", "error", err, "app", r.App, "kind", r.Kind, "summary", r.Summary, "details", r.Details, "email", r.Email)
			continue
		}
		slog.Info("feedback: saved", "id", saved.ID, "app", saved.App, "kind", saved.Kind)
		if notify != nil {
			notify(saved)
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

// emailPattern finds an address written into a report's own words, which the
// reporter may have typed - a colleague's, a parent's - and which has no place
// in a public issue.
var emailPattern = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)

// Redact removes what names a person: every address, wherever it is written.
func Redact(s string) string {
	return emailPattern.ReplaceAllString(s, "[email removed]")
}

// publicAddress is the page's address with its query and fragment cut off,
// since a link a person followed may carry a token or their own address in it.
func publicAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

// Strip is the report as an issue anyone may read: the reporter's own words
// and the context that makes a bug reproducible, with every address redacted,
// the page's query string cut away, and no Reporter section at all. It is
// where the admin page's editor starts; whatever the admin leaves is what
// GitHub gets.
func Strip(r Report) (title, body string, labels []string) {
	title = Redact(r.Summary)
	if r.AppName != "" {
		title = "[" + r.AppName + "] " + title
	}
	var b strings.Builder
	if r.Details == "" {
		b.WriteString("_No details given._\n")
	} else {
		b.WriteString(Redact(r.Details) + "\n")
	}
	b.WriteString("\n## Context\n\n| | |\n|---|---|\n")
	row := func(name, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "| %s | %s |\n", name, cell(value))
	}
	row("App", appCell(r))
	row("Page", r.Page)
	row("Address", address(publicAddress(r.URL)))
	row("Browser", r.UserAgent)
	row("Viewport", r.Viewport)
	row("Screen", r.Screen)
	row("Language", r.Language)
	row("Time zone", r.Timezone)
	row("Reported", r.At.In(school).Format("2006-01-02 15:04 MST"))
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, "\n<details>\n<summary>Recent errors (%d)</summary>\n\n```\n%s\n```\n\n</details>\n", len(r.Errors), Redact(strings.Join(r.Errors, "\n")))
	}
	labels = []string{r.Kind}
	if r.App != "" {
		labels = append(labels, "app:"+r.App)
	}
	return title, b.String(), labels
}

// appCell names the app both ways where a report knows both, which one it
// knows where it knows one, and nothing at all for a report carried over from
// triage that was filed by hand and never named an app.
func appCell(r Report) string {
	switch {
	case r.AppName != "" && r.App != "":
		return r.AppName + " (`" + r.App + "`)"
	case r.AppName != "":
		return r.AppName
	case r.App != "":
		return "`" + r.App + "`"
	}
	return ""
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
