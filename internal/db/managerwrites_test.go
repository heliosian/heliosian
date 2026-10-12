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
	kept := false
	for _, a := range s.Model().Table("ALIAS").All() {
		kept = kept || (a["alias"] == "international-night" && a["target"] == night && a["hidden"] == "Yes")
	}
	if !kept {
		t.Error("a group's old short name isn't kept as a hidden alias")
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
	row := map[string]any{"parent": night, "kind": "activity", "name": "Balloons", "status": "pending", "join": "others", "added_by": student, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}
	balloons, err := write(student,
		Edit{Insert: "GROUP", As: "s", Row: row},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@s", "person": student, "member": "yes", "added_by": student}},
	)
	if err != nil {
		t.Fatalf("someone can't suggest a role and sign up for it at once: %v", err)
	}
	if len(as(t, s, student, `(from GROUP (where (= id "`+balloons[0]+`")))`)) != 1 {
		t.Error("someone can't see the suggestion they made")
	}
	if len(as(t, s, guest, `(from GROUP (where (= id "`+balloons[0]+`")))`)) != 0 {
		t.Error("someone sees a suggestion they neither made nor run")
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

func TestOffersToCoChairWaitForTheChairs(t *testing.T) {
	const night, managers = "grp00000000512", "grp00000000513"
	s, queue := sampleWithQueue(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	offersRow := map[string]any{"kind": "group", "parent": night, "name": "International Night Co-Chair Offers", "status": "open", "join": "direct", "eligible": night,
		"visible_to": "grp00000000004", "posting": "none", "replying": "none"}
	if _, err := write(parent, Edit{Insert: "GROUP", As: "o", Row: mergeRow(offersRow, map[string]any{"added_by": parent, "members_visible_to": "grp00000000004"})}, Edit{Set: night, Cells: map[string]any{"chair_offers": "@o"}}); err == nil {
		t.Error("a chair opens offers that everyone sees")
	}
	if _, err := write(student, Edit{Insert: "GROUP", As: "o", Row: mergeRow(offersRow, map[string]any{"added_by": student})}, Edit{Set: night, Cells: map[string]any{"chair_offers": "@o"}}); err == nil {
		t.Error("someone who doesn't run an event opens it to offers")
	}
	opened, err := write(parent, Edit{Insert: "GROUP", As: "o", Row: mergeRow(offersRow, map[string]any{"added_by": parent})}, Edit{Set: night, Cells: map[string]any{"chair_offers": "@o", "join": "others"}})
	if err != nil {
		t.Fatalf("a chair can't open their event to offers: %v", err)
	}
	offers := opened[0]
	if _, err := write(guest, Edit{Insert: "MEMBER", Row: map[string]any{"group": offers, "person": guest, "member": "yes", "added_by": guest}}); err == nil {
		t.Error("someone not signed up offers to co-chair")
	}
	joined, err := write(student,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": night, "person": student, "member": "yes", "added_by": student}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": offers, "person": student, "member": "yes", "added_by": student}},
	)
	if err != nil {
		t.Fatalf("someone can't sign up and offer to co-chair at once: %v", err)
	}
	offer := joined[1:]
	offered := `(from MEMBER (where (= group "` + offers + `")))`
	for viewer, want := range map[string]int{student: 1, parent: 1, guest: 0} {
		if got := len(as(t, s, viewer, offered)); got != want {
			t.Errorf("%s sees %d offers, want %d", viewer, got, want)
		}
	}
	signUp, _ := s.Model().Table("MEMBER").Find(night, student)
	if _, err := write(student, Edit{Set: signUp["id"], Cells: map[string]any{"lead": true}}); err == nil {
		t.Error("a volunteer makes themselves a co-chair")
	}
	if _, err := write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "member": "yes"}}); err == nil {
		t.Error("a volunteer puts themselves among the managers")
	}
	if _, err := write(parent,
		Edit{Set: signUp["id"], Cells: map[string]any{"lead": true}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "member": "yes"}},
		Edit{Delete: offer[0]},
	); err != nil {
		t.Fatalf("a chair can't make a volunteer who offered a co-chair: %v", err)
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"chair_offers": ""}}, Edit{Delete: offers}); err != nil {
		t.Errorf("a chair can't close their event to offers: %v", err)
	}
}

func mergeRow(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
