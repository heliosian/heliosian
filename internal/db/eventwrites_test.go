package db

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func TestEventCreationReportsDuplicateAddress(t *testing.T) {
	s := sample(t)
	before := s.Model()
	row := map[string]any{"kind": "event", "parent": "grp00000000060", "name": "Winter Party", "slug": "hummingbirds", "status": "open", "start": "2027-01-16 13:00", "added_by": parent}
	batch := Batch{Batch: []Edit{
		{Insert: "GROUP", As: "event", Row: row},
		{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Winter Party Managers", "status": "open", "added_by": parent}},
		{Set: "@hosts", Cells: map[string]any{"managed_by": "@hosts"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": parent, "member": "yes"}},
		{Set: "@event", Cells: map[string]any{"managed_by": "@hosts"}},
	}}
	write := func() ([]string, error) {
		return Write(t.Context(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: parent, Now: testNow}, batch)
	}
	_, err := write()
	if err == nil || !strings.Contains(err.Error(), `two rows have the slug "hummingbirds"`) {
		t.Fatalf("duplicate address answered %v", err)
	}
	var refusal *access.Refusal
	if !errors.As(err, &refusal) || refusal.Status != http.StatusBadRequest {
		t.Fatalf("duplicate address is not a bad request: %v", err)
	}
	if s.Model() != before {
		t.Fatal("refused creation changed the model")
	}
	row["slug"] = "winter-party"
	ids, err := write()
	if err != nil {
		t.Fatalf("retry with a different address: %v", err)
	}
	g, _ := s.Model().Table("GROUP").Get(ids[0])
	if g["slug"] != "winter-party" || g["managed_by"] != ids[1] || g["rsvp_yes"] == "" || g["rsvp_no"] == "" || g["hidden_for"] == "" {
		t.Fatalf("retry did not create the hosted event and its groups: %v", g)
	}
}

func TestEventsAreAddedAnsweredAndApproved(t *testing.T) {
	const community = "grp00000000060"
	s := sample(t)
	apply(t, s, []Edit{
		{Set: community, Cells: map[string]any{"adding": "approval"}},
		{Insert: "GROUP", As: "admins", Row: map[string]any{"kind": "admins", "name": "When Admins", "status": "open", "managed_by": "grp00000000005"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": student, "member": "yes"}},
		{Set: whenApp, Cells: map[string]any{"admins": "@admins"}},
	})
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	event := func(status string) map[string]any {
		return map[string]any{"kind": "event", "parent": community, "name": "Hummingbirds playdate", "status": status, "start": "2026-11-01 10:00", "added_by": parent, "listed": true, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}
	}

	if _, err := write(parent, Edit{Insert: "GROUP", Row: event("open")}); err == nil {
		t.Error("a parent's event under a category that wants approval went live")
	}
	made, err := write(parent, Edit{Insert: "GROUP", Row: event("pending")})
	if err != nil {
		t.Fatalf("a parent can't suggest an event: %v", err)
	}
	g, _ := s.Model().Table("GROUP").Get(made[0])
	if g["rsvp_yes"] == "" || g["rsvp_no"] == "" || g["hidden_for"] == "" {
		t.Fatalf("a new event lacks its answer and hidden groups: %v", g)
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"status": "open"}}); err == nil {
		t.Error("a parent approved their own event")
	}
	if _, err := write(student, Edit{Set: made[0], Cells: map[string]any{"status": "open"}}); err != nil {
		t.Errorf("a When admin can't approve an event: %v", err)
	}
	if _, err := write(student, Edit{Set: made[0], Cells: map[string]any{"name": "Hummingbirds Playdate at the Park"}}); err != nil {
		t.Errorf("a When admin can't edit an event: %v", err)
	}
	if _, err := write(student, Edit{Insert: "GROUP", Row: map[string]any{"kind": "event", "parent": community, "name": "Book swap", "status": "open", "start": "2026-11-02", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}}); err != nil {
		t.Errorf("a When admin's event can't go live: %v", err)
	}

	if _, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": g["rsvp_yes"], "person": parent, "member": "yes", "added_by": parent}}); err != nil {
		t.Errorf("someone can't say they're going to an event they see: %v", err)
	}
	if _, err := write(guest, Edit{Insert: "MEMBER", Row: map[string]any{"group": g["rsvp_no"], "person": parent, "member": "yes", "added_by": guest}}); err == nil {
		t.Error("someone answered for a person outside their family")
	}
}

func TestWhoeverAddsAnEventHostsItAndAPrivateOneGoesLive(t *testing.T) {
	const community = "grp00000000060"
	s := sample(t)
	apply(t, s, []Edit{{Set: community, Cells: map[string]any{"adding": "approval"}}})
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	hosted := func(cells map[string]any) []Edit {
		row := map[string]any{"kind": "event", "parent": community, "name": "Euler Dinner & Set", "status": "open", "start": "2026-11-04 17:30", "added_by": parent}
		for k, v := range cells {
			row[k] = v
		}
		return []Edit{
			{Insert: "GROUP", As: "e", Row: row},
			{Insert: "GROUP", As: "m", Row: map[string]any{"kind": "group", "name": "Euler Dinner & Set Managers", "status": "open", "added_by": parent}},
			{Set: "@m", Cells: map[string]any{"managed_by": "@m"}},
			{Insert: "MEMBER", Row: map[string]any{"group": "@m", "person": parent, "member": "yes"}},
			{Set: "@e", Cells: map[string]any{"managed_by": "@m"}},
		}
	}
	invite, err := write(parent, append(hosted(map[string]any{"visible_to": "", "members_visible_to": ""}), Edit{Set: "@e", Cells: map[string]any{"visible_to": "@e"}})...)
	if err != nil {
		t.Fatalf("someone can't add and host an invite-only event: %v", err)
	}
	if g, _ := s.Model().Table("GROUP").Get(invite[0]); g["status"] != "open" || g["visible_to"] != invite[0] || g["managed_by"] != invite[1] {
		t.Fatalf("the invite-only event reads %v", g)
	}
	if len(as(t, s, guest, `(from GROUP (where (= id "`+invite[0]+`")))`)) != 0 {
		t.Error("someone not invited sees an invite-only event")
	}
	if _, err := write(parent, hosted(map[string]any{"visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "listed": false})...); err != nil {
		t.Errorf("someone can't add a link-only event live: %v", err)
	}
	public := map[string]any{"visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "listed": true}
	if _, err := write(parent, hosted(public)...); err == nil {
		t.Error("a public event went live without approval")
	}
	public["status"] = "pending"
	suggested, err := write(parent, hosted(public)...)
	if err != nil {
		t.Fatalf("someone can't suggest and host a public event: %v", err)
	}
	if _, err := write(parent, Edit{Set: suggested[0], Cells: map[string]any{"status": "open"}}); err == nil {
		t.Error("the host of a suggested event approved it")
	}
	bare, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"kind": "event", "parent": community, "name": "Bare", "status": "pending", "start": "2026-11-05", "added_by": parent, "listed": true, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := write(staff, Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Not Theirs", "status": "open", "added_by": staff}})
	if _, err := write(parent, Edit{Set: bare[0], Cells: map[string]any{"managed_by": other[0]}}); err == nil {
		t.Error("someone handed the event they added to a group they didn't make")
	}
}

func TestSavedSearchesAreTheirOwnersAlone(t *testing.T) {
	s := sample(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	made, err := write(parent, Edit{Insert: "SAVED_SEARCH", Row: map[string]any{"person": parent, "name": "Mine", "search": `[["community"]]`}})
	if err != nil {
		t.Fatalf("someone can't save a search: %v", err)
	}
	row, _ := s.Model().Table("SAVED_SEARCH").Get(made[0])
	if row["token"] == "" {
		t.Fatal("a saved search has no feed token")
	}
	if _, err := write(guest, Edit{Insert: "SAVED_SEARCH", Row: map[string]any{"person": parent, "name": "Theirs", "search": `[]`}}); err == nil {
		t.Error("someone saved a search for someone else")
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"name": "Ours", "search": `[["community"],["going"]]`, "default": true}}); err != nil {
		t.Errorf("someone can't change their own saved search: %v", err)
	}
	if _, err := write(guest, Edit{Set: made[0], Cells: map[string]any{"name": "Taken"}}); err == nil {
		t.Error("someone renamed another's saved search")
	}
	if _, err := write(guest, Edit{Delete: made[0]}); err == nil {
		t.Error("someone removed another's saved search")
	}
	if _, err := write(parent, Edit{Delete: made[0]}); err != nil {
		t.Errorf("someone can't remove their own saved search: %v", err)
	}
}
