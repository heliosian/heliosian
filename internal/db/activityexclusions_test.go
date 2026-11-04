package db

import (
	"encoding/hex"
	"net/http"
	"testing"

	"heliosian/internal/access"
)

func TestActivityExclusionInsertCarriesNoForgedMetadata(t *testing.T) {
	const host = "per00000020001"
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.group()["managed_by"], "person": host, "member": "yes"}},
	})
	insert := func(viewer string, row map[string]any) error {
		row["group"], row["invited"] = h.event, "excluded"
		_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: row}}})
		return err
	}
	for name, row := range map[string]map[string]any{
		"a bringer":        {"person": parent, "added_by": parent, "guest_of": staff},
		"an opened stamp":  {"person": parent, "added_by": parent, "opened": "2026-11-03 12:00"},
		"a note":           {"person": parent, "added_by": parent, "note": "Bringing dumplings"},
		"another's adding": {"person": parent, "added_by": staff},
	} {
		if err := insert(parent, row); err == nil {
			t.Errorf("a guest's own exclusion was written with %s", name)
		}
	}
	if err := insert(host, map[string]any{"person": student, "added_by": host, "guest_of": parent}); err == nil {
		t.Error("a host's exclusion was written naming a bringer")
	}
	if err := insert(parent, map[string]any{"person": parent, "added_by": parent}); err != nil {
		t.Fatalf("a guest keeps themselves off the list: %v", err)
	}
	if err := insert(host, map[string]any{"person": student, "added_by": host}); err != nil {
		t.Fatalf("a host keeps someone off the list: %v", err)
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, parent)
	if row["guest_of"] != "" || row["opened"] != "" || row["member"] != "" || h.s.Model().onList(h.group(), parent) {
		t.Fatalf("the exclusion is not a plain keeping off: %v", row)
	}
}

func TestActivityHostRestoresLegacyExclusionAndOwnsRestoration(t *testing.T) {
	h := newInviteHarness(t)
	ids := h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "excluded", "invited": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "invited": "excluded", "guest_of": parent}},
	})
	if h.s.Model().onList(h.group(), student) || h.s.Model().onList(h.group(), guest) {
		t.Fatal("an excluded row is on the guest list")
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"person": student}, {"person": guest}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("host restores excluded guests: %d %s", rec.Code, rec.Body)
	}
	legacy, _ := h.s.Model().Table("MEMBER").Get(ids[1])
	if legacy["member"] != "" || legacy["invited"] != "yes" || !h.s.Model().onList(h.group(), student) || h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatalf("a legacy exclusion stayed wedged or became a volunteer: %v", legacy)
	}
	brought, _ := h.s.Model().Table("MEMBER").Get(ids[2])
	if brought["invited"] != "yes" || brought["guest_of"] != "" || brought["added_by"] != staff || !h.s.Model().onList(h.group(), guest) {
		t.Fatalf("a host's restoration kept the earlier bringer's link: %v", brought)
	}
}

func TestGuestMailSkipsThoseTakenOffAfterTheirInvitation(t *testing.T) {
	const volunteer = "per00000020002"
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "send": true, "people": []map[string]string{{"person": parent}, {"person": volunteer}, {"name": "Coach Pat", "email": "coach@example.net"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("invite the guests: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	h.waitFor("every invitation", func() bool {
		return h.copyOf(parent)["sent"] != "" && h.copyOf(volunteer)["sent"] != "" && h.copyOf(coach)["sent"] != ""
	})
	m := h.s.Model()
	took, _ := m.Table("MEMBER").Find(h.event, parent)
	signed, _ := m.Table("MEMBER").Find(h.event, volunteer)
	h.setup([]Edit{
		{Set: signed["id"], Cells: map[string]any{"member": "yes"}},
		{Set: took["id"], Cells: map[string]any{"invited": "excluded"}},
		{Set: signed["id"], Cells: map[string]any{"invited": "excluded"}},
	})
	sentTo := func(ask map[string]any) map[string]bool {
		t.Helper()
		before := map[string]bool{}
		for _, msg := range h.s.Model().Table("MESSAGE").Referencing("group", h.event) {
			before[msg["id"]] = true
		}
		ask["group"] = h.event
		if rec := h.do(maya, http.MethodPost, "/api/do/guest-mail", ask); rec.Code != http.StatusOK {
			t.Fatalf("%v: %d %s", ask["kind"], rec.Code, rec.Body)
		}
		to := map[string]bool{}
		for _, msg := range h.s.Model().Table("MESSAGE").Referencing("group", h.event) {
			if before[msg["id"]] {
				continue
			}
			for _, copy := range h.s.Model().Table("RECIPIENT").Referencing("message", msg["id"]) {
				to[copy["person"]] = true
			}
		}
		return to
	}
	everyone := []string{"yes", "no", "none"}
	for _, ask := range []map[string]any{
		{"kind": "reminder", "to": everyone, "subject": "Still hoping", "text": "Can you make it?"},
		{"kind": "message", "to": everyone, "subject": "Parking", "text": "Use the side lot."},
		{"kind": "update"},
	} {
		to := sentTo(ask)
		if to[parent] || !to[volunteer] || !to[coach] {
			t.Errorf("%v went to %v: the guest taken off must be left out, the volunteer and the remaining guest kept", ask["kind"], to)
		}
	}
	if to := sentTo(map[string]any{"kind": "cancel", "notify": true, "text": "Rained out."}); !to[parent] || !to[volunteer] || !to[coach] {
		t.Errorf("the cancellation went to %v: everyone sent the invitation hears of it", to)
	}
}

func TestCalendarReplyFromSomeoneTakenOffRecordsNoAnswer(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "send": true, "people": []map[string]string{{"person": parent}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("invite a guest: %d %s", rec.Code, rec.Body)
	}
	h.waitFor("the invitation sent", func() bool { return h.copyOf(parent)["sent"] != "" })
	organizer := "when+" + hex.EncodeToString([]byte(h.copyOf(parent)["token"])) + "@reply.heliosian.com"
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, parent)
	h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded"}}})
	if code := h.reply(organizer, rowan, h.event+whenUID, "ACCEPTED"); code != http.StatusOK || h.s.Model().answerOf(h.group(), parent) != "" {
		t.Fatalf("an accept from someone taken off: %d, answer %q", code, h.s.Model().answerOf(h.group(), parent))
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"person": parent}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("the host puts them back: %d %s", rec.Code, rec.Body)
	}
	if code := h.reply(organizer, rowan, h.event+whenUID, "ACCEPTED"); code != http.StatusOK || h.s.Model().answerOf(h.group(), parent) != "yes" {
		t.Fatalf("an accept once back on the list: %d, answer %q", code, h.s.Model().answerOf(h.group(), parent))
	}
}

func TestActivityBringerRestorationKeepsTheirLink(t *testing.T) {
	h := newInviteHarness(t)
	ids := h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "invited": "excluded", "guest_of": parent}},
	})
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"person": guest}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("the bringer restores their guest: %d %s", rec.Code, rec.Body)
	}
	row, _ := h.s.Model().Table("MEMBER").Get(ids[1])
	if row["invited"] != "yes" || row["guest_of"] != parent || row["member"] != "" {
		t.Fatalf("the bringer's restoration lost their link or made a volunteer: %v", row)
	}
}

func TestActivityRestorationRechecksCurrentAuthority(t *testing.T) {
	const host = "per00000020001"
	for _, path := range []string{"add-guests", "outside"} {
		t.Run(path, func(t *testing.T) {
			h := newInviteHarness(t)
			ids := h.setup([]Edit{
				{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "invited": "excluded", "guest_of": parent}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.group()["managed_by"], "person": host, "member": "yes"}},
			})
			stale := h.s.Model()
			row, _ := stale.Table("MEMBER").Get(ids[1])
			g, _ := stale.Table("GROUP").Get(h.event)
			joined := guestMembership(g, parent, Edit{Set: row["id"], Cells: map[string]any{"member": "yes", "guest_of": parent, "added_by": parent}})
			if _, err := Write(t.Context(), h.s, access.Actor{Email: maya}, Env{Viewer: staff, Now: testNow}, Batch{Batch: []Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded", "guest_of": ""}}}}); err != nil {
				t.Fatalf("host removes the guest during restoration: %v", err)
			}
			iv := &Invitations{s: h.s}
			var err error
			if path == "add-guests" {
				_, err = iv.addGuests(t.Context(), stale, Env{Viewer: parent, Now: testNow}, access.Actor{Email: rowan}, addGuestsAsk{Group: h.event, People: []guestAsk{{Person: guest}}})
			} else {
				_, err = iv.writeGuests(t.Context(), access.Actor{Email: rowan}, []Edit{joined}, h.event, parent, nil, 0)
			}
			if err == nil {
				t.Fatal("stale bringer authority undid the host's removal")
			}
			t.Logf("restoration refused: %v", err)
			current, _ := h.s.Model().Table("MEMBER").Get(row["id"])
			if current["invited"] != "excluded" || current["guest_of"] != "" || h.s.Model().onList(h.group(), guest) {
				t.Fatalf("the refused restoration changed the guest: %v", current)
			}
			if _, err := iv.addGuests(t.Context(), stale, Env{Viewer: host, Now: testNow}, access.Actor{Email: stale.addressOf(host)}, addGuestsAsk{Group: h.event, People: []guestAsk{{Person: guest}}}); err != nil {
				t.Fatalf("the current host cannot restore from a stale snapshot: %v", err)
			}
		})
	}
}

func TestActivityRestorationPreservesConcurrentSignUp(t *testing.T) {
	h := newInviteHarness(t)
	ids := h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": ""}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "member": "excluded", "invited": "yes"}},
	})
	stale := h.s.Model()
	h.setup([]Edit{{Set: ids[1], Cells: map[string]any{"member": "yes"}}})
	iv := &Invitations{s: h.s}
	if _, err := iv.addGuests(t.Context(), stale, Env{Viewer: staff, Now: testNow}, access.Actor{Email: maya}, addGuestsAsk{Group: h.event, People: []guestAsk{{Person: guest}}}); err == nil {
		t.Fatal("a stale host restoration cancelled a concurrent volunteer sign-up")
	}
	current, _ := h.s.Model().Table("MEMBER").Get(ids[1])
	if current["member"] != "yes" || current["invited"] != "yes" {
		t.Fatalf("the refused restoration changed the volunteer: %v", current)
	}
}
