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
	Issue      string
	Handled    time.Time
	HandledBy  string
}

type Intake struct {
	cache  *Cache
	notify func(Report)
	mu     sync.Mutex
	recent map[string][]time.Time
}

func NewIntake(cache *Cache, notify func(Report)) *Intake {
	return &Intake{cache: cache, notify: notify, recent: map[string][]time.Time{}}
}

func (in *Intake) allow(email string, now time.Time) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	kept := []time.Time{}
	for _, t := range in.recent[email] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perWindow {
		in.recent[email] = kept
		return false
	}
	in.recent[email] = append(kept, now)
	return true
}

type api struct {
	app        string
	name       func() string
	superAdmin func(string) bool
	intake     *Intake
}

func Register(mux *http.ServeMux, app string, name func() string, superAdmin func(string) bool, intake *Intake) {
	a := api{app: app, name: name, superAdmin: superAdmin, intake: intake}
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
	if !a.intake.allow(email, now) {
		http.Error(w, "that's a lot of reports in a few minutes; please wait a little and try again", http.StatusTooManyRequests)
		return
	}
	saved, err := a.intake.cache.save(r.Context(), Report{
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
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] feedback: saving failed", "error", err, "kind", in.Kind, "summary", summary, "details", in.Details)
		http.Error(w, "we couldn't take the report just now; please try again in a moment", http.StatusServiceUnavailable)
		return
	}
	slog.InfoContext(r.Context(), "feedback: saved", "id", saved.ID, "kind", saved.Kind, "summary", summary)
	go a.intake.notify(saved)
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

var emailPattern = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)

func Redact(s string) string {
	return emailPattern.ReplaceAllString(s, "[email removed]")
}

func publicAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

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
