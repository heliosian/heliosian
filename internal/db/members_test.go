package db

import (
	"slices"
	"testing"
)

func TestMembersReadableHoldsOnlyWhenEveryRowIs(t *testing.T) {
	m := sample(t).Model()
	em, _ := Lookup("EFFECTIVE_MEMBER")
	rows := m.generated(em).rows
	whole := 0
	for _, viewer := range []string{nobody, student, parent, staff, guest} {
		r := m.newRun(Env{Viewer: viewer, Now: testNow})
		for _, row := range rows {
			if !r.membersReadable(row["group"]) {
				continue
			}
			whole++
			if !r.readable(em, row) {
				t.Errorf("as %q every member of %s reads as readable, but %s is not", viewer, row["group"], row["person"])
			}
		}
	}
	if whole == 0 {
		t.Fatal("no group's members read as readable as a whole")
	}
}

func TestMembersAndCovers(t *testing.T) {
	s := sample(t)
	people := func(src string) []string {
		got := ids(as(t, s, parent, src), "id")
		slices.Sort(got)
		return got
	}
	byRule := people(`(from PERSON (where (in id (select EFFECTIVE_MEMBER.person (= group "grp00000000030")))))`)
	byMembers := people(`(from PERSON (where (in id (members "grp00000000010" "parents"))))`)
	if len(byRule) == 0 || !slices.Equal(byRule, byMembers) {
		t.Errorf("the Hummingbirds' parents by members are %v, by the Hummingbirds Parents group's rule %v", byMembers, byRule)
	}
	covering := ids(as(t, s, parent, `(from GROUP @e (where (covers @e (members "grp00000000010" "parents"))))`), "id")
	if !slices.Contains(covering, "grp00000000030") {
		t.Errorf("Hummingbirds Parents does not cover the Hummingbirds' parents: %v", covering)
	}
	if slices.Contains(covering, "grp00000000010") {
		t.Errorf("the Hummingbirds classroom covers its students' parents: %v", covering)
	}
	if got := ids(as(t, s, parent, `(from GROUP @e (where (covers @e (members "grp00000000531"))))`), "id"); len(got) != 0 {
		t.Errorf("an empty set covers %v", got)
	}
}
