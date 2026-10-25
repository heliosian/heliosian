package db

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAGroupCannotBorrowTheSuperAdminsAsAnswers(t *testing.T) {
	s := sample(t)
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"insert": "GROUP", "as": "list", "row": {"kind": "group", "name": "My list", "status": "open", "added_by": "`+parent+`", "rsvp_yes": "grp00000000005"}},
		{"set": "@list", "cells": {"managed_by": "@list"}},
		{"insert": "MEMBER", "row": {"group": "@list", "person": "`+parent+`", "member": "yes"}},
		{"insert": "MEMBER", "row": {"group": "grp00000000005", "person": "`+parent+`", "member": "yes"}}
	]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a member borrowing super admins as answers got %d: %s", rec.Code, rec.Body)
	}
	if s.Model().SuperAdmin("rowan.ashdown@example.org") {
		t.Fatal("the refused write left the member a super admin")
	}
}

func TestMembersCannotBorrowAnotherGroupsHelpers(t *testing.T) {
	for _, column := range servingColumns {
		if column == "managed_by" {
			continue
		}
		t.Run(column, func(t *testing.T) {
			s := sample(t)
			before := s.Model().Table("GROUP").Len()
			row := map[string]any{"kind": "group", "name": "My list", "status": "open", "added_by": parent, column: "grp00000000001"}
			body, err := json.Marshal(Batch{Batch: []Edit{{Insert: "GROUP", Row: row}}})
			if err != nil {
				t.Fatal(err)
			}
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("borrowing Students as %s got %d: %s", column, rec.Code, rec.Body)
			}
			if got := s.Model().Table("GROUP").Len(); got != before {
				t.Fatalf("the refused insert left %d groups, want %d", got, before)
			}
		})
	}
}

func TestAppAdminsAssignOnlyTheirEventsOwnAnswerGroups(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{
		{Insert: "GROUP", As: "admins", Row: map[string]any{"kind": "admins", "name": "When Admins", "status": "open"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": parent, "member": "yes"}},
		{Set: whenApp, Cells: map[string]any{"admins": "@admins"}},
	})
	for _, column := range []string{"rsvp_yes", "rsvp_no"} {
		for _, target := range []string{"grp00000000005", "grp00000000001"} {
			body, err := json.Marshal(Batch{Batch: []Edit{{Set: "grp00000000040", Cells: map[string]any{column: target}}}})
			if err != nil {
				t.Fatal(err)
			}
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("a When admin assigning %s to %s got %d: %s", target, column, rec.Code, rec.Body)
			}
		}
	}
	ids := apply(t, s, []Edit{{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Picnic answers", "parent": "grp00000000040", "status": "open"}}})
	body, err := json.Marshal(Batch{Batch: []Edit{{Set: "grp00000000040", Cells: map[string]any{"rsvp_yes": ids[0]}}}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body)); rec.Code != http.StatusOK {
		t.Fatalf("a When admin assigning the event's own answer group got %d: %s", rec.Code, rec.Body)
	}
}

func TestAppAdminsCannotBorrowUnrelatedManagers(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{
		{Insert: "GROUP", As: "admins", Row: map[string]any{"kind": "admins", "name": "When Admins", "status": "open"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": parent, "member": "yes"}},
		{Set: whenApp, Cells: map[string]any{"admins": "@admins"}},
	})
	if !s.Model().effectivelyIn("grp00000000040", parent) || s.Model().runsGroup("grp00000000040", parent) {
		t.Fatal("the caller must be a picnic guest without management authority")
	}
	self := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"set": "grp00000000040", "cells": {"managed_by": "grp00000000040"}}
	]}`)
	if self.Code != http.StatusForbidden {
		t.Fatalf("a When admin making picnic guests managers got %d: %s; caller runs picnic: %v", self.Code, self.Body, s.Model().runsGroup("grp00000000040", parent))
	}
	picnic, _ := s.Model().Table("GROUP").Get("grp00000000040")
	if picnic["managed_by"] != "grp00000000041" || s.Model().runsGroup("grp00000000040", parent) {
		t.Fatal("the refused self-assignment changed picnic managers")
	}
	if s.Model().runsGroup("grp00000020078", parent) {
		t.Fatal("the caller already manages Garden Club")
	}
	if !s.Model().effectivelyIn("grp00000020078", parent) {
		t.Fatal("the caller is not a Garden Club member")
	}
	takeover := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"set": "grp00000000040", "cells": {"managed_by": "grp00000020078"}},
		{"set": "grp00000020078", "cells": {"managed_by": "grp00000020078"}}
	]}`)
	if takeover.Code != http.StatusForbidden {
		t.Fatalf("a member taking over Garden Club got %d: %s; manages Garden Club: %v", takeover.Code, takeover.Body, s.Model().runsGroup("grp00000020078", parent))
	}
	club, _ := s.Model().Table("GROUP").Get("grp00000020078")
	if club["managed_by"] != "grp00000020077" || s.Model().runsGroup("grp00000020078", parent) {
		t.Fatal("the refused takeover changed Garden Club managers")
	}
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"set": "grp00000000040", "cells": {"managed_by": "grp00000020077"}},
		{"insert": "MEMBER", "row": {"group": "grp00000020077", "person": "`+parent+`", "member": "yes"}}
	]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("borrowing Garden Club Managers got %d: %s; manages Garden Club: %v", rec.Code, rec.Body, s.Model().runsGroup("grp00000020078", parent))
	}
	if s.Model().runsGroup("grp00000020078", parent) {
		t.Fatal("the refused batch gave the caller Garden Club")
	}
	apply(t, s, []Edit{{Set: "grp00000020078", Cells: map[string]any{"managed_by": ""}}})
	rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"set": "grp00000000040", "cells": {"managed_by": "grp00000020077"}},
		{"insert": "MEMBER", "row": {"group": "grp00000020077", "person": "`+parent+`", "member": "yes"}}
	]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("borrowing an unrelated group that manages nothing got %d: %s", rec.Code, rec.Body)
	}
	if s.Model().effectivelyIn("grp00000020077", parent) {
		t.Fatal("the refused batch joined the unrelated group")
	}
	rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"insert": "GROUP", "as": "managers", "row": {"kind": "group", "name": "Picnic Managers", "status": "open", "added_by": "`+parent+`"}},
		{"set": "@managers", "cells": {"managed_by": "@managers"}},
		{"insert": "MEMBER", "row": {"group": "@managers", "person": "`+parent+`", "member": "yes"}},
		{"set": "grp00000000040", "cells": {"managed_by": "@managers"}}
	]}`)
	if rec.Code != http.StatusOK || !s.Model().runsGroup("grp00000000040", parent) {
		t.Fatalf("creating dedicated Picnic managers got %d: %s", rec.Code, rec.Body)
	}
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000020077", "person": parent, "member": "yes"}}})
	rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"set": "grp00000000040", "cells": {"managed_by": "grp00000020077"}}
	]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reusing managers the caller already manages got %d: %s", rec.Code, rec.Body)
	}
}
