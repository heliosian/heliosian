package db

import (
	"net/http"
	"testing"

	"heliosian/internal/access"
)

func TestActivityExclusionsClearAnswersInTheTransaction(t *testing.T) {
	for _, answer := range []string{"yes", "no"} {
		for _, mode := range []string{"invited", "member", "rule", "volunteer"} {
			t.Run(answer+"/"+mode, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity"}}})
				if mode == "rule" {
					h.setup(append(answerGroupEdits(h.group()), Edit{Insert: "RULE", Row: map[string]any{"group": h.event, "person": student, "order": "a"}}))
				} else if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": student}}}); rec.Code != http.StatusOK {
					t.Fatalf("invite the guest: %d %s", rec.Code, rec.Body)
				}
				g := h.group()
				if mode == "volunteer" {
					row, _ := h.s.Model().Table("MEMBER").Find(h.event, student)
					h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"member": "yes"}}})
				}
				h.setup(h.s.Model().answerEdits(g, student, answer, student, testNow))
				answerRow, _ := h.s.Model().Table("MEMBER").Find(g["rsvp_"+answer], student)
				row, _ := h.s.Model().Table("MEMBER").Find(h.event, student)
				exclude := Edit{Set: row["id"], Cells: map[string]any{"invited": "excluded"}}
				if mode == "rule" {
					exclude = Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "invited": "excluded", "added_by": student}}
				}
				if mode == "member" {
					exclude.Cells = map[string]any{"member": "excluded"}
				}
				write := func(edits ...Edit) error {
					_, err := Write(t.Context(), h.s, access.Actor{Email: maya}, Env{Viewer: staff, Now: testNow}, Batch{Batch: edits})
					return err
				}
				if err := write(exclude, Edit{Set: h.event, Cells: map[string]any{"parent": h.event}}); err == nil {
					t.Fatal("an invalid batch committed")
				}
				if _, ok := h.s.Model().Table("MEMBER").Get(answerRow["id"]); !ok {
					t.Fatal("a failed exclusion did not roll back answer cleanup")
				}
				if err := write(exclude); err != nil {
					t.Fatalf("exclude the guest: %v", err)
				}
				_, remains := h.s.Model().Table("MEMBER").Get(answerRow["id"])
				if remains != (mode == "volunteer") {
					t.Fatalf("RSVP remains=%v after %s exclusion", remains, mode)
				}
				if mode == "volunteer" {
					if err := write(Edit{Set: row["id"], Cells: map[string]any{"member": ""}}); err != nil {
						t.Fatal(err)
					}
					if _, ok := h.s.Model().Table("MEMBER").Get(answerRow["id"]); ok {
						t.Fatal("an exclusion did not clear the answer when the volunteer commitment ended")
					}
				}
				query := `(from GROUP @g (where (= id "` + h.event + `") (going @g)) (columns name))`
				if got := len(as(t, h.s, student, query)); got != 0 {
					t.Fatal("the removed guest still matches Going")
				}
				answerAgain := Edit{Insert: "MEMBER", Row: map[string]any{"group": g["rsvp_"+answer], "person": student, "member": "yes", "added_by": student}}
				if err := write(answerAgain); err == nil {
					t.Fatal("an excluded guest can recreate an RSVP by a raw write")
				}
				if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": student}}}); rec.Code != http.StatusOK {
					t.Fatalf("restore the guest: %d %s", rec.Code, rec.Body)
				}
				if err := write(answerAgain); err != nil {
					t.Fatalf("a restored guest cannot answer: %v", err)
				}
			})
		}
	}
}

func TestOutsideActivityRemovalClearsItsAnswerOnce(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach-rsvp@example.net"}}}); rec.Code != http.StatusOK {
		t.Fatalf("invite outside guest: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach-rsvp@example.net")
	path := "/open/ext/" + h.copyOf(coach)["token"] + "/guest"
	if rec := h.do("", http.MethodPost, path, map[string]string{"name": "Assistant Kim", "email": "kim-rsvp@example.net"}); rec.Code != http.StatusNoContent {
		t.Fatalf("bring a guest: %d %s", rec.Code, rec.Body)
	}
	kim := h.s.Model().personAt("kim-rsvp@example.net")
	h.setup(h.s.Model().answerEdits(h.group(), kim, "yes", kim, testNow))
	if rec := h.do("", http.MethodDelete, path, map[string]string{"key": kim}); rec.Code != http.StatusNoContent {
		t.Fatalf("take back an answered guest: %d %s", rec.Code, rec.Body)
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, kim)
	if row["invited"] != "excluded" || row["guest_of"] != coach || h.s.Model().answerOf(h.group(), kim) != "" {
		t.Fatalf("outside removal did not clear the answer and preserve attribution: %v", row)
	}
}
