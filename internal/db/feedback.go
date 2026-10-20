package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/feedback"
	"heliosian/internal/hosts"
	"heliosian/internal/mail"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	feedbackActor          = "feedback"
	reportSummaryLimit     = 120
	reportDetailsLimit     = 4000
	reportErrorLimit       = 5
	reportErrorChars       = 300
	feedbackPerWindow      = 5
	feedbackWindow         = 10 * time.Minute
	screenshotLimit        = 10 << 20
	screenshotFolder       = "feedback"
	screenshotStoreTimeout = time.Minute
	issueTitleLimit        = 200
	issueBodyLimit         = 60000
)

type FeedbackConfig struct {
	Sender *mail.Mailgun
	Base   string
	Filer  *feedback.GitHubApp
}

type Feedback struct {
	s      *Store
	queue  *store.Queue
	bucket *blob.Bucket
	tokens auth.Tokens
	now    func() time.Time
	config FeedbackConfig
	recent *ratelimit.Limiter
}

func NewFeedback(s *Store, queue *store.Queue, bucket *blob.Bucket, tokens auth.Tokens, now func() time.Time, config FeedbackConfig) *Feedback {
	return &Feedback{s: s, queue: queue, bucket: bucket, tokens: tokens, now: now, config: config, recent: ratelimit.New(feedbackPerWindow, feedbackWindow)}
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

type screenshot struct {
	name, mimeType string
	content        []byte
}

func (f *Feedback) Register(mux *http.ServeMux, domain string, appName func(key string) string) {
	mux.HandleFunc("POST /api/feedback", func(w http.ResponseWriter, r *http.Request) {
		app, ok := hosts.AppOf(domain, r.Host)
		if !ok {
			serve.Error(w, r, access.Invalid("no app answers at %s", r.Host))
			return
		}
		f.file(w, r, app, appName(app))
	})
}

func (f *Feedback) file(w http.ResponseWriter, r *http.Request, app, appName string) {
	m := f.s.Model()
	env, actor, ok := caller(w, r, m, f.tokens, f.now())
	if !ok {
		return
	}
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
	now := f.now()
	if !f.recent.Allow(actor.Email, now) {
		http.Error(w, "that's a lot of reports in a few minutes; please wait a little and try again", http.StatusTooManyRequests)
		return
	}
	row := map[string]any{
		"app": app, "kind": in.Kind, "summary": summary, "details": strings.TrimSpace(in.Details),
		"url": clipText(in.URL, 1000), "page": clipText(in.Page, 200), "browser": clipText(r.UserAgent(), 400),
		"viewport": clipText(in.Viewport, 40), "screen": clipText(in.Screen, 40), "language": clipText(in.Language, 40),
		"time_zone": clipText(in.Timezone, 80), "errors": strings.Join(clipErrors(in.Errors), "\n"), "screenshot": shot.name,
		"status": "new", "added_by": env.Viewer, "added": now.In(School).Format(cells.StampFormat),
	}
	ids, err := Write(r.Context(), f.s, f.queue, access.System(feedbackActor), Env{System: feedbackActor, Now: now}, Batch{Batch: []Edit{{Insert: "REPORT", Row: row}}})
	if err != nil {
		slog.ErrorContext(r.Context(), "feedback: saving failed", "error", err, "kind", in.Kind, "summary", summary, "details", in.Details)
		http.Error(w, "we couldn't take the report just now; please try again in a moment", http.StatusServiceUnavailable)
		return
	}
	slog.InfoContext(r.Context(), "feedback: saved", "id", ids[0], "kind", in.Kind, "summary", summary, "screenshot", shot.name)
	go f.finish(ids[0], appName, actor.Email, shot)
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

func (f *Feedback) finish(id, appName, email string, shot screenshot) {
	attachments := []mail.Attachment{}
	if shot.name != "" {
		ctx, cancel := context.WithTimeout(context.Background(), screenshotStoreTimeout)
		defer cancel()
		if err := f.bucket.Put(ctx, shot.name, shot.mimeType, shot.content); err != nil {
			slog.Error("feedback: storing the screenshot failed", "error", err, "id", id, "name", shot.name)
		}
		attachments = append(attachments, mail.Attachment{Name: "screenshot" + path.Ext(shot.name), ContentType: shot.mimeType, Content: shot.content})
	}
	m := f.s.Model()
	report, ok := m.Table("REPORT").Get(id)
	if !ok {
		return
	}
	to := m.superAdminAddresses()
	if len(to) == 0 {
		return
	}
	msg := f.notice(to, report, appName, email)
	msg.Attachments = attachments
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := f.config.Sender.Send(ctx, msg); err != nil {
		slog.Error("feedback: announce", "error", err, "id", id)
	}
}

func (m *Model) superAdminAddresses() []string {
	out := []string{}
	for _, g := range m.Table("GROUP").All() {
		if g["slug"] != "super-admins" {
			continue
		}
		for _, row := range m.effectiveRows(g["id"]) {
			if address := m.addressOf(row["person"]); address != "" {
				out = append(out, address)
			}
		}
	}
	return out
}

func (f *Feedback) notice(to []string, r store.Row, appName, email string) mail.Message {
	what := "An idea"
	if r["kind"] == "bug" {
		what = "A problem"
	}
	subject := fmt.Sprintf("[%s] %s: %s", appName, strings.ToLower(what), r["summary"])
	link := strings.TrimSuffix(f.config.Base, "/") + "/admin?tab=feedback&report=" + r["id"]
	details := r["details"]
	if details == "" {
		details = "No details given."
	}
	screen := r["screen"]
	if r["viewport"] != "" && r["screen"] != "" {
		screen = r["viewport"] + " window on a " + r["screen"] + " screen"
	} else if r["viewport"] != "" {
		screen = r["viewport"] + " window"
	}
	facts := [][2]string{{"Browser", r["browser"]}, {"Screen", screen}}
	if r["screenshot"] != "" {
		facts = append(facts, [2]string{"Screenshot", "attached"})
	}
	var b, t strings.Builder
	fmt.Fprintf(&b, "<p>%s reported from %s by %s.</p>", html.EscapeString(what), html.EscapeString(appName), html.EscapeString(email))
	fmt.Fprintf(&b, "<p><strong>%s</strong></p>", html.EscapeString(r["summary"]))
	fmt.Fprintf(&b, "<p>%s</p>", strings.ReplaceAll(html.EscapeString(details), "\n", "<br>"))
	fmt.Fprintf(&b, "<p>They were on %s at %s.</p>", html.EscapeString(r["page"]), html.EscapeString(r["added"]))
	fmt.Fprintf(&t, "%s reported from %s by %s.\n\n%s\n\n%s\n\nThey were on %s at %s.\n\n", what, appName, email, r["summary"], details, r["page"], r["added"])
	b.WriteString("<p>")
	for _, fact := range facts {
		if fact[1] == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s<br>", fact[0], html.EscapeString(fact[1]))
		fmt.Fprintf(&t, "%s: %s\n", fact[0], fact[1])
	}
	b.WriteString("</p>")
	fmt.Fprintf(&b, "<p><a href=%q>Read it, edit it and file it</a></p>", link)
	fmt.Fprintf(&t, "\nRead it, edit it and file it: %s\n", link)
	return mail.Message{To: to, Subject: subject, HTML: b.String(), Text: t.String()}
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
		if e = clipText(e, reportErrorChars); e != "" {
			out = append(out, e)
		}
	}
	return out
}

var reportEmailPattern = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)

func redactEmails(s string) string {
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

type issueDraft struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
	Repo   string   `json:"repo"`
	Filer  bool     `json:"filer"`
}

func reportDraft(r store.Row, appName string) issueDraft {
	title := redactEmails(r["summary"])
	if appName != "" {
		title = "[" + appName + "] " + title
	}
	var b strings.Builder
	if r["details"] == "" {
		b.WriteString("_No details given._\n")
	} else {
		b.WriteString(redactEmails(r["details"]) + "\n")
	}
	b.WriteString("\n## Context\n\n| | |\n|---|---|\n")
	row := func(name, value string) {
		if value == "" {
			return
		}
		fmt.Fprintf(&b, "| %s | %s |\n", name, issueCell(value))
	}
	appCell := appName
	if r["app"] != "" {
		appCell = strings.TrimSpace(appName + " (`" + r["app"] + "`)")
	}
	row("App", appCell)
	row("Page", r["page"])
	row("Address", issueAddress(publicAddress(r["url"])))
	row("Browser", r["browser"])
	row("Viewport", r["viewport"])
	row("Screen", r["screen"])
	row("Language", r["language"])
	row("Time zone", r["time_zone"])
	row("Reported", r["added"])
	if errs := strings.TrimSpace(r["errors"]); errs != "" {
		fmt.Fprintf(&b, "\n<details>\n<summary>Recent errors (%d)</summary>\n\n```\n%s\n```\n\n</details>\n", len(strings.Split(errs, "\n")), redactEmails(errs))
	}
	labels := []string{}
	if r["app"] != "" {
		labels = append(labels, "app:"+r["app"])
	}
	return issueDraft{Title: title, Body: b.String(), Type: issueTypes[r["kind"]], Labels: labels, Repo: feedback.Repo}
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

type reportRequest struct {
	Report string   `json:"report"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
}

func (f *Feedback) RegisterTriage(mux *http.ServeMux, appName func(key string) string) {
	read := func(w http.ResponseWriter, r *http.Request) (store.Row, reportRequest, access.Actor, Env, bool) {
		m := f.s.Model()
		env, actor, ok := caller(w, r, m, f.tokens, f.now())
		if !ok {
			return nil, reportRequest{}, actor, env, false
		}
		if !m.superAdmin(env.Viewer) {
			serve.Error(w, r, access.Forbidden("super admin access required"))
			return nil, reportRequest{}, actor, env, false
		}
		var in reportRequest
		if !serve.Decode(w, r, &in) {
			return nil, reportRequest{}, actor, env, false
		}
		report, ok := m.Table("REPORT").Get(in.Report)
		if !ok {
			serve.Error(w, r, access.Missing("no such report"))
			return nil, reportRequest{}, actor, env, false
		}
		return report, in, actor, env, true
	}
	mux.HandleFunc("POST /api/do/report-draft", func(w http.ResponseWriter, r *http.Request) {
		report, _, _, _, ok := read(w, r)
		if !ok {
			return
		}
		draft := reportDraft(report, appName(report["app"]))
		draft.Filer = f.config.Filer != nil
		serve.Write(w, r, http.StatusOK, draft)
	})
	mux.HandleFunc("POST /api/do/file-report", func(w http.ResponseWriter, r *http.Request) {
		report, in, actor, env, ok := read(w, r)
		if !ok {
			return
		}
		issue, err := f.fileIssue(r.Context(), report, in)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "feedback: filed", "id", report["id"], "issue", issue)
		edit := Edit{Set: report["id"], Cells: map[string]any{"status": "filed", "issue": issue, "handled": f.now().In(School).Format(cells.StampFormat), "handled_by": env.Viewer}}
		if _, err := Write(r.Context(), f.s, f.queue, actor, env, Batch{Batch: []Edit{edit}}); err != nil {
			serve.Error(w, r, err)
			return
		}
		serve.Write(w, r, http.StatusOK, map[string]string{"issue": issue})
	})
}

func (f *Feedback) fileIssue(ctx context.Context, report store.Row, in reportRequest) (string, error) {
	if report["status"] == "filed" {
		return "", access.Refuse(http.StatusConflict, "that report is already filed")
	}
	if f.config.Filer == nil {
		return "", access.Refuse(http.StatusServiceUnavailable, "filing on GitHub is not set up on this server")
	}
	title, body, issueType := strings.TrimSpace(in.Title), strings.TrimSpace(in.Body), strings.TrimSpace(in.Type)
	if title == "" {
		return "", access.Invalid("an issue needs a title")
	}
	if issueType == "" {
		return "", access.Invalid("an issue needs a type")
	}
	if len([]rune(title)) > issueTitleLimit || len([]rune(body)) > issueBodyLimit {
		return "", access.Invalid("that's longer than an issue can be")
	}
	if found := reportEmailPattern.FindString(title + "\n" + body); found != "" {
		return "", access.Invalid("that still has an email address in it (%s); take it out before filing", found)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	issue, err := f.config.Filer.File(ctx, title, body, issueType, in.Labels)
	if err != nil {
		slog.ErrorContext(ctx, "feedback: filing failed", "error", err, "id", report["id"])
		return "", access.Refuse(http.StatusBadGateway, "GitHub would not take the issue: %s", err)
	}
	return issue, nil
}
