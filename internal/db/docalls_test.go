package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
)

func doCall(t *testing.T, mux http.Handler, as, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	serveAs(mux, rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)), as)
	return rec
}

func TestSubscriptionTurnsAListsMailOffAndOn(t *testing.T) {
	const hummingbirdsParents = "grp00000000030"
	s, queue := sampleWithQueue(t)
	mux := http.NewServeMux()
	Register(mux, s, queue, newPictures(s, queue), testTokens, func() time.Time { return testNow })
	rowan := "rowan.ashdown@example.org"
	if rec := doCall(t, mux, rowan, "/api/do/subscription", `{"group": "`+hummingbirdsParents+`", "subscribed": false}`); rec.Code != http.StatusOK {
		t.Fatalf("unsubscribing answered %d %s", rec.Code, rec.Body)
	}
	list, _ := s.Model().Table("GROUP").Get(hummingbirdsParents)
	if !s.Model().unsubscribed(list, parent) || !s.Model().effectivelyIn(hummingbirdsParents, parent) {
		t.Fatal("unsubscribing did not stop the mail and keep the membership")
	}
	if rec := doCall(t, mux, rowan, "/api/do/subscription", `{"group": "`+hummingbirdsParents+`", "subscribed": true}`); rec.Code != http.StatusOK {
		t.Fatalf("subscribing answered %d %s", rec.Code, rec.Body)
	}
	if list, _ := s.Model().Table("GROUP").Get(hummingbirdsParents); s.Model().unsubscribed(list, parent) {
		t.Fatal("subscribing again left them unsubscribed")
	}
	if rec := doCall(t, mux, rowan, "/api/do/subscription", `{"group": "grp00000000010"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unsubscribing from a group they are not in answered %d", rec.Code)
	}
	if rec := doCall(t, mux, rowan, "/api/do/subscription", `{"group": `); rec.Code != http.StatusBadRequest {
		t.Errorf("a body that is not JSON answered %d", rec.Code)
	}
}

func TestFetchedTakesOnlyALinkStillToFetch(t *testing.T) {
	s, queue := sampleWithQueue(t)
	if rec := postFile(t, s, queue, newPictures(s, queue), superAdmin, "fetched", map[string]string{"document": "doc00000000000"}, "body", []byte("<html></html>")); rec.Code != http.StatusBadRequest {
		t.Fatalf("filling a document that is not a link to fetch answered %d %s", rec.Code, rec.Body)
	}
}

func TestClaudeCallsCheckBeforeAskingClaude(t *testing.T) {
	s, _ := sampleWithQueue(t)
	mux := http.NewServeMux()
	now := func() time.Time { return testNow }
	RegisterDrafts(mux, s, describe.New("test", claude.NewLimiter()), testTokens, now)
	RegisterCompose(mux, s, NewComposer("test", claude.NewLimiter()), testTokens, now)
	team, err := Parse(`(from APP (where (= key "birthday") (birthday_team)))`)
	if err != nil {
		t.Fatal(err)
	}
	outsider := ""
	for _, e := range s.Model().Table("PERSON_EMAIL").All() {
		viewer := s.Model().SignedIn(e["address"])
		if viewer != "" && len(s.Model().Run(context.Background(), team, Env{Viewer: viewer, Now: testNow}).IDs) == 0 {
			outsider = e["address"]
			break
		}
	}
	if outsider == "" {
		t.Fatal("everyone who signs in is on the Birthdays team")
	}
	if rec := doCall(t, mux, outsider, "/api/do/describe-charity", `{"name": "Fogline Arts"}`); rec.Code != http.StatusForbidden {
		t.Errorf("someone off the Birthdays team looking a charity up answered %d", rec.Code)
	}
	if rec := doCall(t, mux, superAdmin, "/api/do/describe-charity", `{"name": "  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a charity with no name answered %d", rec.Code)
	}
	if rec := doCall(t, mux, superAdmin, "/api/do/describe-activity", `{"title": `); rec.Code != http.StatusBadRequest {
		t.Errorf("an activity that is not JSON answered %d", rec.Code)
	}
	if rec := doCall(t, mux, superAdmin, "/api/do/compose", `{"words": "  "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("composing from no words answered %d", rec.Code)
	}
}

func TestReportTriageIsForSuperAdmins(t *testing.T) {
	s, queue := sampleWithQueue(t)
	report := apply(t, s, queue, []Edit{{Insert: "REPORT", Row: map[string]any{"app": "who", "kind": "bug", "summary": "Broken", "status": "new", "added_by": parent, "added": "2026-09-01 10:00:00"}}})[0]
	mux := http.NewServeMux()
	NewFeedback(s, queue, blob.NewMemoryBucket(), testTokens, func() time.Time { return testNow }, FeedbackConfig{}).RegisterTriage(mux, func(string) string { return "Helios Who?" })
	ask := `{"report": "` + report + `", "title": "Broken", "body": "It broke.", "type": "Bug"}`
	if rec := doCall(t, mux, "rowan.ashdown@example.org", "/api/do/report-draft", ask); rec.Code != http.StatusForbidden {
		t.Errorf("a parent drafting an issue answered %d", rec.Code)
	}
	if rec := doCall(t, mux, "rowan.ashdown@example.org", "/api/do/file-report", ask); rec.Code != http.StatusForbidden {
		t.Errorf("a parent filing an issue answered %d", rec.Code)
	}
	rec := doCall(t, mux, superAdmin, "/api/do/report-draft", ask)
	var draft issueDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); rec.Code != http.StatusOK || err != nil || draft.Title != "[Helios Who?] Broken" || draft.Filer {
		t.Fatalf("a super admin's draft answered %d %s", rec.Code, rec.Body)
	}
	if rec := doCall(t, mux, superAdmin, "/api/do/report-draft", `{"report": "rep00000000000"}`); rec.Code != http.StatusNotFound {
		t.Errorf("drafting from no report answered %d", rec.Code)
	}
	if rec := doCall(t, mux, superAdmin, "/api/do/file-report", ask); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("filing with no GitHub set up answered %d %s", rec.Code, rec.Body)
	}
	apply(t, s, queue, []Edit{{Set: report, Cells: map[string]any{"status": "filed"}}})
	if rec := doCall(t, mux, superAdmin, "/api/do/file-report", ask); rec.Code != http.StatusConflict {
		t.Errorf("filing a report filed already answered %d", rec.Code)
	}
}
