package db

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestALoopListIsMadeAndRunByItsManagers(t *testing.T) {
	const everyone, hummingbirds, hummingbirdsParents, nightManagers, picnic, picnicManagers = "grp00000000004", "grp00000000010", "grp00000000030", "grp00000000513", "grp00000000040", "grp00000000041"
	s, queue := sampleWithQueue(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": picnicManagers}, store.Row{"members_visible_to": ""})); err != nil {
		t.Fatal(err)
	}
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, newPictures(s, queue), access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	makeList := func(viewer, slug string, rules ...map[string]any) ([]string, error) {
		edits := []Edit{
			{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "name": slug + " Managers", "status": "open", "added_by": viewer}},
			{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
			{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": viewer, "member": "yes"}},
			{Insert: "GROUP", As: "list", Row: map[string]any{"kind": "group", "name": slug, "slug": slug, "status": "open", "mail": true, "listed": true, "visible_to": everyone, "members_visible_to": everyone, "posting": "everyone", "replying": "everyone", "managed_by": "@managers", "added_by": viewer}},
		}
		for i, r := range rules {
			r["group"], r["order"] = "@list", string(rune('a'+i))
			edits = append(edits, Edit{Insert: "RULE", Row: r})
		}
		return write(viewer, edits...)
	}

	ids, err := makeList(parent, "night-parents",
		map[string]any{"target": "grp00000000512", "replace_with": "parents"},
		map[string]any{"target": hummingbirds, "replace_with": "parents"})
	if err != nil {
		t.Fatalf("a parent can't make a list of a sign-up list's and a classroom's parents: %v", err)
	}
	list := ids[3]
	if _, err := makeList(parent, "picnic-hosts", map[string]any{"target": picnicManagers}); err == nil || !strings.Contains(err.Error(), "RULE") {
		t.Errorf("a parent makes a list of a group whose members they can't see: %v", err)
	}
	if _, err := makeList(guest, "guests"); err == nil {
		t.Error("a guest makes a list")
	}
	if _, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Theirs", "slug": "theirs", "status": "open", "mail": true, "managed_by": nightManagers, "added_by": parent}}); err != nil {
		t.Errorf("a parent can't make a list run by managers they're among: %v", err)
	}
	if _, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Hosts", "slug": "hosts", "status": "open", "mail": true, "managed_by": picnicManagers, "added_by": parent}}); err == nil {
		t.Error("a parent makes a list run by managers they aren't among")
	}

	if _, err := write(parent, Edit{Set: list, Cells: map[string]any{"name": "Night Parents", "posting": "members", "visible_to": list}}); err != nil {
		t.Errorf("a list's manager can't rename it and show it to its members: %v", err)
	}
	if _, err := write(parent, Edit{Set: list, Cells: map[string]any{"visible_to": "grp00000000002"}}); err == nil {
		t.Error("a list's manager shows it to another group")
	}
	if _, err := write(parent, Edit{Set: list, Cells: map[string]any{"slug": "renamed"}}); err == nil {
		t.Error("a list's manager changes its address")
	}
	if _, err := write(student, Edit{Set: list, Cells: map[string]any{"name": "Mine"}}); err == nil {
		t.Error("someone who doesn't manage a list renames it")
	}
	if _, err := write(staff, Edit{Set: list, Cells: map[string]any{"description": "Checked"}}); err != nil {
		t.Errorf("a Loop admin can't describe any list: %v", err)
	}

	added, err := write(parent,
		Edit{Insert: "PERSON", As: "aunt", Row: map[string]any{"source": "guest", "name_long_override": "Aunt Ashdown"}},
		Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@aunt", "address": "aunt@example.org", "primary": true, "source": "guest"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": list, "person": "@aunt", "member": "yes"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": list, "person": staff, "member": "excluded"}},
	)
	if err != nil {
		t.Fatalf("a list's manager can't add a guest by hand and keep someone off: %v", err)
	}
	if _, err := write(student, Edit{Insert: "PERSON", Row: map[string]any{"source": "guest", "name_long_override": "Stranger"}}); err == nil {
		t.Error("someone who runs no list adds a guest")
	}
	if _, err := write(parent, Edit{Delete: added[3]}); err != nil {
		t.Errorf("a list's manager can't take a keeping off away: %v", err)
	}

	if _, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": hummingbirdsParents, "person": student, "member": "excluded"}}); err == nil || !strings.Contains(err.Error(), "MEMBER") {
		t.Errorf("someone unsubscribes another person from a list they don't run: %v", err)
	}
	if _, err := write(guest, Edit{Insert: "MEMBER", Row: map[string]any{"group": hummingbirdsParents, "person": guest, "member": "excluded"}}); err == nil || !strings.Contains(err.Error(), "MEMBER") {
		t.Errorf("someone the list doesn't take in unsubscribes: %v", err)
	}
	off, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": hummingbirdsParents, "person": parent, "member": "excluded"}})
	if err != nil {
		t.Fatalf("a parent can't unsubscribe from their classroom's list: %v", err)
	}
	for _, row := range s.Model().effectiveRows(hummingbirdsParents) {
		if row["person"] == parent {
			t.Error("an unsubscribed parent is still on the list")
		}
	}
	if _, err := write(parent, Edit{Delete: off[0]}); err != nil {
		t.Errorf("a parent can't resubscribe: %v", err)
	}

	if _, err := write(parent, Edit{Insert: "ALIAS", Row: map[string]any{"alias": "night", "target": list}}); err != nil {
		t.Errorf("a list's manager can't give it another address: %v", err)
	}
	if _, err := write(parent, Edit{Insert: "ALIAS", Row: map[string]any{"alias": "picnic", "target": picnic}}); err == nil {
		t.Error("a parent gives an event another name")
	}
}

func TestTheMailerRecordsPostsAndUnsubscribes(t *testing.T) {
	const hummingbirdsParents = "grp00000000030"
	s, queue := sampleWithQueue(t)
	write := func(edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, newPictures(s, queue), access.System("loop"), Env{System: "loop", Now: testNow}, Batch{Batch: edits})
	}
	ids, err := write(
		Edit{Insert: "MESSAGE", As: "in", Row: map[string]any{"direction": "in", "kind": "post", "group": hummingbirdsParents, "subject": "Field trip", "created": "2026-10-09 09:00", "state": "received"}},
		Edit{Insert: "MESSAGE", As: "out", Row: map[string]any{"direction": "out", "kind": "post", "group": hummingbirdsParents, "parent": "@in", "subject": "Field trip", "created": "2026-10-09 09:01"}},
		Edit{Insert: "RECIPIENT", As: "copy", Row: map[string]any{"message": "@out", "person": parent, "created": "2026-10-09 09:01"}},
		Edit{Set: "@in", Cells: map[string]any{"state": "sent"}},
		Edit{Set: "@copy", Cells: map[string]any{"sent": "2026-10-09 09:01", "delivered": "2026-10-09 09:02"}},
	)
	if err != nil {
		t.Fatalf("the mailer can't take in and send on a post: %v", err)
	}
	if row, _ := s.Model().Table("MESSAGE").Get(ids[0]); row["state"] != "sent" {
		t.Errorf("the post's state reads %q", row["state"])
	}
	if _, err := write(Edit{Insert: "MESSAGE", Row: map[string]any{"direction": "in", "kind": "post", "group": "grp00000000040", "subject": "Picnic", "created": "2026-10-09 09:00"}}); err == nil {
		t.Error("the mailer takes in a post for a group that is no list")
	}
	if _, err := write(Edit{Insert: "MESSAGE", Row: map[string]any{"direction": "out", "kind": "invitation", "group": hummingbirdsParents, "subject": "Party", "created": "2026-10-09 09:00"}}); err == nil {
		t.Error("the mailer writes an invitation")
	}
	if _, err := write(Edit{Insert: "MEMBER", Row: map[string]any{"group": hummingbirdsParents, "person": parent, "member": "excluded"}}); err != nil {
		t.Errorf("the mailer can't unsubscribe someone by their link: %v", err)
	}
	if _, err := write(Edit{Insert: "MEMBER", Row: map[string]any{"group": hummingbirdsParents, "person": staff, "member": "yes"}}); err == nil {
		t.Error("the mailer puts someone on a list")
	}
	if _, err := Write(context.Background(), s, queue, newPictures(s, queue), access.System("import"), Env{System: "import", Now: testNow}, Batch{Batch: []Edit{{Set: ids[0], Cells: map[string]any{"state": "dropped", "detail": "auto-submitted mail"}}}}); err != nil {
		t.Errorf("the sync can't carry a post's state from the old sheet: %v", err)
	}
}
