package model

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/feedback"
	"heliosian/internal/intercept"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

func sampleReport() Report {
	return Report{
		ID:        "abc123",
		App:       "calendar",
		AppName:   "Helios When",
		Kind:      "bug",
		Status:    ReportStatusNew,
		Summary:   "Next month | shows nothing",
		Details:   "Clicked forward and the grid went blank. Ask casey.lim@example.org too.",
		Email:     "jordan.whitfield@example.org",
		URL:       "https://when.heliosian.com/2026-10?token=s3cret#grid",
		Page:      "October | Helios When",
		Viewport:  "1440×900",
		UserAgent: "Mozilla/5.0",
		Errors:    []string{"TypeError: x is undefined (app.js:12)"},
		At:        time.Date(2026, 9, 13, 21, 3, 0, 0, time.UTC),
	}
}

func TestStripLeavesNothingPersonal(t *testing.T) {
	title, body, issueType, labels := StripReport(sampleReport())
	if title != "[Helios When] Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{
		"Ask [email removed] too.",
		"| Page | October \\| Helios When |",
		"| Address | <https://when.heliosian.com/2026-10> |",
		"| Reported | 2026-09-13 14:03 PDT |",
		"<summary>Recent errors (1)</summary>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"jordan.whitfield", "casey.lim", "Reporter", "token=", "s3cret", "| Role |"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body still carries %q:\n%s", unwanted, body)
		}
	}
	if issueType != "Bug" {
		t.Errorf("type = %q", issueType)
	}
	if strings.Join(labels, ",") != "app:calendar" {
		t.Errorf("labels = %v", labels)
	}
}

func TestStripAnIdeaIsAFeature(t *testing.T) {
	r := sampleReport()
	r.Kind = "idea"
	_, _, issueType, _ := StripReport(r)
	if issueType != "Feature" {
		t.Errorf("type = %q", issueType)
	}
}

func TestStripWithNoApp(t *testing.T) {
	r := sampleReport()
	r.App, r.AppName = "", ""
	title, body, _, labels := StripReport(r)
	if title != "Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	if labels == nil || len(labels) != 0 {
		t.Errorf("labels = %#v", labels)
	}
	if strings.Contains(body, "| App |") {
		t.Errorf("body names an app it has none of:\n%s", body)
	}
}

func TestStripWithOnlyAnAppKey(t *testing.T) {
	r := sampleReport()
	r.AppName = ""
	title, body, _, labels := StripReport(r)
	if title != "Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	if strings.Join(labels, ",") != "app:calendar" {
		t.Errorf("labels = %v", labels)
	}
	if !strings.Contains(body, "| App | `calendar` |") {
		t.Errorf("body lacks the app key:\n%s", body)
	}
}

func TestStripRedactsAnAddressInTheSummary(t *testing.T) {
	r := sampleReport()
	r.Summary = "mail to head@example.org bounces"
	title, _, _, _ := StripReport(r)
	if strings.Contains(title, "head@example.org") || !strings.Contains(title, "[email removed]") {
		t.Errorf("title = %q", title)
	}
}

func reportForm(t *testing.T, report string, shot []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("report", report); err != nil {
		t.Fatal(err)
	}
	if shot != nil {
		part, err := form.CreateFormFile("screenshot", "shot.png")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(shot)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, form.FormDataContentType()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func feedbackPeople(t *testing.T, dir *data.Dir, queue *store.Queue) (*DirectoryCache, *ConfigCache) {
	t.Helper()
	settings, err := NewConfigCache(dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	return sampleDirectory(t, dir, queue), settings
}

func intakeServer(t *testing.T, dir *data.Dir, queue *store.Queue, cache *FeedbackCache, bucket *blob.Bucket, told chan Report) func(report string, shot []byte) *httptest.ResponseRecorder {
	t.Helper()
	directory, settings := feedbackPeople(t, dir, queue)
	mux := http.NewServeMux()
	RegisterFeedback(mux, "calendar", func() string { return "Helios When" }, directory, settings,
		NewFeedbackIntake(cache, bucket, func(r Report, _ []mail.Attachment) { told <- r }))
	h := auth.Fixed("Jordan.Whitfield@heliosschool.org", mux)
	return func(report string, shot []byte) *httptest.ResponseRecorder {
		body, contentType := reportForm(t, report, shot)
		req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("User-Agent", "TestBrowser/1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
}

func TestHandler(t *testing.T) {
	dir, queue, cache := testFeedbackCache(t)
	told := make(chan Report, 4)
	send := intakeServer(t, dir, queue, cache, blob.NewMemoryBucket(), told)
	post := func(report string) *httptest.ResponseRecorder {
		return send(report, nil)
	}
	if rec := post(`{"kind":"bug","summary":"  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty summary: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"kind":"nope","summary":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad kind: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(`{"kind":"bug","summary":"` + strings.Repeat("x", 121) + `"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("long summary: %d %s", rec.Code, rec.Body.String())
	}
	rec := post(`{"kind":"idea","summary":" A dark mode ","details":"Please","url":"https://when.heliosian.com/","page":"Helios When","errors":["a","b","c","d","e","f","g"]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	var got Report
	select {
	case got = <-told:
	case <-time.After(2 * time.Second):
		t.Fatal("nobody was told")
	}
	if saved, ok := cache.Report(got.ID); !ok || saved.Summary != got.Summary {
		t.Fatalf("announced %+v, which is not in memory", got)
	}
	queue.Flush()
	_, rows, err := dir.Table(feedbackAppName, reportsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[4]["ID"] != got.ID {
		t.Fatalf("the sheet holds %d reports, the last %v", len(rows), rows[len(rows)-1])
	}
	_, log, err := dir.Table(feedbackAppName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0]["Action"] != "insert" || log[0]["Actor"] != jordan || log[0]["Key"] != "ID="+got.ID {
		t.Errorf("change log %v", log)
	}
	if got.Email != jordan || !got.SuperAdmin || got.AppName != "Helios When" || got.App != "calendar" {
		t.Errorf("identity = %+v", got)
	}
	if got.Summary != "A dark mode" || got.Kind != "idea" || got.UserAgent != "TestBrowser/1" || len(got.Errors) != reportErrorLimit || got.Screenshot != "" {
		t.Errorf("content = %+v", got)
	}
}

func TestHandlerStoresAScreenshot(t *testing.T) {
	dir, queue, cache := testFeedbackCache(t)
	told := make(chan Report, 4)
	bucket := blob.NewMemoryBucket()
	post := intakeServer(t, dir, queue, cache, bucket, told)
	if rec := post(`{"kind":"bug","summary":"Blank grid"}`, []byte("not a picture")); rec.Code != http.StatusBadRequest {
		t.Errorf("a text file as a screenshot: %d %s", rec.Code, rec.Body.String())
	}
	shot := pngBytes(t)
	if rec := post(`{"kind":"bug","summary":"Blank grid"}`, shot); rec.Code != http.StatusNoContent {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	var got Report
	select {
	case got = <-told:
	case <-time.After(2 * time.Second):
		t.Fatal("nobody was told")
	}
	if !strings.HasPrefix(got.Screenshot, screenshotFolder+"/") || !strings.HasSuffix(got.Screenshot, ".png") {
		t.Fatalf("screenshot = %q", got.Screenshot)
	}
	stored, mimeType, err := bucket.Get(context.Background(), got.Screenshot)
	if err != nil || mimeType != "image/png" || !bytes.Equal(stored, shot) {
		t.Errorf("stored %d bytes as %q: %v", len(stored), mimeType, err)
	}
	if saved, _ := cache.Report(got.ID); saved.Screenshot != got.Screenshot {
		t.Errorf("the report in memory names %q", saved.Screenshot)
	}
}

func TestHandlerRefusesAnOversizedScreenshot(t *testing.T) {
	dir, queue, cache := testFeedbackCache(t)
	post := intakeServer(t, dir, queue, cache, blob.NewMemoryBucket(), make(chan Report, 1))
	big := append(pngBytes(t), make([]byte, screenshotLimit+1<<20)...)
	if rec := post(`{"kind":"bug","summary":"Blank grid"}`, big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized screenshot: %d %s", rec.Code, rec.Body.String())
	}
}

func TestThrottle(t *testing.T) {
	q := NewFeedbackIntake(nil, nil, nil).recent
	now := time.Now()
	for i := range feedbackPerWindow {
		if !q.Allow("a@example.org", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("report %d refused", i)
		}
	}
	if q.Allow("a@example.org", now.Add(time.Minute)) {
		t.Error("sixth report allowed")
	}
	if !q.Allow("b@example.org", now) {
		t.Error("another person refused")
	}
	if !q.Allow("a@example.org", now.Add(feedbackWindow+time.Minute)) {
		t.Error("report after the window refused")
	}
}

func testFeedbackCache(t *testing.T) (*data.Dir, *store.Queue, *FeedbackCache) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	cache, err := NewFeedbackCache(dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	return dir, queue, cache
}

func TestCacheReadsNewestFirst(t *testing.T) {
	_, _, cache := testFeedbackCache(t)
	reports := cache.Reports()
	if len(reports) != 4 {
		t.Fatalf("read %d reports", len(reports))
	}
	if reports[0].ID != "fbk0000000002" {
		t.Errorf("newest = %q", reports[0].ID)
	}
	if reports[0].Kind != "idea" || !reports[0].SuperAdmin || reports[0].Status != ReportStatusNew {
		t.Errorf("newest = %+v", reports[0])
	}
	filed, ok := cache.Report("c3d4e5f6a1b2")
	if !ok || filed.ID != "fbk0000000003" {
		t.Fatalf("the old ID does not reach the report: %+v", filed)
	}
	filed, ok = cache.Report("fbk0000000003")
	if !ok || filed.Status != ReportStatusFiled || filed.Issue == "" || filed.HandledBy == "" {
		t.Errorf("filed = %+v ok = %v", filed, ok)
	}
	if len(reports[0].Errors) != 0 || len(filed.Errors) != 0 {
		t.Errorf("errors read from blank cells")
	}
}

func TestCacheSavesAndHandles(t *testing.T) {
	dir, queue, cache := testFeedbackCache(t)
	directory, settings := feedbackPeople(t, dir, queue)
	actorOf := func(email string) access.Actor {
		return directory.Model().ActorOf(email, settings.SuperHeld(email))
	}
	superAdmin, member := actorOf(jordan), actorOf(feedbackMember)
	if !superAdmin.May(Triage) || member.May(Triage) {
		t.Fatalf("triage held by the super admin %v, by a member %v", superAdmin.May(Triage), member.May(Triage))
	}
	ctx := context.Background()
	saved, err := cache.save(ctx, member, sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "abc123" || saved.ID == "" || saved.Status != ReportStatusNew || saved.SuperAdmin {
		t.Errorf("saved = %+v", saved)
	}
	if got, ok := cache.Report(saved.ID); !ok || got.Summary != saved.Summary {
		t.Errorf("not readable back: %+v %v", got, ok)
	}
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	commit := func(ops []store.Op, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Commit(ctx, superAdmin, ops...); err != nil {
			t.Fatal(err)
		}
	}
	commit(cache.Model().filed(superAdmin, saved.ID, "https://github.com/x/y/issues/3", now))
	got, _ := cache.Report(saved.ID)
	if got.Status != ReportStatusFiled || got.Issue != "https://github.com/x/y/issues/3" || got.HandledBy != jordan {
		t.Errorf("filed = %+v", got)
	}
	commit(cache.Model().dismissed(superAdmin, "fbk0000000001", now))
	if got, _ := cache.Report("fbk0000000001"); got.Status != ReportStatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	var refusal *access.Refusal
	if _, err := cache.Model().filed(superAdmin, "nope", "x", now); !errors.As(err, &refusal) || refusal.Status != http.StatusNotFound {
		t.Errorf("filing a report that does not exist: %v", err)
	}
	if _, err := cache.Model().dismissed(member, saved.ID, now); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("a member dismissing a report: %v", err)
	}
	queue.Flush()
	_, log, err := dir.Table(feedbackAppName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	dismissed := false
	for _, row := range log {
		if row["Actor"] == jordan && row["Action"] == "set" && row["Key"] == "ID=fbk0000000001" && row["Column"] == "Status" && row["Previous"] == ReportStatusNew {
			dismissed = true
		}
	}
	if !dismissed {
		t.Errorf("the dismissal's previous status is not in the change log: %v", log)
	}
}

const feedbackMember = "robin.whitfield@heliosschool.org"

func feedbackAdminServer(t *testing.T, dir *data.Dir, queue *store.Queue, cache *FeedbackCache, bucket *blob.Bucket, filer *feedback.GitHubApp) *http.ServeMux {
	t.Helper()
	directory, settings := feedbackPeople(t, dir, queue)
	mux := http.NewServeMux()
	RegisterFeedbackAdmin(mux, cache, bucket, filer, directory, settings)
	return mux
}

func testFeedbackAdmin(t *testing.T, filer *feedback.GitHubApp, who string) (*FeedbackCache, http.Handler) {
	t.Helper()
	dir, queue, cache := testFeedbackCache(t)
	return cache, auth.Fixed(who, feedbackAdminServer(t, dir, queue, cache, blob.NewMemoryBucket(), filer))
}

func TestAdminShowsTheScreenshot(t *testing.T) {
	dir, queue, cache := testFeedbackCache(t)
	bucket := blob.NewMemoryBucket()
	shot := pngBytes(t)
	r := sampleReport()
	r.Screenshot = screenshotFolder + "/" + blob.Name(shot, "png")
	saved, err := cache.save(context.Background(), access.Actor{Email: feedbackMember}, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := bucket.Put(context.Background(), saved.Screenshot, "image/png", shot); err != nil {
		t.Fatal(err)
	}
	mux := feedbackAdminServer(t, dir, queue, cache, bucket, nil)
	get := func(who, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		auth.Fixed(who, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	rec := get(jordan, "/api/admin/feedback/"+saved.ID)
	var one reportDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil || !one.Screenshot {
		t.Errorf("the detail does not say there is a screenshot: %s %v", rec.Body.String(), err)
	}
	rec = get(jordan, "/api/admin/feedback/"+saved.ID+"/screenshot")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), shot) {
		t.Errorf("screenshot: %d %q %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	if rec := get(feedbackMember, "/api/admin/feedback/"+saved.ID+"/screenshot"); rec.Code != http.StatusForbidden {
		t.Errorf("a member got %d", rec.Code)
	}
	if rec := get(jordan, "/api/admin/feedback/fbk0000000001/screenshot"); rec.Code != http.StatusNotFound {
		t.Errorf("a report with no screenshot: %d", rec.Code)
	}
}

type filedIssue struct {
	Title  string   `json:"title"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
}

func filingApp(t *testing.T) (*feedback.GitHubApp, *filedIssue) {
	t.Helper()
	filed := &filedIssue{}
	intercept.Install(feedback.GitHubHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + feedback.Repo + "/installation":
			io.WriteString(w, `{"id":4242}`)
		case "/app/installations/4242/access_tokens":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"token":"ghs_installation","expires_at":"2099-01-01T00:00:00Z"}`)
		case "/repos/" + feedback.Repo + "/issues":
			if err := json.NewDecoder(r.Body).Decode(filed); err != nil {
				t.Errorf("payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"https://github.com/heliosian/heliosian/issues/77"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &feedback.GitHubApp{ID: "12345", PrivateKey: key}, filed
}

func TestAdminNeedsASuperAdmin(t *testing.T) {
	_, h := testFeedbackAdmin(t, nil, feedbackMember)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member got %d", rec.Code)
	}
}

func TestAdminListsAndOpens(t *testing.T) {
	_, h := testFeedbackAdmin(t, &feedback.GitHubApp{ID: "12345"}, jordan)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Reports []reportSummary `json:"reports"`
		CanFile bool            `json:"canFile"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Reports) != 4 || !list.CanFile {
		t.Fatalf("list = %+v", list)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback/fbk0000000001", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("open: %d %s", rec.Code, rec.Body.String())
	}
	var one reportDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil {
		t.Fatal(err)
	}
	if one.Email != "rowan.avery@example.org" {
		t.Errorf("the admin cannot see who reported it: %+v", one)
	}
	if strings.Contains(one.Draft.Body, "rowan.avery") || strings.Contains(one.Draft.Body, "token=abc123") {
		t.Errorf("the draft carries personal detail:\n%s", one.Draft.Body)
	}
	if one.Draft.Repo != feedback.Repo {
		t.Errorf("repo = %q", one.Draft.Repo)
	}
}

func TestAdminFilesAndDismisses(t *testing.T) {
	app, filed := filingApp(t)
	cache, h := testFeedbackAdmin(t, app, jordan)
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}
	if rec := post("/api/admin/feedback/fbk0000000001/file", `{"title":"","body":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty title: %d", rec.Code)
	}
	if rec := post("/api/admin/feedback/fbk0000000001/file", `{"title":"Blank grid","body":"x","type":" "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty type: %d", rec.Code)
	}
	rec := post("/api/admin/feedback/fbk0000000001/file", `{"title":"Blank grid","body":"Reported by rowan.avery@example.org","type":"Bug"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rowan.avery@example.org") {
		t.Errorf("an address slipped through: %d %s", rec.Code, rec.Body.String())
	}
	rec = post("/api/admin/feedback/fbk0000000001/file", `{"title":"Blank grid","body":"Forward a month and the grid empties.","type":"Bug","labels":["app:calendar"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("file: %d %s", rec.Code, rec.Body.String())
	}
	if filed.Title != "Blank grid" || filed.Type != "Bug" || strings.Join(filed.Labels, ",") != "app:calendar" {
		t.Errorf("filed %q as %q with %v", filed.Title, filed.Type, filed.Labels)
	}
	got, _ := cache.Report("fbk0000000001")
	if got.Status != ReportStatusFiled || got.Issue != "https://github.com/heliosian/heliosian/issues/77" || got.HandledBy != jordan {
		t.Errorf("report after filing = %+v", got)
	}
	if rec := post("/api/admin/feedback/fbk0000000001/file", `{"title":"Again","body":"x"}`); rec.Code != http.StatusConflict {
		t.Errorf("filed twice: %d", rec.Code)
	}
	if rec := post("/api/admin/feedback/fbk0000000002/dismiss", ""); rec.Code != http.StatusNoContent {
		t.Errorf("dismiss: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := cache.Report("fbk0000000002"); got.Status != ReportStatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	if rec := post("/api/admin/feedback/nope/dismiss", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown report: %d", rec.Code)
	}
}

func TestAdminWithoutAGitHubApp(t *testing.T) {
	_, h := testFeedbackAdmin(t, nil, jordan)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/feedback/fbk0000000001/file", strings.NewReader(`{"title":"t","body":"b"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("filing with no app: %d %s", rec.Code, rec.Body.String())
	}
}

func TestNotifyTellsTheSuperAdmins(t *testing.T) {
	sent := mailtest.NewRecorder("HCA-Team <team@example.org>")
	n := FeedbackNotifier{
		Sender:      sent.Mailgun,
		Base:        "https://heliosian.com",
		SuperAdmins: func() []string { return []string{"Admin@example.org", " ", "other@example.org"} },
	}
	n.Notify(sampleReport(), nil)
	if len(sent.Messages()) != 1 {
		t.Fatalf("sent %d messages", len(sent.Messages()))
	}
	m := sent.Messages()[0]
	if strings.Join(m.To, ",") != "admin@example.org,other@example.org" {
		t.Errorf("to = %v", m.To)
	}
	if !strings.Contains(m.Subject, "Helios When") || !strings.Contains(m.Subject, "Next month") {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"jordan.whitfield@example.org", "https://heliosian.com/admin?tab=feedback&report=abc123", "Browser: Mozilla/5.0", "Screen: 1440×900 window"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, m.Text)
		}
	}
	if strings.Contains(m.Text, "Screenshot") || len(m.Attachments) != 0 {
		t.Errorf("a report with no screenshot mentions or attaches one:\n%s", m.Text)
	}
}

func TestNotifyAttachesTheScreenshot(t *testing.T) {
	sent := mailtest.NewRecorder("HCA-Team <team@example.org>")
	n := FeedbackNotifier{Sender: sent.Mailgun, Base: "https://heliosian.com", SuperAdmins: func() []string { return []string{"admin@example.org"} }}
	r := sampleReport()
	r.Screen = "2560×1440 @2x"
	r.Screenshot = "feedback/abc.png"
	shot := pngBytes(t)
	n.Notify(r, []mail.Attachment{{Name: "screenshot.png", ContentType: "image/png", Content: shot}})
	m := sent.Messages()[0]
	if !strings.Contains(m.Text, "Screen: 1440×900 window on a 2560×1440 @2x screen") || !strings.Contains(m.Text, "Screenshot: attached") {
		t.Errorf("text:\n%s", m.Text)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "screenshot.png" || !bytes.Equal(m.Attachments[0].Content, shot) {
		t.Errorf("attachments = %d", len(m.Attachments))
	}
}

func TestNotifyWithNobodyToTell(t *testing.T) {
	sent := mailtest.NewRecorder("HCA-Team <team@example.org>")
	n := FeedbackNotifier{Sender: sent.Mailgun, SuperAdmins: func() []string { return nil }}
	n.Notify(sampleReport(), nil)
	if len(sent.Messages()) != 0 {
		t.Errorf("sent %d messages", len(sent.Messages()))
	}
}
