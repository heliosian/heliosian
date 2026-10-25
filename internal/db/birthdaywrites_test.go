package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
)

const birthdayAdmin = "per00000020001"

func birthdaySample(t *testing.T) (*Store, func(viewer string, edits ...Edit) ([]string, error)) {
	t.Helper()
	s := sample(t)
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000700", "person": birthdayAdmin, "member": "yes"}}})
	return s, func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
}

func TestTheBirthdaysTeamKeepsAStaffMembersYear(t *testing.T) {
	_, write := birthdaySample(t)
	year := func(person, assigned, charity string) Edit {
		return Edit{Insert: "BIRTHDAY_YEAR", Row: map[string]any{"person": person, "year": "2026", "assigned_to": assigned, "charity": charity}}
	}
	if _, err := write(student, year("per00000020005", "", "")); err == nil {
		t.Error("a student off the team began a birthday's year")
	}
	if _, err := write(parent, year("per00000020005", student, "")); err == nil {
		t.Error("a birthday went to someone off the team")
	}
	if _, err := write(parent, year("per00000020014", parent, "")); err == nil {
		t.Error("a birthday was assigned for someone who asked to be left out")
	}
	if _, err := write(parent, year("per00000020005", parent, "chr00000000005")); err == nil {
		t.Error("a birthday gave to a charity that is not allowed")
	}
	made, err := write(parent, year("per00000020005", parent, ""))
	if err != nil {
		t.Fatalf("the team can't assign a birthday to one of its own: %v", err)
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"assigned_to": staff, "contacted": "2026-09-30 09:00", "contacted_by": parent, "charity": "chr00000000001", "note": "For the food bank.", "recorded": "2026-09-30 09:05", "recorded_by": parent}}); err != nil {
		t.Errorf("the team can't record the outreach and the charity: %v", err)
	}
	if _, err := write(parent, Edit{Delete: made[0]}); err == nil {
		t.Error("someone on the team who is not an admin removed a year's record")
	}
	if _, err := write(birthdayAdmin, Edit{Delete: made[0]}); err != nil {
		t.Errorf("a Birthdays admin can't remove a year's record: %v", err)
	}
}

func TestTheBirthdaysTeamKeepsBirthdaysAndCharities(t *testing.T) {
	_, write := birthdaySample(t)
	if _, err := write(parent, Edit{Set: "per00000020004", Cells: map[string]any{"birthday": "2000-06-01", "birthday_participation": "no_newsletter"}}); err != nil {
		t.Errorf("the team can't keep a staff member's birthday: %v", err)
	}
	if _, err := write(parent, Edit{Set: student, Cells: map[string]any{"birthday": "2000-06-01"}}); err == nil {
		t.Error("the team set a birthday on someone who is not staff")
	}
	if _, err := write(parent, Edit{Insert: "CHARITY", Row: map[string]any{"name": "Coastside Hope", "allowed": false}}); err == nil {
		t.Error("someone on the team who is not an admin added a charity that is not allowed")
	}
	made, err := write(parent, Edit{Insert: "CHARITY", Row: map[string]any{"name": "Coastside Hope", "url": "https://example.org/give", "allowed": true}})
	if err != nil {
		t.Fatalf("the team can't add a charity: %v", err)
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"allowed": false}}); err == nil {
		t.Error("someone on the team who is not an admin stopped allowing a charity")
	}
	if _, err := write(birthdayAdmin, Edit{Set: made[0], Cells: map[string]any{"allowed": false, "not_allowed_reason": "Not a non-profit"}}); err != nil {
		t.Errorf("a Birthdays admin can't stop allowing a charity: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Insert: "CHARITY", Row: map[string]any{"name": "Fogline Arts", "allowed": false}}); err != nil {
		t.Errorf("a Birthdays admin can't add a charity that is not allowed: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Delete: "chr00000000002"}); err == nil {
		t.Error("a charity a birthday names was removed")
	}
	if _, err := write(birthdayAdmin, Edit{Delete: "chr00000000005"}); err != nil {
		t.Errorf("a Birthdays admin can't remove a charity nothing names: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Set: "set00000000009", Cells: map[string]any{"value": "10"}}); err != nil {
		t.Errorf("a Birthdays admin can't change a setting: %v", err)
	}
	if _, err := write(parent, Edit{Set: "set00000000009", Cells: map[string]any{"value": "12"}}); err == nil {
		t.Error("someone on the team who is not an admin changed a setting")
	}
}

func TestTheBirthdaysAppOpensToTheTeamAndItsAdmins(t *testing.T) {
	s, _ := birthdaySample(t)
	for text, want := range map[string][]bool{
		`(from APP (where (= key "birthday") (birthday_team)))`:       {false, true, true},
		`(from APP (where (= key "birthday") (admin_of "birthday")))`: {false, false, true},
	} {
		q, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		for i, viewer := range []string{student, parent, birthdayAdmin} {
			if got := len(s.Model().Run(context.Background(), q, Env{Viewer: viewer, Now: testNow}).IDs) == 1; got != want[i] {
				t.Errorf("%s answers %s %v, want %v", text, viewer, got, want[i])
			}
		}
	}
}

func TestBirthdaysAdminsKeepTheTeam(t *testing.T) {
	_, write := birthdaySample(t)
	join := func(group string) Edit {
		return Edit{Insert: "MEMBER", Row: map[string]any{"group": group, "person": "per00000020006", "member": "yes", "added_by": birthdayAdmin}}
	}
	if _, err := write(parent, join("grp00000000702")); err == nil {
		t.Error("someone on the team who is not an admin put a person on the comms team")
	}
	if _, err := write(birthdayAdmin, join("grp00000000004")); err == nil {
		t.Error("a Birthdays admin added someone to a group the app is not open to")
	}
	made, err := write(birthdayAdmin, join("grp00000000702"))
	if err != nil {
		t.Fatalf("a Birthdays admin can't put a person on the comms team: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Delete: made[0]}); err != nil {
		t.Errorf("a Birthdays admin can't take a person off the comms team: %v", err)
	}
}

func TestBirthdaysAdminsKeepTheNewsletterIssues(t *testing.T) {
	_, write := birthdaySample(t)
	issue := Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "parent": "grp00000000710", "name": "Helios Weekly Newsletter", "status": "open", "added_by": staff, "listed": false, "visible_to": "grp00000000004", "posting": "members", "replying": "members", "start": "2026-12-11", "all_day": true}}
	if _, err := write(parent, issue); err == nil {
		t.Error("someone on the team who is not an admin added a newsletter issue")
	}
	issue.Row["added_by"] = birthdayAdmin
	made, err := write(birthdayAdmin, issue)
	if err != nil {
		t.Fatalf("a Birthdays admin can't add a newsletter issue: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Set: made[0], Cells: map[string]any{"start": "2026-12-12"}}); err != nil {
		t.Errorf("a Birthdays admin can't move a newsletter issue: %v", err)
	}
	if _, err := write(birthdayAdmin, Edit{Delete: made[0]}); err != nil {
		t.Errorf("a Birthdays admin can't remove a newsletter issue: %v", err)
	}
}
