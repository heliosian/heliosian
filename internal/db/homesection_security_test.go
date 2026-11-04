package db

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHomeSectionsCannotGrantManagementOfOtherGroups(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000015", "person": parent, "member": "yes"}}})
	if s.Model().runsGroup("grp00000000040", parent) {
		t.Fatal("the Home admin already manages Fall Picnic")
	}
	widgets := s.Model().Table("WIDGET").Len()
	payload, err := json.Marshal(Batch{Batch: []Edit{
		{Insert: "WIDGET", Row: map[string]any{"key": "links", "visible_to": "grp00000000004", "group": "grp00000000060", "name": "Borrowed category"}},
		{Insert: "WIDGET", Row: map[string]any{"key": "links", "visible_to": "grp00000000004", "group": "grp00000000020", "name": "My family"}},
		{Set: "grp00000000040", Cells: map[string]any{"parent": "grp00000000020"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000041", "person": parent, "member": "yes"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(payload))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a Home admin moving Fall Picnic under their family got %d: %s; manages picnic: %v", rec.Code, rec.Body, s.Model().runsGroup("grp00000000040", parent))
	}
	if s.Model().runsGroup("grp00000000040", parent) {
		t.Fatal("the refused batch granted management of Fall Picnic")
	}
	if got := s.Model().Table("WIDGET").Len(); got != widgets {
		t.Fatalf("the refused batch left %d widgets, want %d", got, widgets)
	}
}

func TestHomeSectionsCannotEditUnrelatedGroups(t *testing.T) {
	for _, target := range []string{"grp00000000041", "grp00000020078", "grp00000000001", "grp00000000042"} {
		t.Run(target, func(t *testing.T) {
			s := sample(t)
			apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000015", "person": parent, "member": "yes"}}})
			payload, err := json.Marshal(Batch{Batch: []Edit{
				{Insert: "WIDGET", Row: map[string]any{"key": "links", "visible_to": "grp00000000004", "group": target, "name": "Borrowed group"}},
				{Set: target, Cells: map[string]any{"name": "Hijacked", "visible_to": "grp00000000004"}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(payload))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("a Home admin editing an unrelated group got %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestHomeSectionsCannotBorrowUnmanagedManagers(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{
		{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000015", "person": parent, "member": "yes"}},
		{Set: "grp00000000041", Cells: map[string]any{"managed_by": ""}},
	})
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch":[
		{"insert":"WIDGET","row":{"key":"links","group":"grp00000000041","visible_to":"grp00000000004"}},
		{"set":"grp00000000041","cells":{"name":"Hijacked"}}
	]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a Home admin editing unmanaged Picnic Managers got %d: %s", rec.Code, rec.Body)
	}
}
