package db

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
)

func (h *inviteHarness) mailCount(address, subject string) int {
	n := 0
	for _, m := range h.sender.Messages() {
		if slices.Contains(m.To, address) && strings.Contains(m.Subject, subject) {
			n++
		}
	}
	return n
}

func TestANewInvitationResendsAfterThreeDays(t *testing.T) {
	s := sample(t)
	ids, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{
		{Insert: "GROUP", As: "event", Row: map[string]any{"name": "Picnic", "start": "2026-12-01 12:00", "end": "2026-12-01 14:00", "kind": "event", "parent": "grp00000000062", "status": "open", "added_by": parent, "listed": false}},
		{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Picnic Managers", "status": "open", "added_by": parent}},
		{Set: "@hosts", Cells: map[string]any{"managed_by": "@hosts"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": parent, "member": "yes"}},
		{Set: "@event", Cells: map[string]any{"managed_by": "@hosts"}},
		{Set: "@event", Cells: map[string]any{"visible_to": "@event", "resend_days": "3"}},
	}})
	if err != nil {
		t.Fatalf("a parent can't make an invitation that resends after three days: %v", err)
	}
	if g, _ := s.Model().Table("GROUP").Get(ids[0]); g["resend_days"] != "3" {
		t.Errorf("the new invitation resends after %q days", g["resend_days"])
	}
}

func TestAThankYouNoteGoesOnceTheEventHasBegun(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}, {"name": "Kim Lee", "email": "kim@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds people from outside: %d %s", rec.Code, rec.Body)
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"auto_invite": true}}})
	h.mailTo("kim@example.net", "You're invited")
	h.mailTo("coach@example.net", "You're invited")
	kim := h.s.Model().personAt("kim@example.net")
	if rec := h.do("", http.MethodPost, "/open/ext/"+h.copyOf(kim)["token"], map[string]string{"answer": "yes"}); rec.Code != http.StatusNoContent {
		t.Fatalf("kim answers: %d", rec.Code)
	}
	thanks := guestMailAsk{Group: h.event, Kind: "thanks", To: []string{"yes"}, Subject: "Thank you for coming!", Text: "It was a joy."}
	if rec := h.do(maya, http.MethodPost, "/api/do/guest-mail", thanks); rec.Code != http.StatusBadRequest {
		t.Errorf("a thank-you note before the event: %d %s", rec.Code, rec.Body)
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"start": testNow.In(School).AddDate(0, 0, -2).Format("2006-01-02") + " 18:00"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/guest-mail", thanks); rec.Code != http.StatusOK {
		t.Fatalf("a thank-you note after the event: %d %s", rec.Code, rec.Body)
	}
	note := h.mailTo("kim@example.net", "Thank you for coming!")
	if len(note.Attachments) != 0 {
		t.Errorf("the thank-you note carries a calendar invite")
	}
	time.Sleep(100 * time.Millisecond)
	if n := h.mailCount("coach@example.net", "Thank you for coming!"); n != 0 {
		t.Errorf("someone who never answered was thanked for coming")
	}
}

func TestRemindersAndResendsGoOnTheirOwn(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}, {"name": "Kim Lee", "email": "kim@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds people from outside: %d %s", rec.Code, rec.Body)
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"auto_invite": true}}})
	h.mailTo("coach@example.net", "You're invited")
	h.mailTo("kim@example.net", "You're invited")
	m := h.s.Model()
	coach, kim := m.personAt("coach@example.net"), m.personAt("kim@example.net")
	if rec := h.do("", http.MethodPost, "/open/ext/"+h.copyOf(kim)["token"], map[string]string{"answer": "yes"}); rec.Code != http.StatusNoContent {
		t.Fatalf("kim answers: %d", rec.Code)
	}

	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"remind_days": "40"}}})
	h.mailTo("coach@example.net", "Reminder: please RSVP")
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"location": "9 Kelp Lane"}}})
	time.Sleep(100 * time.Millisecond)
	if n := h.mailCount("coach@example.net", "Reminder: please RSVP"); n != 1 {
		t.Errorf("the reminder went %d times, want once", n)
	}
	if n := h.mailCount("kim@example.net", "Reminder: please RSVP"); n != 0 {
		t.Errorf("someone who answered was reminded")
	}

	original := h.copyOf(coach)
	h.setup([]Edit{
		{Set: original["id"], Cells: map[string]any{"sent": stamp(time.Now().AddDate(0, 0, -3))}},
		{Set: h.event, Cells: map[string]any{"resend_days": "2"}},
	})
	again := h.mailTo("coach@example.net", "Reminder: you're invited")
	if h.copyOf(coach)["id"] != original["id"] {
		t.Errorf("the resend replaced the invitation's copy")
	}
	if !strings.Contains(again.HTML, "/ext/") || strings.Contains(again.HTML, "/ext/"+original["token"]) {
		t.Errorf("the resend does not carry a link of its own")
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"location": "10 Kelp Lane"}}})
	time.Sleep(100 * time.Millisecond)
	if n := h.mailCount("coach@example.net", "Reminder: you're invited"); n != 1 {
		t.Errorf("the invitation was resent %d times, want once", n)
	}
}
