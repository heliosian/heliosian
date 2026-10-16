package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
)

func TestAGuestIsFoundOrAddedByAddress(t *testing.T) {
	s, queue := sampleWithQueue(t)
	mux := http.NewServeMux()
	Register(mux, s, queue, newPictures(s, queue), testTokens, func() time.Time { return testNow })
	ask := func(as, body string) (int, string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/do/guest", strings.NewReader(body))
		rec := httptest.NewRecorder()
		auth.Fixed(as, mux).ServeHTTP(rec, r)
		var out written
		json.Unmarshal(rec.Body.Bytes(), &out)
		if len(out.Result) == 0 {
			return rec.Code, ""
		}
		return rec.Code, out.Result[0]
	}
	code, coach := ask("maya.lindqvist@example.org", `{"email": "Coach@Example.net", "name": "Pat  Coach"}`)
	if code != http.StatusOK || coach == "" {
		t.Fatalf("a super admin can't add a guest: %d", code)
	}
	if p, _ := s.Model().Table("PERSON").Get(coach); p["source"] != "guest" || p["name_long_override"] != "Pat Coach" {
		t.Fatalf("the guest reads %v", p)
	}
	if _, again := ask("maya.lindqvist@example.org", `{"email": "coach@example.net"}`); again != coach {
		t.Fatalf("the same address made a second guest: %s", again)
	}
	if _, rowan := ask("maya.lindqvist@example.org", `{"email": "rowan.ashdown@example.org"}`); rowan != parent {
		t.Fatalf("a directory address answered %s", rowan)
	}
	apply(t, s, queue, []Edit{
		{Delete: memberRow(t, s, "grp00000000513", parent)},
		{Delete: memberRow(t, s, "grp00000020073", parent)},
		{Delete: memberRow(t, s, "grp00000020075", parent)},
		{Set: tacoNight, Cells: map[string]any{"join": "none"}},
	})
	if code, _ := ask("rowan.ashdown@example.org", `{"email": "friend@example.net"}`); code != http.StatusForbidden {
		t.Fatalf("someone who runs no group, with no party to bring a guest to, added a guest: %d", code)
	}
	apply(t, s, queue, []Edit{{Set: tacoNight, Cells: map[string]any{"join": "direct"}}})
	if code, _ := ask("rowan.ashdown@example.org", `{"email": "friend@example.net"}`); code != http.StatusOK {
		t.Fatalf("someone who sees a party they could buy at can't add a guest to bring: %d", code)
	}
}

func TestADraftsMembersAreWorkedOutWithoutSaving(t *testing.T) {
	const hummingbirds, picnicManagers = "grp00000000010", "grp00000000041"
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: picnicManagers, Cells: map[string]any{"members_visible_to": ""}}})
	mux := http.NewServeMux()
	RegisterDrafts(mux, s, describe.New("test", claude.NewLimiter()), testTokens, func() time.Time { return testNow })
	ask := func(as, body string) (*httptest.ResponseRecorder, draftAnswer) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/do/draft-members", strings.NewReader(body))
		rec := httptest.NewRecorder()
		auth.Fixed(as, mux).ServeHTTP(rec, r)
		var out draftAnswer
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return rec, out
	}
	rec, out := ask("rowan.ashdown@example.org", `{"rules": [{"target": "`+hummingbirds+`", "replace_with": "parents"}], "members": [{"person": "`+staff+`", "member": "yes"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the draft answered %d: %s", rec.Code, rec.Body)
	}
	if len(out.RuleCounts) != 1 || out.RuleCounts[0] != 11 || len(out.Members) != 12 {
		t.Fatalf("the draft reads %+v", out)
	}
	for _, m := range out.Members {
		switch m.Person {
		case parent:
			if len(m.Reasons) != 1 || m.Reasons[0].Rule == nil || *m.Reasons[0].Rule != 0 || m.Name != "Rowan Ashdown" {
				t.Errorf("the parent's place reads %+v", m)
			}
		case staff:
			if len(m.Reasons) != 1 || !m.Reasons[0].Added {
				t.Errorf("the hand addition reads %+v", m)
			}
		default:
			if len(m.Reasons) != 1 || m.Reasons[0].Rule == nil || *m.Reasons[0].Rule != 0 {
				t.Errorf("someone else is on it: %+v", m)
			}
		}
	}
	if _, out := ask("rowan.ashdown@example.org", `{"rules": [{"target": "`+hummingbirds+`", "replace_with": "parents"}], "members": [{"person": "`+parent+`", "member": "excluded"}]}`); len(out.Members) != 10 || slices.ContainsFunc(out.Members, func(m draftPerson) bool { return m.Person == parent }) {
		t.Fatalf("someone kept off is on the draft: %+v", out)
	}
	if rec, _ := ask("rowan.ashdown@example.org", `{"rules": [{"target": "`+picnicManagers+`"}]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("a rule naming a group whose members are hidden answered %d", rec.Code)
	}
	if rec, _ := ask("rowan.ashdown@example.org", `{"list": "grp00000000030", "rules": []}`); rec.Code != http.StatusForbidden {
		t.Fatalf("a draft of a list the viewer doesn't run answered %d", rec.Code)
	}
	if rec, _ := ask("maya.lindqvist@example.org", `{"list": "grp00000000030", "rules": []}`); rec.Code != http.StatusOK {
		t.Fatalf("a Loop admin's draft of any list answered %d: %s", rec.Code, rec.Body)
	}
}
