package db

import (
	"context"
	"slices"
	"testing"

	"heliosian/internal/access"
)

func TestHidingIsJoiningForOneselfAlone(t *testing.T) {
	const night = "grp00000000512"
	s := sample(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	made, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"parent": night, "kind": "activity", "name": "Decor", "status": "open", "added_by": parent, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}})
	if err != nil {
		t.Fatal(err)
	}
	decor, _ := s.Model().Table("GROUP").Get(made[0])
	hidden, ok := s.Model().Table("GROUP").Get(decor["hidden_for"])
	if !ok || hidden["parent"] != "" || hidden["join"] != "self" || hidden["name"] != "Decor Hidden" || hidden["visible_to"] != "" {
		t.Fatalf("a new activity's hidden group is %v", hidden)
	}
	hide := func(viewer, person string) ([]string, error) {
		return write(viewer, Edit{Insert: "MEMBER", Row: map[string]any{"group": hidden["id"], "person": person, "member": "yes", "added_by": viewer}})
	}

	if _, err := hide(student, parent); err == nil {
		t.Error("someone hides an activity for someone in their family")
	}
	if _, err := hide(parent, staff); err == nil {
		t.Error("someone hides an activity for someone else")
	}
	staffHid, err := hide(staff, staff)
	if err != nil {
		t.Fatalf("someone can't hide an activity they see: %v", err)
	}
	if rows := as(t, s, parent, `(from MEMBER (where (= group "`+hidden["id"]+`")))`); len(rows) != 0 {
		t.Errorf("the activity's chair reads who hid it: %v", rows)
	}
	if rows := as(t, s, parent, `(from EFFECTIVE_MEMBER (where (= group "`+hidden["id"]+`")))`); len(rows) != 0 {
		t.Errorf("the activity's chair reads who in effect hid it: %v", rows)
	}

	shown := func(viewer string) bool {
		t.Helper()
		got := s.Model().Run(t.Context(), mustParse(t, `(from GROUP @e (where (in kind "event" "party" "activity") `+searchCondition(nil)+`))`), Env{Viewer: viewer, Now: testNow})
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		return slices.Contains(got.IDs, decor["id"])
	}
	if shown(staff) {
		t.Error("a saved search shows what its viewer hid")
	}
	if !shown(parent) {
		t.Error("one person hiding an activity hides it from another")
	}

	parentHid, err := hide(parent, parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write(student, Edit{Delete: parentHid[0]}); err == nil {
		t.Error("someone shows an activity again for someone in their family")
	}
	if _, err := write(parent, Edit{Delete: parentHid[0]}); err != nil {
		t.Errorf("someone can't show again what they hid: %v", err)
	}

	if _, err := write(parent, Edit{Delete: decor["id"]}); err != nil {
		t.Fatalf("a chair can't delete an empty role someone hid: %v", err)
	}
	for _, id := range []string{hidden["id"], staffHid[0]} {
		if s.Model().Has(id) {
			t.Errorf("%s outlived the activity it hid", id)
		}
	}
}
