package db

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHomeAudiencesCannotGrantManagementOfOtherGroups(t *testing.T) {
	for _, binding := range []struct {
		name, id string
	}{
		{"widget", "wdg00000000008"},
		{"app", "app00000000001"},
		{"link", "grp00000000550"},
	} {
		for _, action := range []string{"add member", "remove member", "add rule", "remove rule", "unmanaged managers"} {
			t.Run(binding.name+"/"+action, func(t *testing.T) {
				s := sample(t)
				const managers = "grp00000000041"
				made := apply(t, s, []Edit{
					{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000015", "person": parent, "member": "yes"}},
					{Insert: "RULE", Row: map[string]any{"group": managers, "order": "a", "person": staff}},
				})
				if action == "unmanaged managers" {
					apply(t, s, []Edit{{Set: managers, Cells: map[string]any{"managed_by": ""}}})
				}
				if s.Model().runsGroup("grp00000000040", parent) {
					t.Fatal("the Home admin already manages Fall Picnic")
				}
				edit := Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": parent, "member": "yes"}}
				switch action {
				case "remove member":
					members := s.Model().Table("MEMBER").Referencing("group", managers)
					if len(members) == 0 {
						t.Fatal("the picnic has no managers")
					}
					edit = Edit{Delete: members[0]["id"]}
				case "add rule":
					edit = Edit{Insert: "RULE", Row: map[string]any{"group": managers, "order": "z", "target": "grp00000000004"}}
				case "remove rule":
					edit = Edit{Delete: made[1]}
				}
				payload, err := json.Marshal(Batch{Batch: []Edit{
					{Set: binding.id, Cells: map[string]any{"visible_to": managers}},
					edit,
				}})
				if err != nil {
					t.Fatal(err)
				}
				rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(payload))
				if rec.Code != http.StatusForbidden {
					t.Fatalf("a Home admin borrowing Picnic Managers got %d: %s; manages picnic: %v", rec.Code, rec.Body, s.Model().runsGroup("grp00000000040", parent))
				}
				if s.Model().runsGroup("grp00000000040", parent) {
					t.Fatal("the refused write granted management of Fall Picnic")
				}
			})
		}
	}
}

func TestHomeAudiencesCannotEditAnotherGroupsUnmanagedChildren(t *testing.T) {
	s := sample(t)
	made := apply(t, s, []Edit{
		{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000015", "person": parent, "member": "yes"}},
		{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Picnic helpers", "status": "open", "parent": "grp00000000040"}},
	})
	payload, err := json.Marshal(Batch{Batch: []Edit{
		{Set: "wdg00000000008", Cells: map[string]any{"visible_to": made[1]}},
		{Insert: "MEMBER", Row: map[string]any{"group": made[1], "person": parent, "member": "yes"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(payload))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a Home admin editing a child of another app's group got %d: %s", rec.Code, rec.Body)
	}
}
