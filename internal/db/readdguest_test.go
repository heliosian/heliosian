package db

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestARemovedGuestAddedBackIsSentTheInvitationAgain(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("adding coach: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	h.waitFor("coach's first copy sent", func() bool { return h.copyOf(coach)["sent"] != "" })
	first := h.copyOf(coach)["id"]
	if rec := h.do(maya, http.MethodPost, "/api/do/remove-guest", map[string]string{"group": h.event, "person": coach}); rec.Code != http.StatusOK {
		t.Fatalf("removing coach: %d %s", rec.Code, rec.Body)
	}
	add["people"] = []map[string]string{{"person": coach}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("adding coach back: %d %s", rec.Code, rec.Body)
	}
	h.waitFor("a second copy for coach", func() bool {
		c := h.copyOf(coach)
		return c["id"] != first && c["sent"] != ""
	})
	sent := 0
	for _, m := range h.sender.Messages() {
		if slices.Contains(m.To, "coach@example.net") {
			sent++
		}
	}
	if sent != 2 {
		t.Errorf("coach got %d invitations, want 2", sent)
	}
}

func TestAHostResendsTheInvitation(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}, {"name": "Sam Tse", "email": "sam@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("adding guests: %d %s", rec.Code, rec.Body)
	}
	coach, sam := h.s.Model().personAt("coach@example.net"), h.s.Model().personAt("sam@example.net")
	if rec := h.do(maya, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event, "people": []string{coach}, "again": true}); rec.Code != http.StatusBadRequest {
		t.Errorf("resending to someone never sent it answered %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event, "people": []string{coach, sam}}); rec.Code != http.StatusOK {
		t.Fatalf("sending: %d %s", rec.Code, rec.Body)
	}
	h.mailTo("coach@example.net", "You're invited")
	if rec := h.do(rowan, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event, "people": []string{coach}, "again": true}); rec.Code != http.StatusForbidden {
		t.Errorf("someone who isn't a host resending answered %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event, "people": []string{coach}, "again": true}); rec.Code != http.StatusOK {
		t.Fatalf("resending: %d %s", rec.Code, rec.Body)
	}
	h.mailTo("coach@example.net", "Reminder")
	for _, m := range h.sender.Messages() {
		if slices.Contains(m.To, "sam@example.net") && strings.Contains(m.Subject, "Reminder") {
			t.Errorf("sam, not named, was sent it again: %s", m.Subject)
		}
	}
}

func TestAGuestAddedBackAfterTheirCopyIsWaitingForTheInvitation(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("adding coach: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	if pending := h.s.Model().pendingOf(h.event); slices.Contains(pending, coach) {
		t.Errorf("coach, just sent the invitation, is still waiting for it")
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, coach)
	h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"added": "2099-01-01 00:00:00"}}})
	if pending := h.s.Model().pendingOf(h.event); !slices.Contains(pending, coach) {
		t.Errorf("coach, added back since their copy, isn't waiting for the invitation: %v", pending)
	}
}
