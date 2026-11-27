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

func TestAHostPreviewsTheirInvitationAsTheMcTweets(t *testing.T) {
	h := newInviteHarness(t)
	household := func() []string {
		t.Helper()
		rec := h.do(maya, http.MethodPost, "/api/do/preview-invitation", map[string]string{"group": h.event})
		if rec.Code != http.StatusOK {
			t.Fatalf("previewing answered %d %s", rec.Code, rec.Body)
		}
		var view extView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		names := []string{view.Name}
		for _, p := range view.Family {
			names = append(names, p.Name)
		}
		return names
	}
	if got := household(); len(got) != 2 || got[0] != "Piper McTweet" || got[1] != "Wren McTweet" {
		t.Errorf("a party with no kids previewed as %v", got)
	}
	h.setup([]Edit{{Insert: "QUESTION", Row: map[string]any{"group": h.event, "text": "Any allergies?", "kind": "text", "asked": "kids", "order": "1"}}})
	if got := household(); len(got) != 4 || got[2] != "Jay McTweet" || got[3] != "Raven McTweet" {
		t.Errorf("a party asking the kids a question previewed as %v", got)
	}
	before := len(h.s.Model().Table("MEMBER").Referencing("group", h.event))
	if rec := h.do(rowan, http.MethodPost, "/api/do/preview-invitation", map[string]string{"group": h.event}); rec.Code == http.StatusOK {
		t.Errorf("someone who isn't a host previewed it: %s", rec.Body)
	}
	if after := len(h.s.Model().Table("MEMBER").Referencing("group", h.event)); after != before {
		t.Errorf("previewing changed the guest list from %d rows to %d", before, after)
	}
}

func TestClaudeWritesAPageOrMessageFromTheInvitationItself(t *testing.T) {
	asked := ""
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
		asked = testkit.UserText(request)
		return `{"description": "- 6:00pm Doors open"}`
	}))
	h := newInviteHarness(t)
	RegisterDrafts(h.mux, h.s, describe.New("test", claude.NewLimiter()), testTokens)
	rec := h.do(maya, http.MethodPost, "/api/do/describe-guest-words", map[string]string{"group": h.event, "kind": "page", "heading": "Schedule", "notes": "doors 6"})
	if rec.Code != http.StatusOK {
		t.Fatalf("writing a page answered %d %s", rec.Code, rec.Body)
	}
	for _, want := range []string{"Title: Fondue Night", "Where: 8 Kelp Lane", "The page's title: Schedule", "Current text of the page:\ndoors 6"} {
		if !strings.Contains(asked, want) {
			t.Errorf("claude wasn't told %q: %s", want, asked)
		}
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/describe-guest-words", map[string]string{"group": h.event, "kind": "mail"}); rec.Code == http.StatusOK {
		t.Errorf("someone who isn't a host had words written: %s", rec.Body)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/describe-guest-words", map[string]string{"group": h.event, "kind": "poem"}); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind answered %d", rec.Code)
	}
}

func TestAGuestsInvitationCarriesItsPagesInOrder(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Insert: "INVITE_PAGE", Row: map[string]any{"group": h.event, "order": "b", "title": "Onsite Childcare", "body": "Upstairs, **6 to 10**."}},
		{Insert: "INVITE_PAGE", Row: map[string]any{"group": h.event, "order": "a", "title": "Schedule", "body": "Doors at 6."}},
	})
	rec := h.do(maya, http.MethodPost, "/api/do/preview-invitation", map[string]string{"group": h.event})
	var view extView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("previewing answered %d %s", rec.Code, rec.Body)
	}
	if len(view.Pages) != 2 || view.Pages[0].Title != "Schedule" || view.Pages[1].Body != "Upstairs, **6 to 10**." {
		t.Errorf("the invitation's pages came as %+v", view.Pages)
	}
}

func TestAPartyWithAKidOnItsListPreviewsWithTheKids(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "yes"}}})
	if !h.s.Model().hasKids(h.group(), nil) {
		t.Error("a party with a student on its list was taken for one without kids")
	}
}
