package db

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/intercept"
	"heliosian/internal/testkit"
)

func TestClaudeWritesAnInvitationsSummaryFromItsNotes(t *testing.T) {
	asked := ""
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
		asked = testkit.UserText(request)
		return `{"description": "Calling all Earthlings! Theo is turning eight."}`
	}))
	s := sample(t)
	mux := http.NewServeMux()
	RegisterDrafts(mux, s, describe.New("test", claude.NewLimiter()), testTokens)
	rec := doCall(t, mux, "rowan.ashdown@example.org", "/api/do/describe-event", `{"title": "Theo Turns Eight", "when": "Saturday, December 12, 2:00 – 4:30 PM", "where": "14 Cypress Ave", "hosts": "Maya Lindqvist", "kicker": "Greetings, Earthlings", "notes": "bounce house, pizza, cake"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("describing an invitation answered %d %s", rec.Code, rec.Body)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["description"] != "Calling all Earthlings! Theo is turning eight." {
		t.Errorf("the summary came back as %s", rec.Body)
	}
	for _, want := range []string{"Title: Theo Turns Eight", "Hosted by: Maya Lindqvist", "The design's line above the title: Greetings, Earthlings", "Current summary:\nbounce house, pizza, cake"} {
		if !strings.Contains(asked, want) {
			t.Errorf("claude wasn't told %q: %s", want, asked)
		}
	}
	if rec := doCall(t, mux, "rowan.ashdown@example.org", "/api/do/describe-event", `{"title": `); rec.Code != http.StatusBadRequest {
		t.Errorf("an invitation that is not JSON answered %d", rec.Code)
	}
}
