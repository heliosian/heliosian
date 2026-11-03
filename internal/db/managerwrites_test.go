package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
)

func TestAGroupsManagersRunWhatIsUnderIt(t *testing.T) {
	const night = "grp00000000512"
	s := sample(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
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
	apply(t, s, []Edit{{Set: night, Cells: map[string]any{"adding": "approval"}}})
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
		Edit{Set: "@m", Cells: map[string]any{"managed_by": "@m"}},
		Edit{Insert: "MEMBER", As: "creator", Row: map[string]any{"group": "@m", "person": parent, "member": "yes"}},
		Edit{Set: decor[0], Cells: map[string]any{"managed_by": "@m"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@m", "person": student, "member": "yes"}},
		Edit{Delete: "@creator"},
	)
	if err != nil {
		t.Fatalf("a chair can't make someone co-chair of a role under their event: %v", err)
	}
	if _, err := write(student, Edit{Set: decor[0], Cells: map[string]any{"timing": "Before doors"}}); err != nil {
		t.Errorf("a role's new co-chair can't change it: %v", err)
	}
	if _, err := write(parent, Edit{Delete: cochair[4]}); err != nil {
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

	apply(t, s, []Edit{{Insert: "GROUP_SOURCE", Row: map[string]any{"group": night, "calendar_event": "x"}}})
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"start": "2027-02-07 17:30"}}); err == nil {
		t.Error("a chair changes the time of an event the calendar import brings")
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"timing": "All evening long"}}); err != nil {
		t.Errorf("a chair can't change the words of an event the calendar import brings: %v", err)
	}
}

func TestOffersToCoChairWaitForTheChairs(t *testing.T) {
	const night, managers, leads = "grp00000000512", "grp00000000513", "grp00000000514"
	s := sample(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
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
	if _, err := write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": student, "member": "yes"}}); err == nil {
		t.Error("a volunteer makes themselves a co-chair")
	}
	if _, err := write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "member": "yes"}}); err == nil {
		t.Error("a volunteer puts themselves among the managers")
	}
	if _, err := write(parent,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": student, "member": "yes"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "member": "yes"}},
		Edit{Delete: offer[0]},
	); err != nil {
		t.Fatalf("a chair can't make a volunteer who offered a co-chair: %v", err)
	}
	if _, err := write(parent, Edit{Set: night, Cells: map[string]any{"chair_offers": ""}}, Edit{Delete: offers}); err != nil {
		t.Errorf("a chair can't close their event to offers: %v", err)
	}
}

func TestAChairNamesAnActivitysLeads(t *testing.T) {
	const night = "grp00000000512"
	s := sample(t)
	decor := apply(t, s, []Edit{
		{Insert: "GROUP", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Decor", "visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "others"}},
	})[0]
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	leadsRow := map[string]any{"kind": "group", "parent": decor, "name": "Decor Leads", "status": "open",
		"visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none"}
	name := Edit{Set: decor, Cells: map[string]any{"leads": "@l"}}
	if _, err := write(student, Edit{Insert: "GROUP", As: "l", Row: mergeRow(leadsRow, map[string]any{"added_by": student})}, name); err == nil {
		t.Error("someone who doesn't run an activity names its leads")
	}
	if _, err := write(parent, Edit{Insert: "GROUP", As: "l", Row: mergeRow(leadsRow, map[string]any{"added_by": parent, "join": "direct"})}, name); err == nil {
		t.Error("a chair makes a leads group anyone joins")
	}
	made, err := write(parent,
		Edit{Insert: "GROUP", As: "l", Row: mergeRow(leadsRow, map[string]any{"added_by": parent})}, name,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@l", "person": student, "member": "yes"}},
	)
	if err != nil {
		t.Fatalf("a chair can't name a co-chair: %v", err)
	}
	led := `(from MEMBER (where (= group "` + made[0] + `")))`
	for viewer, want := range map[string]int{student: 1, staff: 1, guest: 0} {
		if got := len(as(t, s, viewer, led)); got != want {
			t.Errorf("%s sees %d co-chairs of an activity whose volunteers are hidden, want %d", viewer, got, want)
		}
	}
	if _, err := write(student, Edit{Delete: made[2]}); err == nil {
		t.Error("a co-chair stops being one by themselves")
	}
	if _, err := write(parent, Edit{Delete: made[2]}); err != nil {
		t.Errorf("a chair can't stop someone being a co-chair: %v", err)
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

func TestJoiningClaimsAnOpenedInvitation(t *testing.T) {
	const night, managers, leads = "grp00000000512", "grp00000000513", "grp00000000514"
	for _, chair := range []bool{false, true} {
		t.Run(map[bool]string{false: "volunteer", true: "co-chair"}[chair], func(t *testing.T) {
			s := sample(t)
			apply(t, s, []Edit{{Set: night, Cells: map[string]any{"join": "others"}}})
			opened := apply(t, s, []Edit{
				{Insert: "MEMBER", Row: map[string]any{"group": night, "person": student, "opened": "2026-11-02 23:57"}},
				{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": student, "opened": "2026-11-02 23:57"}},
				{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "opened": "2026-11-02 23:57"}},
			})
			viewer := student
			if chair {
				viewer = parent
			}
			if s.Model().superAdmin(viewer) {
				t.Fatal("regression check must use a non-super-admin")
			}
			edits := []Edit{{Insert: "MEMBER", As: "joined", Row: map[string]any{"group": night, "person": student, "member": "yes", "note": "Movie co-chair", "added_by": viewer, "added": "2026-11-03 12:00"}}}
			if chair {
				edits = append(edits,
					Edit{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": student, "member": "yes", "added_by": viewer}},
					Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": student, "member": "yes"}},
				)
			}
			edits = append(edits, Edit{Set: "@joined", Cells: map[string]any{"opened": "2026-11-02 23:57"}})
			written, err := Write(t.Context(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
			if err != nil {
				t.Fatalf("joining over an opened invitation: %v", err)
			}
			for i, id := range written[:len(written)-1] {
				if id != opened[i] {
					t.Errorf("write returned %s, want existing membership %s", id, opened[i])
				}
				row, _ := s.Model().Table("MEMBER").Get(id)
				if row["member"] != "yes" || row["opened"] != "2026-11-02 23:57" {
					t.Errorf("joined membership: %v", row)
				}
			}
			row, _ := s.Model().Table("MEMBER").Get(opened[0])
			if row["note"] != "Movie co-chair" || row["added_by"] != viewer || row["added"] != "2026-11-03 12:00" {
				t.Errorf("sign-up metadata: %v", row)
			}
		})
	}
}

func TestClaimingAnInvitationRequiresNormalJoiningPermissions(t *testing.T) {
	const night, leads = "grp00000000512", "grp00000000514"
	for _, tc := range []struct {
		name   string
		group  string
		cells  map[string]any
		member string
	}{
		{name: "full", group: night, cells: map[string]any{"capacity": "1"}},
		{name: "ineligible", group: night, cells: map[string]any{"eligible": "grp00000000003"}},
		{name: "closed joining", group: night, cells: map[string]any{"join": "none"}},
		{name: "self appointment", group: leads},
		{name: "already signed up", group: night, member: "yes"},
		{name: "excluded", group: night, member: "excluded"},
		{name: "cancelled", group: night, member: "cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sample(t)
			apply(t, s, []Edit{{Set: night, Cells: map[string]any{"join": "others"}}})
			if len(tc.cells) > 0 {
				apply(t, s, []Edit{{Set: night, Cells: tc.cells}})
			}
			id := apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": tc.group, "person": student, "member": tc.member, "opened": "2026-11-02 23:57"}}})[0]
			_, err := Write(t.Context(), s, access.Actor{Email: "student@example.org"}, Env{Viewer: student, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": tc.group, "person": student, "member": "yes", "added_by": student}}}})
			if err == nil {
				t.Fatal("joining was allowed")
			}
			row, _ := s.Model().Table("MEMBER").Get(id)
			if row["member"] != tc.member || row["opened"] != "2026-11-02 23:57" {
				t.Errorf("refused join changed the membership: %v", row)
			}
		})
	}
}

func TestAChairMovesASignUpOntoAnOpenedInvitation(t *testing.T) {
	const night, leads = "grp00000000512", "grp00000000514"
	s := sample(t)
	member, _ := s.Model().Table("MEMBER").Find(night, parent)
	lead, _ := s.Model().Table("MEMBER").Find(leads, parent)
	made := apply(t, s, []Edit{
		{Set: member["id"], Cells: map[string]any{"member": "", "opened": "2026-11-02 23:57"}},
		{Delete: lead["id"]},
		{Insert: "GROUP", As: "tech", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Tech Setup", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "join": "others"}},
		{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "status": "open", "name": "Tech Setup Managers"}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Set: "@tech", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "GROUP", As: "leads", Row: map[string]any{"kind": "group", "parent": "@tech", "status": "open", "name": "Tech Setup Leads"}},
		{Set: "@tech", Cells: map[string]any{"leads": "@leads"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@tech", "person": parent, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@leads", "person": parent, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": parent, "member": "yes"}},
	})
	if s.Model().superAdmin(parent) {
		t.Fatal("move check must use a non-super-admin")
	}
	_, err := Write(t.Context(), s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{
		{Insert: "MEMBER", Row: map[string]any{"group": night, "person": parent, "member": "yes", "note": "Movie co-chair", "added_by": parent, "added": "2026-11-03 12:00"}},
		{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": parent, "member": "yes", "added_by": parent}},
		{Delete: made[8]},
		{Delete: made[9]},
		{Delete: made[10]},
	}})
	if err != nil {
		t.Fatalf("co-chair moving a sign-up: %v", err)
	}
	for _, id := range made[8:] {
		if _, ok := s.Model().Table("MEMBER").Get(id); ok {
			t.Errorf("old sign-up, lead or management membership remains: %s", id)
		}
	}
	joined, _ := s.Model().Table("MEMBER").Find(night, parent)
	if joined["id"] != member["id"] || joined["member"] != "yes" || joined["opened"] != "2026-11-02 23:57" {
		t.Errorf("destination membership: %v", joined)
	}
}

func TestASoleChairMovesToAChild(t *testing.T) {
	const night, leads, managers = "grp00000000512", "grp00000000514", "grp00000000513"
	s := sample(t)
	if s.Model().superAdmin(parent) || len(s.Model().Table("MEMBER").Referencing("group", managers)) != 1 {
		t.Fatal("move check requires a sole non-super-admin co-chair")
	}
	made := apply(t, s, []Edit{
		{Insert: "GROUP", As: "tech", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Tech Setup", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "join": "others"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@tech", "person": parent, "opened": "2026-11-02 23:57"}},
	})
	member, _ := s.Model().Table("MEMBER").Find(night, parent)
	lead, _ := s.Model().Table("MEMBER").Find(leads, parent)
	manager, _ := s.Model().Table("MEMBER").Find(managers, parent)
	grant := []Edit{
		{Insert: "MEMBER", Row: map[string]any{"group": made[0], "person": parent, "member": "yes", "note": "Tech co-chair", "added_by": parent}},
		{Insert: "GROUP", As: "leads", Row: map[string]any{"parent": made[0], "kind": "group", "status": "open", "name": "Tech Setup Leads", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none", "added_by": parent}},
		{Set: made[0], Cells: map[string]any{"leads": "@leads"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@leads", "person": parent, "member": "yes", "added_by": parent}},
		{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "status": "open", "name": "Tech Setup Managers", "added_by": parent}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": parent, "member": "yes"}},
		{Set: made[0], Cells: map[string]any{"managed_by": "@managers"}},
	}
	remove := []Edit{{Delete: member["id"]}, {Delete: lead["id"]}, {Delete: manager["id"]}}
	write := func(edits []Edit) error {
		_, err := Write(t.Context(), s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: edits})
		return err
	}
	if err := write(append(append([]Edit{}, remove...), grant...)); err == nil {
		t.Fatal("granting the child role after losing source authority succeeded")
	} else {
		t.Logf("removing source authority first is refused: %v", err)
	}
	if err := write(append(grant, remove...)); err != nil {
		t.Fatalf("moving the sole co-chair to a child: %v", err)
	}
	for _, e := range remove {
		if _, ok := s.Model().Table("MEMBER").Get(e.Delete); ok {
			t.Errorf("source membership remains: %s", e.Delete)
		}
	}
	joined, _ := s.Model().Table("MEMBER").Get(made[1])
	if joined["member"] != "yes" || joined["opened"] != "2026-11-02 23:57" {
		t.Errorf("destination membership: %v", joined)
	}
	for group, want := range map[string]int{night: 0, made[0]: 1} {
		if got := len(as(t, s, parent, `(from GROUP @g (where (= id "`+group+`") (runs_list @g)))`)); got != want {
			t.Errorf("management of %s: %d, want %d", group, got, want)
		}
	}
}

func TestAChildChairMovesToAParentAsAVolunteer(t *testing.T) {
	const night, leads, managers = "grp00000000512", "grp00000000514", "grp00000000513"
	s := sample(t)
	made := apply(t, s, []Edit{
		{Set: night, Cells: map[string]any{"join": "others"}},
		{Insert: "GROUP", As: "tech", Row: map[string]any{"parent": night, "kind": "activity", "status": "open", "name": "Tech Setup", "visible_to": "grp00000000004", "join": "others"}},
		{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "status": "open", "name": "Tech Setup Managers"}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Set: "@tech", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "GROUP", As: "leads", Row: map[string]any{"kind": "group", "parent": "@tech", "status": "open", "name": "Tech Setup Leads"}},
		{Set: "@tech", Cells: map[string]any{"leads": "@leads"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@tech", "person": student, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@leads", "person": student, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": student, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": night, "person": student, "opened": "2026-11-02 23:57"}},
	})
	for group, want := range map[string]int{night: 0, made[1]: 1} {
		if got := len(as(t, s, student, `(from GROUP @g (where (= id "`+group+`") (runs_list @g)))`)); got != want {
			t.Fatalf("source-only management of %s: %d, want %d", group, got, want)
		}
	}
	write := func(edits ...Edit) error {
		_, err := Write(t.Context(), s, access.Actor{Email: "student@example.org"}, Env{Viewer: student, Now: testNow}, Batch{Batch: edits})
		return err
	}
	if err := write(Edit{Insert: "MEMBER", Row: map[string]any{"group": leads, "person": student, "member": "yes", "added_by": student}}); err == nil {
		t.Fatal("a child co-chair appointed themselves to the parent")
	}
	if err := write(
		Edit{Insert: "MEMBER", Row: map[string]any{"group": night, "person": student, "member": "yes", "note": "Movie volunteer", "added_by": student}},
		Edit{Delete: made[7]},
		Edit{Delete: made[8]},
		Edit{Delete: made[9]},
	); err != nil {
		t.Fatalf("moving a child co-chair to the parent as a volunteer: %v", err)
	}
	joined, _ := s.Model().Table("MEMBER").Get(made[10])
	if joined["member"] != "yes" || joined["opened"] != "2026-11-02 23:57" || joined["note"] != "Movie volunteer" {
		t.Errorf("parent sign-up: %v", joined)
	}
	for _, group := range []string{leads, managers} {
		if _, ok := s.Model().Table("MEMBER").Find(group, student); ok {
			t.Errorf("mover gained a parent co-chair membership in %s", group)
		}
	}
	for _, id := range made[7:10] {
		if _, ok := s.Model().Table("MEMBER").Get(id); ok {
			t.Errorf("old role membership remains: %s", id)
		}
	}
}
