package db

import (
	"testing"
)

func birthdayOf(t *testing.T, s *Store, viewer, person, year string) map[string]string {
	t.Helper()
	rows := as(t, s, viewer, `(from BIRTHDAY (where (= person "`+person+`") (= year `+year+`)))`)
	if len(rows) != 1 {
		t.Fatalf("%d birthdays of %s in %s, want 1", len(rows), person, year)
	}
	return rows[0]
}

func TestABirthdayIsAnnouncedInTheLastIssueBeforeIt(t *testing.T) {
	s := sample(t)
	for _, c := range []struct {
		person, year, day, newsletter, askBy, dueBy, record string
	}{
		{"per00000020001", "2026", "2026-08-20", "grp00000000721", "2026-08-13", "2026-08-19", "bdy00000000002"},
		{"per00000020002", "2026", "2026-09-15", "grp00000000724", "2026-09-03", "2026-09-09", "bdy00000000003"},
		{"per00000020002", "2025", "2025-09-15", "grp00000000712", "2025-08-28", "2025-09-03", ""},
		{"per00000020001", "2025", "2025-08-20", "grp00000000711", "2025-08-14", "2025-08-20", "bdy00000000007"},
		{"per00000020008", "2026", "2027-02-28", "grp00000000736", "2027-02-18", "2027-02-24", ""},
	} {
		b := birthdayOf(t, s, parent, c.person, c.year)
		if b["day"] != c.day || b["newsletter"] != c.newsletter || b["ask_by"] != c.askBy || b["due_by"] != c.dueBy || b["record"] != c.record {
			t.Errorf("%s in %s: %v, want day %s, issue %s, ask by %s, due by %s, record %q", c.person, c.year, b, c.day, c.newsletter, c.askBy, c.dueBy, c.record)
		}
	}
}

func TestBirthdaysShowToTheBirthdaysTeam(t *testing.T) {
	s := sample(t)
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		if got, want := len(as(t, s, viewer, `(from BIRTHDAY (where (= person "per00000020001")))`)), []int{0, 0, 2, 2, 0}[i]; got != want {
			t.Errorf("as %q: %d of Imogen's birthdays, want %d", viewer, got, want)
		}
	}
}

func TestABirthdayFollowsTheIssuesAndSettings(t *testing.T) {
	s, queue := sampleWithQueue(t)
	if b := birthdayOf(t, s, parent, "per00000020003", "2026"); b["newsletter"] != "grp00000000729" || b["ask_by"] != "2026-10-08" {
		t.Fatalf("Nalini before: %v", b)
	}
	made := apply(t, s, queue, []Edit{{Insert: "GROUP", Row: map[string]any{"kind": "group", "parent": "grp00000000710", "name": "Helios Weekly Newsletter", "status": "open", "listed": false, "visible_to": "grp00000000004", "posting": "members", "replying": "members", "start": "2026-10-20", "all_day": true}}})
	if b := birthdayOf(t, s, parent, "per00000020003", "2026"); b["newsletter"] != made[0] || b["ask_by"] != "2026-10-12" {
		t.Fatalf("Nalini after an issue on 2026-10-20: %v", b)
	}
	apply(t, s, queue, []Edit{{Set: "set00000000009", Cells: map[string]any{"value": "10"}}})
	if b := birthdayOf(t, s, parent, "per00000020003", "2026"); b["ask_by"] != "2026-10-10" {
		t.Fatalf("Nalini after Request Lead Days went to 10: %v", b)
	}
}
