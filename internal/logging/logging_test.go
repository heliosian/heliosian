package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/serve"
	"heliosian/internal/testkit/authtest"
	"heliosian/internal/workload"
)

func TestTraceOf(t *testing.T) {
	trace, span := traceOf("105445aa7843bc8bf206b12000100000/255;o=1")
	if trace != "projects/heliosian/traces/105445aa7843bc8bf206b12000100000" {
		t.Errorf("trace = %q", trace)
	}
	if span != "00000000000000ff" {
		t.Errorf("span = %q", span)
	}
	for _, header := range []string{"", "abc", "105445aa7843bc8bf206b12000100000/x"} {
		if trace, span := traceOf(header); trace != "" || span != "" {
			t.Errorf("traceOf(%q) = %q, %q", header, trace, span)
		}
	}
}

func TestRequestsTagsRecords(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(handler{slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	inner := http.NewServeMux()
	inner.HandleFunc("GET /people", func(w http.ResponseWriter, r *http.Request) {
		slog.WarnContext(r.Context(), "inside", "n", 1)
		w.WriteHeader(http.StatusTeapot)
	})
	h := authtest.Fixed("Someone@heliosschool.org", Requests("who", inner))
	r := httptest.NewRequest("GET", "/people?x=1", nil)
	r.Header.Set("X-Cloud-Trace-Context", "105445aa7843bc8bf206b12000100000/255;o=1")
	h.ServeHTTP(httptest.NewRecorder(), r)

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("got %d records: %s", len(lines), out.String())
	}
	var inside, request map[string]any
	if err := json.Unmarshal(lines[0], &inside); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lines[1], &request); err != nil {
		t.Fatal(err)
	}
	for _, record := range []map[string]any{inside, request} {
		if record["app"] != "who" || record["user"] != "someone@heliosschool.org" {
			t.Errorf("app/user = %v/%v", record["app"], record["user"])
		}
		if record["logging.googleapis.com/trace"] != "projects/heliosian/traces/105445aa7843bc8bf206b12000100000" {
			t.Errorf("trace = %v", record["logging.googleapis.com/trace"])
		}
		if record["logging.googleapis.com/spanId"] != "00000000000000ff" {
			t.Errorf("span = %v", record["logging.googleapis.com/spanId"])
		}
		if _, ok := record["level"]; ok {
			t.Errorf("level key survived: %v", record)
		}
		if _, ok := record["msg"]; ok {
			t.Errorf("msg key survived: %v", record)
		}
	}
	if inside["severity"] != "WARNING" || inside["message"] != "inside" || inside["n"] != float64(1) {
		t.Errorf("inside = %v", inside)
	}
	if request["severity"] != "INFO" || request["message"] != "request" {
		t.Errorf("request = %v", request)
	}
	summary, _ := request["httpRequest"].(map[string]any)
	if summary["requestMethod"] != "GET" || summary["requestUrl"] != "GET /people" || summary["status"] != float64(418) {
		t.Errorf("httpRequest = %v", summary)
	}
}

func TestRequestsDoNotLogBearerURLs(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(handler{slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /open/feed/{file}", func(w http.ResponseWriter, r *http.Request) {
		serve.Error(w, r, errors.New("feed failed"))
	})
	r := httptest.NewRequest("GET", "/open/feed/fed-secret.ics?token=query-secret", nil)
	w := httptest.NewRecorder()
	Requests("when", mux).ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if bytes.Contains(out.Bytes(), []byte("fed-secret")) || bytes.Contains(out.Bytes(), []byte("query-secret")) {
		t.Fatalf("bearer URL leaked into application log: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"requestUrl":"GET /open/feed/{file}"`)) {
		t.Errorf("matched route missing from request log: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"path":"GET /open/feed/{file}"`)) {
		t.Errorf("matched route missing from error log: %s", out.String())
	}
}

func TestRequestsThroughWorkloadLogMatchedRoute(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(handler{slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	mux := http.NewServeMux()
	mux.HandleFunc("QUERY /api/q", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r := httptest.NewRequest("QUERY", "/api/q?token=query-secret", nil)
	workload.New().Handler(Requests("who", mux)).ServeHTTP(httptest.NewRecorder(), r)
	if bytes.Contains(out.Bytes(), []byte("query-secret")) {
		t.Fatalf("query leaked into API request log: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"requestUrl":"QUERY /api/q"`)) {
		t.Errorf("matched API route missing from request log: %s", out.String())
	}
}

func TestRequestsTagATokensRecordsOnceItIsVerified(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(handler{slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "before")
		auth.Identify(r, "Admin@heliosschool.org", "parent@heliosschool.org")
		slog.WarnContext(r.Context(), "after")
	})
	r := httptest.NewRequest("POST", "/api/q", nil)
	r.Header.Set("Authorization", "Bearer a-token")
	authtest.Fixed("", Requests("who", inner)).ServeHTTP(httptest.NewRecorder(), r)

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("got %d records: %s", len(lines), out.String())
	}
	records := []map[string]any{}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if _, ok := records[0]["user"]; ok || records[0]["message"] != "before" {
		t.Errorf("before the token check = %v, want no user", records[0])
	}
	for _, record := range records[1:] {
		if record["user"] != "admin@heliosschool.org" || record["as"] != "parent@heliosschool.org" {
			t.Errorf("%v: user/as = %v/%v, want the token's person and whom it ran as", record["message"], record["user"], record["as"])
		}
	}
	if records[2]["message"] != "request" {
		t.Errorf("last record = %v, want the request line", records[2])
	}
}

func TestSeverity(t *testing.T) {
	for level, want := range map[slog.Level]string{slog.LevelDebug: "DEBUG", slog.LevelInfo: "INFO", slog.LevelWarn: "WARNING", slog.LevelError: "ERROR"} {
		if got := severity(level); got != want {
			t.Errorf("severity(%v) = %q, want %q", level, got, want)
		}
	}
}
