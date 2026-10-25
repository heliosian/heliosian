package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/testkit/mailtest"
)

func deliveryEvent(kind, severity, copy, address, detail string) string {
	token := "event-" + kind + severity + copy
	stamp, sig := mailtest.SignMailgun(loopSigningKey, token, time.Now())
	body, _ := json.Marshal(map[string]any{
		"signature": map[string]string{"timestamp": stamp, "token": token, "signature": sig},
		"event-data": map[string]any{
			"event": kind, "severity": severity, "timestamp": float64(time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC).Unix()), "recipient": address,
			"user-variables":  map[string]string{mail.CopyVariable: copy},
			"delivery-status": map[string]string{"description": detail},
		},
	})
	return string(body)
}

func copiesSent(t *testing.T, sender *mailtest.Recorder) []string {
	t.Helper()
	out := []string{}
	for _, raw := range sender.Raws() {
		if raw.Recipient == "" {
			t.Fatalf("a send to %v names no copy", raw.To)
		}
		out = append(out, raw.Recipient)
	}
	slices.Sort(out)
	return out
}

func TestEveryCopyIsSentAloneAndItsDeliveryRecorded(t *testing.T) {
	h := newListMailHarness(t)
	NewDeliveries(h.s, loopSigningKey).Register(h.mux)
	h.addMember(parentsList, staff, "yes")
	h.inbound(loopNotice(loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "events@example.org", "Events", ""), hummingbirdsTo))
	h.settled(parentsList)
	out := h.posts(parentsList, "out")[0]
	copies := map[string]string{}
	ids := []string{}
	for _, c := range h.s.Model().Table("RECIPIENT").Referencing("message", out["id"]) {
		copies[c["person"]] = c["id"]
		ids = append(ids, c["id"])
	}
	slices.Sort(ids)
	if sent := copiesSent(t, h.sender); !slices.Equal(sent, ids) {
		t.Fatalf("the sends name %v, the copies are %v", sent, ids)
	}
	for _, body := range []string{
		deliveryEvent("delivered", "", copies[parent], "rowan.ashdown@example.org", ""),
		deliveryEvent("failed", "temporary", copies[staff], "maya.lindqvist@example.org", "try later"),
		deliveryEvent("failed", "permanent", copies[staff], "maya.lindqvist@example.org", "no such mailbox"),
		deliveryEvent("delivered", "", "rcpNoSuchCopy1", "rowan.ashdown@example.org", ""),
		deliveryEvent("delivered", "", "", "rowan.ashdown@example.org", ""),
	} {
		if rec := h.serve(httptest.NewRequest(http.MethodPost, "/hooks/events", strings.NewReader(body))); rec.Code != http.StatusOK {
			t.Fatalf("the event answered %d: %s", rec.Code, rec.Body)
		}
	}
	h.waitFor("the deliveries", func() bool {
		a, _ := h.s.Model().Table("RECIPIENT").Get(copies[parent])
		b, _ := h.s.Model().Table("RECIPIENT").Get(copies[staff])
		return a["delivered"] != "" && b["failed"] != ""
	})
	if c, _ := h.s.Model().Table("RECIPIENT").Get(copies[parent]); c["delivered"] != "2026-10-09 09:00:00" || c["failed"] != "" {
		t.Errorf("the delivered copy reads %v", c)
	}
	if c, _ := h.s.Model().Table("RECIPIENT").Get(copies[staff]); c["failed"] != "2026-10-09 09:00:00" || c["detail"] != "no such mailbox" || c["delivered"] != "" {
		t.Errorf("the bounced copy reads %v", c)
	}
}

func TestAnInvitationCopyNamesItselfToMailgun(t *testing.T) {
	h := newInviteHarness(t)
	h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"person": parent}}})
	h.mailTo(rowan, "You're invited")
	if sent := copiesSent(t, h.sender); !slices.Equal(sent, []string{h.copyOf(parent)["id"]}) {
		t.Errorf("the sends name %v, the copy is %s", sent, h.copyOf(parent)["id"])
	}
}
