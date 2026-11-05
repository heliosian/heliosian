package db

import (
	"context"
	"slices"
	"testing"
)

func TestLoadedSchoolRostersContainOnlyStudents(t *testing.T) {
	m := sample(t).Model()
	students := m.students()
	for _, group := range m.Table("GROUP").All() {
		if !slices.Contains([]string{"classroom", "crew", "grade", "gradeband"}, group["kind"]) {
			continue
		}
		for _, row := range m.effectiveRows(group["id"]) {
			if !students[row["person"]] {
				person, _ := m.Table("PERSON").Get(row["person"])
				t.Errorf("%s's roster includes non-student %s", group["name"], person["name_show"])
			}
		}
	}
}

func TestTeachingAssignmentsDoNotJoinStudentRosters(t *testing.T) {
	s := sample(t)
	const classroom, crew, department = "grp00000000010", "grp00000000501", "grp00000020040"
	apply(t, s, []Edit{{Set: staff, Cells: map[string]any{"classroom_override": classroom, "crew_override": crew, "department_override": department}}})
	if got := schoolGroupsHeld(s, staff); !slices.Equal(got, []string{department}) {
		t.Fatalf("a teacher's school memberships are %v", got)
	}
	p, _ := s.Model().Table("PERSON").Get(staff)
	if p["classroom"] != classroom || p["crew"] != crew {
		t.Fatalf("the teacher's assignments were lost: %v", p)
	}
	if got := s.Model().teaches(staff); !slices.Equal(got, []string{"Hummingbirds"}) {
		t.Fatalf("the mail classifier's teaching classrooms are %v", got)
	}
	for _, target := range []string{classroom, crew} {
		if s.Model().effectivelyIn(target, staff) || !s.Model().effectivelyIn(target, student) {
			t.Fatalf("%s does not contain only its student", target)
		}
		answer, err := s.Model().draftMembers(context.Background(), setupEnv, groupDraft{Rules: []draftRule{{Target: target}}})
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(answer.Members, func(p draftPerson) bool { return p.Person == staff }) || !slices.ContainsFunc(answer.Members, func(p draftPerson) bool { return p.Person == student }) {
			t.Fatalf("the guest preview for %s has %v", target, answer.Members)
		}
	}
}

func TestSchoolRostersFollowStudentRoleChanges(t *testing.T) {
	s := sample(t)
	const students, classroom, crew, grade = "grp00000000001", "grp00000000010", "grp00000000501", "grp00000000011"
	want := []string{classroom, grade, crew}
	role := memberRow(t, s, students, student)
	check := func(person string, want []string) {
		t.Helper()
		if got := schoolGroupsHeld(s, person); !slices.Equal(got, want) {
			t.Fatalf("%s's school memberships are %v, want %v", person, got, want)
		}
	}
	for _, member := range []string{"excluded", "yes", ""} {
		apply(t, s, []Edit{{Set: role, Cells: map[string]any{"member": member}}})
		if member == "yes" {
			check(student, want)
		} else {
			check(student, nil)
		}
	}
	apply(t, s, []Edit{{Delete: role}})
	check(student, nil)
	role = apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": students, "person": student, "member": "yes"}}})[0]
	check(student, want)
	apply(t, s, []Edit{{Delete: role}})
	check(student, nil)
	apply(t, s, []Edit{
		{Set: staff, Cells: map[string]any{"classroom_override": classroom, "crew_override": crew}},
		{Insert: "MEMBER", Row: map[string]any{"group": students, "person": staff, "member": "yes"}},
	})
	check(staff, []string{classroom, crew})
}
