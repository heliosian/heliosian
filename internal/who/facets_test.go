package who

import (
	"slices"
	"testing"
)

// A grade or classroom is read one way for every filter: a student's own,
// a parent's children's, and none for staff - a teacher is not in the room
// they teach. The sample directory has the Whitfields in Jays and Ospreys
// and Ruth Amari teaching the Hummingbirds.
func TestFacetsReadOneWay(t *testing.T) {
	m := sampleModel(t)
	for email, want := range map[string][]string{
		"sam.whitfield@heliosschool.org":    {"Jays"},
		"jordan.whitfield@heliosschool.org": {"Jays", "Ospreys"},
		"ruth.amari@heliosschool.org":       {},
	} {
		p := m.Person(email)
		if p == nil {
			t.Fatalf("%s is not in the sample", email)
		}
		got := m.ClassroomsOf(p)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("classrooms of %s = %v, want %v", email, got, want)
		}
	}
	if ruth := m.Person("ruth.amari@heliosschool.org"); ruth.Classroom != "Hummingbirds" {
		t.Fatalf("the sample no longer has Ruth teaching the Hummingbirds; the staff case tests nothing")
	}
	if got := m.Facets(m.Person("jordan.whitfield@heliosschool.org"), false); len(got) != 2 {
		t.Errorf("grades of a parent of two = %v", got)
	}
}
