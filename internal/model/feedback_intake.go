package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/mail"
	"heliosian/internal/ratelimit"
)

const (
	reportSummaryLimit     = 120
	reportDetailsLimit     = 4000
	reportErrorLimit       = 5
	reportErrorChars       = 300
	feedbackPerWindow      = 5
	feedbackWindow         = 10 * time.Minute
	screenshotLimit        = 10 << 20
	screenshotFolder       = "feedback"
	screenshotStoreTimeout = time.Minute
)

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
	Screenshot string
	At         time.Time
	Issue      string
	Handled    time.Time
	HandledBy  string
}

type FeedbackIntake struct {
	cache  *FeedbackCache
	bucket *blob.Bucket
	notify func(Report, []mail.Attachment)
	recent *ratelimit.Limiter
}

func NewFeedbackIntake(cache *FeedbackCache, bucket *blob.Bucket, notify func(Report, []mail.Attachment)) *FeedbackIntake {
	return &FeedbackIntake{cache: cache, bucket: bucket, notify: notify, recent: ratelimit.New(feedbackPerWindow, feedbackWindow)}
}

type screenshot struct {
	name     string
	mimeType string
	content  []byte
}

type feedbackRoutes struct {
	app       string
	name      func() string
	directory *DirectoryCache
	settings  *ConfigCache
	intake    *FeedbackIntake
}

func RegisterFeedback(mux *http.ServeMux, app string, name func() string, directory *DirectoryCache, settings *ConfigCache, intake *FeedbackIntake) {
	a := feedbackRoutes{app: app, name: name, directory: directory, settings: settings, intake: intake}
	mux.HandleFunc("POST /api/feedback", a.file)
}

type feedbackSubmission struct {
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

func feedbackActor(r *http.Request, directory *DirectoryCache, settings *ConfigCache) access.Actor {
	return directory.Actor(r, settings.SuperHeld)
}

func (a feedbackRoutes) file(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, screenshotLimit+1<<20)
	if err := r.ParseMultipartForm(screenshotLimit + 1<<20); err != nil {
		http.Error(w, "that's more than a report can carry; a screenshot can be 10 MB at most", http.StatusRequestEntityTooLarge)
		return
	}
	var in feedbackSubmission
	if err := json.Unmarshal([]byte(r.FormValue("report")), &in); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	shot, err := readScreenshot(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if len([]rune(summary)) > reportSummaryLimit || len([]rune(in.Details)) > reportDetailsLimit {
		http.Error(w, "that's longer than a report can be", http.StatusBadRequest)
		return
	}
	actor := feedbackActor(r, a.directory, a.settings)
	now := time.Now()
	if !a.intake.recent.Allow(actor.Email, now) {
		http.Error(w, "that's a lot of reports in a few minutes; please wait a little and try again", http.StatusTooManyRequests)
		return
	}
	saved, err := a.intake.cache.save(r.Context(), actor, Report{
		App:        a.app,
		AppName:    a.name(),
		Kind:       in.Kind,
		Summary:    summary,
		Details:    strings.TrimSpace(in.Details),
		URL:        clipText(in.URL, 1000),
		Page:       clipText(in.Page, 200),
		Viewport:   clipText(in.Viewport, 40),
		Screen:     clipText(in.Screen, 40),
		Language:   clipText(in.Language, 40),
		Timezone:   clipText(in.Timezone, 80),
		UserAgent:  clipText(r.UserAgent(), 400),
		Errors:     clipErrors(in.Errors),
		Screenshot: shot.name,
		At:         now,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "feedback: saving failed", "error", err, "kind", in.Kind, "summary", summary, "details", in.Details)
		http.Error(w, "we couldn't take the report just now; please try again in a moment", http.StatusServiceUnavailable)
		return
	}
	slog.InfoContext(r.Context(), "feedback: saved", "id", saved.ID, "kind", saved.Kind, "summary", summary, "screenshot", shot.name)
	go a.intake.finish(saved, shot)
	w.WriteHeader(http.StatusNoContent)
}

func readScreenshot(r *http.Request) (screenshot, error) {
	file, _, err := r.FormFile("screenshot")
	if errors.Is(err, http.ErrMissingFile) {
		return screenshot{}, nil
	}
	if err != nil {
		return screenshot{}, fmt.Errorf("we couldn't read that screenshot")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		return screenshot{}, fmt.Errorf("we couldn't read that screenshot")
	}
	mimeType := http.DetectContentType(content)
	ext, ok := blob.ImageExtensions[mimeType]
	if !ok {
		return screenshot{}, fmt.Errorf("a screenshot has to be a picture: PNG, JPEG, GIF or WebP")
	}
	return screenshot{name: screenshotFolder + "/" + blob.Name(content, ext), mimeType: mimeType, content: content}, nil
}

func (in *FeedbackIntake) finish(saved Report, shot screenshot) {
	if shot.name == "" {
		in.notify(saved, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), screenshotStoreTimeout)
	defer cancel()
	if err := in.bucket.Put(ctx, shot.name, shot.mimeType, shot.content); err != nil {
		slog.Error("feedback: storing the screenshot failed", "error", err, "id", saved.ID, "name", shot.name)
	}
	in.notify(saved, []mail.Attachment{{Name: "screenshot" + path.Ext(shot.name), ContentType: shot.mimeType, Content: shot.content}})
}

func clipText(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

func clipErrors(errors []string) []string {
	out := []string{}
	for _, e := range errors {
		if len(out) == reportErrorLimit {
			break
		}
		e = clipText(e, reportErrorChars)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

var reportEmailPattern = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)

func RedactEmails(s string) string {
	return reportEmailPattern.ReplaceAllString(s, "[email removed]")
}

func publicAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

var issueTypes = map[string]string{"bug": "Bug", "idea": "Feature"}

func StripReport(r Report) (title, body, issueType string, labels []string) {
	title = RedactEmails(r.Summary)
	if r.AppName != "" {
		title = "[" + r.AppName + "] " + title
	}
	var b strings.Builder
	if r.Details == "" {
		b.WriteString("_No details given._\n")
	} else {
		b.WriteString(RedactEmails(r.Details) + "\n")
	}
	b.WriteString("\n## Context\n\n| | |\n|---|---|\n")
	row := func(name, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "| %s | %s |\n", name, issueCell(value))
	}
	row("App", appCell(r))
	row("Page", r.Page)
	row("Address", issueAddress(publicAddress(r.URL)))
	row("Browser", r.UserAgent)
	row("Viewport", r.Viewport)
	row("Screen", r.Screen)
	row("Language", r.Language)
	row("Time zone", r.Timezone)
	row("Reported", r.At.In(Location).Format("2006-01-02 15:04 MST"))
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, "\n<details>\n<summary>Recent errors (%d)</summary>\n\n```\n%s\n```\n\n</details>\n", len(r.Errors), RedactEmails(strings.Join(r.Errors, "\n")))
	}
	labels = []string{}
	if r.App != "" {
		labels = append(labels, "app:"+r.App)
	}
	return title, b.String(), issueTypes[r.Kind], labels
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

func issueCell(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "|", "\\|")), " ")
}

func issueAddress(u string) string {
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

func reportRole(superAdmin bool) string {
	if superAdmin {
		return "super admin"
	}
	return "member"
}
