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

func TestAGuestIsFoundByAddressOrAdded(t *testing.T) {
	s := sample(t)
	mux := http.NewServeMux()
	Register(mux, s, newPictures(s), testTokens, func() time.Time { return testNow })
	personAt := func(as, address string) string {
		t.Helper()
		rec := doCall(t, mux, as, "/api/do/person-at", `{"address": "`+address+`"}`)
		var out struct {
			Person string `json:"person"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); rec.Code != http.StatusOK || err != nil {
			t.Fatalf("person-at answered %d %s", rec.Code, rec.Body)
		}
		return out.Person
	}
	addGuest := func(as, address string) *httptest.ResponseRecorder {
		t.Helper()
		return send(t, s, http.MethodPost, "application/json", as, `{"batch": [
			{"insert": "PERSON", "as": "g", "row": {"source": "guest", "name_long_override": "Pat Coach"}},
			{"insert": "PERSON_EMAIL", "row": {"person": "@g", "address": "`+address+`", "primary": true, "source": "guest"}}]}`)
	}
	if got := personAt(superAdmin, "coach@example.net"); got != "" {
		t.Fatalf("an address nobody holds answered %s", got)
	}
	if rec := addGuest(superAdmin, "coach@example.net"); rec.Code != http.StatusOK {
		t.Fatalf("a super admin can't add a guest: %d %s", rec.Code, rec.Body)
	}
	coach := personAt(superAdmin, " Coach@Example.net ")
	if p, _ := s.Model().Table("PERSON").Get(coach); p["source"] != "guest" || p["name_long_override"] != "Pat Coach" {
		t.Fatalf("the guest reads %v", p)
	}
	if got := personAt("rowan.ashdown@example.org", "coach@example.net"); got != coach {
		t.Fatalf("someone who can't read the guest was answered %q", got)
	}
	if got := personAt(superAdmin, "rowan.ashdown@example.org"); got != parent {
		t.Fatalf("a directory address answered %s", got)
	}
	apply(t, s, []Edit{
		{Delete: memberRow(t, s, "grp00000000513", parent)},
		{Delete: memberRow(t, s, "grp00000020073", parent)},
		{Delete: memberRow(t, s, "grp00000020075", parent)},
		{Set: tacoNight, Cells: map[string]any{"join": "none"}},
	})
	if rec := addGuest("rowan.ashdown@example.org", "friend@example.net"); rec.Code != http.StatusForbidden {
		t.Fatalf("someone who runs no group, with no party to bring a guest to, added a guest: %d", rec.Code)
	}
	apply(t, s, []Edit{{Set: tacoNight, Cells: map[string]any{"join": "direct"}}})
	if rec := addGuest("rowan.ashdown@example.org", "friend@example.net"); rec.Code != http.StatusOK {
		t.Fatalf("someone who sees a party they could buy at can't add a guest to bring: %d %s", rec.Code, rec.Body)
	}
}

func TestADraftsMembersAreWorkedOutWithoutSaving(t *testing.T) {
	const hummingbirds, picnicManagers = "grp00000000010", "grp00000000041"
	s := sample(t)
	apply(t, s, []Edit{{Set: picnicManagers, Cells: map[string]any{"members_visible_to": ""}}})
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
