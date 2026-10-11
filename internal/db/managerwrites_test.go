package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestAGroupsManagersRunWhatIsUnderIt(t *testing.T) {
	const night = "grp00000000512"
	s, queue := sampleWithQueue(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	child := func(viewer string, cells map[string]any) ([]string, error) {
		row := map[string]any{"parent": night, "kind": "activity", "name": "Decor", "status": "open", "added_by": viewer, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}
		for k, v := range cells {
			row[k] = v
		}
		return write(viewer, Edit{Insert: "GROUP", Row: row})
	}

	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"timing": "All evening", "capacity": "40", "join": "below", "adding": "managers", "start": "2027-02-06 17:30", "slug": "inight"}}); err != nil {
		t.Errorf("a chair can't change their event: %v", err)
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"priority": true}}); err == nil {
		t.Error("a chair marks their event a priority")
	}
	if _, err := write(student, Edit{Set: night, Cells: map[string]any{"timing": "Mine"}}); err == nil {
		t.Error("someone who doesn't run an event changes it")
	}

	decor, err := child(parent, nil)
	if err != nil {
		t.Fatalf("a chair can't add a role under their event: %v", err)
	}
	if _, err := child(parent, map[string]any{"kind": "party"}); err == nil {
		t.Error("a chair adds a party under an activity")
	}
	if _, err := child(parent, map[string]any{"managed_by": "grp00000000041"}); err == nil {
		t.Error("a chair adds a role run by someone else's managers")
	}
	if _, err := child(student, map[string]any{"status": "pending"}); err == nil {
		t.Error("someone suggests a role under an event that takes no additions")
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": night}, store.Row{"adding": "approval"})); err != nil {
		t.Fatal(err)
	}
	if _, err := child(student, nil); err == nil {
		t.Error("a suggestion under an event that wants approval goes live")
	}
	suggested, err := child(student, map[string]any{"status": "pending", "name": "Face Painting"})
	if err != nil {
		t.Fatalf("someone can't suggest a role under an event that takes suggestions: %v", err)
	}
	if _, err := write(student, Edit{Set: suggested[0], Cells: map[string]any{"status": "open"}}); err == nil {
		t.Error("someone approves their own suggestion")
	}
	if _, err := write(parent, Edit{Set: suggested[0], Cells: map[string]any{"status": "open"}}); err != nil {
		t.Errorf("a chair can't approve a suggestion under their event: %v", err)
	}

	heading, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"parent": night, "kind": "category", "name": "Booths", "status": "open", "added_by": parent, "visible_to": "grp00000000004"}})
	if err != nil {
		t.Fatalf("a chair can't add a heading under their event: %v", err)
	}
	if _, err := write(parent, Edit{Set: decor[0], Cells: map[string]any{"parent": heading[0]}}); err != nil {
		t.Errorf("a chair can't move a role under their event's heading: %v", err)
	}
	if _, err := write(parent, Edit{Set: decor[0], Cells: map[string]any{"parent": ""}}); err == nil {
		t.Error("a chair moves a role out from under everything")
	}

	if _, err := write(student, Edit{Set: decor[0], Cells: map[string]any{"managed_by": "grp00000000030"}}); err == nil {
		t.Error("someone who doesn't run a role hands it to managers")
	}
	cochair, err := write(parent,
		Edit{Insert: "GROUP", As: "m", Row: map[string]any{"kind": "group", "name": "Decor Managers", "status": "open", "added_by": parent}},
		Edit{Set: decor[0], Cells: map[string]any{"managed_by": "@m"}},
		Edit{Set: "@m", Cells: map[string]any{"managed_by": "@m"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@m", "person": student, "member": "yes"}},
	)
	if err != nil {
		t.Fatalf("a chair can't make someone co-chair of a role under their event: %v", err)
	}
	if _, err := write(student, Edit{Set: decor[0], Cells: map[string]any{"timing": "Before doors"}}); err != nil {
		t.Errorf("a role's new co-chair can't change it: %v", err)
	}
	if _, err := write(parent, Edit{Delete: cochair[3]}); err != nil {
		t.Errorf("a chair can't stop someone co-chairing a role under their event: %v", err)
	}

	if _, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": suggested[0], "person": staff, "member": "yes"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := write(parent, Edit{Delete: suggested[0]}); err == nil {
		t.Error("a chair deletes a role someone is signed up for")
	}
	empty, err := child(parent, map[string]any{"name": "Spare"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write(parent, Edit{Delete: empty[0]}); err != nil {
		t.Errorf("a chair can't delete an empty role: %v", err)
	}

	if err := commit(s, GroupsSheet, store.Insert("GROUP_SOURCE", store.Row{"id": "src00000000900", "group": night, "calendar_event": "x"})); err != nil {
		t.Fatal(err)
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"start": "2027-02-07 17:30"}}); err == nil {
		t.Error("a chair changes the time of an event the calendar import brings")
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"timing": "All evening long"}}); err != nil {
		t.Errorf("a chair can't change the words of an event the calendar import brings: %v", err)
	}
}
