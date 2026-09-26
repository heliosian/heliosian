package feedback

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

func sample() Report {
	return Report{
		ID:        "abc123",
		App:       "calendar",
		AppName:   "Helios When",
		Kind:      "bug",
		Status:    StatusNew,
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
	title, body, issueType, labels := Strip(sample())
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
	r := sample()
	r.Kind = "idea"
	_, _, issueType, _ := Strip(r)
	if issueType != "Feature" {
		t.Errorf("type = %q", issueType)
	}
}

func TestStripWithNoApp(t *testing.T) {
	r := sample()
	r.App, r.AppName = "", ""
	title, body, _, labels := Strip(r)
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
	r := sample()
	r.AppName = ""
	title, body, _, labels := Strip(r)
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
	r := sample()
	r.Summary = "mail to head@example.org bounces"
	title, _, _, _ := Strip(r)
	if strings.Contains(title, "head@example.org") || !strings.Contains(title, "[email removed]") {
		t.Errorf("title = %q", title)
	}
}

func appServer(t *testing.T, issue string) (*httptest.Server, *map[string]any, *string) {
	t.Helper()
	var created map[string]any
	var issueAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/installation":
			io.WriteString(w, `{"id":4242}`)
		case "/app/installations/4242/access_tokens":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
				t.Errorf("token minted without a JWT: %q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"token":"ghs_installation","expires_at":"2099-01-01T00:00:00Z"}`)
		case "/repos/" + Repo + "/issues":
			issueAuth = r.Header.Get("Authorization")
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &created); err != nil {
				t.Errorf("payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"`+issue+`"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	return server, &created, &issueAuth
}

func testApp(t *testing.T, endpoint string) *GitHubApp {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &GitHubApp{ID: "12345", PrivateKey: key, Endpoint: endpoint}
}

func TestGitHubAppFiles(t *testing.T) {
	server, created, issueAuth := appServer(t, "https://github.com/heliosian/heliosian/issues/9")
	defer server.Close()
	g := testApp(t, server.URL)
	issue, err := g.File(context.Background(), "A title", "A body", "Bug", []string{"app:calendar"})
	if err != nil {
		t.Fatal(err)
	}
	if issue != "https://github.com/heliosian/heliosian/issues/9" {
		t.Errorf("issue = %q", issue)
	}
	if *issueAuth != "Bearer ghs_installation" {
		t.Errorf("the issue was filed as %q, not the installation", *issueAuth)
	}
	if (*created)["title"] != "A title" {
		t.Errorf("title = %v", (*created)["title"])
	}
	if (*created)["type"] != "Bug" {
		t.Errorf("type = %v", (*created)["type"])
	}
}

func TestGitHubAppReusesItsToken(t *testing.T) {
	mints := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/installation":
			io.WriteString(w, `{"id":7}`)
		case "/app/installations/7/access_tokens":
			mints++
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"token":"t","expires_at":"2099-01-01T00:00:00Z"}`)
		default:
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"https://github.com/x/y/issues/1"}`)
		}
	}))
	defer server.Close()
	g := testApp(t, server.URL)
	for range 3 {
		if _, err := g.File(context.Background(), "t", "b", "Bug", nil); err != nil {
			t.Fatal(err)
		}
	}
	if mints != 1 {
		t.Errorf("minted %d tokens for three issues", mints)
	}
}

func TestGitHubAppRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	g := testApp(t, server.URL)
	_, err := g.File(context.Background(), "t", "b", "Bug", nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v", err)
	}
}

func TestGitHubAppNotSetUp(t *testing.T) {
	g, err := NewGitHubApp("", "")
	if err != nil || g != nil {
		t.Errorf("g = %v err = %v", g, err)
	}
}

func TestHandler(t *testing.T) {
	dir, queue, cache := testCache(t)
	told := make(chan Report, 4)
	mux := http.NewServeMux()
	Register(mux, "calendar", func() string { return "Helios When" }, func(string) bool { return true },
		NewIntake(cache, func(r Report) { told <- r }))
	h := auth.Fixed("Jordan.Whitfield@example.org", mux)
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(body))
		req.Header.Set("User-Agent", "TestBrowser/1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
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
	if rec.Code != http.StatusAccepted {
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
	_, rows, err := dir.Table(appName, reportsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[4]["ID"] != got.ID {
		t.Fatalf("the sheet holds %d reports, the last %v", len(rows), rows[len(rows)-1])
	}
	_, log, err := dir.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0]["Action"] != "insert" || log[0]["Actor"] != "jordan.whitfield@example.org" || log[0]["Key"] != "ID="+got.ID {
		t.Errorf("change log %v", log)
	}
	if got.Email != "jordan.whitfield@example.org" || !got.SuperAdmin || got.AppName != "Helios When" || got.App != "calendar" {
		t.Errorf("identity = %+v", got)
	}
	if got.Summary != "A dark mode" || got.Kind != "idea" || got.UserAgent != "TestBrowser/1" || len(got.Errors) != errorLimit {
		t.Errorf("content = %+v", got)
	}
}

func TestThrottle(t *testing.T) {
	q := NewIntake(nil, nil).recent
	now := time.Now()
	for i := range perWindow {
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
	if !q.Allow("a@example.org", now.Add(window+time.Minute)) {
		t.Error("report after the window refused")
	}
}

func testCache(t *testing.T) (*data.Dir, *store.Queue, *Cache) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	cache, err := NewCache(dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	return dir, queue, cache
}

func TestCacheReadsNewestFirst(t *testing.T) {
	_, _, cache := testCache(t)
	reports := cache.Reports()
	if len(reports) != 4 {
		t.Fatalf("read %d reports", len(reports))
	}
	if reports[0].ID != "b2c3d4e5f6a1" {
		t.Errorf("newest = %q", reports[0].ID)
	}
	if reports[0].Kind != "idea" || !reports[0].SuperAdmin || reports[0].Status != StatusNew {
		t.Errorf("newest = %+v", reports[0])
	}
	filed, ok := cache.Report("c3d4e5f6a1b2")
	if !ok || filed.Status != StatusFiled || filed.Issue == "" || filed.HandledBy == "" {
		t.Errorf("filed = %+v ok = %v", filed, ok)
	}
	if len(reports[0].Errors) != 0 || len(filed.Errors) != 0 {
		t.Errorf("errors read from blank cells")
	}
}

func TestCacheSavesAndHandles(t *testing.T) {
	dir, queue, cache := testCache(t)
	ctx := context.Background()
	saved, err := cache.save(ctx, sample())
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "abc123" || saved.ID == "" || saved.Status != StatusNew {
		t.Errorf("saved = %+v", saved)
	}
	if got, ok := cache.Report(saved.ID); !ok || got.Summary != saved.Summary {
		t.Errorf("not readable back: %+v %v", got, ok)
	}
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	if err := cache.Filed(ctx, saved.ID, "https://github.com/x/y/issues/3", "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	got, _ := cache.Report(saved.ID)
	if got.Status != StatusFiled || got.Issue != "https://github.com/x/y/issues/3" || got.HandledBy != "admin@example.org" {
		t.Errorf("filed = %+v", got)
	}
	if err := cache.Dismissed(ctx, "a1b2c3d4e5f6", "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := cache.Report("a1b2c3d4e5f6"); got.Status != StatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	if err := cache.Filed(ctx, "nope", "x", "y", now); err == nil {
		t.Error("filed a report that does not exist")
	}
	queue.Flush()
	_, log, err := dir.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	dismissed := false
	for _, row := range log {
		if row["Actor"] == "admin@example.org" && row["Action"] == "set" && row["Key"] == "ID=a1b2c3d4e5f6" && row["Column"] == "Status" && row["Previous"] == StatusNew {
			dismissed = true
		}
	}
	if !dismissed {
		t.Errorf("the dismissal's previous status is not in the change log: %v", log)
	}
}

func adminServer(t *testing.T, cache *Cache, filer IssueFiler, who string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	RegisterAdmin(mux, cache, filer, func(email string) bool { return email == "admin@example.org" })
	return auth.Fixed(who, mux)
}

func testAdmin(t *testing.T, filer IssueFiler, who string) (*Cache, http.Handler) {
	t.Helper()
	_, _, cache := testCache(t)
	return cache, adminServer(t, cache, filer, who)
}

type fakeFiler struct {
	title, body, issueType string
	labels                 []string
}

func (f *fakeFiler) File(ctx context.Context, title, body, issueType string, labels []string) (string, error) {
	f.title, f.body, f.issueType, f.labels = title, body, issueType, labels
	return "https://github.com/heliosian/heliosian/issues/77", nil
}

func TestAdminNeedsASuperAdmin(t *testing.T) {
	_, h := testAdmin(t, nil, "member@example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member got %d", rec.Code)
	}
}

func TestAdminListsAndOpens(t *testing.T) {
	_, h := testAdmin(t, &fakeFiler{}, "admin@example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Reports []summary `json:"reports"`
		CanFile bool      `json:"canFile"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Reports) != 4 || !list.CanFile {
		t.Fatalf("list = %+v", list)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback/a1b2c3d4e5f6", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("open: %d %s", rec.Code, rec.Body.String())
	}
	var one detail
	if err := json.Unmarshal(rec.Body.Bytes(), &one); err != nil {
		t.Fatal(err)
	}
	if one.Email != "rowan.avery@example.org" {
		t.Errorf("the admin cannot see who reported it: %+v", one)
	}
	if strings.Contains(one.Draft.Body, "rowan.avery") || strings.Contains(one.Draft.Body, "token=abc123") {
		t.Errorf("the draft carries personal detail:\n%s", one.Draft.Body)
	}
	if one.Draft.Repo != Repo {
		t.Errorf("repo = %q", one.Draft.Repo)
	}
}

func TestAdminFilesAndDismisses(t *testing.T) {
	filer := &fakeFiler{}
	cache, h := testAdmin(t, filer, "admin@example.org")
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}
	if rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"","body":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty title: %d", rec.Code)
	}
	if rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Blank grid","body":"x","type":" "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty type: %d", rec.Code)
	}
	rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Blank grid","body":"Reported by rowan.avery@example.org","type":"Bug"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rowan.avery@example.org") {
		t.Errorf("an address slipped through: %d %s", rec.Code, rec.Body.String())
	}
	rec = post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Blank grid","body":"Forward a month and the grid empties.","type":"Bug","labels":["app:calendar"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("file: %d %s", rec.Code, rec.Body.String())
	}
	if filer.title != "Blank grid" || filer.issueType != "Bug" || strings.Join(filer.labels, ",") != "app:calendar" {
		t.Errorf("filed %q as %q with %v", filer.title, filer.issueType, filer.labels)
	}
	got, _ := cache.Report("a1b2c3d4e5f6")
	if got.Status != StatusFiled || got.Issue != "https://github.com/heliosian/heliosian/issues/77" || got.HandledBy != "admin@example.org" {
		t.Errorf("report after filing = %+v", got)
	}
	if rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Again","body":"x"}`); rec.Code != http.StatusConflict {
		t.Errorf("filed twice: %d", rec.Code)
	}
	if rec := post("/api/admin/feedback/b2c3d4e5f6a1/dismiss", ""); rec.Code != http.StatusNoContent {
		t.Errorf("dismiss: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := cache.Report("b2c3d4e5f6a1"); got.Status != StatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	if rec := post("/api/admin/feedback/nope/dismiss", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown report: %d", rec.Code)
	}
}

func TestAdminWithoutAGitHubApp(t *testing.T) {
	_, h := testAdmin(t, nil, "admin@example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/feedback/a1b2c3d4e5f6/file", strings.NewReader(`{"title":"t","body":"b"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("filing with no app: %d %s", rec.Code, rec.Body.String())
	}
}

type sentMail struct {
	mu       sync.Mutex
	messages []mail.Message
}

func (s *sentMail) Send(ctx context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	return nil
}

func TestNotifyTellsTheSuperAdmins(t *testing.T) {
	sent := &sentMail{}
	n := Notifier{
		Sender:      sent,
		From:        "HCA-Team <team@example.org>",
		Base:        "https://heliosian.com",
		SuperAdmins: func() []string { return []string{"Admin@example.org", " ", "other@example.org"} },
	}
	n.Notify(sample())
	if len(sent.messages) != 1 {
		t.Fatalf("sent %d messages", len(sent.messages))
	}
	m := sent.messages[0]
	if strings.Join(m.To, ",") != "admin@example.org,other@example.org" {
		t.Errorf("to = %v", m.To)
	}
	if !strings.Contains(m.Subject, "Helios When") || !strings.Contains(m.Subject, "Next month") {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"jordan.whitfield@example.org", "https://heliosian.com/admin?panel=feedback&report=abc123"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, m.Text)
		}
	}
}

func TestNotifyWithNobodyToTell(t *testing.T) {
	sent := &sentMail{}
	n := Notifier{Sender: sent, SuperAdmins: func() []string { return nil }}
	n.Notify(sample())
	if len(sent.messages) != 0 {
		t.Errorf("sent %d messages", len(sent.messages))
	}
}
