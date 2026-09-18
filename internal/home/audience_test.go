package home

import (
	"testing"
)

// A link kept to some roles or classrooms goes to the people in them and to
// nobody else; one kept to neither goes to everyone. The sample sheet keeps
// the Parent Portal to parents, the Staff Room to staff, one chat to a
// classroom, one to the parents of two classrooms.
func TestLinkAudience(t *testing.T) {
	c := sampleCache(t)
	links := map[string]Link{}
	for _, cat := range c.Model().Categories {
		for _, l := range cat.Links {
			links[l.Title] = l
		}
	}
	if got := links["Hawks and Falcons Chat"]; len(got.Roles) != 1 || got.Roles[0] != RoleParents || len(got.Classrooms) != 2 || got.Classrooms[1] != "Falcons" {
		t.Fatalf("the chat's audience = %v %v", got.Roles, got.Classrooms)
	}
	parent := func(rooms ...string) ([]string, []string) { return []string{RoleParents}, rooms }
	for _, tc := range []struct {
		link  string
		roles []string
		rooms []string
		want  bool
	}{
		{"Directory", nil, nil, true},
		{"Parent Portal", []string{RoleParents}, nil, true},
		{"Parent Portal", []string{RoleStudents}, []string{"Jays"}, false},
		{"Staff Room", []string{RoleStaff}, []string{"Jays"}, true},
		{"Staff Room", []string{RoleParents}, []string{"Jays"}, false},
		{"Hummingbirds Chat", []string{RoleStudents}, []string{"Hummingbirds"}, true},
		{"Hummingbirds Chat", []string{RoleStaff}, []string{"Hummingbirds"}, true},
		{"Hummingbirds Chat", []string{RoleParents}, []string{"Hawks"}, false},
		{"Hawks and Falcons Chat", []string{RoleStudents}, []string{"Hawks"}, false},
	} {
		if got := links[tc.link].For(tc.roles, tc.rooms); got != tc.want {
			t.Errorf("%s for %v %v = %v, want %v", tc.link, tc.roles, tc.rooms, got, tc.want)
		}
	}
	roles, rooms := parent("Jays", "Ospreys")
	if !links["Jays Chat"].For(roles, rooms) || links["Hawks and Falcons Chat"].For(roles, rooms) {
		t.Errorf("a Jays and Ospreys parent should see the Jays chat and not the Hawks and Falcons one")
	}
	// A section kept to some people keeps its links with it: the sample
	// Chats are for parents and staff.
	var chats Category
	for _, cat := range c.Model().Categories {
		if cat.Title == "Chats" {
			chats = cat
		}
	}
	if len(chats.Roles) != 2 || chats.For([]string{RoleStudents}, []string{"Jays"}) || !chats.For([]string{RoleStaff}, nil) {
		t.Errorf("Chats = %v, want kept to parents and staff", chats.Roles)
	}
	// A misspelled role refuses the load.
	tables := c.Tables().withRow(linksTab, "Directory", map[string]string{RolesColumn: "Teachers"})
	if _, err := BuildModel(tables, noImages{}); err == nil {
		t.Fatalf("a role that is not one of the three loaded")
	}
}
