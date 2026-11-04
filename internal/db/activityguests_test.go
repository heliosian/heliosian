package db

import (
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func TestActivityInvitationsAndVolunteerCommitmentsAreIndependent(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": "", "guests_invited_by": "grp00000000004", "capacity": "1"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"person": parent}, {"person": student}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("host adds activity guests: %d %s", rec.Code, rec.Body)
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, parent)
	if row["member"] != "" || row["invited"] != "yes" || !h.s.Model().onList(h.group(), parent) || h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatalf("guests became volunteers or were not invited: %v", row)
	}
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	if _, err := write(parent, Edit{Set: row["id"], Cells: map[string]any{"opened": "2026-11-03 12:00"}}); err != nil {
		t.Fatal(err)
	}
	joined, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "added_by": parent, "note": "Bringing dumplings"}})
	if err != nil {
		t.Fatalf("an invited person signs up: %v", err)
	}
	promoted, _ := h.s.Model().Table("MEMBER").Get(joined[0])
	if joined[0] != row["id"] || promoted["invited"] != "yes" || promoted["opened"] != "2026-11-03 12:00" || h.s.Model().directMemberCount(h.event) != 1 {
		t.Fatalf("signing up lost invitation state: %v", promoted)
	}
	if _, err := write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "yes", "added_by": student}}); err == nil {
		t.Fatal("an invitation bypassed volunteer capacity")
	}
	if _, err := write(staff, Edit{Set: row["id"], Cells: map[string]any{"invited": "excluded"}}); err != nil {
		t.Fatal(err)
	}
	if !h.s.Model().onList(h.group(), parent) || h.s.Model().directMemberCount(h.event) != 1 {
		t.Fatal("a guest-list action removed a volunteer's membership")
	}
	if _, err := write(parent, Edit{Set: row["id"], Cells: map[string]any{"member": ""}}); err != nil {
		t.Fatalf("cancel the volunteer commitment: %v", err)
	}
	left, _ := h.s.Model().Table("MEMBER").Get(row["id"])
	if left["invited"] != "excluded" || left["opened"] != promoted["opened"] || h.s.Model().onList(h.group(), parent) || h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatalf("cancelling did not retain the independent invitation: %v", left)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"person": parent}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("host restores an excluded guest: %d %s", rec.Code, rec.Body)
	}
	restored, _ := h.s.Model().Table("MEMBER").Get(row["id"])
	if restored["member"] != "" || restored["invited"] != "yes" || !h.s.Model().onList(h.group(), parent) {
		t.Fatalf("restoring a guest made a volunteer: %v", restored)
	}
	if changes := placeChanges(Change{Table: "MEMBER", Old: left, New: restored}, Env{Viewer: staff}); len(changes) != 0 {
		t.Fatalf("a guest-list change queued volunteer mail: %v", changes)
	}
	joined, err = write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "added_by": parent, "note": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := h.s.Model().Table("MEMBER").Get(joined[0]); again["note"] != "" {
		t.Fatalf("signing up with an empty note retained the old note: %v", again)
	}
	if _, err := write(parent, Edit{Set: joined[0], Cells: map[string]any{"member": ""}}); err != nil {
		t.Fatal(err)
	}
	if !h.s.Model().onList(h.group(), parent) {
		t.Fatal("cancelling a volunteer commitment removed an explicit invitation")
	}
}

func TestOutsideActivityGuestsNeverBecomeVolunteers(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("invite an outside guest: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	token := h.copyOf(coach)["token"]
	if rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Assistant Kim", "email": "kim@example.net"}); rec.Code != http.StatusNoContent {
		t.Fatalf("bring a guest from the outside page: %d %s", rec.Code, rec.Body)
	}
	kim := h.s.Model().personAt("kim@example.net")
	kimToken := h.copyOf(kim)["token"]
	mailed := h.s.Model().invitationsFrom(h.event, coach)
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, kim)
	if row["member"] != "" || row["invited"] != "yes" || row["guest_of"] != coach || h.s.Model().directMemberCount(h.event) != 0 {
		t.Fatalf("an outside invitation made a volunteer: %v", row)
	}
	if len(h.s.Model().broughtGuests(h.group(), coach)) != 1 {
		t.Fatal("the invitation page lost the guest that was brought")
	}
	if rec := h.do("", http.MethodDelete, "/open/ext/"+token+"/guest", map[string]string{"key": kim}); rec.Code != http.StatusNoContent {
		t.Fatalf("take back a guest from the outside page: %d %s", rec.Code, rec.Body)
	}
	removed, _ := h.s.Model().Table("MEMBER").Get(row["id"])
	if h.s.Model().onList(h.group(), kim) || removed["invited"] != "excluded" || removed["guest_of"] != coach || removed["member"] != "" {
		t.Fatalf("taking back a guest changed the volunteer commitment: %v", removed)
	}
	if changes := placeChanges(Change{Table: "MEMBER", Old: row, New: removed}, Env{Viewer: coach}); len(changes) != 0 {
		t.Fatalf("taking back a guest queued volunteer mail: %v", changes)
	}
	if rec := h.do("", http.MethodGet, "/open/ext/"+kimToken, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("a removed guest can still open their invitation: %d %s", rec.Code, rec.Body)
	}
	if rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Assistant Kim", "email": "kim@example.net"}); rec.Code != http.StatusNoContent {
		t.Fatalf("bring the guest again: %d %s", rec.Code, rec.Body)
	}
	if !h.s.Model().onList(h.group(), kim) || h.copyOf(kim)["token"] != kimToken || h.s.Model().invitationsFrom(h.event, coach) != mailed {
		t.Fatal("restoring a guest did not reuse their existing invitation")
	}
	if rec := h.do("", http.MethodGet, "/open/ext/"+kimToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("a restored guest cannot open their original invitation: %d %s", rec.Code, rec.Body)
	}
	if _, err := Write(t.Context(), h.s, access.Actor{Email: maya}, Env{Viewer: staff, Now: testNow}, Batch{Batch: []Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded", "guest_of": ""}}}}); err != nil {
		t.Fatalf("host takes off a brought guest: %v", err)
	}
	if rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Assistant Kim", "email": "kim@example.net"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a bringer restored someone the hosts took off: %d %s", rec.Code, rec.Body)
	}
}

func TestActivityGuestExclusionOverridesRulesWithoutRemovingSignUps(t *testing.T) {
	s := sample(t)
	const activity = "grp00000000512"
	ids := apply(t, s, []Edit{
		{Insert: "RULE", Row: map[string]any{"group": activity, "order": "a", "person": student}},
		{Insert: "MEMBER", Row: map[string]any{"group": activity, "person": student, "invited": "excluded"}},
	})
	if s.Model().effectivelyIn(activity, student) {
		t.Fatal("a rule brought an excluded guest back")
	}
	apply(t, s, []Edit{{Set: ids[1], Cells: map[string]any{"member": "yes"}}})
	if !s.Model().effectivelyIn(activity, student) {
		t.Fatal("a guest exclusion removed an active volunteer")
	}
}

func TestInvitationStateCannotGrantGroupMembership(t *testing.T) {
	s := sample(t)
	for _, tc := range []struct {
		name string
		row  map[string]any
	}{
		{"opened role", map[string]any{"group": "grp00000000003", "person": parent, "opened": "2026-11-03 12:00", "invited": "yes"}},
		{"opened activity", map[string]any{"group": "grp00000000512", "person": student, "opened": "2026-11-03 12:00", "invited": "yes"}},
		{"joined activity", map[string]any{"group": "grp00000000512", "person": student, "member": "yes", "added_by": parent, "invited": "yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Write(t.Context(), s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: tc.row}}})
			if err == nil {
				t.Fatal("a membership insert smuggled in an invitation")
			}
		})
	}
	ids := apply(t, s, []Edit{
		{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Unrelated role", "status": "open"}},
	})
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": ids[0], "person": parent, "invited": "yes"}}})
	if s.Model().effectivelyIn(ids[0], parent) {
		t.Fatal("invitation state granted membership of a non-activity group")
	}
}

func TestActivityGuestsCanBringAndRemoveAnOpenedInvitee(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": "", "guests_invited_by": "grp00000000004"}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "opened": "2026-11-03 12:00"}},
	})
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "guest_of": parent, "answer": "yes", "people": []map[string]string{{"person": student}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("bring an existing invitee: %d %s", rec.Code, rec.Body)
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, student)
	if row["member"] != "" || row["invited"] != "yes" || row["guest_of"] != parent || row["opened"] != "2026-11-03 12:00" || row["added_by"] != parent {
		t.Fatalf("bringing an opened invitee lost their metadata: %v", row)
	}
	if _, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: student, Now: testNow}, Batch{Batch: []Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded", "guest_of": ""}}}}); err != nil {
		t.Fatalf("an invited person takes themselves off: %v", err)
	}
	if h.s.Model().onList(h.group(), student) {
		t.Fatal("a past invitation copy kept an excluded guest on the list")
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "guest_of": parent, "people": []map[string]string{{"person": student}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("re-add someone who took themselves off: %d %s", rec.Code, rec.Body)
	}
	if h.s.Model().onList(h.group(), student) {
		t.Fatal("a bringer overrode the guest's own removal")
	}
}

func TestActivityBringersRestoreTheirOwnSentGuests(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": "", "guests_invited_by": "grp00000000004"}}})
	add := map[string]any{"group": h.event, "guest_of": parent, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("bring an activity guest: %d %s", rec.Code, rec.Body)
	}
	guest := h.s.Model().personAt("coach@example.net")
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, guest)
	token := h.copyOf(guest)["token"]
	mailed := h.s.Model().invitationsFrom(h.event, parent)
	if _, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded"}}}}); err != nil {
		t.Fatalf("take back the guest: %v", err)
	}
	if h.s.Model().onList(h.group(), guest) {
		t.Fatal("a sent guest remained invited after being taken back")
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("bring the same guest again: %d %s", rec.Code, rec.Body)
	}
	restored, _ := h.s.Model().Table("MEMBER").Get(row["id"])
	if restored["invited"] != "yes" || restored["member"] != "" || h.copyOf(guest)["token"] != token || h.s.Model().invitationsFrom(h.event, parent) != mailed {
		t.Fatalf("restoring a guest changed their commitment or invitation: %v", restored)
	}
}

func TestGuestListsKeepOpenedRuleMembersAsTheyAre(t *testing.T) {
	for _, kind := range []string{"event", "party", "activity"} {
		t.Run(kind, func(t *testing.T) {
			h := newInviteHarness(t)
			ids := h.setup([]Edit{
				{Set: h.event, Cells: map[string]any{"kind": kind}},
				{Insert: "RULE", Row: map[string]any{"group": h.event, "person": student, "order": "a"}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "opened": "2026-11-03 12:00"}},
			})
			if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "guest_of": parent, "people": []map[string]string{{"person": student}}}); rec.Code != http.StatusOK {
				t.Fatalf("add an existing rule member: %d %s", rec.Code, rec.Body)
			}
			row, _ := h.s.Model().Table("MEMBER").Get(ids[2])
			if row["member"] != "" || row["invited"] != "" || row["guest_of"] != "" || row["added_by"] != "" || row["added"] != "" || row["opened"] != "2026-11-03 12:00" {
				t.Fatalf("an existing rule member became a hand addition: %v", row)
			}
			h.setup([]Edit{{Delete: ids[1]}})
			if h.s.Model().effectivelyIn(h.event, student) {
				t.Fatal("adding an existing rule member kept them on the list after the rule was removed")
			}
		})
	}
}

func TestActivityBringersCannotChangeVolunteerCommitments(t *testing.T) {
	for _, operation := range []string{"delete", "cancel", "exclude"} {
		t.Run(operation, func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "eligible": "", "guests_invited_by": "grp00000000004"}}})
			if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
				"group": h.event, "guest_of": parent, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
			}); rec.Code != http.StatusOK {
				t.Fatalf("bring an activity guest: %d %s", rec.Code, rec.Body)
			}
			guest := h.s.Model().personAt("coach@example.net")
			joined, err := Write(t.Context(), h.s, access.Actor{Email: "coach@example.net"}, Env{Viewer: guest, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "member": "yes", "added_by": guest}}}})
			if err != nil {
				t.Fatalf("an invited guest volunteers: %v", err)
			}
			edit := Edit{Delete: joined[0]}
			if operation == "cancel" {
				edit = Edit{Set: joined[0], Cells: map[string]any{"member": "cancelled"}}
			}
			if operation == "exclude" {
				edit = Edit{Set: joined[0], Cells: map[string]any{"invited": "excluded"}}
			}
			if _, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{edit}}); err == nil {
				t.Fatalf("a bringer can %s a volunteer's membership", operation)
			}
			row, _ := h.s.Model().Table("MEMBER").Get(joined[0])
			if row["member"] != "yes" || row["invited"] != "yes" || row["guest_of"] != parent {
				t.Fatalf("a refused guest action changed volunteer state: %v", row)
			}
		})
	}
}

func TestActivityBringerLinkOnlyClearsOnGuestRemoval(t *testing.T) {
	for _, viewer := range []string{student, parent, guest} {
		t.Run(viewer, func(t *testing.T) {
			h := newInviteHarness(t)
			ids := h.setup([]Edit{
				{Set: h.event, Cells: map[string]any{"kind": "activity"}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "invited": "yes", "guest_of": staff}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.group()["managed_by"], "person": guest, "member": "yes"}},
			})
			write := func(cells map[string]any) error {
				_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: ids[1], Cells: cells}}})
				return err
			}
			if err := write(map[string]any{"guest_of": ""}); err == nil {
				t.Fatal("a guest's bringer link can be cleared while they remain invited")
			}
			if err := write(map[string]any{"invited": "excluded", "guest_of": ""}); err != nil {
				t.Fatalf("taking off a guest cannot clear their bringer link: %v", err)
			}
		})
	}
}

func TestActivityGuestsMustVolunteerBeforeOfferingToCoChair(t *testing.T) {
	h := newInviteHarness(t)
	ids := h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"kind": "activity"}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "invited": "yes"}},
		{Insert: "GROUP", As: "offers", Row: map[string]any{"kind": "group", "parent": h.event, "name": "Fondue Night Co-Chair Offers", "status": "open", "join": "direct", "eligible": h.event, "visible_to": h.group()["visible_to"], "posting": "none", "replying": "none"}},
		{Set: h.event, Cells: map[string]any{"chair_offers": "@offers"}},
	})
	offer := Edit{Insert: "MEMBER", Row: map[string]any{"group": ids[2], "person": student, "member": "yes", "added_by": student}}
	write := func(edits ...Edit) error {
		_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: student, Now: testNow}, Batch{Batch: edits})
		return err
	}
	if err := write(offer); err == nil {
		t.Fatal("an invited-only guest can offer to co-chair without volunteering")
	}
	if err := write(Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "yes", "added_by": student}}, offer); err != nil {
		t.Fatalf("an invited guest cannot volunteer and offer to co-chair in one batch: %v", err)
	}
}

func TestExcludedOutsideActivityGuestsCannotAddVolunteers(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("invite an outside guest: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	write := func(person string) error {
		_, err := Write(t.Context(), h.s, access.Actor{Email: "coach@example.net"}, Env{Viewer: coach, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": person, "member": "yes", "added_by": coach}}}})
		return err
	}
	if err := write(coach); err != nil {
		t.Fatalf("an outside invitee cannot volunteer: %v", err)
	}
	row, _ := h.s.Model().Table("MEMBER").Find(h.event, coach)
	h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"member": "excluded"}}})
	if h.s.Model().effectivelyIn(h.event, coach) || h.s.Model().onList(h.group(), coach) {
		t.Fatal("the excluded outside guest remains on the activity list")
	}
	if err := write(student); err == nil {
		t.Fatal("an excluded outside guest can still add a volunteer")
	}
	h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"member": "yes"}}})
	if err := write(student); err != nil {
		t.Fatalf("a restored outside volunteer cannot add another volunteer: %v", err)
	}
}

func TestActivityDeletionClearsInactiveInvitationsAndAnswerGroups(t *testing.T) {
	for _, history := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsent", true: "mailed"}[history], func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity", "parent": "grp00000000512"}}})
			if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "answer": "yes", "people": []map[string]string{{"person": student}}}); rec.Code != http.StatusOK {
				t.Fatalf("add an activity guest: %d %s", rec.Code, rec.Body)
			}
			row, _ := h.s.Model().Table("MEMBER").Find(h.event, student)
			h.setup([]Edit{{Set: row["id"], Cells: map[string]any{"invited": "excluded"}}})
			if history {
				h.setup([]Edit{{Insert: "MESSAGE", Row: map[string]any{"direction": "out", "kind": "invitation", "group": h.event, "created": stamp(testNow)}}})
			}
			write := func(edits ...Edit) error {
				_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: edits})
				return err
			}
			group := h.group()
			h.setup([]Edit{{Insert: "MEMBER", Row: map[string]any{"group": group["rsvp_yes"], "person": parent, "member": "yes"}}})
			if err := write(Edit{Set: h.event, Cells: map[string]any{"rsvp_yes": ""}}); err == nil {
				t.Fatal("a nonempty answer group can be detached")
			}
			if err := write(Edit{Set: h.event, Cells: map[string]any{"rsvp_no": group["rsvp_yes"]}}); err == nil {
				t.Fatal("a list runner can replace an answer group")
			}
			edits := []Edit{}
			for _, column := range []string{"rsvp_yes", "rsvp_no"} {
				id := group[column]
				for _, answer := range h.s.Model().Table("MEMBER").Referencing("group", id) {
					edits = append(edits, Edit{Delete: answer["id"]})
				}
				edits = append(edits, Edit{Set: h.event, Cells: map[string]any{column: ""}}, Edit{Delete: id})
			}
			edits = append(edits, Edit{Delete: row["id"]}, Edit{Delete: h.event})
			err := write(edits...)
			if history {
				if err == nil {
					t.Fatal("an activity with mail history was deleted")
				}
				if !strings.Contains(err.Error(), "MESSAGE") {
					t.Fatalf("deletion failed before checking mail history: %v", err)
				}
				if _, ok := h.s.Model().Table("MEMBER").Get(row["id"]); !ok {
					t.Fatal("a refused deletion removed invitation metadata")
				}
				if h.group()["rsvp_yes"] != group["rsvp_yes"] {
					t.Fatal("a refused deletion changed answer groups")
				}
				return
			}
			if err != nil {
				t.Fatalf("delete an empty activity and its inactive invitation state: %v", err)
			}
			for _, id := range []string{h.event, group["rsvp_yes"], group["rsvp_no"]} {
				if _, ok := h.s.Model().Table("GROUP").Get(id); ok {
					t.Fatalf("deletion left group %s", id)
				}
			}
		})
	}
}

func TestOutsidePartyGuestsCannotRestoreCancelledTickets(t *testing.T) {
	h := newInviteHarness(t)
	h.event = tacoNight
	h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "eligible": "", "capacity": "1"}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": tacoNight, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("invite an outside ticket holder: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	ids := h.setup([]Edit{
		{Insert: "PERSON", As: "friend", Row: map[string]any{"source": "guest", "name_long_override": "Pat's Friend"}},
		{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@friend", "address": "friend@example.net", "primary": true, "source": "guest"}},
		{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": "@friend", "member": "cancelled", "guest_of": parent}},
	})
	if h.s.Model().onList(h.group(), ids[0]) || h.copyOf(ids[0]) != nil {
		t.Fatal("the cancelled ticket is already invited")
	}
	rec := h.do("", http.MethodPost, "/open/ext/"+h.copyOf(coach)["token"]+"/guest", map[string]string{"name": "Pat's Friend", "email": "friend@example.net"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("an outside guest restored a cancelled paid ticket: %d %s", rec.Code, rec.Body)
	}
	row, _ := h.s.Model().Table("MEMBER").Get(ids[2])
	if row["member"] != "cancelled" || row["guest_of"] != parent || h.copyOf(ids[0]) != nil || h.s.Model().invitationsFrom(tacoNight, coach) != 0 {
		t.Fatalf("a refused restoration changed the cancelled ticket or sent an invitation: %v", row)
	}
}

func TestActivityInvitationsRemainEffectiveMembershipForRulesAndMail(t *testing.T) {
	h := newListMailHarness(t)
	app, _ := h.s.Model().Table("APP").Find("birthday")
	ids := apply(t, h.s, []Edit{
		{Set: parentsList, Cells: map[string]any{"kind": "activity", "posting": "members", "replying": "members"}},
		{Insert: "MEMBER", Row: map[string]any{"group": parentsList, "person": student, "invited": "yes"}},
		{Insert: "GROUP", As: "viewers", Row: map[string]any{"kind": "group", "name": "Birthdays Viewers", "status": "open", "visible_to": "grp00000000004"}},
		{Insert: "RULE", Row: map[string]any{"group": "@viewers", "target": parentsList, "order": "a"}},
		{Set: app["id"], Cells: map[string]any{"visible_to": "@viewers"}},
		{Insert: "PERSON_EMAIL", Row: map[string]any{"person": student, "address": "juni.ashdown@example.org", "primary": true, "source": "manual"}},
	})
	m := h.s.Model()
	row, _ := m.Table("MEMBER").Get(ids[1])
	if row["member"] != "" || !m.effectivelyIn(ids[2], student) {
		t.Fatalf("invitation-only guest did not reach the activity-target rule: %v", row)
	}
	if got := len(as(t, h.s, student, `(from APP (where (= key "birthday")) (columns key))`)); got != 1 {
		t.Fatalf("invitation-derived app access returned %d apps", got)
	}
	group, _ := m.Table("GROUP").Get(parentsList)
	if !m.mayPost(group, student, false) || !m.mayPost(group, student, true) {
		t.Fatal("an invitation-only guest cannot post or reply as a list member")
	}
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "activity-guests@example.org", "Welcome", "")
	if rec := h.inbound(loopNotice(raw, hummingbirdsTo)); rec.Code != http.StatusOK {
		t.Fatalf("post to the activity list: %d %s", rec.Code, rec.Body)
	}
	if post := h.settled(parentsList); post["state"] != postSent {
		t.Fatalf("the activity post did not send: %v", post)
	}
	outgoing := h.posts(parentsList, "out")
	invited := false
	for _, copy := range h.s.Model().Table("RECIPIENT").Referencing("message", outgoing[0]["id"]) {
		if copy["person"] == student {
			invited = true
		}
	}
	if !invited {
		t.Fatalf("activity Loop mail omitted an invitation-only guest: address %q, post %v", m.addressOf(student), outgoing[0])
	}
}

func TestSelfEligibleActivityGuestsCannotSignUpOtherVolunteers(t *testing.T) {
	for _, outside := range []bool{false, true} {
		t.Run(map[bool]string{false: "community", true: "outside"}[outside], func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "activity"}}})
			person := map[string]string{"person": parent}
			if outside {
				person = map[string]string{"name": "Coach Pat", "email": "coach@example.net"}
			}
			if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{person}}); rec.Code != http.StatusOK {
				t.Fatalf("invite the guest: %d %s", rec.Code, rec.Body)
			}
			viewer := parent
			if outside {
				viewer = h.s.Model().personAt("coach@example.net")
			}
			join := func(person string) error {
				_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": person, "member": "yes", "added_by": viewer}}}})
				return err
			}
			if err := join(staff); err == nil {
				t.Fatal("an invitation-only guest can sign up another volunteer on a self-eligible activity")
			}
			if h.s.Model().directMemberCount(h.event) != 0 {
				t.Fatal("the rejected volunteer addition changed the sign-up count")
			}
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"eligible": "grp00000000001"}}})
			if err := join(staff); err == nil {
				t.Fatal("an activity guest bypassed its volunteer eligibility group")
			}
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"eligible": ""}}})
			if err := join(staff); err != nil {
				t.Fatalf("an open activity requires volunteering before signing up someone else: %v", err)
			}
			volunteer, _ := h.s.Model().Table("MEMBER").Find(h.event, staff)
			h.setup([]Edit{{Delete: volunteer["id"]}, {Set: h.event, Cells: map[string]any{"eligible": h.event}}})
			if outside {
				if rec := h.do("", http.MethodPost, "/open/ext/"+h.copyOf(viewer)["token"]+"/guest", map[string]string{"name": "Assistant Kim", "email": "kim@example.net"}); rec.Code != http.StatusNoContent {
					t.Fatalf("the guest cannot invite another guest: %d %s", rec.Code, rec.Body)
				}
			} else if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": student}}}); rec.Code != http.StatusOK {
				t.Fatalf("the guest cannot invite another guest: %d %s", rec.Code, rec.Body)
			}
			if h.s.Model().directMemberCount(h.event) != 0 {
				t.Fatal("inviting another guest created a volunteer")
			}
			if err := join(viewer); err != nil {
				t.Fatalf("an invited guest cannot volunteer: %v", err)
			}
			if err := join(staff); err != nil {
				t.Fatalf("an actual volunteer cannot sign up another volunteer: %v", err)
			}
		})
	}
}

func TestActivityVolunteerCancellationDoesNotDependOnInvitationMetadata(t *testing.T) {
	for _, metadata := range []string{"plain", "invited", "opened"} {
		for _, viewer := range []string{parent, student} {
			t.Run(metadata+"/"+viewer, func(t *testing.T) {
				h := newInviteHarness(t)
				cells := map[string]any{"group": h.event, "person": parent, "member": "yes"}
				if metadata == "invited" {
					cells["invited"] = "yes"
				}
				if metadata == "opened" {
					cells["opened"] = "2026-11-03 12:00"
				}
				ids := h.setup([]Edit{
					{Set: h.event, Cells: map[string]any{"kind": "activity", "join": ""}},
					{Insert: "MEMBER", Row: cells},
					{Insert: "PERSON", Row: map[string]any{"source": "guest", "name_long_override": "Unrelated guest"}},
				})
				edit := Edit{Delete: ids[1]}
				if metadata != "plain" {
					edit = Edit{Set: ids[1], Cells: map[string]any{"member": ""}}
				}
				write := func(person string) error {
					_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: person, Now: testNow}, Batch{Batch: []Edit{edit}})
					return err
				}
				if err := write(ids[2]); err == nil {
					t.Fatal("someone outside the household cancelled a volunteer commitment")
				}
				if err := write(viewer); err == nil {
					t.Fatal("blank-join cancellation differs by invitation metadata")
				}
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"join": "self"}}})
				if viewer == student {
					if err := write(viewer); err == nil {
						t.Fatal("self-only membership can be cancelled by a household member")
					}
				}
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"join": "none"}}})
				if err := write(viewer); err != nil {
					t.Fatalf("a volunteer or their household cannot cancel a complete activity: %v", err)
				}
				left, ok := h.s.Model().Table("MEMBER").Get(ids[1])
				if metadata == "plain" {
					if ok {
						t.Fatalf("the ordinary sign-up remains: %v", left)
					}
					return
				}
				if !ok || left["member"] != "" || (metadata == "invited" && left["invited"] != "yes") || (metadata == "opened" && left["opened"] != cells["opened"]) {
					t.Fatalf("cancellation lost invitation metadata: %v", left)
				}
			})
		}
	}
}
