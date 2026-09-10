package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/auth"
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

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.WarnContext(r.Context(), "inside", "n", 1)
		w.WriteHeader(http.StatusTeapot)
	})
	h := auth.Fixed("Someone@heliosschool.org", Requests("who", func(string) bool { return false }, inner))
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
	if summary["requestMethod"] != "GET" || summary["requestUrl"] != "/people?x=1" || summary["status"] != float64(418) {
		t.Errorf("httpRequest = %v", summary)
	}
}

func TestRequestsSkipsMediaRecords(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(handler{slog.NewJSONHandler(&out, nil)})
	previous := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(previous)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.InfoContext(r.Context(), "served")
	})
	h := Requests("who", func(path string) bool { return path == "/photos/x.jpg" }, inner)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/photos/x.jpg", nil))

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("expected exactly one record, got %q: %v", out.String(), err)
	}
	if record["msg"] != "served" || record["app"] != "who" {
		t.Errorf("record = %v", record)
	}
	if _, ok := record["user"]; ok {
		t.Errorf("user set with no session: %v", record)
	}
}

func TestSeverity(t *testing.T) {
	for level, want := range map[slog.Level]string{slog.LevelDebug: "DEBUG", slog.LevelInfo: "INFO", slog.LevelWarn: "WARNING", slog.LevelError: "ERROR"} {
		if got := severity(level); got != want {
			t.Errorf("severity(%v) = %q, want %q", level, got, want)
		}
	}
}
