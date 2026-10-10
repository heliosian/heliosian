package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/ops"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/vitals"
)

func dashboardAs(t *testing.T, s *Store, queue *store.Queue, board *ops.Board, as, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	RegisterDashboard(mux, s, queue, board)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	return rec
}

func TestTheDashboardIsForSuperAdmins(t *testing.T) {
	s, queue := sampleWithQueue(t)
	board := ops.New(testkit.OpsDeps(t, map[string]ops.Measurable{}))
	vitals.RecordError(time.Now(), "dashboard test failure", map[string]string{"app": "admin"})
	if rec := dashboardAs(t, s, queue, board, "rowan.ashdown@example.org", "/api/dashboard"); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent reads the dashboard: %d", rec.Code)
	}
	if rec := dashboardAs(t, s, queue, board, "rowan.ashdown@example.org", "/"); rec.Code != http.StatusFound || rec.Header().Get("Location") != "/resources" {
		t.Fatalf("a parent at / gets %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	var system map[string]bool
	if rec := dashboardAs(t, s, queue, board, "rowan.ashdown@example.org", "/api/system"); json.Unmarshal(rec.Body.Bytes(), &system) != nil || system["allowed"] {
		t.Fatalf("a parent is told %s", rec.Body.String())
	}
	rec := dashboardAs(t, s, queue, board, "maya.lindqvist@example.org", "/api/dashboard")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("a super admin reads the dashboard: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if strings.Contains(rec.Body.String(), "null") {
		t.Errorf("a list goes out as null: %s", rec.Body.String())
	}
	events := strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n")
	var last dashboard
	if err := json.Unmarshal([]byte(strings.TrimPrefix(events[len(events)-1], "data: ")), &last); err != nil {
		t.Fatal(err)
	}
	if last.ErrorCount == 0 || last.Errors[0].Message != "dashboard test failure" {
		t.Errorf("errors: %d %+v", last.ErrorCount, last.Errors)
	}
	people := 0
	for _, table := range last.Tables {
		if table.Name == "PERSON" {
			people = table.Rows
		}
	}
	if people != 4 {
		t.Errorf("PERSON rows %d", people)
	}
	if len(last.Queues) == 0 || last.Queues[len(last.Queues)-1].Name != "pending writes" {
		t.Errorf("queues: %+v", last.Queues)
	}
}
