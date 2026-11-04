package db

import (
	"encoding/json"
	"net/http"
	"testing"

	"heliosian/internal/claude"
	"heliosian/internal/describe"
)

func TestActivityDraftsPreserveInvitationAndVolunteerStates(t *testing.T) {
	h := newInviteHarness(t)
	ids := h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity"}},
		{Insert: "PERSON", As: "opened", Row: map[string]any{"source": "guest", "name_long_override": "Opened visitor"}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": "@opened", "opened": "2026-11-03 12:00"}},
		{Insert: "PERSON", As: "cancelled", Row: map[string]any{"source": "guest", "name_long_override": "Cancelled visitor"}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": "@cancelled", "member": "cancelled"}},
	})
	RegisterDrafts(h.mux, h.s, describe.New("test", claude.NewLimiter()), testTokens)
	draft := groupDraft{
		List:  h.event,
		Rules: []draftRule{{Person: student}},
		Members: []draftMember{
			{Person: parent, Invited: "yes"},
			{Person: student, Invited: "excluded"},
			{Person: staff, Member: "yes", Invited: "excluded"},
			{Person: ids[1]},
			{Person: ids[3], Member: "cancelled"},
		},
	}
	ask := func(d groupDraft) (int, draftAnswer) {
		t.Helper()
		rec := h.do(maya, http.MethodPost, "/api/do/draft-members", d)
		var out draftAnswer
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Logf("draft refused: %d %s", rec.Code, rec.Body)
		}
		return rec.Code, out
	}
	code, out := ask(draft)
	if code != http.StatusOK || len(out.Members) != 2 {
		t.Fatalf("activity draft with invitation metadata: %d %+v", code, out)
	}
	for _, person := range out.Members {
		if person.Person == parent && (len(person.Reasons) != 1 || !person.Reasons[0].Invited) {
			t.Fatalf("invitation reason lost: %+v", person)
		}
		if person.Person == staff && (len(person.Reasons) != 1 || !person.Reasons[0].Added) {
			t.Fatalf("volunteer exclusion precedence changed: %+v", person)
		}
		if person.Person != parent && person.Person != staff {
			t.Fatalf("inactive invitation or cancelled row entered the draft: %+v", person)
		}
	}
	draft.Members[0].Member = "excluded"
	if code, out := ask(draft); code != http.StatusOK || len(out.Members) != 1 || out.Members[0].Person != staff {
		t.Fatalf("ordinary exclusion did not outweigh invitation: %d %+v", code, out)
	}
	draft.Members[0].Member = ""
	draft.Members[0].Invited = "wrong"
	if code, _ := ask(draft); code != http.StatusBadRequest {
		t.Fatalf("invalid invitation state accepted: %d", code)
	}
	draft.List = "grp00000000040"
	draft.Members[0].Invited = "yes"
	if code, _ := ask(draft); code != http.StatusBadRequest {
		t.Fatalf("nonactivity invitation state accepted: %d", code)
	}
	if h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatal("preview wrote a volunteer membership")
	}
}
