package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
)

func TestJoiningFollowsTheGroupsJoinEligibleCapacityAndPrice(t *testing.T) {
	const night, nightLeads, taco, students = "grp00000000512", "grp00000000514", "grp00000000521", "grp00000000001"
	s, queue := sampleWithQueue(t)
	ids := apply(t, s, queue, []Edit{
		{Insert: "GROUP", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Decor", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "others", "capacity": "2"}},
		{Insert: "GROUP", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Booth", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "direct", "eligible": students}},
	})
	decor, booth := ids[0], ids[1]
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	join := func(viewer, group, person string, extra map[string]any) ([]string, error) {
		row := map[string]any{"group": group, "person": person, "member": "yes", "added_by": viewer}
		for k, v := range extra {
			row[k] = v
		}
		return write(viewer, Edit{Insert: "MEMBER", Row: row})
	}

	mine, err := join(student, decor, student, nil)
	if err != nil {
		t.Fatalf("a student can't sign themselves up where join is others: %v", err)
	}
	theirs, err := join(student, decor, staff, nil)
	if err != nil {
		t.Errorf("a student can't sign up someone outside their family where join is others: %v", err)
	}
	if _, err := join(student, decor, parent, nil); err == nil {
		t.Error("a sign-up goes past the capacity")
	}
	apply(t, s, queue, []Edit{{Set: decor, Cells: map[string]any{"join": "direct", "capacity": ""}}})
	if _, err := join(student, decor, staff, nil); err == nil {
		t.Error("someone outside the family is signed up where join is direct")
	}
	if _, err := join(student, decor, parent, nil); err != nil {
		t.Errorf("someone can't sign up their family where join is direct: %v", err)
	}
	if _, err := join(student, booth, parent, nil); err == nil {
		t.Error("a parent who isn't eligible is signed up")
	}
	if _, err := join(student, nightLeads, student, nil); err == nil {
		t.Error("someone makes themselves a lead")
	}
	if _, err := write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": booth, "person": student, "member": "yes", "added_by": parent}}); err == nil {
		t.Error("a sign-up names someone else as who made it")
	}
	if _, err := join(student, taco, student, nil); err == nil {
		t.Error("someone joins a group with a price without paying")
	}
	inBooth, err := join(student, booth, student, nil)
	if err != nil {
		t.Fatalf("an eligible student can't sign themselves up where join is direct: %v", err)
	}

	if _, err := write(student, Edit{Delete: theirs[0]}); err == nil {
		t.Error("someone takes another family's sign-up away")
	}
	if _, err := write(student, Edit{Delete: mine[0]}); err != nil {
		t.Errorf("someone can't leave: %v", err)
	}
	apply(t, s, queue, []Edit{{Set: booth, Cells: map[string]any{"join": "none"}}})
	if _, err := write(student, Edit{Delete: inBooth[0]}); err != nil {
		t.Errorf("someone can't leave once it is closed: %v", err)
	}
}

func TestWhoeverMayJoinAGroupTakingOthersAddsAnyone(t *testing.T) {
	s, queue := sampleWithQueue(t)
	party := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "party", Row: map[string]any{"kind": "event", "status": "open", "name": "Birthday Party", "visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "others"}},
		{Set: "@party", Cells: map[string]any{"eligible": "@party"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@party", "person": student, "member": "yes"}},
	})[0]
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	join := func(viewer, person string) ([]string, error) {
		return write(viewer, Edit{Insert: "MEMBER", Row: map[string]any{"group": party, "person": person, "member": "yes", "added_by": viewer}})
	}
	if _, err := join(parent, parent); err == nil {
		t.Error("someone not on an invitee-only list puts themselves on it")
	}
	if _, err := join(student, parent); err != nil {
		t.Errorf("someone on the list can't add a guest from outside it: %v", err)
	}
	if _, err := join(parent, staff); err != nil {
		t.Errorf("a guest just added can't add another: %v", err)
	}
}

func TestARuleUnderItsTargetTakesInEverythingBelow(t *testing.T) {
	const night, picnic, picnicGoing = "grp00000000512", "grp00000000040", "grp00000000042"
	s, queue := sampleWithQueue(t)
	list := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "decor", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Decor", "visible_to": "grp00000000004", "posting": "members", "replying": "members"}},
		{Insert: "GROUP", As: "list", Row: map[string]any{"kind": "group", "status": "open", "name": "Picnic Everyone", "visible_to": "grp00000000004", "posting": "members", "replying": "members"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@decor", "person": staff, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": picnicGoing, "person": student, "member": "yes"}},
		{Insert: "RULE", Row: map[string]any{"group": night, "order": "a", "target": night, "under": "Yes"}},
		{Insert: "RULE", Row: map[string]any{"group": "@list", "order": "a", "target": picnic, "under": "Yes"}},
	})[1]
	people := func(group string) map[string]bool {
		out := map[string]bool{}
		for _, row := range s.Model().effectiveRows(group) {
			out[row["person"]] = true
		}
		return out
	}
	if got := people(night); !got[parent] || !got[staff] || len(got) != 2 {
		t.Errorf("a group's own rule under itself gives %v, want its own member and everyone signed up under it", got)
	}
	if got := people(list); got[student] || !got[parent] || !got[staff] {
		t.Errorf("a rule under a group gives %v, want its members and none of its answer groups'", got)
	}
}
