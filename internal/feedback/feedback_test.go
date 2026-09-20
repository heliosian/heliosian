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
	title, body, labels := Strip(sample())
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
	if strings.Join(labels, ",") != "bug,app:calendar" {
		t.Errorf("labels = %v", labels)
	}
}

// A report carried over from triage may name no app at all, having been filed
// on GitHub by hand rather than through the toolbar.
func TestStripWithNoApp(t *testing.T) {
	r := sample()
	r.App, r.AppName = "", ""
	title, body, labels := Strip(r)
	if title != "Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	if strings.Join(labels, ",") != "bug" {
		t.Errorf("labels = %v", labels)
	}
	if strings.Contains(body, "| App |") {
		t.Errorf("body names an app it has none of:\n%s", body)
	}
}

func TestStripWithOnlyAnAppKey(t *testing.T) {
	r := sample()
	r.AppName = ""
	title, body, labels := Strip(r)
	if title != "Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	if strings.Join(labels, ",") != "bug,app:calendar" {
		t.Errorf("labels = %v", labels)
	}
	if !strings.Contains(body, "| App | `calendar` |") {
		t.Errorf("body lacks the app key:\n%s", body)
	}
}

func TestStripRedactsAnAddressInTheSummary(t *testing.T) {
	r := sample()
	r.Summary = "mail to head@example.org bounces"
	title, _, _ := Strip(r)
	if strings.Contains(title, "head@example.org") || !strings.Contains(title, "[email removed]") {
		t.Errorf("title = %q", title)
	}
}

// appServer is a stand-in GitHub: it mints an installation token for the app's
// JWT and takes one issue.
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

// testKey is a throwaway RSA key generated per run; nothing signs anything real.
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
	issue, err := g.File(context.Background(), "A title", "A body", []string{"bug", "app:calendar"})
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
		if _, err := g.File(context.Background(), "t", "b", nil); err != nil {
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
	_, err := g.File(context.Background(), "t", "b", nil)
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

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

type recorded struct {
	mu      sync.Mutex
	reports []Report
}

func (f *recorded) Save(r Report) (Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = "id"
	f.reports = append(f.reports, r)
	return r, nil
}

func (f *recorded) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reports)
}

func TestHandler(t *testing.T) {
	saver := &recorded{}
	told := make(chan Report, 4)
	mux := http.NewServeMux()
	Register(mux, "calendar", func() string { return "Helios When" }, func(string) bool { return true },
		NewQueue(saver, func(r Report) { told <- r }))
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
	select {
	case got := <-told:
		if got.ID != "id" {
			t.Errorf("announced before it was saved: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nobody was told")
	}
	if saver.count() != 1 {
		t.Fatalf("saved %d reports", saver.count())
	}
	got := saver.reports[0]
	if got.Email != "jordan.whitfield@example.org" || !got.SuperAdmin || got.AppName != "Helios When" || got.App != "calendar" {
		t.Errorf("identity = %+v", got)
	}
	if got.Summary != "A dark mode" || got.Kind != "idea" || got.UserAgent != "TestBrowser/1" || len(got.Errors) != errorLimit {
		t.Errorf("content = %+v", got)
	}
}

func TestThrottle(t *testing.T) {
	q := &Queue{recent: map[string][]time.Time{}}
	now := time.Now()
	for i := range perWindow {
		if !q.allow("a@example.org", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("report %d refused", i)
		}
	}
	if q.allow("a@example.org", now.Add(time.Minute)) {
		t.Error("sixth report allowed")
	}
	if !q.allow("b@example.org", now) {
		t.Error("another person refused")
	}
	if !q.allow("a@example.org", now.Add(window+time.Minute)) {
		t.Error("report after the window refused")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	store, err := NewStore(dir, dir, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStoreReadsNewestFirst(t *testing.T) {
	store := testStore(t)
	reports := store.Reports()
	if len(reports) != 4 {
		t.Fatalf("read %d reports", len(reports))
	}
	if reports[0].ID != "b2c3d4e5f6a1" {
		t.Errorf("newest = %q", reports[0].ID)
	}
	if reports[0].Kind != "idea" || !reports[0].SuperAdmin || reports[0].Status != StatusNew {
		t.Errorf("newest = %+v", reports[0])
	}
	filed, ok := store.Report("c3d4e5f6a1b2")
	if !ok || filed.Status != StatusFiled || filed.Issue == "" || filed.HandledBy == "" {
		t.Errorf("filed = %+v ok = %v", filed, ok)
	}
	if len(reports[0].Errors) != 0 || len(filed.Errors) != 0 {
		t.Errorf("errors read from blank cells")
	}
}

func TestStoreSavesAndHandles(t *testing.T) {
	store := testStore(t)
	saved, err := store.Save(sample())
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "abc123" || saved.ID == "" || saved.Status != StatusNew {
		t.Errorf("saved = %+v", saved)
	}
	if got, ok := store.Report(saved.ID); !ok || got.Summary != saved.Summary {
		t.Errorf("not readable back: %+v %v", got, ok)
	}
	now := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	if err := store.Filed(saved.ID, "https://github.com/x/y/issues/3", "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Report(saved.ID)
	if got.Status != StatusFiled || got.Issue != "https://github.com/x/y/issues/3" || got.HandledBy != "admin@example.org" {
		t.Errorf("filed = %+v", got)
	}
	if err := store.Dismissed("a1b2c3d4e5f6", "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Report("a1b2c3d4e5f6"); got.Status != StatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	if err := store.Filed("nope", "x", "y", now); err == nil {
		t.Error("filed a report that does not exist")
	}
}

func adminServer(t *testing.T, store *Store, filer IssueFiler, who string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	RegisterAdmin(mux, store, filer, func(email string) bool { return email == "admin@example.org" })
	return auth.Fixed(who, mux)
}

type fakeFiler struct {
	title, body string
	labels      []string
}

func (f *fakeFiler) File(ctx context.Context, title, body string, labels []string) (string, error) {
	f.title, f.body, f.labels = title, body, labels
	return "https://github.com/heliosian/heliosian/issues/77", nil
}

func TestAdminNeedsASuperAdmin(t *testing.T) {
	h := adminServer(t, testStore(t), nil, "member@example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("a member got %d", rec.Code)
	}
}

func TestAdminListsAndOpens(t *testing.T) {
	h := adminServer(t, testStore(t), &fakeFiler{}, "admin@example.org")
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
	store := testStore(t)
	filer := &fakeFiler{}
	h := adminServer(t, store, filer, "admin@example.org")
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}
	if rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"","body":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty title: %d", rec.Code)
	}
	rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Blank grid","body":"Reported by rowan.avery@example.org","labels":["bug"]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "rowan.avery@example.org") {
		t.Errorf("an address slipped through: %d %s", rec.Code, rec.Body.String())
	}
	rec = post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Blank grid","body":"Forward a month and the grid empties.","labels":["bug","app:calendar"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("file: %d %s", rec.Code, rec.Body.String())
	}
	if filer.title != "Blank grid" || strings.Join(filer.labels, ",") != "bug,app:calendar" {
		t.Errorf("filed %q with %v", filer.title, filer.labels)
	}
	got, _ := store.Report("a1b2c3d4e5f6")
	if got.Status != StatusFiled || got.Issue != "https://github.com/heliosian/heliosian/issues/77" || got.HandledBy != "admin@example.org" {
		t.Errorf("report after filing = %+v", got)
	}
	if rec := post("/api/admin/feedback/a1b2c3d4e5f6/file", `{"title":"Again","body":"x"}`); rec.Code != http.StatusConflict {
		t.Errorf("filed twice: %d", rec.Code)
	}
	if rec := post("/api/admin/feedback/b2c3d4e5f6a1/dismiss", ""); rec.Code != http.StatusNoContent {
		t.Errorf("dismiss: %d %s", rec.Code, rec.Body.String())
	}
	if got, _ := store.Report("b2c3d4e5f6a1"); got.Status != StatusDismissed {
		t.Errorf("dismissed = %+v", got)
	}
	if rec := post("/api/admin/feedback/nope/dismiss", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown report: %d", rec.Code)
	}
}

func TestAdminWithoutAGitHubApp(t *testing.T) {
	h := adminServer(t, testStore(t), nil, "admin@example.org")
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
