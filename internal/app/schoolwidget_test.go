package app

import (
	"testing"

	"heliosian/internal/artifacts"
)

// TestForClassrooms checks which school lists reach a Jays family: the
// newsletter and every family's lists, the Jays' own lists and the grade band
// that holds them, and no other classroom's.
func TestForClassrooms(t *testing.T) {
	mine := []seat{{"jays", "Grade 3"}}
	grades := []string{"Kindergarten", "Grade 1", "Grade 2", "Grade 3", "Grade 4", "Grade 5", "Grade 6", "Grade 7", "Grade 8"}
	for channel, want := range map[string]bool{
		"parentsandstaff": true, "community": true, "jays.parents": true, "jays.students": true,
		"jaysandravens": true, "ravens.parents": false, "hawksandfalcons": false, "condorsandospreys": false,
	} {
		d := &artifacts.Document{Kind: artifacts.KindList, Channel: channel}
		if got := forClassrooms(d, mine, "", grades); got != want {
			t.Errorf("%s: %v, want %v", channel, got, want)
		}
	}
	// Veracross mail waits to be judged, then reaches everyone, or a seat in
	// one of the classrooms and one of the grades it names.
	news := &artifacts.Document{Kind: artifacts.KindNewsletter, Channel: "newsletter"}
	for _, tc := range []struct {
		audience string
		want     bool
	}{{"", false}, {artifacts.Everyone, true}, {"Jays", true}, {"Condors", false}, {"Condors, Jays", true},
		{"Grade 3", true}, {"Grade 2, Grade 4", false}, {"Jays, Grade 3", true}, {"Jays, Grade 4", false}, {"Ravens, Grade 3", false}} {
		if got := forClassrooms(news, mine, tc.audience, grades); got != tc.want {
			t.Errorf("veracross mail to %q: %v, want %v", tc.audience, got, tc.want)
		}
	}
	if artifacts.School(&artifacts.Document{Kind: artifacts.KindList, Channel: "chat"}) || artifacts.School(&artifacts.Document{Kind: artifacts.KindGroup, Channel: "x"}) {
		t.Error("the chat and a group's post are not school mail")
	}
}

// TestGradesMissAFamilyInTheClassroom is the note to "parents of 2nd, 4th,
// 6th, and 8th graders": a Condors family of a 5th and a 7th grader does
// not get it, a 6th grader's does, and the Condors' teacher does.
func TestGradesMissAFamilyInTheClassroom(t *testing.T) {
	grades := []string{"Grade 2", "Grade 4", "Grade 5", "Grade 6", "Grade 7", "Grade 8"}
	news := &artifacts.Document{Kind: artifacts.KindNewsletter, Channel: "newsletter"}
	audience := "Grade 2, Grade 4, Grade 6, Grade 8"
	if forClassrooms(news, []seat{{"condors", "Grade 5"}, {"herons", "Grade 7"}}, audience, grades) {
		t.Error("a 5th and 7th grader's family got a note to even grades")
	}
	if !forClassrooms(news, []seat{{"condors", "Grade 6"}}, audience, grades) {
		t.Error("a 6th grader's family missed it")
	}
	if !forClassrooms(news, []seat{{"condors", ""}}, audience, grades) {
		t.Error("the Condors' teacher missed it")
	}
}
