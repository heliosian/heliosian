package db

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func TestAnInvitationsQuestionsAreTheHostsAndItsAnswersTheHouseholds(t *testing.T) {
	const (
		theo      = "grp00000009001"
		allergies = "qst00000009001"
		cleanup   = "qst00000009004"
		ashdowns  = "grp00000000020"
		outsider  = "per00000020050"
	)
	s := sample(t)
	write := func(viewer string, edits ...Edit) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
		return err
	}
	question := func(viewer string) error {
		return write(viewer, Edit{Insert: "QUESTION", Row: map[string]any{"group": theo, "order": "m", "text": "Favorite cake?", "kind": "text", "asked": "everyone"}})
	}
	if err := question(parent); err == nil {
		t.Error("a guest gave the invitation a question")
	}
	if err := question(staff); err != nil {
		t.Errorf("the host can't give the invitation a question: %v", err)
	}
	answer := func(viewer string, row map[string]any) error {
		row["added_by"] = viewer
		return write(viewer, Edit{Insert: "ANSWER", Row: row})
	}
	if err := answer(parent, map[string]any{"question": allergies, "person": student, "value": "Peanuts"}); err != nil {
		t.Errorf("a parent can't answer for their child: %v", err)
	}
	if err := answer(parent, map[string]any{"question": allergies, "person": staff, "value": "None"}); err == nil {
		t.Error("a parent answered for someone outside their household")
	}
	if err := answer(parent, map[string]any{"question": cleanup, "person": parent, "value": "yes"}); err == nil {
		t.Error("a parent answered a family's question as their own")
	}
	if err := answer(parent, map[string]any{"question": cleanup, "family": ashdowns, "value": "yes"}); err != nil {
		t.Errorf("a parent can't answer for their family: %v", err)
	}
	for viewer, want := range map[string]int{parent: 2, student: 2, staff: 2, outsider: 0} {
		if got := len(as(t, s, viewer, "(from ANSWER)")); got != want {
			t.Errorf("%s reads %d answers, want %d", viewer, got, want)
		}
	}
	apply(t, s, []Edit{{Delete: allergies}})
	if got := len(as(t, s, staff, "(from ANSWER)")); got != 1 {
		t.Errorf("a question taken away left %d answers, want the family's alone", got)
	}
}

func TestAHostGivesAnInvitationAShortLink(t *testing.T) {
	const (
		theo   = "grp00000009001"
		winter = "grp00000009011"
	)
	s := sample(t)
	slug := func(group, word string) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: staff, Now: testNow}, Batch{Batch: []Edit{{Set: group, Cells: map[string]any{"slug": word}}}})
		return err
	}
	if err := slug(theo, "theo8"); err != nil {
		t.Fatalf("the host can't give the invitation a short link: %v", err)
	}
	m := s.Model()
	for _, key := range []string{"theo8", "theo-turns-eight", theo} {
		if g := m.sharedGroup(key); g == nil || g["id"] != theo {
			t.Errorf("/g/%s does not open the invitation", key)
		}
	}
	if g, _ := m.Table("GROUP").Get(theo); g["path"] != "/g/theo8" {
		t.Errorf("the invitation's path is %q", g["path"])
	}
	if err := slug(winter, "theo8"); err == nil || !strings.Contains(err.Error(), "two rows have the slug") {
		t.Errorf("a second invitation took a link already in use: %v", err)
	}
	if err := slug(theo, ""); err != nil || s.Model().sharedGroup("theo8") == nil {
		t.Errorf("taking the short link away broke it: %v", err)
	}
}

func TestAnOutsideGuestAnswersQuestionsFromTheirOwnPage(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds someone from outside: %d %s", rec.Code, rec.Body)
	}
	ids := h.setup([]Edit{
		{Insert: "QUESTION", Row: map[string]any{"group": h.event, "order": "m", "text": "Shirt size?", "kind": "choice", "choices": "S\nM\nL", "asked": "kids", "required": true}},
		{Insert: "QUESTION", Row: map[string]any{"group": h.event, "order": "n", "text": "Can you help clean up?", "kind": "yes_no", "asked": "household"}},
		{Set: h.event, Cells: map[string]any{"auto_invite": true}},
	})
	shirt, cleanup := ids[0], ids[1]
	coach := h.s.Model().personAt("coach@example.net")
	h.mailTo("coach@example.net", "You're invited")
	token := h.copyOf(coach)["token"]
	path := "/open/ext/" + token
	if rec := h.do("", http.MethodPost, path+"/guest", map[string]any{"name": "Kid Pat", "child": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("bringing a child: %d %s", rec.Code, rec.Body)
	}
	var view extView
	rec := h.do("", http.MethodGet, path, nil)
	json.Unmarshal(rec.Body.Bytes(), &view)
	if len(view.Questions) != 2 || view.Questions[0].ID != shirt || !view.Questions[0].Required || len(view.Questions[0].Choices) != 3 {
		t.Fatalf("the outside page's questions: %+v", view.Questions)
	}
	if view.Key != coach || view.Kid || len(view.Brought) != 1 || !view.Brought[0].Kid {
		t.Fatalf("the outside page's people: key %q kid %v brought %+v", view.Key, view.Kid, view.Brought)
	}
	kid := view.Brought[0].Key
	reply := func(body map[string]string) int {
		return h.do("", http.MethodPost, path+"/reply", body).Code
	}
	if code := reply(map[string]string{"question": shirt, "person": kid, "value": "M"}); code != http.StatusNoContent {
		t.Errorf("answering for a guest they brought: %d", code)
	}
	if code := reply(map[string]string{"question": shirt, "person": kid, "value": "L"}); code != http.StatusNoContent {
		t.Errorf("changing an answer: %d", code)
	}
	if code := reply(map[string]string{"question": shirt, "person": parent, "value": "S"}); code != http.StatusForbidden {
		t.Errorf("answering for a stranger: %d", code)
	}
	if code := reply(map[string]string{"question": cleanup, "person": coach, "value": "yes"}); code != http.StatusForbidden {
		t.Errorf("answering a family's question as one person: %d", code)
	}
	rec = h.do("", http.MethodGet, path, nil)
	json.Unmarshal(rec.Body.Bytes(), &view)
	if len(view.Replies) != 1 || view.Replies[0].Person != kid || view.Replies[0].Value != "L" {
		t.Errorf("the outside page's answers: %+v", view.Replies)
	}
	if code := reply(map[string]string{"question": shirt, "person": kid, "value": ""}); code != http.StatusNoContent || len(h.s.Model().Table("ANSWER").Referencing("question", shirt)) != 0 {
		t.Errorf("clearing an answer: %d", code)
	}
}
