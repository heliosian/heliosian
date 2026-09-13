package feedback

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/auth"
)

func sample() Report {
	return Report{
		App:       "calendar",
		AppName:   "Helios Calendar",
		Kind:      "bug",
		Summary:   "Next month | shows nothing",
		Details:   "Clicked forward and the grid went blank.",
		Email:     "jordan.whitfield@heliosschool.org",
		URL:       "https://when.heliosian.com/2026-10",
		Page:      "October | Helios Calendar",
		Viewport:  "1440×900",
		UserAgent: "Mozilla/5.0",
		Errors:    []string{"TypeError: x is undefined (app.js:12)"},
		At:        time.Date(2026, 9, 13, 21, 3, 0, 0, time.UTC),
	}
}

func TestRender(t *testing.T) {
	title, body, labels := Render(sample())
	if title != "[Helios Calendar] Next month | shows nothing" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{
		"Clicked forward and the grid went blank.\n",
		"| Page | October \\| Helios Calendar |",
		"| Address | <https://when.heliosian.com/2026-10> |",
		"| Role | member |",
		"| Reported | 2026-09-13 14:03 PDT |",
		"<summary>Recent errors (1)</summary>",
		"- Email: jordan.whitfield@heliosschool.org",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Join(labels, ",") != "bug,app:calendar,untriaged" {
		t.Errorf("labels = %v", labels)
	}
}

func TestGitHubFiles(t *testing.T) {
	var got map[string]any
	var auth, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"html_url":"https://github.com/heliosian/triage/issues/1"}`)
	}))
	defer server.Close()
	g := &GitHub{Token: "tok", Endpoint: server.URL}
	if err := g.File(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer tok" || path != "/repos/heliosian/triage/issues" {
		t.Errorf("auth = %q path = %q", auth, path)
	}
	if got["title"] != "[Helios Calendar] Next month | shows nothing" {
		t.Errorf("title = %v", got["title"])
	}
	if labels, _ := got["labels"].([]any); len(labels) != 3 {
		t.Errorf("labels = %v", got["labels"])
	}
}

func TestGitHubRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	g := &GitHub{Token: "bad", Endpoint: server.URL}
	err := g.File(context.Background(), sample())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v", err)
	}
}

type recorded struct {
	mu      sync.Mutex
	reports []Report
}

func (f *recorded) File(ctx context.Context, r Report) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, r)
	return nil
}

func (f *recorded) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reports)
}

func TestHandler(t *testing.T) {
	filer := &recorded{}
	mux := http.NewServeMux()
	Register(mux, "calendar", func() string { return "Helios Calendar" }, func(string) bool { return true }, NewQueue(filer))
	h := auth.Fixed("Jordan.Whitfield@heliosschool.org", mux)
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
	rec := post(`{"kind":"idea","summary":" A dark mode ","details":"Please","url":"https://when.heliosian.com/","page":"Helios Calendar","errors":["a","b","c","d","e","f","g"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("report: %d %s", rec.Code, rec.Body.String())
	}
	for range 50 {
		if filer.count() == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if filer.count() != 1 {
		t.Fatalf("filed %d reports", filer.count())
	}
	got := filer.reports[0]
	if got.Email != "jordan.whitfield@heliosschool.org" || !got.SuperAdmin || got.AppName != "Helios Calendar" || got.App != "calendar" {
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
		if !q.allow("a@heliosschool.org", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("report %d refused", i)
		}
	}
	if q.allow("a@heliosschool.org", now.Add(time.Minute)) {
		t.Error("sixth report allowed")
	}
	if !q.allow("b@heliosschool.org", now) {
		t.Error("another person refused")
	}
	if !q.allow("a@heliosschool.org", now.Add(window+time.Minute)) {
		t.Error("report after the window refused")
	}
}
