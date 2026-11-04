package db

import (
	"encoding/json"
	"net/http"
	"testing"

	"heliosian/internal/access"
)

func TestActivityGuestInvitationSettingsPreserveVolunteerRules(t *testing.T) {
	for _, join := range []string{"", "direct", "others", "none"} {
		t.Run(join, func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "join": join, "eligible": "grp00000000001", "capacity": "1", "guests_invited_by": ""}}})
			if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": parent}}}); rec.Code != http.StatusOK {
				t.Fatalf("host invites a guest: %d %s", rec.Code, rec.Body)
			}
			write := func(viewer, inviters string) error {
				_, err := Write(t.Context(), h.s, access.Actor{Email: maya}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: h.event, Cells: map[string]any{"guests_invited_by": inviters}}}})
				return err
			}
			if err := write(parent, h.event); err == nil {
				t.Fatal("a guest changed who may invite")
			}
			add := func() int {
				return h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": student}}}).Code
			}
			if code := add(); code != http.StatusForbidden {
				t.Fatalf("invitations off: %d", code)
			}
			if err := write(staff, h.event); err != nil {
				t.Fatal(err)
			}
			if code := add(); code != http.StatusOK {
				t.Fatalf("an invited guest adds another despite volunteer restrictions: %d", code)
			}
			if err := write(staff, ""); err != nil {
				t.Fatal(err)
			}
			if code := add(); code != http.StatusForbidden {
				t.Fatalf("invitations off again: %d", code)
			}
			g := h.group()
			if g["join"] != join || g["eligible"] != "grp00000000001" || g["capacity"] != "1" || h.s.Model().directMemberCount(h.event) != 0 {
				t.Fatalf("invitation changes affected volunteer rules or commitments: %v", g)
			}
		})
	}
}

func TestOutsideActivityGuestInvitationsUseIndependentPermission(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "join": "none", "eligible": "grp00000000001", "guests_invited_by": h.event}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}); rec.Code != http.StatusOK {
		t.Fatalf("invite an outside guest: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	path := "/open/ext/" + h.copyOf(coach)["token"]
	for _, on := range []bool{true, false} {
		inviters := ""
		if on {
			inviters = "grp00000000004"
		}
		h.setup([]Edit{{Set: h.event, Cells: map[string]any{"guests_invited_by": inviters}}})
		rec := h.do("", http.MethodGet, path, nil)
		var view extView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != http.StatusOK || view.Guests != on {
			t.Fatalf("outside invitation controls: %d %s, %v", rec.Code, rec.Body, err)
		}
		rec = h.do("", http.MethodPost, path+"/guest", map[string]string{"name": "Assistant Kim"})
		want := http.StatusForbidden
		if on {
			want = http.StatusNoContent
		}
		if rec.Code != want {
			t.Fatalf("outside guest addition with invitations %v: %d %s", on, rec.Code, rec.Body)
		}
	}
	if h.group()["join"] != "none" || h.group()["eligible"] != "grp00000000001" || h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatal("outside invitations affected volunteer sign-ups")
	}
}

func TestActivityInvitationsOffStillAllowsVolunteerSignUp(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "join": "direct", "eligible": "", "guests_invited_by": ""}}})
	_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "added_by": parent}}}})
	if err != nil {
		t.Fatalf("guest invitations being off closed volunteer sign-ups: %v", err)
	}
	if h.s.Model().directMemberCount(h.event) != 1 {
		t.Fatal("volunteer sign-up was not recorded")
	}
}

func TestActivityGuestAdditionRechecksInvitationPermission(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "guests_invited_by": h.event}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "invited": "yes"}},
	})
	edits := []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "invited": "yes", "added_by": parent}}}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"guests_invited_by": ""}}})
	iv := &Invitations{s: h.s}
	if _, err := iv.writeGuests(t.Context(), access.Actor{Email: rowan}, edits, h.event, parent, nil, 0); err == nil {
		t.Fatal("a prepared guest addition bypassed the current invitation setting")
	}
	if _, ok := h.s.Model().Table("MEMBER").Find(h.event, student); ok {
		t.Fatal("the refused guest was inserted")
	}
}
